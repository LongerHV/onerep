// Tests for the companion's pure logic. Run with: node --test "internal/web/jstest/*.test.mjs"
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  addExercise, addSet, currentStep, deleteSet, finish, groups, logSet, logged, mergeServerSets,
  newState, parseReps, restAfter, sessionE1RM, settle, skip, status, steps, swap, target, uuidv7,
} from "../static/js/companion-core.js";

const bar = { kind: "barbell", unit: "kg", config: { bar: 20, plates: [25, 20, 15, 10, 5, 2.5, 1.25] } };

// A day with a squat block (RPE sets then a drop set) and a pull-up/raise superset.
function boot() {
  return {
    session: { id: "sess", name: "Day", notes: "", finished: false },
    unit: "kg",
    snapshot: {
      name: "Day",
      groups: [
        {
          rest_s: 180,
          exercises: [{
            slug: "barbell-back-squat",
            alternatives: ["hack-squat"],
            sets: [
              { kind: "warmup", reps: 5, load_kind: "pct_tm", load_value: 0.5 },
              { kind: "working", reps: 5, load_kind: "rpe", load_value: 8, target_rpe: 8 },
              { kind: "working", reps: 5, load_kind: "rpe", load_value: 8, target_rpe: 8 },
              { kind: "drop", reps: 10, load_kind: "drop_pct", load_value: 0.2 },
            ],
          }],
        },
        {
          rest_s: 90,
          exercises: [
            { slug: "pull-up", sets: [{ kind: "working", reps: "6-10" }, { kind: "working", reps: "6-10" }] },
            { slug: "cable-lateral-raise", sets: [{ kind: "working", reps: 15, load_kind: "weight", load_value: 7.5 }] },
          ],
        },
      ],
    },
    exercises: {
      "barbell-back-squat": { name: "Squat", measurement: "weight_reps", equipment_kind: "barbell", tm_kg: 140, e1rm_kg: 150, equipment: bar, alternatives: ["hack-squat", "leg-press"], last: [] },
      "hack-squat": { name: "Hack Squat", measurement: "weight_reps", equipment_kind: "machine", tm_kg: 200, alternatives: [], last: [] },
      "pull-up": { name: "Pull-Up", measurement: "bw_reps", equipment_kind: "bodyweight", alternatives: [], last: [] },
      "cable-lateral-raise": { name: "Raise", measurement: "weight_reps", equipment_kind: "cable", alternatives: [], last: [] },
    },
    catalog: [{ slug: "plank", name: "Plank", measurement: "time", equipment_kind: "bodyweight" }],
    equipment_defaults: { barbell: bar },
  };
}

const T0 = Date.UTC(2026, 8, 24, 10, 0, 0);

test("uuidv7 is a time-ordered UUID", () => {
  const a = uuidv7(T0);
  const b = uuidv7(T0 + 1);
  assert.match(a, /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
  assert.ok(a < b, "later ids sort after earlier ones");
  assert.equal(a.slice(0, 13).replace("-", ""), T0.toString(16).padStart(12, "0"));
});

test("parseReps", () => {
  assert.deepEqual(parseReps(5), { min: 5, max: 5, amrap: false });
  assert.deepEqual(parseReps("6-10"), { min: 6, max: 10, amrap: false });
  assert.deepEqual(parseReps("AMRAP"), { min: 0, max: 0, amrap: true });
  assert.equal(parseReps(undefined), null);
});

test("steps run groups in order and supersets round by round", () => {
  const b = boot();
  assert.deepEqual(steps(b, newState(b)).map((s) => s.key), [
    "0:0:0", "0:0:1", "0:0:2", "0:0:3",
    "1:0:0", "1:1:0", "1:0:1",
  ]);
});

test("rest follows each round, not each superset exercise", () => {
  const b = boot();
  const s = newState(b);
  const at = (k) => steps(b, s).find((x) => x.key === k);
  assert.equal(restAfter(b, s, at("0:0:0")), 180);
  assert.equal(restAfter(b, s, at("0:0:3")), 0, "last set of a group");
  assert.equal(restAfter(b, s, at("1:0:0")), 0, "A1 goes straight to B1");
  assert.equal(restAfter(b, s, at("1:1:0")), 90, "rest after B1 completes the round");
});

test("targets resolve and adapt to the sets actually done", () => {
  const b = boot();
  let s = newState(b);
  const [warm, rpe1, rpe2, drop] = steps(b, s);
  assert.equal(target(b, s, warm).kg, 70); // 50% of 140
  assert.equal(target(b, s, rpe1).kg, 120); // 0.811 * 150 = 121.65 -> 120
  assert.deepEqual(target(b, s, rpe1).per_side, [25, 25]);
  assert.equal(target(b, s, drop).kg, 95, "drop from the planned 120 before anything is logged");

  // The first RPE set felt easier than planned: 120 x 5 @ 7 means an e1RM of
  // 120 / 0.786 = 152.67, so the next 5 @ 8 is 0.811 * 152.67 = 123.8 -> 122.5.
  ({ state: s } = logSet(b, s, warm, { weight_kg: 70, reps: 5 }, T0));
  ({ state: s } = logSet(b, s, rpe1, { weight_kg: 120, reps: 5, rpe: 7 }, T0 + 1000));
  assert.ok(Math.abs(sessionE1RM(s, "barbell-back-squat") - 152.67) < 0.01);
  assert.equal(target(b, s, rpe2).kg, 122.5);
  assert.deepEqual(target(b, s, rpe2).per_side, [25, 25, 1.25]);
  // The drop set follows the weight actually lifted on the set before it.
  ({ state: s } = logSet(b, s, rpe2, { weight_kg: 115, reps: 5, rpe: 8 }, T0 + 2000));
  // 80% of 115 = 92; these plates only make multiples of 2.5 kg above the bar -> 90.
  assert.equal(target(b, s, drop).kg, 90);
});

test("logging produces idempotent operations and moves on", () => {
  const b = boot();
  let s = newState(b);
  const first = currentStep(b, s);
  let op;
  ({ state: s, op } = logSet(b, s, first, { weight_kg: 70, reps: 5 }, T0));
  assert.equal(op.op, "upsert_set");
  assert.equal(op.payload.id, logged(s, first).id);
  assert.equal(op.payload.session_id, "sess");
  assert.equal(op.payload.kind, "warmup");
  assert.equal(s.restEnd, T0 + 180_000);
  assert.equal(currentStep(b, s).key, "0:0:1");

  // Correcting the same step keeps the set id (the server applies the newer edit).
  const again = logSet(b, s, first, { weight_kg: 72.5, reps: 5 }, T0 + 5000);
  assert.equal(again.op.payload.id, op.payload.id);
  assert.equal(again.op.payload.done_at, op.payload.done_at);
  assert.notEqual(again.op.op_id, op.op_id);
});

test("deleting a set reopens its step; skipping moves past it", () => {
  const b = boot();
  let s = newState(b);
  const first = currentStep(b, s);
  let { state, op } = logSet(b, s, first, { weight_kg: 70, reps: 5 }, T0);
  ({ state, op } = deleteSet(state, op.payload.id, T0 + 1000));
  assert.equal(op.op, "delete_set");
  assert.equal(status(state, first), "todo");
  state = skip(state, first);
  assert.equal(status(state, first), "skipped");
  assert.equal(currentStep(b, state).key, "0:0:1");
});

test("swapping, extra sets and added exercises", () => {
  const b = boot();
  let s = swap(newState(b), 0, 0, "hack-squat", "barbell-back-squat");
  const warm = steps(b, s)[0];
  assert.equal(target(b, s, warm).slug, "hack-squat");
  assert.equal(target(b, s, warm).kg, 100, "50% of the hack squat's own TM, no equipment -> 0.5 kg steps");
  assert.ok(groups(b, s)[0].exercises[0].alternatives.includes("barbell-back-squat"), "can swap back");

  s = addSet(s, 1, 1);
  assert.equal(steps(b, s).filter((x) => x.g === 1 && x.e === 1).length, 2);
  s = addExercise(b, s, "plank");
  const last = steps(b, s).at(-1);
  assert.equal(last.g, 2);
  assert.equal(target(b, s, last).measurement, "time");
});

test("server sets merge in; the newer edit wins", () => {
  const b = boot();
  const [first] = steps(b, newState(b));
  const { state: local, op } = logSet(b, newState(b), first, { weight_kg: 70, reps: 5 }, T0 + 60_000);
  const older = { ...op.payload, weight_kg: 60, updated_at: new Date(T0).toISOString() };
  assert.equal(logged(mergeServerSets(local, [older]), first).weight_kg, 70);
  const newer = { ...op.payload, weight_kg: 75, updated_at: new Date(T0 + 120_000).toISOString() };
  assert.equal(logged(mergeServerSets(local, [newer]), first).weight_kg, 75);
  // A set logged on another device beyond the plan shows up as an extra step.
  const extra = { ...op.payload, id: uuidv7(T0), set_pos: 9 };
  assert.equal(steps(b, mergeServerSets(newState(b), [extra])).filter((x) => x.g === 0).length, 10);
});

test("finishing and settling the outbox", () => {
  const b = boot();
  const { state, op } = finish(newState(b), T0);
  assert.ok(state.finished);
  assert.equal(op.op, "finish_session");
  const a = { op_id: "a" }, d = { op_id: "d" }, r = { op_id: "r" }, p = { op_id: "p" };
  const { remaining, failed } = settle([a, d, r, p], [
    { op_id: "a", status: "applied" }, { op_id: "d", status: "duplicate" }, { op_id: "r", status: "rejected", reason: "bad" },
  ]);
  assert.deepEqual(remaining, [p]);
  assert.deepEqual(failed, [{ op_id: "r", reason: "bad" }]);
});

test("timestamps compare as times, not strings (Go omits zero milliseconds)", () => {
  const b = boot();
  const [first] = steps(b, newState(b));
  const { state: local, op } = logSet(b, newState(b), first, { weight_kg: 70, reps: 5 }, T0 + 500);
  // The server's copy of an older edit, formatted by Go: "…T10:00:00Z" sorts after "…T10:00:00.500Z" as a string.
  const serverOlder = { ...op.payload, weight_kg: 60, updated_at: "2026-09-24T10:00:00Z" };
  assert.equal(logged(mergeServerSets(local, [serverOlder]), first).weight_kg, 70);
});
