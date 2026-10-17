import net from "node:net";

export class Marionette {
  constructor(port = 2828, host = "127.0.0.1") {
    this.port = port;
    this.host = host;
    this.nextId = 0;
    this.pending = new Map();
    this.buffer = Buffer.alloc(0);
    this.greeting = null;
  }

  async connect(timeoutMs = 60000) {
    const deadline = Date.now() + timeoutMs;
    for (;;) {
      try {
        await this.#open();
        return this.greeting;
      } catch (err) {
        if (Date.now() > deadline) {
          throw new Error(`Marionette not reachable on ${this.port}: ${err.message}`);
        }
        await new Promise(r => setTimeout(r, 500));
      }
    }
  }

  #open() {
    return new Promise((resolve, reject) => {
      const socket = net.connect(this.port, this.host);
      let greeted = false;
      socket.once("error", reject);
      socket.on("data", chunk => {
        this.buffer = Buffer.concat([this.buffer, chunk]);
        for (;;) {
          const colon = this.buffer.indexOf(":");
          if (colon < 0) {
            break;
          }
          const len = parseInt(this.buffer.subarray(0, colon).toString(), 10);
          if (this.buffer.length < colon + 1 + len) {
            break;
          }
          const msg = JSON.parse(this.buffer.subarray(colon + 1, colon + 1 + len).toString("utf8"));
          this.buffer = this.buffer.subarray(colon + 1 + len);
          if (!greeted) {
            greeted = true;
            this.greeting = msg;
            this.socket = socket;
            resolve();
            continue;
          }
          const [, id, error, result] = msg;
          const waiter = this.pending.get(id);
          if (waiter) {
            this.pending.delete(id);
            error ? waiter.reject(new Error(`${error.error}: ${error.message}`)) : waiter.resolve(result);
          }
        }
      });
      socket.on("close", () => {
        for (const waiter of this.pending.values()) {
          waiter.reject(new Error("connection closed"));
        }
        this.pending.clear();
      });
    });
  }

  send(name, params = {}) {
    const id = ++this.nextId;
    const body = Buffer.from(JSON.stringify([0, id, name, params]), "utf8");
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      this.socket.write(`${body.length}:`);
      this.socket.write(body);
    });
  }

  async session() {
    await this.send("WebDriver:NewSession", { capabilities: { alwaysMatch: { acceptInsecureCerts: false } } });
    await this.send("Marionette:SetContext", { value: "chrome" });
  }

  async exec(script, args = []) {
    const result = await this.send("WebDriver:ExecuteScript", { script, args, sandbox: "system" });
    return result.value;
  }

  async screenshot() {
    const result = await this.send("WebDriver:TakeScreenshot", { full: false, hash: false });
    return Buffer.from(result.value, "base64");
  }

  close() {
    this.socket?.destroy();
  }
}
