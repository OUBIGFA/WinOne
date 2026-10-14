// @ts-nocheck
// Original Magpie ede1e32 account allowance curves, cycle breaks and zoom (MIT).
export function magpieQuotaCurve(language,t,quotaHist,quotaRange){
const intlLang=()=>language;
const el=(tag,cls='',text)=>{const e=document.createElement(tag);e.className=cls;if(text!=null)e.textContent=text;return e;};
const sv=(tag,attrs,style)=>{const e=document.createElementNS('http://www.w3.org/2000/svg',tag);for(const key in attrs)e.setAttribute(key,attrs[key]);if(style)Object.assign(e.style,style);return e;};
function quotaHistOf(sub) {
  const user = (sub.user || "").toLowerCase();
  return quotaHist?.find?.((h) => h.provider === sub.provider && h.user === user);
}
// the lines of sub's windows, as its card lists them, with points
function quotaLines(sub) {
  const h = quotaHistOf(sub);
  if (!h) return [];
  const out = [];
  for (const w of sub.windows || []) {
    if (w.unlimited || w.family) continue;
    // a pool's window is kept under the pool's name and its own
    const name = w.pool ? w.pool + " · " + w.name : w.name;
    const l = h.lines.find((x) => x.name === name);
    if (!l?.points?.length || out.some((x) => x.name === l.name)) continue;
    out.push({ name, label: w.pool ? w.pool + " · " + t(w.name) : t(w.name), window: w, points: l.points.map((p) => ({
      at: Date.parse(p.at), left: p.left,
      start: p.start ? Date.parse(p.start) : null, reset: p.resetsAt ? Date.parse(p.resetsAt) : null,
    })) });
  }
  return out;
}
// quotaCycleBreak: whether b, read after a, is of its window started again,
// as magpie's own record of them says (quota_history.go's newCycle)
function quotaCycleBreak(a, b) {
  if (a.reset == null && b.reset == null) return b.left > a.left + 5;
  if (a.reset == null || b.reset == null) return true;
  if (a.reset <= b.at) return true;
  const moved = b.reset - a.reset, slack = 5 * 60e3;
  return moved > b.at - a.at + slack || moved < -slack;
}
// the current cycle of a line: its start and its reset, the window's as
// the card has it, else its last reading's
function quotaCycle(l, now) {
  const last = l.points[l.points.length - 1];
  let reset = l.window.resetsAt ? Date.parse(l.window.resetsAt) : last.reset;
  let start = last.start != null && last.reset != null && reset != null ? last.start + (reset - last.reset) : last.start;
  if (start == null) { // its first reading of this cycle
    let i = l.points.length - 1;
    while (i > 0 && !quotaCycleBreak(l.points[i - 1], l.points[i])) i--;
    start = l.points[i].at;
  }
  if (reset != null && reset <= now) reset = null;
  return { start: Math.min(start, now), reset };
}
function quotaSpan(lines, range, now) {
  if (range === "2d") return [now - 48 * 3600e3, now];
  let x0 = Infinity, x1 = now;
  for (const l of lines) {
    const c = quotaCycle(l, now);
    x0 = Math.min(x0, c.start);
    if (c.reset) x1 = Math.max(x1, c.reset);
  }
  if (!(x0 < now)) x0 = now - 3600e3;
  return [x0, x1];
}
// quotaPaths: a line's points in [x0, x1] as path data on a w×h box, a
// new subpath at each cycle's start; the point before x0 and the one after
// x1 go along, so the line reaches the box's edges
function quotaPaths(points, x0, x1, w, h) {
  const X = (t) => ((t - x0) / (x1 - x0)) * w, Y = (v) => h - (Math.max(0, Math.min(100, v)) / 100) * h;
  let d = "", prev = null;
  for (let i = 0; i < points.length; i++) {
    const p = points[i];
    const next = points[i + 1];
    if (p.at < x0 && (!next || next.at < x0 || quotaCycleBreak(p, next))) continue;
    if (prev && p.at > x1 && prev.at > x1) break;
    const brk = !prev || quotaCycleBreak(prev, p);
    d += (brk ? "M" : "L") + X(p.at).toFixed(1) + " " + Y(p.left).toFixed(1);
    if (brk && (!points[i + 1] || quotaCycleBreak(p, points[i + 1]))) d += "h0.01"; // a lone point is a dot
    prev = p;
  }
  return d;
}
function quotaTimeText(at, now) {
  const d = new Date(at), lang = intlLang();
  if (Math.abs(at - now) < 60e3) return t("Now");
  const sameDay = d.toDateString() === new Date(now).toDateString();
  const time = d.toLocaleTimeString(lang, { hour: "2-digit", minute: "2-digit", hour12: false });
  return sameDay ? time : d.toLocaleDateString(lang, { month: "short", day: "numeric" }) + " " + time;
}
// drawQuotaPlot puts lines' curves for [x0, x1] in svg, a w×h box:
// the grid, the even burns, the lines, and now.
function drawQuotaPlot(g, lines, x0, x1, w, h, now, thin) {
  g.replaceChildren();
  if (!thin) for (const v of [0, 50, 100]) g.append(sv("line", { x1: 0, x2: w, y1: h - (v / 100) * h, y2: h - (v / 100) * h, class: "qc-grid" }));
  const X = (t) => ((t - x0) / (x1 - x0)) * w;
  lines.forEach((l, i) => {
    const color = `var(--usage-c${(i % 7) + 1})`;
    const c = quotaCycle(l, now);
    // a window far shorter than the span (a 5-hour one over a week) is
    // drawn faint, without its even burn, an all but upright dash
    const short = !thin && c.reset && c.reset - c.start < (x1 - x0) / 12;
    if (c.reset && c.start < c.reset && !short) {
      const e = sv("line", { x1: X(c.start), y1: 0, x2: X(c.reset), y2: h, class: "qc-even" }, { stroke: color });
      g.append(e);
    }
    const d = quotaPaths(l.points, x0, x1, w, h);
    if (d) g.append(sv("path", { d, class: "qc-line" + (short ? " qc-short" : ""), "data-name": l.name }, { stroke: color }));
  });
  if (now >= x0 && now <= x1) g.append(sv("line", { x1: X(now), x2: X(now), y1: 0, y2: h, class: "qc-now" }));
}
// quotaCurve: an account's windows over time, under its meters, with the
// range turned for every card at once
function quotaCurve(sub) {
  if (quotaRange === "off") return null;
  const lines = quotaLines(sub);
  if (!lines.length) return null;
  const box = el("div", "quota-curve");
  const head = el("div", "qc-head");
  const range = el("span", "qc-range");
  head.append(el("span", "", t("Left over time")), range);
  // what the dashes are isn't plain from the plot (John on Discord asked)
  box.title = t("Solid: what was left. Dashed: an even pace, full at the cycle's start to empty at its reset; a line above its dashes lasts the cycle, one below runs out before the reset.");
  const W = 300, H = 60;
  const g = sv("svg", { viewBox: `0 0 ${W} ${H}`, preserveAspectRatio: "none", class: "qc-plot", role: "img" });
  curveZoom(box, g);
  const axis = el("div", "qc-axis");
  const legend = el("div", "qc-legend");
  lines.forEach((l, i) => {
    const k = el("span", "qc-key");
    const sw = el("i");
    sw.style.background = `var(--usage-c${(i % 7) + 1})`;
    const last = l.points[l.points.length - 1];
    k.append(sw, el("span", "", l.label), el("b", "", Math.round(last.left) + "%"));
    k.title = [t("{name}: {n} left, read {when}", { name: l.label, n: Math.round(last.left) + "%", when: quotaTimeText(last.at, Date.now()) }),
      "", t("Latest readings:"), ...quotaReadings(l, Date.now())].join("\n");
    legend.append(k);
  });
  box.draw = () => {
    const now = Date.now();
    const [x0, x1] = quotaSpan(lines, quotaRange, now);
    drawQuotaPlot(g, lines, x0, x1, W, H, now);
    const r = t(quotaRange === "2d" ? "2 days" : "Cycle");
    range.textContent = r;
    g.setAttribute("aria-label", t("Left over time") + " · " + r);
    axis.replaceChildren(el("span", "", quotaTimeText(x0, now)), el("span", "", quotaTimeText(x1, now)));
  };
  box.append(head, g, axis, legend);
  box.draw();
  return box;
}
// quotaReadings: a line's last eight readings, newest first, each what was
// left and when it was read (#802), a line each, ↻ where the window
// started again; for its legend key's title, the card keeping no control
// of its own
function quotaReadings(l, now) {
  const pts = l.points.slice(-8);
  return pts.map((p, i) => (i > 0 && quotaCycleBreak(pts[i - 1], p) ? "↻ " : "") + Math.round(p.left) + "% · " + quotaTimeText(p.at, now)).reverse();
}
// curveZoom: a curve made larger on a click, or Enter, and back on the
// next (TJHHHH on Discord); the page stays where it is
function curveZoom(box, g) {
  g.setAttribute("tabindex", "0");
  g.setAttribute("aria-expanded", "false");
  const flip = (e) => {
    e.stopPropagation();
    const big = box.classList.toggle("big");
    g.setAttribute("aria-expanded", String(big));
  };
  g.addEventListener("click", flip);
  g.addEventListener("keydown", (e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); flip(e); } });
}
return quotaCurve;
}
