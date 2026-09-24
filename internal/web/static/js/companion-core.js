// Companion mode logic, free of DOM and storage so Node can test it
// (internal/web/jstest/companion.test.mjs). companion.js renders it and
// persists the state and outbox in IndexedDB.
//
// A session's steps come from its snapshot (plan.ExpandedDay): group g,
// exercise e, set s. Supersets alternate A1, B1, A2, B2 within a group.
// Every logged set has a client-generated UUIDv7, so replaying an operation
// after a lost response is harmless (spec §9).

import { dropLoad, e1rm, resolveLoad } from "./calc.js";

// uuidv7 returns a time-ordered UUID (RFC 9562).
export function uuidv7(now = Date.now(), random = crypto.getRandomValues(new Uint8Array(16))) {
  const b = new Uint8Array(random);
  let ts = BigInt(now);
  for (let i = 5; i >= 0; i--) {
    b[i] = Number(ts & 0xffn);
    ts >>= 8n;
  }
  b[6] = (b[6] & 0x0f) | 0x70;
  b[8] = (b[8] & 0x3f) | 0x80;
  const hex = [...b].map((x) => x.toString(16).padStart(2, "0")).join("");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

const iso = (now) => new Date(now).toISOString();
const key = (g, e, s) => `${g}:${e}:${s}`;

// parseReps reads a prescription's reps: 5, "6-10" or "AMRAP" (absent for timed sets).
export function parseReps(v) {
  if (v === undefined || v === null) return null;
  if (typeof v === "number") return { min: v, max: v, amrap: false };
  if (v === "AMRAP") return { min: 0, max: 0, amrap: true };
  const [lo, hi] = String(v).split("-").map(Number);
  return { min: lo, max: hi, amrap: false };
}

// newState is the empty local state of a session.
export function newState(boot) {
  return {
    sessionId: boot.session.id,
    sets: {}, // id -> set (as sent to the server), deleted ones kept with deleted: true
    positions: {}, // "g:e:s" -> set id
    skipped: {}, // "g:e:s" -> true
    extra: {}, // "g:e" -> sets added beyond the prescription
    swaps: {}, // "g:e" -> slug done instead of the planned one
    added: [], // exercises added during the session: [{slug}]
    restEnd: null, // ms timestamp the rest timer ends at
    notes: boot.session.notes || "",
    notesAt: null,
    finished: boot.session.finished,
  };
}

// mergeServerSets folds the server's copy of the session's sets into local
// state; the newer edit of each set wins.
export function mergeServerSets(state, serverSets) {
  const s = structuredClone(state);
  for (const set of serverSets) {
    const local = s.sets[set.id];
    if (local && Date.parse(local.updated_at) >= Date.parse(set.updated_at)) continue;
    s.sets[set.id] = set;
    s.positions[key(set.group_pos, set.exercise_pos, set.set_pos)] = set.id;
  }
  return s;
}

// groups returns the session's groups with swaps, extra and added exercises applied.
export function groups(boot, state) {
  const out = boot.snapshot.groups.map((g, gi) => ({
    rest_s: g.rest_s,
    exercises: g.exercises.map((slot, ei) => exerciseView(boot, state, gi, ei, slot.slug, slot.sets, slot.alternatives || [])),
  }));
  state.added.forEach((a, i) => {
    const gi = boot.snapshot.groups.length + i;
    out.push({ rest_s: 90, exercises: [exerciseView(boot, state, gi, 0, a.slug, [], [])] });
  });
  return out;
}

function exerciseView(boot, state, g, e, planned, prescribed, planAlts) {
  const slug = state.swaps[`${g}:${e}`] || planned;
  let count = prescribed.length + (state.extra[`${g}:${e}`] || 0);
  for (const k of Object.keys(state.positions)) {
    const [pg, pe, ps] = k.split(":").map(Number);
    if (pg === g && pe === e && ps >= count) count = ps + 1; // sets logged elsewhere (another device)
  }
  const info = exerciseInfo(boot, slug);
  const alternatives = [...new Set([...(planAlts || []), ...((boot.exercises[planned] || {}).alternatives || [])])].filter((a) => a !== slug);
  if (slug !== planned) alternatives.unshift(planned);
  return { g, e, planned, slug, name: info.name, measurement: info.measurement, prescribed, count, alternatives };
}

// exerciseInfo is what is known about slug: session exercises carry TM, e1RM
// and equipment; anything else comes from the catalog.
export function exerciseInfo(boot, slug) {
  const ex = boot.exercises[slug];
  if (ex) return ex;
  const c = (boot.catalog || []).find((x) => x.slug === slug) || { name: slug, measurement: "weight_reps" };
  return { name: c.name, measurement: c.measurement, equipment_kind: c.equipment_kind, alternatives: [], last: [] };
}

// steps lists every set in the order it is done: each group round by round.
export function steps(boot, state) {
  const out = [];
  groups(boot, state).forEach((grp, g) => {
    const rounds = Math.max(0, ...grp.exercises.map((x) => x.count));
    for (let s = 0; s < rounds; s++) {
      grp.exercises.forEach((x, e) => {
        if (s < x.count) out.push({ g, e, s, key: key(g, e, s) });
      });
    }
  });
  return out;
}

// logged returns the live (not deleted) set logged at a step, if any.
export function logged(state, step) {
  const id = state.positions[step.key];
  const set = id && state.sets[id];
  return set && !set.deleted ? set : null;
}

export function status(state, step) {
  if (logged(state, step)) return "done";
  return state.skipped[step.key] ? "skipped" : "todo";
}

// currentStep is the first step neither logged nor skipped, or null.
export function currentStep(boot, state) {
  return steps(boot, state).find((st) => status(state, st) === "todo") || null;
}

// sessionE1RM is the e1RM of the last working set of slug done this session
// (spec §7 in-session adjustment), or null.
export function sessionE1RM(state, slug) {
  const sets = Object.values(state.sets)
    .filter((x) => !x.deleted && x.slug === slug && (x.kind === "working" || x.kind === "amrap") && x.weight_kg > 0 && x.reps > 0)
    .sort((a, b) => Date.parse(a.done_at) - Date.parse(b.done_at));
  const last = sets.at(-1);
  if (!last) return null;
  const est = e1rm(last.weight_kg, last.reps, last.rpe || 0);
  return est ? est.kg : null;
}

// prescription returns the planned set for a step; extra sets repeat the last one.
function prescription(ex, s) {
  return ex.prescribed[s] || ex.prescribed.at(-1) || null;
}

// target is what the lifter should do at a step, with the weight resolved
// from the current state (session e1RM, previous actual weight for drops).
export function target(boot, state, step) {
  const ex = groups(boot, state)[step.g].exercises[step.e];
  const p = prescription(ex, step.s);
  const info = exerciseInfo(boot, ex.slug);
  const out = {
    slug: ex.slug,
    name: ex.name,
    measurement: ex.measurement,
    kind: p ? p.kind : "working",
    reps: p ? parseReps(p.reps) : null,
    duration_s: p && p.duration_s ? p.duration_s : null,
    rpe: p && p.target_rpe ? p.target_rpe : null,
    load_kind: p ? p.load_kind || "" : "",
    load_value: p ? p.load_value : null,
    prescribed: p,
    kg: null,
    per_side: [],
  };
  const ctx = {
    unit: boot.unit,
    tm_kg: info.tm_kg ?? null,
    e1rm_kg: sessionE1RM(state, ex.slug) ?? info.e1rm_kg ?? null,
    equipment: info.equipment || (boot.equipment_defaults || {})[info.equipment_kind] || null,
  };
  let r = null;
  switch (out.load_kind) {
    case "weight":
      r = { kg: p.load_value, per_side: [] };
      break;
    case "pct_tm":
      r = resolveLoad({ pct_tm: p.load_value }, 1, ctx);
      break;
    case "rpe":
      if (out.reps && !out.reps.amrap) r = resolveLoad({ rpe: p.load_value }, out.reps.min, ctx);
      break;
    case "drop_pct": {
      const prev = step.s > 0 ? { ...step, s: step.s - 1, key: key(step.g, step.e, step.s - 1) } : null;
      const prevKg = prev ? (logged(state, prev) || {}).weight_kg ?? target(boot, state, prev).kg : null;
      if (prevKg) r = dropLoad(prevKg, p.load_value, ctx);
      break;
    }
  }
  if (r) {
    out.kg = r.kg;
    out.per_side = r.per_side || [];
  }
  return out;
}

// restAfter is the rest in seconds after a step: a group's rest follows each
// complete round (after the last exercise of the round).
export function restAfter(boot, state, step) {
  const all = steps(boot, state);
  const i = all.findIndex((x) => x.key === step.key);
  const next = all[i + 1];
  if (next && next.g === step.g && next.s === step.s) return 0; // superset: go straight to the next exercise
  if (!next || next.g !== step.g) {
    const later = all.slice(i + 1).some((x) => x.g === step.g);
    if (!later) return 0; // last set of the group
  }
  return groups(boot, state)[step.g].rest_s || 0;
}

function op(name, payload, now) {
  return { op_id: uuidv7(now), op: name, payload, client_ts: iso(now) };
}

// logSet records values at a step (or corrects what is logged there).
// values: {weight_kg, reps, rpe, duration_s, distance_m}, null for "not recorded".
export function logSet(boot, state, step, values, now = Date.now()) {
  const s = structuredClone(state);
  const t = target(boot, s, step);
  const existing = logged(s, step);
  const set = {
    id: existing ? existing.id : uuidv7(now),
    session_id: s.sessionId,
    slug: existing ? existing.slug : t.slug,
    group_pos: step.g,
    exercise_pos: step.e,
    set_pos: step.s,
    kind: t.kind,
    prescribed: t.prescribed,
    weight_kg: values.weight_kg ?? null,
    reps: values.reps ?? null,
    rpe: values.rpe ?? null,
    duration_s: values.duration_s ?? null,
    distance_m: values.distance_m ?? null,
    done_at: existing ? existing.done_at : iso(now),
    updated_at: iso(now),
  };
  s.sets[set.id] = set;
  s.positions[step.key] = set.id;
  delete s.skipped[step.key];
  if (!existing) {
    const rest = restAfter(boot, s, step);
    s.restEnd = rest > 0 ? now + rest * 1000 : null;
  }
  return { state: s, op: op("upsert_set", set, now) };
}

// deleteSet removes a logged set; its step becomes "todo" again.
export function deleteSet(state, id, now = Date.now()) {
  const s = structuredClone(state);
  const set = s.sets[id];
  if (!set) return { state: s, op: null };
  set.deleted = true;
  set.updated_at = iso(now);
  for (const [k, v] of Object.entries(s.positions)) if (v === id) delete s.positions[k];
  return { state: s, op: op("delete_set", { id, session_id: s.sessionId, deleted_at: iso(now) }, now) };
}

export function skip(state, step) {
  const s = structuredClone(state);
  s.skipped[step.key] = true;
  return s;
}

export function unskip(state, step) {
  const s = structuredClone(state);
  delete s.skipped[step.key];
  return s;
}

// addSet appends one more set to exercise (g, e).
export function addSet(state, g, e) {
  const s = structuredClone(state);
  s.extra[`${g}:${e}`] = (s.extra[`${g}:${e}`] || 0) + 1;
  return s;
}

// addExercise appends a new group with slug and one set.
export function addExercise(boot, state, slug) {
  const s = structuredClone(state);
  s.added.push({ slug });
  const g = boot.snapshot.groups.length + s.added.length - 1;
  s.extra[`${g}:0`] = 1;
  return s;
}

// swap does the remaining sets of (g, e) as slug instead.
export function swap(state, g, e, slug, planned) {
  const s = structuredClone(state);
  if (slug === planned) delete s.swaps[`${g}:${e}`];
  else s.swaps[`${g}:${e}`] = slug;
  return s;
}

export function editNotes(state, notes, now = Date.now()) {
  const s = structuredClone(state);
  s.notes = notes;
  s.notesAt = iso(now);
  return { state: s, op: op("edit_notes", { session_id: s.sessionId, notes, updated_at: iso(now) }, now) };
}

export function finish(state, now = Date.now()) {
  const s = structuredClone(state);
  s.finished = true;
  s.restEnd = null;
  return { state: s, op: op("finish_session", { session_id: s.sessionId, finished_at: iso(now) }, now) };
}

// settle removes answered operations from the outbox: applied and duplicate
// ones are done, rejected ones move to failed with their reason.
export function settle(outbox, results) {
  const byId = new Map(results.map((r) => [r.op_id, r]));
  const remaining = [];
  const failed = [];
  for (const o of outbox) {
    const r = byId.get(o.op_id);
    if (!r) remaining.push(o);
    else if (r.status === "rejected") failed.push({ ...o, reason: r.reason || "rejected" });
  }
  return { remaining, failed };
}
