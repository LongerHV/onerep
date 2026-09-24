import { test } from "node:test";
import assert from "node:assert/strict";
import { e1rmData, muscleColor, stackMuscles } from "../static/js/chart-data.js";

test("empty series draws no chart", () => {
  assert.equal(e1rmData([]), null);
  assert.equal(e1rmData(undefined), null);
});

test("e1RM points split into RPE-based and rep-based series", () => {
  const data = e1rmData([
    { t: 100, e1rm: 120, rpe_based: true },
    { t: 200, e1rm: 118.5, rpe_based: false },
  ]);
  assert.deepEqual(data, [[100, 200], [120, 118.5], [120, null], [null, 118.5]]);
});

test("muscles stack cumulatively, topmost series first", () => {
  const { data, order } = stackMuscles([
    { muscle: "quads", label: "quads", sets: [2, 4] },
    { muscle: "glutes", label: "glutes", sets: [1, 0.5] },
  ]);
  assert.deepEqual(data[0], [0, 1]); // week indexes
  // Drawn first = whole stack (quads + glutes), then quads alone on top of it.
  assert.deepEqual(order.map((m) => m.muscle), ["glutes", "quads"]);
  assert.deepEqual(data[1], [3, 4.5]);
  assert.deepEqual(data[2], [2, 4]);
});

test("no muscles, no chart", () => {
  assert.equal(stackMuscles([]), null);
});

test("muscle colors differ", () => {
  const colors = new Set(Array.from({ length: 20 }, (_, i) => muscleColor(i, 20)));
  assert.equal(colors.size, 20);
});
