// End-to-end check that offline data never outlives its user: logout clears
// the companion's IndexedDB, outbox and cached workout pages (Clear-Site-Data,
// and companion.js on the signed-out page), asks before dropping unsynced
// changes, a signed-in page discards data another user left, and old synced
// sessions are pruned. Harness (server, Chromium, checks): harness.mjs.
import { setup } from "./harness.mjs";

const { BASE, startServer, stopServer, browser, check, finish, waitFor } = setup("logout");

// In-page helpers for the companion's IndexedDB ("onerep", version 1).
const OPEN_DB = `new Promise((res, rej) => {
  const r = indexedDB.open("onerep", 1);
  r.onupgradeneeded = () => {
    r.result.createObjectStore("sessions", { keyPath: "sessionId" });
    r.result.createObjectStore("outbox", { keyPath: "op_id" });
    r.result.createObjectStore("failed", { keyPath: "op_id" });
  };
  r.onsuccess = () => res(r.result);
  r.onerror = () => rej(r.error);
})`;
const idbDo = (store, mode, body) => `${OPEN_DB}.then((d) => new Promise((res, rej) => {
  const t = d.transaction(${JSON.stringify(store)}, ${JSON.stringify(mode)});
  const q = (${body})(t.objectStore(${JSON.stringify(store)}));
  t.oncomplete = () => { d.close(); res(q && q.result); };
  t.onerror = () => rej(t.error);
}))`;

let b;
try {
  await startServer();
  b = await browser();
  const { send, evaluate } = b;
  const go = async (path) => {
    await send("Page.navigate", { url: BASE + path });
    await waitFor(() => evaluate("document.readyState === 'complete'"), `load of ${path}`);
  };
  const count = (store) => evaluate(idbDo(store, "readonly", "(s) => s.count()"));
  const put = (store, value) => evaluate(idbDo(store, "readwrite", `(s) => s.put(${JSON.stringify(value)})`));
  const pageKeys = () => evaluate(`caches.open("pages").then((c) => c.keys()).then((ks) => ks.map((k) => new URL(k.url).pathname))`);
  const owner = () => evaluate(`localStorage.getItem("onerep.offline-owner")`);

  // An empty workout: its state is saved and its page cached for offline use.
  await go("/");
  const user = await evaluate("document.body.dataset.user");
  await evaluate(`[...document.querySelectorAll('button')].find(b => b.textContent.trim() === 'Start an empty workout').click()`);
  const addExercise = `document.querySelector('#companion select[aria-label="Add exercise"]')`;
  await waitFor(() => evaluate(`!!${addExercise}`), "the empty workout screen");
  const live = await evaluate("location.pathname");
  const sessionID = live.split("/")[2];
  await waitFor(async () => (await count("sessions")) === 1 && (await pageKeys()).includes(live), "offline copies");
  check("the workout is stored offline for its user", (await owner()) === user, `owner ${await owner()}, user ${user}`);

  // Old synced sessions and their cached pages are pruned; the open one stays.
  const month = 30 * 86_400_000;
  await put("sessions", { sessionId: "old-session", finished: true, savedAt: Date.now() - month, sets: {} });
  await evaluate(`caches.open("pages").then((c) => c.put("/sessions/old-session/live", new Response("old")))`);
  await send("Page.reload");
  await waitFor(() => evaluate(`!!${addExercise}`), "the workout screen after reload");
  await waitFor(async () => (await count("sessions")) === 1 && !(await pageKeys()).includes("/sessions/old-session/live"), "pruning")
    .catch(() => {});
  check("old synced sessions and their pages are pruned",
    (await count("sessions")) === 1 && JSON.stringify(await pageKeys()) === JSON.stringify([live]),
    `sessions ${await count("sessions")}, pages ${JSON.stringify(await pageKeys())}`);

  // Data another user left (their session ended without logging out) is
  // discarded, not synced under this user.
  await evaluate(`localStorage.setItem("onerep.offline-owner", "someone-else")`);
  await put("outbox", { op_id: "01900000-0000-7000-8000-00000000e2e0", user: "someone-else", op: "finish_session",
    payload: { session_id: "their-session", finished_at: new Date().toISOString() }, client_ts: new Date().toISOString() });
  await put("sessions", { sessionId: "their-session", finished: false, savedAt: Date.now(), sets: {} });
  await evaluate(`caches.open("pages").then((c) => c.put("/sessions/their-session/live", new Response("theirs")))`);
  await go("/");
  await waitFor(async () => (await owner()) === user, "the owner check");
  check("another user's offline data is discarded",
    (await count("outbox")) === 0 && (await count("sessions")) === 0 && (await pageKeys()).length === 0,
    `outbox ${await count("outbox")}, sessions ${await count("sessions")}, pages ${JSON.stringify(await pageKeys())}`);

  // Back to the workout; with the server gone, a set stays queued.
  await go(live);
  await waitFor(() => evaluate(`!!${addExercise}`), "the workout screen");
  await stopServer();
  await evaluate(`const s = ${addExercise}; s.value = 'dumbbell-curl'; s.dispatchEvent(new Event('change'))`);
  const doneButton = `[...document.querySelectorAll('#companion button')].find(b => b.textContent.trim() === 'Done')`;
  await waitFor(() => evaluate(`!!${doneButton}`), "the curl set");
  await evaluate(`document.querySelector('input[name=weight]').value = '12'; document.querySelector('input[name=reps]').value = '10'; ${doneButton}.click()`);
  await waitFor(async () => (await count("outbox")) === 1, "the queued set");

  // Logging out now asks first; saying no keeps the change.
  const logout = `document.querySelector('form[action="/auth/logout"] button').click()`;
  await evaluate(`window.asked = 0; window.confirm = () => (window.asked++, false)`);
  await evaluate(logout);
  await waitFor(() => evaluate("window.asked === 1"), "the unsynced-changes question").catch(() => {});
  check("logout asks before dropping unsynced changes",
    (await evaluate("window.asked")) === 1 && (await evaluate("location.pathname")) === live && (await count("outbox")) === 1);

  // The server is back: the set syncs and logout clears everything.
  await startServer();
  await evaluate("window.dispatchEvent(new Event('online'))");
  await waitFor(async () => (await count("outbox")) === 0, "the set to sync", 20_000);
  // A cache companion.js never touches: gone only if Clear-Site-Data ran.
  await evaluate(`caches.open("e2e-marker").then((c) => c.put("/marker", new Response("x")))`);
  await evaluate(logout);
  await waitFor(() => evaluate("location.pathname === '/auth/signed-out' && document.readyState === 'complete'"), "the signed-out page");
  const caches = await evaluate("caches.keys()");
  check("logout's Clear-Site-Data clears the browser's storage", !caches.includes("e2e-marker") && !caches.includes("pages"),
    JSON.stringify(caches));
  await waitFor(async () => (await count("sessions")) === 0, "cleared IndexedDB").catch(() => {});
  check("logout clears the offline workout, outbox and owner",
    (await count("sessions")) === 0 && (await count("outbox")) === 0 && (await owner()) === null);

  // Browsers that ignore Clear-Site-Data: the signed-out page clears it too.
  await put("sessions", { sessionId: sessionID, finished: false, savedAt: Date.now(), sets: {} });
  await put("outbox", { op_id: "01900000-0000-7000-8000-00000000e2e1", user, op: "edit_notes", payload: { session_id: sessionID } });
  await evaluate(`caches.open("pages").then((c) => c.put(${JSON.stringify(live)}, new Response("mine")))`);
  await send("Page.reload");
  await waitFor(async () => (await count("outbox")) === 0 && (await count("sessions")) === 0, "the signed-out page to clear")
    .catch(() => {});
  check("the signed-out page clears offline data itself",
    (await count("outbox")) === 0 && (await count("sessions")) === 0 && !(await evaluate("caches.keys()")).includes("pages"));

  // Signing in again (the dev bypass) shows the set reached the server before logout.
  const sets = await evaluate(`fetch("/history/${sessionID}").then(r => r.text()).then(t => (t.match(/name="set_id"/g) || []).length)`);
  check("the queued set synced before logging out", sets === 1, `server has ${sets}`);
} catch (err) {
  check("scenario ran to the end", false, err.stack || String(err));
} finally {
  await finish(b);
}
