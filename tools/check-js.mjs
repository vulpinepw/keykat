import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const roots = process.argv.slice(2);
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "keycat-check-"));
let files = 0, failed = 0;
const walk = dir => fs.readdirSync(dir, { withFileTypes: true }).flatMap(e => e.isDirectory() ? walk(path.join(dir, e.name)) : [path.join(dir, e.name)]);
for (const root of roots) {
  for (const file of walk(root)) {
    const ext = path.extname(file).toLowerCase();
    if (![".js", ".mjs", ".cfg"].includes(ext)) continue;
    files++;
    const src = fs.readFileSync(file, "utf8");
    const target = path.join(tmp, `f${files}${ext === ".cfg" ? ".cjs" : ".mjs"}`);
    fs.writeFileSync(target, src);
    try {
      execFileSync(process.execPath, ["--check", target], { stdio: "pipe" });
    } catch (err) {
      failed++;
      console.log("SYNTAX ERROR in", file, "\n", String(err.stderr).split("\n").slice(0, 6).join("\n"));
    }
    const lines = src.split("\n").filter(l => /^\s*(\/\/|\/\*|\*\s|\*\/)/.test(l));
    if (lines.length) console.log("comment-like lines left in", file, lines.slice(0, 3));
  }
}
fs.rmSync(tmp, { recursive: true, force: true });
console.log(`${files} files checked, ${failed} with syntax errors`);
process.exit(failed ? 1 : 0);
