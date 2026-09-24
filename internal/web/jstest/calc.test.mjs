// Runs the shared calc vectors against the browser implementation.
// Run with: node --test internal/web/jstest/
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dropLoad, e1rm, resolveLoad, round, rtsPercent } from "../static/js/calc.js";

const cases = JSON.parse(readFileSync(new URL("../../../testdata/calc_cases.json", import.meta.url)));

const near = (a, b) => Math.abs(a - b) < 1e-6;
const samePlates = (a, b) => a.length === b.length && a.every((v, i) => near(v, b[i]));

test("rts", () => {
  for (const c of cases.rts) {
    const got = rtsPercent(c.reps, c.rpe);
    if (c.pct === null) assert.equal(got, null, `rts(${c.reps}, ${c.rpe})`);
    else assert.ok(got !== null && near(got, c.pct), `rts(${c.reps}, ${c.rpe}) = ${got}, want ${c.pct}`);
  }
});

test("e1rm", () => {
  for (const c of cases.e1rm) {
    const got = e1rm(c.weight_kg, c.reps, c.rpe);
    if (c.kg === null) assert.equal(got, null, `e1rm(${c.weight_kg}, ${c.reps}, ${c.rpe})`);
    else assert.ok(got && near(got.kg, c.kg) && got.method === c.method,
      `e1rm(${c.weight_kg}, ${c.reps}, ${c.rpe}) = ${JSON.stringify(got)}, want ${c.kg} ${c.method}`);
  }
});

test("round", () => {
  for (const c of cases.round) {
    const got = round(c.target_kg, c.equipment, c.fallback_unit);
    assert.ok(near(got.kg, c.kg) && samePlates(got.per_side, c.per_side),
      `${c.name}: got ${JSON.stringify(got)}, want ${c.kg} ${JSON.stringify(c.per_side)}`);
  }
});

test("resolve", () => {
  for (const c of cases.resolve) {
    const got = resolveLoad(c.load, c.reps, c.ctx);
    if (c.kg === null) assert.equal(got, null, c.name);
    else assert.ok(got && near(got.kg, c.kg) && samePlates(got.per_side, c.per_side),
      `${c.name}: got ${JSON.stringify(got)}, want ${c.kg} ${JSON.stringify(c.per_side)}`);
  }
});

test("drop", () => {
  for (const c of cases.drop) {
    const got = dropLoad(c.prev_kg, c.pct, c.ctx);
    assert.ok(near(got.kg, c.kg) && samePlates(got.per_side, c.per_side), c.name);
  }
});

test("absurd targets stay bounded", () => {
  const bar = { kind: "barbell", unit: "kg", config: { bar: 20, plates: [25, 1.25] } };
  assert.ok(near(round(1e9, bar, "kg").kg, 2020));
  assert.equal(round(NaN, null, "kg").kg, 0);
});

test("many limited plate sizes stay fast", () => {
  const plates = [];
  const pairs = {};
  for (let i = 1; i <= 30; i++) {
    plates.push(i / 100);
    pairs[String(i / 100)] = 1000;
  }
  const start = performance.now();
  round(2250, { kind: "barbell", unit: "kg", config: { bar: 20, plates, plate_pairs: pairs } }, "kg");
  const ms = performance.now() - start;
  assert.ok(ms < 200, `round took ${ms} ms`);
});
