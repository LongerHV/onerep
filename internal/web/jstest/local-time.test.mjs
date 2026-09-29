import { test } from "node:test";
import assert from "node:assert/strict";

process.env.TZ = "Europe/Warsaw"; // UTC+2 in September
const { formatLocal } = await import("../static/js/local-time.js");

test("times are shown in the browser's timezone", () => {
  const late = "2026-09-28T22:30:00Z"; // already Tuesday in Warsaw
  assert.equal(formatLocal(late, "datetime"), "2026-09-29 00:30");
  assert.equal(formatLocal(late, "weekday"), "Tue 2026-09-29 00:30");
  assert.equal(formatLocal(late, "date"), "2026-09-29");
});

test("an unknown style or a bad timestamp is left alone", () => {
  assert.equal(formatLocal("2026-09-28T22:30:00Z", "century"), null);
  assert.equal(formatLocal("not a time", "date"), null);
});
