import { h } from "./dom.js";

const reducedMotion = matchMedia("(prefers-reduced-motion: reduce)");

const KDF_MS = {
  "argon2id-standard": 1200,
  "argon2id-strong": 2500,
  "argon2id-paranoid": 5000,
};


const trackers = new Map();

function track(id, st) {
  const now = performance.now();
  let t = trackers.get(id);
  if (!t || t.state !== st.state) {
    t = { state: st.state, phase: null, phaseStart: now, lastF: 0, lastT: now, rate: 0 };
    trackers.set(id, t);
  }
  if (st.phase !== t.phase) {
    Object.assign(t, { phase: st.phase, phaseStart: now, lastF: 0, lastT: now, rate: 0 });
  }
  const f = st.progress || 0;
  if (f > t.lastF) {
    const rate = (f - t.lastF) / Math.max(1, now - t.lastT);
    t.rate = t.rate ? t.rate * 0.75 + rate * 0.25 : rate;
    t.lastF = f;
    t.lastT = now;
  }
  return { ...t, now, f };
}

function steps(p, st) {
  const stats = p.stats || {};
  if (st.state === "sealing") {
    const total = stats.lastSealMs || Math.max(1500, (stats.dataBytes || 0) / 60000);
    return [["encrypt", total * 0.6], ["verify", total * 0.3], ["wipe", total * 0.1]];
  }
  const key = stats.lastUnlockMs || KDF_MS[p.security?.kdf] || 1500;
  const decrypt = stats.lastUnsealMs || Math.max(600, (stats.vaultBytes || 0) / 90000);
  return [["key", key], ["decrypt", decrypt], ["launch", 1200]];
}

const LABELS = {
  key: "Deriving the key from your password",
  decrypt: "Decrypting the vault",
  launch: "Starting the browser",
  encrypt: "Encrypting the vault",
  verify: "Checking the new vault",
  wipe: "Wiping the decrypted copy",
};

export function estimate(p, st) {
  if (st.state === "closing") {
    return { fraction: 0.04, label: "Closing the browser", left: null };
  }
  const t = track(p.id, st);
  const list = steps(p, st);
  const index = Math.max(0, list.findIndex(([name]) => name === st.phase));
  const elapsed = t.now - t.phaseStart;
  const [, expected] = list[index];
  let stepFraction;
  let stepLeft;
  if (t.f > 0) {
    stepFraction = t.f;
    stepLeft = t.rate > 0 && t.f > 0.02 ? (1 - t.f) / t.rate : expected * (1 - t.f);
  } else {
    stepFraction = Math.min(0.95, elapsed / expected);
    stepLeft = Math.max(0, expected - elapsed);
  }
  const total = list.reduce((sum, [, ms]) => sum + ms, 0);
  const done = list.slice(0, index).reduce((sum, [, ms]) => sum + ms, 0) + expected * stepFraction;
  const after = list.slice(index + 1).reduce((sum, [, ms]) => sum + ms, 0);
  return {
    fraction: Math.min(1, done / total),
    label: LABELS[st.phase] || (st.state === "sealing" ? "Encrypting" : "Unlocking"),
    left: stepLeft + after,
  };
}

export function formatLeft(ms) {
  if (ms == null || !isFinite(ms)) {
    return "";
  }
  if (ms < 1500) {
    return "almost done";
  }
  const s = Math.round(ms / 1000);
  if (s < 60) {
    return `about ${s} s left`;
  }
  return `about ${Math.round(s / 60)} min left`;
}


function cyclePath(cols, rows) {
  const path = [];
  for (let x = 0; x < cols; x++) {
    path.push([x, 0]);
  }
  for (let y = 1; y < rows; y++) {
    if (y % 2 === 1) {
      for (let x = cols - 1; x >= 1; x--) {
        path.push([x, y]);
      }
    } else {
      for (let x = 1; x < cols; x++) {
        path.push([x, y]);
      }
    }
  }
  for (let y = rows - 1; y >= 1; y--) {
    path.push([0, y]);
  }
  return path;
}

function mix(hex, alpha) {
  const n = parseInt(hex.slice(1), 16);
  return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`;
}

export function createSnake(color = "#b89cff", size = "sm") {
  const cell = size === "lg" ? 12 : 9;
  const rows = size === "lg" ? 8 : 6;
  const canvas = h("canvas", { class: "snake-board", "aria-hidden": "true" });
  const label = h("span", { class: "snake-label" });
  const left = h("span", { class: "snake-left" });
  const node = h("div", {
    class: `snake snake-${size}`,
    role: "progressbar",
    "aria-valuemin": "0",
    "aria-valuemax": "100",
  }, canvas, h("div", { class: "snake-text" }, label, left));

  let cols = 0;
  let path = [];
  let head = 0;
  let length = 3;
  let target = 0;
  let fraction = 0;
  let food = 8;
  let eaten = 0;
  let last = performance.now();
  let lastEstimate = 0;
  let running = false;
  let current = null;
  let colors = null;
  const speed = size === "lg" ? 15 : 12;
  const snakeColor = /^#[0-9a-f]{6}$/i.test(color) ? color : "#b89cff";

  function refresh() {
    if (!current) {
      return;
    }
    const e = estimate(current.p, current.st);
    fraction = e.fraction;
    const max = Math.max(6, Math.floor(path.length * 0.55));
    target = 3 + (max - 3) * fraction;
    const pct = Math.round(fraction * 100);
    label.textContent = `${e.label} · ${pct}%`;
    left.textContent = formatLeft(e.left);
    node.setAttribute("aria-valuenow", String(pct));
    node.setAttribute("aria-label", label.textContent);
  }

  function layout() {
    const width = Math.max(120, Math.floor(node.clientWidth || (size === "lg" ? 360 : 240)));
    const nextCols = Math.max(8, Math.floor(width / cell));
    if (nextCols === cols) {
      return;
    }
    cols = nextCols;
    path = cyclePath(cols, rows);
    const dpr = window.devicePixelRatio || 1;
    canvas.width = cols * cell * dpr;
    canvas.height = rows * cell * dpr;
    canvas.style.width = `${cols * cell}px`;
    canvas.style.height = `${rows * cell}px`;
    head %= path.length;
    food = (Math.floor(head) + 6) % path.length;
    colors = null;
  }

  function at(position) {
    const n = path.length;
    const i = ((Math.floor(position) % n) + n) % n;
    const [x0, y0] = path[i];
    const [x1, y1] = path[(i + 1) % n];
    const f = position - Math.floor(position);
    return [x0 + (x1 - x0) * f, y0 + (y1 - y0) * f];
  }

  function draw(now) {
    const ctx = canvas.getContext("2d");
    const dpr = window.devicePixelRatio || 1;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, cols * cell, rows * cell);
    if (!colors) {
      const style = getComputedStyle(node);
      colors = {
        dot: style.getPropertyValue("--snake-dot").trim() || "rgba(128,128,128,0.18)",
        food: style.getPropertyValue("--snake-food").trim() || "#F5A524",
      };
    }
    const { dot, food: foodColor } = colors;

    ctx.fillStyle = dot;
    for (let y = 0; y < rows; y++) {
      for (let x = 0; x < cols; x++) {
        ctx.fillRect(x * cell + cell / 2 - 0.75, y * cell + cell / 2 - 0.75, 1.5, 1.5);
      }
    }

    const [fx, fy] = path[food];
    const pulse = 1 + 0.12 * Math.sin(now / 180);
    ctx.fillStyle = foodColor;
    ctx.beginPath();
    ctx.arc(fx * cell + cell / 2, fy * cell + cell / 2, cell * 0.3 * pulse, 0, Math.PI * 2);
    ctx.fill();

    const segments = Math.max(2, Math.round(length * 2));
    for (let s = segments; s >= 0; s--) {
      const pos = head - (s / segments) * length;
      const [x, y] = at(pos);
      const t = 1 - s / segments;
      ctx.fillStyle = mix(snakeColor, 0.35 + 0.65 * t);
      const r = cell * (0.28 + 0.14 * t);
      ctx.beginPath();
      ctx.arc(x * cell + cell / 2, y * cell + cell / 2, r, 0, Math.PI * 2);
      ctx.fill();
    }

    const [hx, hy] = at(head);
    const cx = hx * cell + cell / 2;
    const cy = hy * cell + cell / 2;
    if (now - eaten < 220) {
      ctx.fillStyle = mix(snakeColor, 0.25 * (1 - (now - eaten) / 220));
      ctx.beginPath();
      ctx.arc(cx, cy, cell * 0.95, 0, Math.PI * 2);
      ctx.fill();
    }
    ctx.fillStyle = snakeColor;
    ctx.beginPath();
    ctx.arc(cx, cy, cell * 0.44, 0, Math.PI * 2);
    ctx.fill();
    const [nx, ny] = at(head + 0.5);
    const angle = Math.atan2(ny - hy, nx - hx);
    ctx.fillStyle = "rgba(20, 18, 24, 0.85)";
    for (const side of [-1, 1]) {
      const ex = cx + Math.cos(angle) * cell * 0.16 + Math.cos(angle + side * Math.PI / 2) * cell * 0.17;
      const ey = cy + Math.sin(angle) * cell * 0.16 + Math.sin(angle + side * Math.PI / 2) * cell * 0.17;
      ctx.beginPath();
      ctx.arc(ex, ey, cell * 0.07, 0, Math.PI * 2);
      ctx.fill();
    }
  }

  function frame(now) {
    if (!node.isConnected) {
      running = false;
      return;
    }
    layout();
    const dt = Math.min(0.1, (now - last) / 1000);
    last = now;
    if (now - lastEstimate > 250) {
      lastEstimate = now;
      refresh();
    }
    if (reducedMotion.matches) {
      head += (fraction * path.length * 0.8 - head) * Math.min(1, dt * 3);
    } else {
      head += speed * dt;
    }
    length += (target - length) * Math.min(1, dt * 4);
    const n = path.length;
    const reach = ((food - Math.floor(head)) % n + n) % n;
    if (reach === 0 || reach > n - 2) {
      eaten = now;
      food = (Math.floor(head) + 5 + Math.floor(Math.random() * Math.max(4, n / 5))) % n;
    }
    head %= n;
    draw(now);
    requestAnimationFrame(frame);
  }

  function start() {
    if (!running) {
      running = true;
      last = performance.now();
      requestAnimationFrame(frame);
    }
  }

  return {
    node,
    update(p, st) {
      current = { p, st };
      layout();
      refresh();
      start();
    },
  };
}

const cardSnakes = new Map();

export function unlockProgress(p, st) {
  let snake = cardSnakes.get(p.id);
  if (!snake) {
    snake = createSnake(p.color, "sm");
    cardSnakes.set(p.id, snake);
  }
  snake.update(p, st);
  queueMicrotask(() => {
    for (const [id, s] of cardSnakes) {
      if (!s.node.isConnected && id !== p.id) {
        cardSnakes.delete(id);
      }
    }
  });
  return snake.node;
}

export function resetProgress(id) {
  trackers.delete(id);
  cardSnakes.delete(id);
}
