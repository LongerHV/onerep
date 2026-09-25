import { test } from "node:test";
import assert from "node:assert/strict";
import * as f from "../static/js/plan-form-core.js";

test("parse and format each kind", () => {
  assert.deepEqual(f.parseInput("integer", "90"), { value: 90 });
  assert.ok(f.parseInput("integer", "1.5").error);
  assert.deepEqual(f.parseInput("count", ""), { value: undefined });
  assert.deepEqual(f.parseInput("reps", "6-10"), { value: "6-10" });
  assert.deepEqual(f.parseInput("reps", "amrap"), { value: "AMRAP" });
  assert.deepEqual(f.parseInput("reps", "5"), { value: 5 });
  assert.ok(f.parseInput("reps", "10-6x").error);
  assert.deepEqual(f.parseInput("number", "102,5"), { value: 102.5 });
  assert.deepEqual(f.parseInput("rpe", "8.5"), { value: 8.5 });
  assert.equal(f.formatValue("reps", "AMRAP"), "AMRAP");
  assert.equal(f.formatValue("number", undefined), "");
});

test("percent round-trips without float noise", () => {
  assert.deepEqual(f.parseInput("percent", "82.5"), { value: 0.825 });
  assert.deepEqual(f.parseInput("percent", "75"), { value: 0.75 });
  assert.equal(f.formatValue("percent", 0.825), "82.5");
  assert.equal(f.formatValue("percent", 0.7), "70");
});

test("per-week state from a single value and from an array", () => {
  assert.deepEqual(f.perWeekFrom(5, 4), { vary: false, values: [5] });
  assert.deepEqual(f.perWeekFrom([3, 3, 4, 2], 4), { vary: true, values: [3, 3, 4, 2] });
  assert.deepEqual(f.perWeekFrom(undefined, 4), { vary: false, values: [undefined] });
});

test("per-week value collapses equal weeks and keeps count skips", () => {
  assert.equal(f.perWeekValue({ vary: true, values: [5, 5, 5] }), 5);
  assert.deepEqual(f.perWeekValue({ vary: true, values: [3, null, 3] }), [3, null, 3]);
  assert.equal(f.perWeekValue({ vary: false, values: [undefined] }), undefined);
  assert.equal(f.perWeekValue({ vary: true, values: [undefined, undefined] }), undefined);
});

test("per-week resize copies the last week and trims", () => {
  assert.deepEqual(f.resize({ vary: true, values: [3, 4] }, 4).values, [3, 4, 4, 4]);
  assert.deepEqual(f.resize({ vary: true, values: [3, 4, 5] }, 2).values, [3, 4]);
  assert.deepEqual(f.resize({ vary: false, values: [5] }, 6), { vary: false, values: [5] });
});

test("week set keeps weeks beyond the plan", () => {
  assert.deepEqual(f.weekSetFrom([5, 2, 2]), [2, 5]);
  assert.deepEqual(f.weekSetValue([2, 5]), [2, 5]);
  assert.equal(f.weekSetValue([]), undefined);
});

test("clean drops empty optional values", () => {
  const doc = { name: "P", weeks: 1, days: [{ name: "D", only_weeks: [], groups: [{ exercises: [
    { slug: "a", alternatives: [], notes: "", sets: [{ kind: "working", count: 3, reps: 5, load: {} }] }] }] }] };
  assert.deepEqual(f.clean(doc), { name: "P", weeks: 1, days: [{ name: "D", groups: [{ exercises: [
    { slug: "a", sets: [{ count: 3, reps: 5 }] }] }] }] });
  assert.deepEqual(f.clean({ days: [{ groups: [] }] }), { days: [{ groups: [] }] }, "an empty day is meaningful");
});

test("sameDoc ignores key order, fills the unit, and detects real changes", () => {
  const a = { name: "P", weeks: 1, days: [] };
  assert.ok(f.sameDoc(a, { days: [], weeks: 1, name: "P", unit: "lb" }, "lb"), "sameDoc fills the unit");
  assert.ok(!f.sameDoc(a, { days: [], weeks: 1, name: "P", unit: "kg" }, "lb"));
  assert.ok(!f.sameDoc({ ...a, foo: 1 }, a, "kg"), "sameDoc detects unknown keys");
  assert.ok(!f.sameDoc({ ...a, weeks: 2 }, a, "kg"), "sameDoc detects changed values");
  assert.ok(f.sameDoc({ ...a, days: [{ name: "D", groups: [{ exercises: [{ slug: "x", sets: [{ kind: "working", count: 1, reps: 1 }] }] }] }] },
    { ...a, days: [{ name: "D", groups: [{ exercises: [{ slug: "x", sets: [{ count: 1, reps: 1 }] }] }] }] }, "kg"));
});

test("pointer to path, walking up to an existing editor", () => {
  assert.equal(f.pointerToPath("/days/0/groups/1/exercises/0/sets/2/count"), "root.days.0.groups.1.exercises.0.sets.2.count");
  assert.equal(f.pointerToPath(""), "root");
  assert.equal(f.pointerToPath("/a~1b/c~0d"), "root.a/b.c~d");
  const editors = new Set(["root", "root.days.0.groups.0.exercises.0.sets.0.count", "root.days.0.groups.0.exercises.0.sets.0.load"]);
  const has = (p) => editors.has(p);
  const perWeek = (p) => p.endsWith(".count");
  assert.deepEqual(f.nearestPath("root.days.0.groups.0.exercises.0.sets.0.count.2", has, perWeek), { path: "root.days.0.groups.0.exercises.0.sets.0.count", week: 3 });
  assert.deepEqual(f.nearestPath("root.days.0.groups.0.exercises.0.sets.0.load.pct_tm", has, perWeek), { path: "root.days.0.groups.0.exercises.0.sets.0.load", week: null });
  assert.deepEqual(f.nearestPath("root.nowhere.9", has, perWeek), { path: "root", week: null });
});

test("problems become json-editor errors", () => {
  const has = (p) => p === "root" || p === "root.weeks" || p === "root.days.0.groups.0.rest_s";
  const errs = f.problemsToErrors([
    { pointer: "/weeks", message: "must be at least 1" },
    { pointer: "/days/0/groups/0/rest_s/1", message: "too long", warning: true },
    { pointer: "", message: "no days in week 3" },
  ], has, (p) => p.endsWith("rest_s"));
  assert.deepEqual(errs, [
    { path: "root.weeks", property: "onerep", message: "must be at least 1" },
    { path: "root.days.0.groups.0.rest_s", property: "onerep", message: "Warning: W2: too long" },
    { path: "root", property: "onerep", message: "no days in week 3" },
  ]);
  assert.deepEqual(f.problemsToErrors(null, has), []);
});

test("header templates", () => {
  const names = { "barbell-back-squat": "Barbell Back Squat" };
  assert.equal(f.headerText("@day", { i1: 2, self: { name: "Lower" } }, names), "Lower");
  assert.equal(f.headerText("@day", { i1: 2, self: {} }, names), "Day 2");
  assert.equal(f.headerText("@group", { i1: 1, self: { exercises: [{}, {}] } }, names), "Group 1 · superset");
  assert.equal(f.headerText("@group", { i1: 3, self: { exercises: [{}] } }, names), "Group 3");
  assert.equal(f.headerText("@slot", { i1: 1, self: { slug: "barbell-back-squat" } }, names), "Barbell Back Squat");
  assert.equal(f.headerText("@set", { i1: 2, self: {} }, names), "Set line 2");
  assert.equal(f.headerText("Plan", {}, names), "Plan");
});

test("slug names from the editor schema", () => {
  const schema = { definitions: { slug: { enum: ["a", "b"], options: { enum_titles: ["A", "B"] } } } };
  assert.deepEqual(f.slugNames(schema), { a: "A", b: "B" });
  assert.deepEqual(f.slugNames({}), {});
});

test("a stored per-week array keeps its length until weeks changes", () => {
  // Fields can be set before the plan's weeks is; trimming then would lose weeks.
  assert.deepEqual(f.perWeekFrom([3, 4, 5], 1), { vary: true, values: [3, 4, 5] });
});

test("markRequired keeps every optional field in the form", () => {
  const schema = { properties: { a: { type: "string" }, b: { $ref: "#/definitions/x" } },
    definitions: { x: { type: "object", properties: { c: { type: "integer" } } },
      l: { oneOf: [{ type: "object", properties: { d: {} } }] } } };
  const out = f.markRequired(schema);
  assert.equal(out.properties.a.required, true);
  assert.equal(out.properties.b.required, true);
  assert.equal(out.definitions.x.properties.c.required, true);
  assert.equal(out.definitions.l.oneOf[0].properties.d.required, true);
  assert.equal(schema.properties.a.required, undefined, "the input is not modified");
});

test("only a valid weeks value resizes per-week fields", () => {
  assert.equal(f.validWeeks(4), 4);
  for (const bad of [undefined, null, "", 0, 53, 2.5, "4"]) assert.equal(f.validWeeks(bad), null, String(bad));
});

test("shrinking then growing weeks restores the trimmed weeks", () => {
  const short = f.resize({ vary: true, values: [3, 3, 4, 2] }, 2);
  assert.deepEqual(short.values, [3, 3]);
  assert.deepEqual(f.perWeekValue(short), 3);
  assert.deepEqual(f.resize(short, 4).values, [3, 3, 4, 2]);
  assert.deepEqual(f.resize(short, 5).values, [3, 3, 4, 2, 2]);
});

test("week labels only on per-week fields", () => {
  const editors = new Set(["root", "root.days.0.only_weeks", "root.days.0.groups.0.exercises.0.alternatives", "root.days.0.groups.0.rest_s"]);
  const has = (p) => editors.has(p);
  const perWeek = (p) => p.endsWith("rest_s");
  assert.deepEqual(f.nearestPath("root.days.0.only_weeks.1", has, perWeek), { path: "root.days.0.only_weeks", week: null });
  assert.deepEqual(f.nearestPath("root.days.0.groups.0.exercises.0.alternatives.1", has, perWeek), { path: "root.days.0.groups.0.exercises.0.alternatives", week: null });
  assert.deepEqual(f.nearestPath("root.days.0.groups.0.rest_s.1", has, perWeek), { path: "root.days.0.groups.0.rest_s", week: 2 });
  assert.equal(f.problemsToErrors([{ pointer: "/days/0/only_weeks/1", message: "week 5 is past the plan" }], has, perWeek)[0].message,
    "week 5 is past the plan");
});

test("a uniform per-week array the length of the plan is the same plan as one value", () => {
  const plan = (restS) => ({ name: "P", unit: "kg", weeks: 4, days: [{ name: "D", groups: [{ rest_s: restS, exercises: [] }] }] });
  assert.ok(f.sameDoc(plan([120, 120, 120, 120]), plan(120), "kg"));
  assert.ok(!f.sameDoc(plan([120, 120]), plan(120), "kg"), "a wrong-length array is not silently fixed");
  assert.ok(!f.sameDoc(plan([120, 90, 120, 120]), plan(120), "kg"));
});

test("an alternatives list keeps its order", () => {
  assert.deepEqual(f.listAdd(["b", "a"], "c"), ["b", "a", "c"]);
  assert.deepEqual(f.listAdd(["b", "a"], "a"), ["b", "a"], "no duplicates");
  assert.deepEqual(f.listRemove(["b", "a", "c"], 1), ["b", "c"]);
  assert.equal(f.listValue([]), undefined);
  assert.deepEqual(f.listValue(["b"]), ["b"]);
});
