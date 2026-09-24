// Draws [data-chart] elements with uPlot from their data-src JSON (spec §13).
// Loaded once from the layout like the other page scripts: htmx swaps pages
// into <main>, so charts are set up from htmx.onLoad, and uPlot (vendored) is
// only fetched on a page that has one. Charts whose element left the page are
// destroyed on the next swap. The tables next to each chart carry the numbers,
// so a failed chart only logs.

import { e1rmData, muscleColor, stackMuscles } from "./chart-data.js";

let lib; // the uPlot module, loaded on first use
const live = new Map(); // element → { plot, observer }

const dark = () => window.matchMedia("(prefers-color-scheme: dark)").matches;
const ink = () => (dark() ? "#a1a1aa" : "#52525b"); // zinc-400 / zinc-600
const grid = () => (dark() ? "#27272a" : "#e4e4e7"); // zinc-800 / zinc-200
const paper = () => (dark() ? "#09090b" : "#fafafa"); // zinc-950 / zinc-50

function axes(extraX = {}) {
  const common = { stroke: ink(), grid: { stroke: grid(), width: 1 }, ticks: { stroke: grid(), width: 1 } };
  return [{ ...common, ...extraX }, { ...common }];
}

function e1rmOptions(json, el) {
  const data = e1rmData(json.points);
  if (!data) return null;
  const accent = dark() ? "#e4e4e7" : "#18181b";
  const noLine = () => null;
  return {
    data,
    opts: {
      width: el.clientWidth, height: 256, legend: { live: false },
      axes: axes(),
      scales: { y: { auto: true } },
      series: [
        {},
        { label: `e1RM (${json.unit})`, stroke: accent, width: 2, points: { show: false } },
        { label: "RPE-based", stroke: accent, paths: noLine, points: { show: true, size: 8, fill: accent } },
        { label: "rep-based", stroke: accent, paths: noLine, points: { show: true, size: 8, fill: paper(), width: 2 } },
      ],
    },
  };
}

function musclesOptions(json, el) {
  const stack = stackMuscles(json.muscles);
  if (!stack) return null;
  const n = json.muscles.length;
  const bars = lib.paths.bars({ size: [0.7, 48] });
  return {
    data: stack.data,
    opts: {
      width: el.clientWidth, height: 288, legend: { live: false },
      axes: axes({ values: (_, ticks) => ticks.map((i) => (json.weeks[i] || "").slice(5)), space: 30 }),
      scales: { x: { time: false, range: [-0.5, json.weeks.length - 0.5] }, y: { range: (_, __, max) => [0, Math.max(1, max)] } },
      series: [
        {},
        ...stack.order.map((m) => {
          const color = muscleColor(json.muscles.indexOf(m), n);
          return { label: m.label, fill: color, stroke: color, paths: bars, points: { show: false } };
        }),
      ],
    },
  };
}

const builders = { e1rm: e1rmOptions, muscles: musclesOptions };

async function draw(el) {
  const build = builders[el.dataset.chart];
  if (!build || el.dataset.drawn) return;
  el.dataset.drawn = "starting";
  try {
    const res = await fetch(el.dataset.src, { headers: { Accept: "application/json" } });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const json = await res.json();
    lib ??= (await import("/static/vendor/uplot/uPlot.esm.js")).default;
    const chart = build(json, el);
    if (!chart || !el.isConnected) return;
    const plot = new lib(chart.opts, chart.data, el);
    const observer = new ResizeObserver(() => plot.setSize({ width: el.clientWidth, height: chart.opts.height }));
    observer.observe(el);
    live.set(el, { plot, observer });
    el.dataset.drawn = "done";
  } catch (err) {
    console.error("chart unavailable", el.dataset.src, err);
    el.dataset.drawn = "failed";
  }
}

function sweep() {
  for (const [el, { plot, observer }] of live) {
    if (el.isConnected) continue;
    observer.disconnect();
    plot.destroy();
    live.delete(el);
  }
}

htmx.onLoad((root) => {
  sweep();
  const els = root.matches?.("[data-chart]") ? [root] : [...(root.querySelectorAll?.("[data-chart]") || [])];
  els.forEach(draw);
});
