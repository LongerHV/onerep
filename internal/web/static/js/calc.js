// Training math shared with the server. This is a line-for-line port of
// internal/calc (Go); testdata/calc_cases.json keeps both in agreement.
// Weights are kg unless an equipment profile says otherwise.

export const KG_PER_LB = 0.45359237;

export function toKg(v, unit) {
  return unit === "lb" ? v * KG_PER_LB : v;
}

export function fromKg(kg, unit) {
  return unit === "lb" ? kg / KG_PER_LB : kg;
}

// Integer hundredths, tolerating float noise (99.99999999 counts as 100.00).
function cents(v) {
  return Math.floor(v * 100 + 1e-6);
}

const RTS_SEQUENCE = [
  1.0, 0.978, 0.955, 0.939, 0.922, 0.907, 0.892, 0.878, 0.863, 0.85,
  0.837, 0.824, 0.811, 0.799, 0.786, 0.774, 0.762, 0.751, 0.739, 0.723,
  0.707, 0.694, 0.68, 0.667, 0.653, 0.64, 0.626, 0.613, 0.599, 0.586,
  0.572,
];

// Fraction of 1RM for reps at rpe (reps 1–12, RPE 6–10 in 0.5 steps), or null.
export function rtsPercent(reps, rpe) {
  if (!Number.isInteger(reps) || reps < 1 || reps > 12 || !(rpe >= 6 && rpe <= 10)) {
    return null;
  }
  const halfSteps = (10 - rpe) * 2;
  if (!Number.isInteger(halfSteps)) return null;
  return RTS_SEQUENCE[2 * (reps - 1) + halfSteps];
}

// Estimated 1RM as {kg, method} ("rpe" or "reps"), or null. rpe 0 = not recorded.
export function e1rm(weightKg, reps, rpe) {
  if (!(weightKg > 0) || reps < 1 || reps > 12) return null;
  const pct = rtsPercent(reps, rpe);
  if (pct !== null) return { kg: weightKg / pct, method: "rpe" };
  if (reps === 1) return { kg: weightKg, method: "reps" };
  return { kg: weightKg * (1 + reps / 30), method: "reps" };
}

const MAX_SIDE_CENTS = 100000;

// Heaviest achievable load not above targetKg (or the lightest achievable one),
// as {kg, per_side}. per_side lists one side's plates in the equipment's unit.
export function round(targetKg, equipment, fallbackUnit) {
  if (Number.isNaN(targetKg) || targetKg < 0) targetKg = 0;
  if (!equipment || equipment.kind === "bodyweight") {
    const unit = equipment ? equipment.unit : fallbackUnit;
    const step = unit === "lb" ? 100 : 50;
    const t = Math.max(0, cents(fromKg(targetKg, unit)));
    return { kg: toKg((Math.floor(t / step) * step) / 100, unit), per_side: [] };
  }
  const unit = equipment.unit;
  const c = equipment.config || {};
  const t = cents(fromKg(targetKg, unit));
  switch (equipment.kind) {
    case "barbell":
      return roundBarbell(t, unit, c);
    case "dumbbell":
      return { kg: toKg(pickFromList(t, c.weights) / 100, unit), per_side: [] };
    default: {
      if (c.stack && c.stack.length > 0) {
        return { kg: toKg(pickFromList(t, c.stack) / 100, unit), per_side: [] };
      }
      const lo = cents(c.min || 0), step = cents(c.step), hi = cents(c.max || 0);
      let v = lo;
      if (t > lo) v = lo + Math.floor((t - lo) / step) * step;
      if (v > hi) v = hi;
      return { kg: toKg(v / 100, unit), per_side: [] };
    }
  }
}

function pickFromList(t, values) {
  let best = -1, smallest = Infinity;
  for (const v of values) {
    const c = cents(v);
    if (c <= t && c > best) best = c;
    if (c < smallest) smallest = c;
  }
  return best >= 0 ? best : smallest;
}

function roundBarbell(t, unit, c) {
  const bar = cents(c.bar || 0);
  if (t <= bar) return { kg: toKg(bar / 100, unit), per_side: [] };
  const side = Math.min(Math.floor((t - bar) / 2), MAX_SIDE_CENTS);

  const pairs = c.plate_pairs || {};
  const plates = (c.plates || [])
    .map((p) => {
      const size = cents(p);
      let max = -1;
      for (const [k, n] of Object.entries(pairs)) {
        if (cents(parseFloat(k)) === size) max = n;
      }
      return { size, max };
    })
    .filter((p) => p.size > 0)
    .sort((a, b) => b.size - a.size);

  // reach[i][s]: a per-side sum of s is achievable with plates[i:].
  const n = plates.length;
  const reach = Array.from({ length: n + 1 }, () => new Uint8Array(side + 1));
  reach[n][0] = 1;
  // used[s] is the fewest plates of the current size needed to reach s,
  // which keeps limited sizes O(side) instead of O(side * pairs).
  const used = new Int32Array(side + 1);
  for (let i = n - 1; i >= 0; i--) {
    const p = plates[i];
    for (let s = 0; s <= side; s++) {
      if (reach[i + 1][s]) {
        reach[i][s] = 1;
        used[s] = 0;
      } else if (s >= p.size && reach[i][s - p.size] && (p.max < 0 || used[s - p.size] < p.max)) {
        reach[i][s] = 1;
        used[s] = used[s - p.size] + 1;
      }
    }
  }

  let best = side;
  while (!reach[0][best]) best--;
  const perSide = [];
  let rest = best;
  plates.forEach((p, i) => {
    let k = Math.floor(rest / p.size);
    if (p.max >= 0 && k > p.max) k = p.max;
    while (k > 0 && !reach[i + 1][rest - k * p.size]) k--;
    for (let j = 0; j < k; j++) perSide.push(p.size / 100);
    rest -= k * p.size;
  });
  return { kg: toKg((bar + 2 * best) / 100, unit), per_side: perSide };
}

// Achievable weight for a prescription {weight} | {pct_tm} | {rpe}, or null
// when the needed training max or e1RM is unknown. Absolute weights pass through.
export function resolveLoad(load, reps, ctx) {
  if (load.weight != null) return { kg: load.weight, per_side: [] };
  if (load.pct_tm != null) {
    if (ctx.tm_kg == null) return null;
    return round(load.pct_tm * ctx.tm_kg, ctx.equipment, ctx.unit);
  }
  if (load.rpe != null) {
    const pct = rtsPercent(reps, load.rpe);
    if (pct === null || ctx.e1rm_kg == null) return null;
    return round(pct * ctx.e1rm_kg, ctx.equipment, ctx.unit);
  }
  return null;
}

// Weight for a drop set pct below the previous set's weight.
export function dropLoad(prevKg, pct, ctx) {
  return round(prevKg * (1 - pct), ctx.equipment, ctx.unit);
}
