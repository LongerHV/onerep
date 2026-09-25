// Shared harness for the browser end-to-end checks: starts onerep with a
// fresh database and headless Chromium, drives Chromium over the DevTools
// Protocol (Node's built-in WebSocket, no npm dependencies), and counts
// checks. Each script calls setup(name) once.
//
// Usage: ONEREP_BIN=bin/onerep CHROMIUM=chromium node test/e2e/<script>.mjs
// (task e2e builds the binary and provides Chromium from nixpkgs).
import { spawn } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

export async function waitFor(check, what, timeout = 15_000) {
  const end = Date.now() + timeout;
  for (;;) {
    if (await check()) return;
    if (Date.now() > end) throw new Error(`timed out waiting for ${what}`);
    await sleep(100);
  }
}

export function setup(name) {
  const BIN = process.env.ONEREP_BIN || "bin/onerep";
  const CHROMIUM = process.env.CHROMIUM || "chromium";
  const PORT = 18100 + Math.floor(Math.random() * 800);
  const BASE = `http://127.0.0.1:${PORT}`;
  const dir = mkdtempSync(join(tmpdir(), `onerep-e2e-${name}-`));
  const results = [];
  let server = null;

  function startServer() {
    server = spawn(BIN, ["serve"], {
      env: { ...process.env, ONEREP_ENV: "dev", ONEREP_DEV_USER: "e2e", ONEREP_DB: join(dir, "e2e.db"), ONEREP_LISTEN: `127.0.0.1:${PORT}` },
      stdio: ["ignore", "ignore", "pipe"],
    });
    server.stderr.on("data", (d) => process.env.E2E_VERBOSE && process.stderr.write(d));
    return waitFor(async () => (await fetch(`${BASE}/healthz`).catch(() => null))?.ok, "server to start");
  }

  async function stopServer() {
    const exited = new Promise((r) => server.once("exit", r));
    server.kill("SIGTERM");
    await exited;
  }

  async function browser() {
    const proc = spawn(CHROMIUM, ["--headless=new", "--no-sandbox", "--disable-gpu", "--remote-debugging-port=0",
      `--user-data-dir=${join(dir, "chrome")}`, "about:blank"], { stdio: ["ignore", "ignore", "pipe"] });
    const wsURL = await new Promise((resolve, reject) => {
      let buf = "";
      proc.stderr.on("data", (d) => {
        buf += d;
        const m = buf.match(/DevTools listening on (ws:\/\/\S+)/);
        if (m) resolve(m[1]);
      });
      proc.on("exit", () => reject(new Error("chromium exited:\n" + buf)));
    });
    const http = wsURL.replace("ws://", "http://").replace(/\/devtools\/.*/, "");
    const target = await (await fetch(`${http}/json/new?about:blank`, { method: "PUT" })).json();
    const ws = new WebSocket(target.webSocketDebuggerUrl);
    await new Promise((r) => ws.addEventListener("open", r, { once: true }));
    let id = 0;
    const pending = new Map();
    ws.addEventListener("message", (e) => {
      const msg = JSON.parse(e.data);
      if (msg.id && pending.has(msg.id)) pending.get(msg.id)(msg);
    });
    const send = (method, params = {}) => new Promise((resolve) => {
      const i = ++id;
      pending.set(i, resolve);
      ws.send(JSON.stringify({ id: i, method, params }));
    });
    const evaluate = async (expression) => {
      const res = await send("Runtime.evaluate", { expression, awaitPromise: true, returnByValue: true });
      if (res.result?.exceptionDetails) throw new Error(`in page: ${res.result.exceptionDetails.exception?.description}`);
      return res.result?.result?.value;
    };
    await send("Runtime.enable");
    await send("Page.enable");
    return { proc, ws, send, evaluate };
  }

  function check(name, ok, detail = "") {
    results.push({ name, ok });
    console.log(`${ok ? "ok  " : "FAIL"} ${name}${ok ? "" : "  " + detail}`);
  }

  // finish closes the browser and the server, removes the temp dir, prints
  // the summary and exits with the result.
  async function finish(b) {
    b?.ws.close();
    b?.proc.kill("SIGKILL");
    if (server && server.exitCode === null) await stopServer().catch(() => {});
    rmSync(dir, { recursive: true, force: true });
    const failed = results.filter((r) => !r.ok).length;
    console.log(`\n${results.length - failed}/${results.length} checks passed`);
    process.exit(failed ? 1 : 0);
  }

  return { BASE, startServer, stopServer, browser, check, finish, waitFor, sleep };
}
