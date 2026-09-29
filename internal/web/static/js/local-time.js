// Shows <time data-local="…" datetime="…Z"> in the browser's timezone. The
// server renders UTC (views/localtime.templ) because it doesn't know where the
// user is; loaded once from the layout, it rewrites each swapped-in page from
// htmx.onLoad.

const pad = (n) => String(n).padStart(2, "0");
const days = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

// formatLocal formats an ISO timestamp in the local timezone like the Go
// layouts of the same style, or returns null when it can't.
export function formatLocal(iso, style) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return null;
  const date = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  const time = `${pad(d.getHours())}:${pad(d.getMinutes())}`;
  switch (style) {
    case "date": return date;
    case "datetime": return `${date} ${time}`;
    case "weekday": return `${days[d.getDay()]} ${date} ${time}`;
  }
  return null;
}

function localize(root) {
  const els = root.matches?.("time[data-local]") ? [root] : [...(root.querySelectorAll?.("time[data-local]") || [])];
  for (const el of els) {
    const text = formatLocal(el.getAttribute("datetime"), el.dataset.local);
    if (text !== null) el.textContent = text;
  }
}

globalThis.htmx?.onLoad(localize);
