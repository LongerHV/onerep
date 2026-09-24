// End-to-end check of companion mode's offline logging (spec §9, §17).
// Starts onerep and headless Chromium, then drives a workout over the Chrome
// DevTools Protocol: log a set, lose the server, keep logging and editing,
// reload from the offline cache, get the server back, and check every set
// arrived exactly once. No npm dependencies: Node's built-in WebSocket.
//
// Usage: ONEREP_BIN=bin/onerep CHROMIUM=chromium node test/e2e/companion.mjs
// (task e2e builds the binary and provides Chromium from nixpkgs).
import { spawn } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const BIN = process.env.ONEREP_BIN || "bin/onerep";
const CHROMIUM = process.env.CHROMIUM || "chromium";
const PORT = 18100 + Math.floor(Math.random() * 800);
const BASE = `http://127.0.0.1:${PORT}`;
const dir = mkdtempSync(join(tmpdir(), "onerep-e2e-"));
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const PLAN = JSON.stringify({
  name: "E2E", weeks: 1, days: [
    { name: "Bench day", groups: [{ rest_s: 60, exercises: [{ slug: "barbell-bench-press", sets: [{ count: 4, reps: 5, load: { pct_tm: 0.8 } }] }] }] },
    { name: "Second day", groups: [{ exercises: [{ slug: "pull-up", sets: [{ count: 1, reps: 5 }] }] }] },
  ],
});

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

async function waitFor(check, what, timeout = 15_000) {
  const end = Date.now() + timeout;
  for (;;) {
    if (await check()) return;
    if (Date.now() > end) throw new Error(`timed out waiting for ${what}`);
    await sleep(100);
  }
}

// --- CDP ---------------------------------------------------------------------

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

// --- the scenario ---------------------------------------------------------------

const results = [];
function check(name, ok, detail = "") {
  results.push({ name, ok });
  console.log(`${ok ? "ok  " : "FAIL"} ${name}${ok ? "" : "  " + detail}`);
}

let b;
try {
  await startServer();
  b = await browser();
  const { send, evaluate } = b;
  const go = async (path) => {
    await send("Page.navigate", { url: BASE + path });
    await waitFor(() => evaluate("document.readyState === 'complete'"), `load of ${path}`);
  };
  const csrf = "JSON.parse(document.body.getAttribute('hx-headers'))['X-CSRF-Token']";
  const form = (path, fields) => evaluate(`fetch(${JSON.stringify(path)}, {method: 'POST',
    body: new URLSearchParams({...${JSON.stringify(fields)}, csrf_token: ${csrf}})}).then(r => r.url)`);

  // A plan with four bench sets at 80% of a 100 kg training max.
  await go("/");
  const planURL = await form("/plans", { doc: PLAN, action: "activate" });
  await form(new URL(planURL).pathname + "/follow", {});
  await form("/exercises/barbell-bench-press/settings", { training_max: "100" });

  // Start from the home page (a boosted form: no full page load).
  await go("/");
  await evaluate(`[...document.querySelectorAll('button')].find(b => b.textContent.trim() === 'Start workout').click()`);
  const doneButton = `[...document.querySelectorAll('#companion button')].find(b => b.textContent.trim() === 'Done')`;
  await waitFor(() => evaluate(`!!${doneButton}`), "the workout screen");
  const sessionID = await evaluate("location.pathname.split('/')[2]");
  check("start workout opens the live page", /^[0-9a-f-]{36}$/.test(sessionID), sessionID);
  check("target is 80% of the TM", await evaluate(`document.querySelector('#companion').textContent.includes('@ 80 kg')`));

  const done = () => evaluate(`document.querySelectorAll('#companion [data-status=done]').length`);
  const synced = () => evaluate(`(document.querySelector('#companion [data-sync]')?.textContent || '').includes('saved')`);

  // Set 1 online.
  await evaluate(`${doneButton}.click()`);
  // Wait on the server itself: the badge can say "saved" before the new operation is queued.
  const serverSets = () => evaluate(`fetch("/history/${sessionID}").then(r => r.text()).then(t => (t.match(/name="set_id"/g) || []).length)`);
  await waitFor(async () => (await done()) === 1 && (await serverSets()) === 1, "set 1 to reach the server");
  check("set 1 logged and synced", true);
  await waitFor(() => evaluate("navigator.serviceWorker.controller !== null || (location.reload(), false)"), "the service worker", 20_000)
    .catch(() => {});

  // The server goes away. Keep training.
  await stopServer();
  await evaluate(`document.querySelector('input[name=weight]').value = '82.5'; ${doneButton}.click()`);
  await waitFor(async () => (await done()) === 2, "set 2 locally");
  await evaluate(`${doneButton}.click()`);
  await waitFor(async () => (await done()) === 3, "set 3 locally");
  // Correct set 1 to 7 reps.
  await evaluate(`document.querySelector('#companion [data-status=done] button').click()`);
  await evaluate(`const f = document.querySelector('#companion li form'); f.querySelector('input[name=reps]').value = '7';
    [...f.querySelectorAll('button')].find(b => b.textContent === 'Save').click()`);
  await waitFor(() => evaluate(`document.querySelector('#companion [data-status=done] button').textContent.includes('× 7')`), "the edit");
  check("logging works without the server", await evaluate(
    `(document.querySelector('#companion [data-sync]').textContent || '').includes('unsynced')`), "badge should show unsynced changes");

  // Reload with the server still down: the page comes from the offline cache,
  // the sets from IndexedDB.
  await send("Page.reload");
  await waitFor(() => evaluate(`!!${doneButton}`), "the cached workout screen", 20_000);
  check("offline reload keeps all 3 sets", (await done()) === 3, `found ${await done()}`);

  // Back online: everything syncs once.
  await startServer();
  await send("Page.reload");
  await waitFor(async () => (await synced()) && (await done()) === 3, "sync after reconnecting", 20_000);
  const history = await evaluate(`fetch('/history/${sessionID}').then(r => r.text())`);
  const rows = (history.match(/name="set_id"/g) || []).length;
  check("server has exactly 3 sets", rows === 3, `found ${rows}`);
  check("the offline edit reached the server", /name="reps"[^>]*value="7"/.test(history));
  check("the offline weight change reached the server", history.includes('value="82.5"'));

  // Replaying everything again changes nothing.
  await send("Page.reload");
  await waitFor(() => synced(), "idle sync");
  const again = await evaluate(`fetch('/history/${sessionID}').then(r => r.text())`);
  check("reloading does not duplicate sets", (again.match(/name="set_id"/g) || []).length === 3);

  // A value the server would reject is refused on the spot, not queued.
  await evaluate(`document.querySelector('input[name=rpe]').value = '80'; ${doneButton}.click()`);
  await sleep(300);
  check("an impossible RPE is refused with a message",
    (await evaluate(`document.querySelector('#companion').textContent.includes('RPE must be between 1 and 10')`)) && (await done()) === 3);

  // Notes typed but not saved survive logging a set.
  await evaluate(`const n = document.querySelector('#companion textarea'); n.value = 'left knee ok'; n.dispatchEvent(new Event('input'))`);
  await evaluate(`document.querySelector('input[name=rpe]').value = ''; ${doneButton}.click()`);
  await waitFor(async () => (await done()) === 4, "set 4");
  check("typed notes survive logging a set", await evaluate(`document.querySelector('#companion textarea').value === 'left knee ok'`));

  // One more set, then leave the workout and come back with the Back button.
  await evaluate(`[...document.querySelectorAll('#companion button')].find(b => b.textContent.trim() === 'Add set').click()`);
  await waitFor(() => evaluate(`!!${doneButton}`), "the added set");
  await evaluate(`document.querySelector('nav a[href="/history"]').click()`);
  await waitFor(() => evaluate(`location.pathname === '/history'`), "the history page");
  await evaluate("history.back()");
  await waitFor(() => evaluate(`!!${doneButton}`), "the workout screen after Back");
  await evaluate(`${doneButton}.click()`);
  await waitFor(async () => (await serverSets()) === 5, "set 5 after Back").catch(() => {});
  check("the workout screen works after Back", (await serverSets()) === 5, `server has ${await serverSets()} sets`);

  // Finish: the plan moves on to its second day.
  await evaluate(`[...document.querySelectorAll('#companion button')].find(b => /Finish/.test(b.textContent)).click()`);
  await waitFor(() => evaluate(`document.querySelector('#companion').textContent.includes('Workout finished')`), "the finished screen");
  let home = "";
  await waitFor(async () => (home = await evaluate(`fetch('/').then(r => r.text())`)).includes("Next: Second day"), "the plan to move on")
    .catch(() => {});
  check("finishing advances the plan", home.includes("Next: Second day"), (home.match(/<main[\s\S]*?<\/main>/) || [""])[0].replace(/<[^>]+>/g, " ").replace(/\s+/g, " ").slice(0, 400));

  // An empty workout: add an exercise, log a set, add another set and log it.
  await go("/");
  await evaluate(`[...document.querySelectorAll('button')].find(b => b.textContent.trim() === 'Start an empty workout').click()`);
  const addExercise = `document.querySelector('#companion select[aria-label="Add exercise"]')`;
  await waitFor(() => evaluate(`!!${addExercise}`), "the empty workout screen");
  const adhocID = await evaluate("location.pathname.split('/')[2]");
  await evaluate(`const s = ${addExercise}; s.value = 'dumbbell-curl'; s.dispatchEvent(new Event('change'))`);
  await waitFor(() => evaluate(`!!${doneButton}`), "the curl set");
  await evaluate(`document.querySelector('input[name=weight]').value = '12'; document.querySelector('input[name=reps]').value = '10'; ${doneButton}.click()`);
  await waitFor(async () => (await done()) === 1, "curl set 1");
  await evaluate(`[...document.querySelectorAll('#companion button')].find(b => b.textContent.trim() === 'Add set').click()`);
  await waitFor(() => evaluate(`!!${doneButton}`), "curl set 2");
  await evaluate(`document.querySelector('input[name=reps]').value = '8'; ${doneButton}.click()`);
  const adhocSets = () => evaluate(`fetch("/history/${adhocID}").then(r => r.text()).then(t => (t.match(/name="set_id"/g) || []).length)`);
  await waitFor(async () => (await adhocSets()) === 2, "both curl sets on the server").catch(() => {});
  check("an empty workout can log several sets of an exercise", (await adhocSets()) === 2, `server has ${await adhocSets()}`);
} catch (err) {
  check("scenario ran to the end", false, err.stack || String(err));
} finally {
  b?.ws.close();
  b?.proc.kill("SIGKILL");
  if (server && server.exitCode === null) await stopServer().catch(() => {});
  rmSync(dir, { recursive: true, force: true });
}

const failed = results.filter((r) => !r.ok).length;
console.log(`\n${results.length - failed}/${results.length} checks passed`);
process.exit(failed ? 1 : 0);
