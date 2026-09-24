// Companion mode (spec §9): the live workout screen. It works offline: state
// and queued operations live in IndexedDB and sync when the network allows.
// Loaded once from the layout; sets itself up when a #companion element
// appears (htmx.onLoad) and tears down when it is swapped away.
import * as core from "./companion-core.js";
import { fromKg, toKg } from "./calc.js";

if ("serviceWorker" in navigator) {
  navigator.serviceWorker.register("/sw.js").catch((err) => console.warn("service worker not registered", err));
}

// --- IndexedDB -------------------------------------------------------------

const DB_NAME = "onerep";
let dbPromise;

function db() {
  dbPromise ??= new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, 1);
    req.onupgradeneeded = () => {
      req.result.createObjectStore("sessions", { keyPath: "sessionId" });
      req.result.createObjectStore("outbox", { keyPath: "op_id" });
      req.result.createObjectStore("failed", { keyPath: "op_id" });
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
  return dbPromise;
}

async function tx(store, mode, fn) {
  const d = await db();
  return new Promise((resolve, reject) => {
    const t = d.transaction(store, mode);
    const result = fn(t.objectStore(store));
    t.oncomplete = () => resolve(result && "result" in result ? result.result : undefined);
    t.onerror = () => reject(t.error);
  });
}

const idb = {
  get: (store, k) => tx(store, "readonly", (s) => s.get(k)),
  all: (store) => tx(store, "readonly", (s) => s.getAll()),
  put: (store, v) => tx(store, "readwrite", (s) => s.put(v)),
  del: (store, k) => tx(store, "readwrite", (s) => s.delete(k)),
};

// --- sync --------------------------------------------------------------------

let flushing = false;
let syncListeners = new Set();

function csrfToken() {
  try {
    return JSON.parse(document.body.getAttribute("hx-headers") || "{}")["X-CSRF-Token"] || "";
  } catch {
    return "";
  }
}

async function refreshToken() {
  const res = await fetch("/api/csrf", { credentials: "same-origin" });
  if (!res.ok) return false;
  const { csrf } = await res.json();
  document.body.setAttribute("hx-headers", JSON.stringify({ "X-CSRF-Token": csrf }));
  return true;
}

// syncStatus is shown in the header: pending count, failures, sign-in needed.
const syncStatus = { pending: 0, failedOps: [], relogin: false, error: null, offline: !navigator.onLine };

function notify() {
  for (const fn of syncListeners) fn();
}

async function refreshCounts() {
  syncStatus.pending = (await idb.all("outbox")).length;
  syncStatus.failedOps = await idb.all("failed");
  syncStatus.offline = !navigator.onLine;
  notify();
}

// flush sends queued operations in order. It is safe to call any time.
async function flush() {
  if (flushing || !navigator.onLine) return refreshCounts();
  flushing = true;
  try {
    for (let retried = false; ; ) {
      const ops = (await idb.all("outbox")).sort((a, b) => (a.op_id < b.op_id ? -1 : 1)).slice(0, 100);
      if (ops.length === 0) break;
      const res = await fetch("/api/sync", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrfToken() },
        body: JSON.stringify({ ops }),
      });
      if (res.status === 401) {
        syncStatus.relogin = true;
        break;
      }
      if (res.status === 403 && !retried && (await refreshToken())) {
        retried = true;
        continue;
      }
      if (!res.ok) {
        // Say so instead of showing "unsynced" forever; retried every 30 s.
        syncStatus.error = `sync failed (HTTP ${res.status}), will retry`;
        break;
      }
      syncStatus.relogin = false;
      syncStatus.error = null;
      const { results } = await res.json();
      const { failed } = core.settle(ops, results);
      const answered = new Set(results.map((r) => r.op_id));
      for (const o of ops) if (answered.has(o.op_id)) await idb.del("outbox", o.op_id);
      for (const f of failed) await idb.put("failed", f);
    }
  } catch (err) {
    console.warn("sync failed, will retry", err);
  } finally {
    flushing = false;
    await refreshCounts();
  }
}

window.addEventListener("online", flush);
window.addEventListener("offline", refreshCounts);
setInterval(() => {
  if (syncStatus.pending > 0) flush();
}, 30_000);

// --- rendering helpers ---------------------------------------------------------

function h(tag, attrs = {}, ...children) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v === null || v === undefined || v === false) continue;
    if (k.startsWith("on")) el.addEventListener(k.slice(2), v);
    else if (k === "class") el.className = v;
    else el.setAttribute(k, v === true ? "" : v);
  }
  for (const c of children.flat()) if (c !== null && c !== undefined && c !== false) el.append(c);
  return el;
}

const fmt = (v) => String(Math.round(v * 100) / 100);
const weightText = (kg, unit) => (kg === null || kg === undefined ? "" : `${fmt(fromKg(kg, unit))} ${unit}`);
const repsText = (r) => (!r ? "" : r.amrap ? "AMRAP" : r.min === r.max ? String(r.min) : `${r.min}-${r.max}`);
const btn = "rounded bg-zinc-900 px-3 py-2 text-sm font-medium text-white dark:bg-zinc-100 dark:text-zinc-900";
const btn2 = "rounded border border-zinc-300 px-3 py-2 text-sm dark:border-zinc-700";
const prBadge = "rounded bg-amber-200 px-1.5 py-0.5 text-xs font-medium text-amber-900 dark:bg-amber-900 dark:text-amber-100";
const input = "w-full rounded border border-zinc-300 bg-white px-2 py-2 text-lg dark:border-zinc-700 dark:bg-zinc-900";

function setText(set, unit) {
  const parts = [];
  if (set.weight_kg) parts.push(weightText(set.weight_kg, unit));
  if (set.reps !== null && set.reps !== undefined) parts.push(`× ${set.reps}`);
  if (set.duration_s) parts.push(`${set.duration_s}s`);
  if (set.distance_m) parts.push(`${set.distance_m} m`);
  if (set.rpe) parts.push(`@ ${set.rpe}`);
  return parts.join(" ") || "done";
}

// --- the companion ----------------------------------------------------------------

class Companion {
  constructor(root, boot) {
    this.root = root;
    this.boot = boot;
    this.unit = boot.unit;
    this.editing = null; // set id being edited from the overview
    this.timer = null;
    this.wakeLock = null;
    this.audio = null;
    this.onSync = () => this.renderSync();
  }

  async start() {
    const saved = await idb.get("sessions", this.boot.session.id).catch(() => null);
    this.state = core.mergeServerSets(this.boot, saved || core.newState(this.boot), this.boot.sets);
    if (this.boot.session.finished) this.state.finished = true;
    await this.save();
    syncListeners.add(this.onSync);
    this.timer = setInterval(() => this.tick(), 250);
    this.onVisible = () => document.visibilityState === "visible" && this.lockScreen();
    document.addEventListener("visibilitychange", this.onVisible);
    this.lockScreen();
    // Keep a copy of this page for offline reloads, however the user got here.
    window.caches?.open("pages").then((c) => c.add(location.pathname)).catch(() => {});
    this.render();
    flush();
  }

  stop() {
    clearInterval(this.timer);
    syncListeners.delete(this.onSync);
    document.removeEventListener("visibilitychange", this.onVisible);
    this.wakeLock?.release().catch(() => {});
  }

  async lockScreen() {
    if (this.state?.finished || !("wakeLock" in navigator) || this.wakeLock) return;
    try {
      this.wakeLock = await navigator.wakeLock.request("screen");
      this.wakeLock.addEventListener("release", () => (this.wakeLock = null));
    } catch {
      // Not allowed right now (e.g. page hidden); retried when visible again.
    }
  }

  async save() {
    await idb.put("sessions", this.state);
  }

  // apply stores a new state and queues its operation, then syncs.
  async apply({ state, op }) {
    this.state = state;
    await this.save();
    if (op) await idb.put("outbox", op);
    this.render();
    await refreshCounts();
    flush();
  }

  async update(state) {
    this.state = state;
    await this.save();
    this.render();
  }

  // --- rest timer ---

  tick() {
    const el = this.root.querySelector("[data-rest]");
    if (!document.body.contains(this.root)) return this.stop(); // swapped away
    if (!el) return;
    const left = this.state.restEnd ? Math.ceil((this.state.restEnd - Date.now()) / 1000) : 0;
    if (left > 0) {
      el.textContent = `Rest ${Math.floor(left / 60)}:${String(left % 60).padStart(2, "0")}`;
      el.hidden = false;
    } else if (this.state.restEnd) {
      this.state.restEnd = null;
      this.save();
      el.hidden = true;
      this.alarm();
    } else {
      el.hidden = true;
    }
  }

  alarm() {
    navigator.vibrate?.([300, 150, 300]);
    try {
      this.audio ??= new AudioContext();
      const o = this.audio.createOscillator();
      const g = this.audio.createGain();
      o.frequency.value = 880;
      g.gain.setValueAtTime(0.2, this.audio.currentTime);
      g.gain.exponentialRampToValueAtTime(0.001, this.audio.currentTime + 0.6);
      o.connect(g).connect(this.audio.destination);
      o.start();
      o.stop(this.audio.currentTime + 0.6);
    } catch {
      // No audio available.
    }
  }

  // --- views ---

  render() {
    const b = this.boot;
    const s = this.state;
    const step = s.finished ? null : core.currentStep(b, s);
    this.root.replaceChildren(
      h("div", { class: "flex items-center justify-between" },
        h("h1", { class: "text-2xl font-semibold" }, b.session.name),
        h("span", { "data-sync": true, class: "text-sm" })),
      h("p", { "data-rest": true, class: "mt-2 text-3xl font-semibold tabular-nums", hidden: true }),
      this.prBanner && h("p", {
        "data-pr-banner": true, role: "status",
        class: "mt-3 rounded bg-amber-100 p-2 text-sm font-medium text-amber-900 dark:bg-amber-950 dark:text-amber-100",
      }, this.prBanner),
      s.finished ? this.finishedView() : step ? this.stepView(step) : this.doneView(),
      h("div", { "data-failed": true }, this.failedView()),
      this.overview(),
      this.notesView(),
    );
    this.renderSync();
    this.tick();
  }

  renderSync() {
    const el = this.root.querySelector("[data-sync]");
    if (!el) return;
    const st = syncStatus;
    const rejected = core.failedFor(st.failedOps, this.state.sessionId).length;
    el.replaceChildren(
      st.relogin ? h("a", { href: "/auth/login", "hx-boost": "false", class: "text-red-600 underline" }, "Sign in again to sync")
        : st.error ? h("span", { class: "text-red-600" }, st.error)
        : st.pending > 0 ? h("span", { class: "text-amber-600" }, `${st.pending} unsynced${st.offline ? " (offline)" : ""}`)
        : h("span", { class: "text-zinc-500" }, st.offline ? "offline" : "saved"),
      rejected > 0 && h("span", { class: "ml-2 text-red-600" }, `· ${rejected} rejected`));
    const list = this.root.querySelector("[data-failed]");
    if (list) list.replaceChildren(...this.failedView());
  }

  // failedView lists this session's rejected changes with the server's reason.
  failedView() {
    const mine = core.failedFor(syncStatus.failedOps, this.state.sessionId);
    if (mine.length === 0) return [];
    const describe = (f) => (f.op === "upsert_set" ? `Set: ${setText(f.payload, this.unit)}` : f.op.replace("_", " "));
    return [h("div", { class: "mt-4 rounded border border-red-300 p-3 text-sm dark:border-red-800" },
      h("p", { class: "font-medium text-red-700 dark:text-red-400" }, "The server rejected these changes:"),
      h("ul", { class: "mt-1 list-inside list-disc" }, mine.map((f) => h("li", {}, `${describe(f)}: ${f.reason}`))),
      h("button", {
        class: btn2 + " mt-2",
        onclick: async () => {
          for (const f of mine) await idb.del("failed", f.op_id);
          await refreshCounts();
        },
      }, "Dismiss"))];
  }

  // submitSet validates a set form and logs it at step, or shows why not.
  submitSet(form, t, step, measurement) {
    const values = this.read(form, t);
    const problem = core.validateValues(measurement, values);
    const msg = form.querySelector("[data-error]");
    if (problem) {
      msg.textContent = problem;
      return;
    }
    const res = core.logSet(this.boot, this.state, step, values);
    const set = res.op.payload;
    this.prBanner = core.isPR(this.boot, res.state, set) ? `New PR: ${core.exerciseInfo(this.boot, set.slug).name} ${setText(set, this.unit)}` : null;
    this.apply(res);
  }

  stepView(step) {
    const b = this.boot;
    const t = core.target(b, this.state, step);
    const ex = core.groups(b, this.state)[step.g].exercises[step.e];
    const grp = core.groups(b, this.state)[step.g];
    const label = grp.exercises.length > 1 ? `${String.fromCharCode(65 + step.e)}${step.s + 1} · ` : "";
    const info = core.exerciseInfo(b, t.slug);
    const last = (info.last || []).map((x) => setText(x, this.unit)).join(", ");
    const fields = this.inputs(t);
    const form = h("form", {
      class: "mt-3 space-y-3",
      onsubmit: (e) => {
        e.preventDefault();
        this.submitSet(form, t, step, t.measurement);
      },
    }, fields,
    h("p", { "data-error": true, class: "text-sm text-red-600", role: "alert" }),
    h("div", { class: "flex flex-wrap gap-2" },
      h("button", { type: "submit", class: btn }, "Done"),
      h("button", { type: "button", class: btn2, onclick: () => this.update(core.skip(this.state, step)) }, "Skip"),
      h("button", { type: "button", class: btn2, onclick: () => this.update(core.addSet(this.state, step.g, step.e)) }, "Add set"),
      ex.alternatives.length > 0 && h("select", {
        class: btn2, "aria-label": "Swap exercise",
        onchange: (e) => e.target.value && this.update(core.swap(this.state, step.g, step.e, e.target.value, ex.planned)),
      }, h("option", { value: "" }, "Swap…"), ex.alternatives.map((a) => h("option", { value: a }, core.exerciseInfo(b, a).name)))));
    return h("section", { class: "mt-4 rounded border border-zinc-200 p-4 dark:border-zinc-800" },
      h("p", { class: "text-sm text-zinc-500" }, `${label}${t.kind !== "working" ? t.kind : "set " + (step.s + 1)}`),
      h("h2", { class: "text-xl font-semibold" }, t.name),
      h("p", { class: "mt-1 text-lg" }, this.targetText(t)),
      t.per_side.length > 0 && h("p", { class: "text-sm text-zinc-500" }, `per side: ${t.per_side.map(fmt).join(" · ")}`),
      last && h("p", { class: "text-sm text-zinc-500" }, `Last time: ${last}`),
      form);
  }

  targetText(t) {
    const parts = [];
    if (t.duration_s) parts.push(`${t.duration_s}s`);
    else if (t.reps) parts.push(`${repsText(t.reps)} reps`);
    if (t.kg !== null) parts.push(`@ ${weightText(t.kg, this.unit)}`);
    else if (t.load_kind) parts.push("@ pick weight");
    if (t.rpe) parts.push(`RPE ${t.rpe}`);
    return parts.join(" ") || "as you like";
  }

  // inputs returns the fields a set of this measurement records, pre-filled
  // with the target.
  inputs(t, set = null) {
    const m = t.measurement;
    const field = (name, label, value, attrs = {}) =>
      h("label", { class: "block text-sm" }, label,
        h("input", { name, class: input, inputmode: "decimal", value: value ?? "", ...attrs }));
    const weight = set ? set.weight_kg : t.kg;
    const out = [];
    if (m === "weight_reps" || m === "bw_reps") {
      out.push(field("weight", m === "bw_reps" ? `Added weight (${this.unit})` : `Weight (${this.unit})`,
        weight !== null && weight !== undefined ? fmt(fromKg(weight, this.unit)) : ""));
    }
    if (m === "weight_reps" || m === "bw_reps" || m === "reps") {
      out.push(field("reps", "Reps", set ? set.reps : t.reps && !t.reps.amrap ? t.reps.min : "", { inputmode: "numeric" }));
      out.push(field("rpe", "RPE (optional)", set ? set.rpe : t.rpe, { placeholder: "6-10" }));
    }
    if (m === "time" || m === "distance_time") {
      out.push(field("duration", "Seconds", set ? set.duration_s : t.duration_s, { inputmode: "numeric" }));
    }
    if (m === "distance_time") out.push(field("distance", "Distance (m)", set ? set.distance_m : ""));
    return h("div", { class: "grid grid-cols-3 gap-2" }, out);
  }

  read(form, t) {
    const num = (name) => {
      const el = form.elements.namedItem(name);
      if (!el || el.value.trim() === "") return null;
      const v = Number(el.value.replace(",", "."));
      return Number.isFinite(v) ? v : null;
    };
    const w = num("weight");
    return {
      weight_kg: w === null ? (t.measurement === "bw_reps" ? 0 : null) : toKg(w, this.unit),
      reps: num("reps") === null ? null : Math.round(num("reps")),
      rpe: num("rpe"),
      duration_s: num("duration") === null ? null : Math.round(num("duration")),
      distance_m: num("distance"),
    };
  }

  doneView() {
    const empty = core.steps(this.boot, this.state).length === 0;
    return h("section", { class: "mt-4 rounded border border-zinc-200 p-4 dark:border-zinc-800" },
      h("p", {}, empty ? "Add an exercise to start." : "All sets are done or skipped. Add a set below, or finish."),
      h("div", { class: "mt-3 flex flex-wrap gap-2" },
        !empty && h("button", { class: btn, onclick: () => this.finish() }, "Finish workout"),
        this.addExerciseControl()));
  }

  finishedView() {
    return h("section", { class: "mt-4 rounded border border-zinc-200 p-4 dark:border-zinc-800" },
      h("p", { class: "font-medium" }, "Workout finished."),
      h("p", { class: "mt-2 flex gap-4" },
        h("a", { href: `/history/${this.boot.session.id}`, class: "underline" }, "See it in history"),
        h("a", { href: "/", class: "underline" }, "Home")));
  }

  addExerciseControl() {
    return h("select", {
      class: btn2, "aria-label": "Add exercise",
      onchange: (e) => e.target.value && this.update(core.addExercise(this.boot, this.state, e.target.value)),
    }, h("option", { value: "" }, "Add exercise…"), (this.boot.catalog || []).map((c) => h("option", { value: c.slug }, c.name)));
  }

  async finish() {
    await this.apply(core.finish(this.state));
  }

  // overview lists every exercise with its sets; each exercise can take
  // another set (planned or not, e.g. in an empty workout).
  overview() {
    const b = this.boot;
    const s = this.state;
    const all = core.steps(b, s);
    const sections = [];
    core.groups(b, s).forEach((grp, g) => grp.exercises.forEach((ex, e) => {
      const rows = all.filter((st) => st.g === g && st.e === e).map((step) => {
        const set = core.logged(s, step);
        const t = core.target(b, s, step);
        const st = core.status(s, step);
        if (set && this.editing === set.id) return this.editRow(step, set, t);
        return h("li", { class: "flex items-center justify-between gap-2 py-1", "data-status": st },
          h("span", {}, `${step.s + 1}. ${set ? core.exerciseInfo(b, set.slug).name : t.name}`),
          st === "done" ? h("span", { class: "flex items-center gap-2" },
            core.isPR(b, s, set) && h("span", { "data-pr-badge": true, class: prBadge }, "PR"),
            h("button", { class: "text-sm underline", onclick: () => { this.editing = set.id; this.render(); } }, setText(set, this.unit)))
            : st === "skipped" ? h("button", { class: "text-sm text-zinc-500 underline", onclick: () => this.update(core.unskip(s, step)) }, "skipped")
            : h("span", { class: "text-sm text-zinc-500" }, this.targetText(t)));
      });
      sections.push(h("li", { class: "py-2" },
        h("div", { class: "flex items-center justify-between gap-2" },
          h("span", { class: "font-medium" }, ex.name),
          !s.finished && h("button", { class: btn2, onclick: () => this.update(core.addSet(s, g, e)) }, "Add set")),
        h("ul", { class: "ml-2" }, rows)));
    }));
    return h("details", { class: "mt-6", open: true },
      h("summary", { class: "cursor-pointer font-medium" }, "All sets"),
      h("ul", { class: "mt-2 divide-y divide-zinc-200 dark:divide-zinc-800" }, sections),
      !s.finished && h("div", { class: "mt-3 flex flex-wrap gap-2" },
        this.addExerciseControl(),
        core.currentStep(b, s) && h("button", { class: btn2, onclick: () => this.finish() }, "Finish early")));
  }

  editRow(step, set, t) {
    // Edit with the fields of the exercise actually done, even after a swap.
    const measurement = core.exerciseInfo(this.boot, set.slug).measurement;
    const form = h("form", {
      class: "space-y-2 py-2",
      onsubmit: (e) => {
        e.preventDefault();
        const values = this.read(form, t);
        const problem = core.validateValues(measurement, values);
        if (problem) {
          form.querySelector("[data-error]").textContent = problem;
          return;
        }
        this.editing = null;
        this.apply(core.logSet(this.boot, this.state, step, values));
      },
    }, this.inputs({ ...t, measurement }, set),
    h("p", { "data-error": true, class: "text-sm text-red-600", role: "alert" }),
    h("div", { class: "flex gap-2" },
      h("button", { type: "submit", class: btn }, "Save"),
      h("button", { type: "button", class: btn2, onclick: () => { this.editing = null; this.apply(core.deleteSet(this.state, set.id)); } }, "Delete"),
      h("button", { type: "button", class: btn2, onclick: () => { this.editing = null; this.render(); } }, "Cancel")));
    return h("li", {}, form);
  }

  notesView() {
    // Keep what is typed across re-renders (every logged set re-renders).
    const area = h("textarea", {
      class: input + " text-base", rows: "2", "aria-label": "Notes",
      oninput: () => (this.notesDraft = area.value),
    });
    area.value = this.notesDraft ?? this.state.notes;
    return h("label", { class: "mt-6 block text-sm" }, "Notes",
      area,
      h("button", {
        class: btn2 + " mt-2",
        onclick: () => {
          this.notesDraft = null;
          this.apply(core.editNotes(this.state, area.value));
        },
      }, "Save notes"));
  }
}

let current = null;

// started holds workout screens already set up. It lives here rather than in
// a data attribute because htmx's history cache restores the DOM (attributes
// included) without the event handlers, and that restored copy needs a
// fresh start.
const started = new WeakSet();

htmx.onLoad((root) => {
  const el = root.id === "companion" ? root : root.querySelector?.("#companion");
  if (!el || started.has(el)) return;
  started.add(el);
  // A copy restored from htmx's history cache still shows buttons with no
  // handlers; clear it right away so nothing can be tapped until start() renders.
  el.replaceChildren(h("p", { class: "text-zinc-500" }, "Loading your workout…"));
  const boot = JSON.parse(document.getElementById("companion-boot").textContent);
  current?.stop();
  current = new Companion(el, boot);
  current.start().catch((err) => {
    console.error(err);
    el.textContent = "The workout screen failed to start: " + err.message;
  });
});
