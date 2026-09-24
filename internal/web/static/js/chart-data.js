// Shapes the stats API's JSON for uPlot. Pure, so Node can test it
// (internal/web/jstest/charts.test.mjs); stats.js draws the result.

// e1rmData turns [{t, e1rm, rpe_based}] into uPlot columns: time, every
// point (the line), RPE-based points and rep-based points. null when empty.
export function e1rmData(points) {
  if (!points || points.length === 0) return null;
  return [
    points.map((p) => p.t),
    points.map((p) => p.e1rm),
    points.map((p) => (p.rpe_based ? p.e1rm : null)),
    points.map((p) => (p.rpe_based ? null : p.e1rm)),
  ];
}

// stackMuscles builds a stacked bar chart from [{muscle, label, sets[]}]
// (most volume first): each series is the cumulative sum up to that muscle,
// and the tallest is drawn first so shorter ones paint over it. order lists
// the muscles in draw order, matching data[1..]. null when there is nothing.
export function stackMuscles(muscles) {
  if (!muscles || muscles.length === 0) return null;
  const weeks = muscles[0].sets.length;
  const running = new Array(weeks).fill(0);
  const cumulative = muscles.map((m) => m.sets.map((v, i) => (running[i] += v)));
  return {
    data: [Array.from({ length: weeks }, (_, i) => i), ...cumulative.reverse()],
    order: [...muscles].reverse(),
  };
}

// muscleColor spreads n colors around the hue wheel.
export function muscleColor(i, n) {
  return `hsl(${Math.round((i * 360) / n)} 65% 55%)`;
}
