// @ts-nocheck
// Directly adapted from yetone/magpie ede1e32aec1a4f6bf06aeef6ed2e03d4218db226,
// internal/gui/assets/app.js (LED_METRICS through ledRail). MIT; see third_party/licenses.
// The adapter supplies locale and DOM helpers; chart arithmetic and interaction
// remain upstream so speed, cache tokens, day selection and Top 7 + Other agree.
export function magpieLedgerCharts(translate, language, options = {}) {
 const t=(key,values={})=>translate(key,values);
 const intlLang=()=>language;
 const ledNum=value=>new Intl.NumberFormat(language).format(value||0);
 const fmtN=value=>new Intl.NumberFormat(language,{notation:'compact',maximumFractionDigits:1}).format(value||0);
 const ledMoney=value=>new Intl.NumberFormat(language,{style:'currency',currency:'USD',maximumFractionDigits:2}).format(value||0);
 const fmtCost=value=>new Intl.NumberFormat(language,{style:'currency',currency:'USD',maximumFractionDigits:6}).format(value.cost||0);
 const ledTook=value=>(value/1000).toFixed(2)+' s';
 const el=(tag,cls='',text)=>{const e=document.createElement(tag);e.className=cls;if(text!=null)e.textContent=text;return e;};
 const icon=()=>el('span');
// WinOne lets overview rankings expand every row; trends retain Top 7 + Other.
const LED_METRICS = [["tokens", "Tokens"], ["cost", "Cost"], ["calls", "Requests"]];
// the window's chart can also say how fast the replies came (#860)
const LED_TREND = [...LED_METRICS, ["speed", "Speed"]];
// "modelAt" is a model at the provider it went to, so one model's speed at
// each provider is ranked side by side (inaction on Discord)
const LED_SPLITS = [["model", "Model"], ["modelAt", "Model · provider"], ["provider", "Provider"], ["agent", "Agent"]];
const LED_SHOWN = Math.max(7, options.shown || 7); // told apart in a chart; the rest are "Other"
const allTokens = (x) => x.input + x.output + x.cache_read + x.cache_write;
// cacheTTLs: a call's cache writes split by how long they are kept, where
// some were for an hour (Anthropic bills those at 2× input, 5 minutes'
// at 1.25×); nothing when none were
function cacheTTLs(x, fmt) {
  const h = Math.min(x.cache_write_1h || 0, x.cache_write || 0);
  return h ? " · " + t("{m} for 5 minutes, {h} for an hour", { m: fmt(x.cache_write - h), h: fmt(h) }) : "";
}
// how fast the timed replies of a point, a share or a part wrote, in tokens
// a second after their first (usage.Totals.Speed): 0 for none timed
const ledSpeed = (x) => x?.decode_ms > 0 ? (1000 * x.decode_out) / x.decode_ms : 0;
// their mean wait for the first token, 0 for none
const ledTTFT = (x) => x?.timed > 0 ? x.ttft_ms / x.timed : 0;
// a metric's value of a point or a share, as the chart counts it
const ledValue = (m, x) => m === "speed" ? ledSpeed(x) : m === "cost" ? +x.cost || 0 : m === "calls" ? +x.calls || 0 : allTokens(x);
// of a point's part, which has its calls, tokens, cost and speed
const ledPart = (m, x) => m === "speed" ? ledSpeed(x) : m === "cost" ? +x.cost || 0 : m === "calls" ? +x.calls || 0 : +x.tokens || 0;
// a metric as its axis and its values say it
const ledFormat = (m, v) => m === "cost" ? ledMoney(v) : m === "calls" || m === "speed" ? ledNum(Math.round(v)) : fmtN(Math.round(v));
const ledFormatLong = (m, v) => m === "speed" ? (v > 0 ? t("{n} tok/s", { n: ledNum(Math.round(v)) }) : "—") : m === "cost" ? (fmtCost({ cost: v, unpriced: 0 }) || "—") : ledNum(Math.round(v));

// an axis's top with four steps under it: round numbers, the top at least the largest value
function ledAxis(max) {
  if (!(max > 0)) return 4;
  const e = 10 ** Math.floor(Math.log10(max / 4));
  for (const m of [1, 2, 2.5, 5, 10]) if (m * e * 4 >= max) return m * e * 4;
  return 10 * e * 4;
}

// what a chart tells apart, most first, and the colour of each: those of the
// ranking's top, and the rest together as "Other"
function ledCategories(l, split, metric) {
  const list = (l.by?.[split] || []).slice().sort((a, b) => ledValue(metric, b) - ledValue(metric, a) || allTokens(b) - allTokens(a));
  const top = list.slice(0, LED_SHOWN).map((x, i) => ({ ...x, color: `var(--usage-c${i % 7 + 1})` }));
  return { top, rest: list.slice(LED_SHOWN), list };
}

// when a point is: the hour, the day or the week it stands for
function ledWhen(p, bucket) {
  const d = new Date(p.time), lang = intlLang() || "en";
  if (bucket === "hour") return d.toLocaleString(lang, { year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false });
  const day = d.toLocaleDateString(lang, { year: "numeric", month: "2-digit", day: "2-digit" });
  return bucket === "week" ? t("week of {label}", { label: day }) : day;
}
function ledTick(p, bucket) {
  const d = new Date(p.time);
  if (bucket === "hour") return String(d.getHours()).padStart(2, "0") + ":00";
  return d.toLocaleDateString(intlLang() || "en", { month: "numeric", day: "numeric" });
}

const SVGNS = "http://www.w3.org/2000/svg";
function sv(tag, attrs, style) {
  const e = document.createElementNS(SVGNS, tag);
  for (const k in attrs) e.setAttribute(k, attrs[k]);
  if (style) Object.assign(e.style, style);
  return e;
}

// the columns: one per point of the answer's series, its part of each thing
// told apart stacked in the colours of the ranking. box is where it goes, its
// size the plot's; compact is for the tray panel, which has little room.
function drawLedColumns(box, l, split, metric, compact, chooseDay, focusDay) {
  focusDay ??= box.contains(document.activeElement) ? document.activeElement.dataset.day : "";
  if (chooseDay) box.chooseDay = chooseDay;
  box.replaceChildren();
  const plot = el("div", "plot");
  box.append(plot);
  const W = plot.clientWidth, H = plot.clientHeight, pts = l.series || [], n = pts.length;
  if (W < 100 || !n) return;
  const { top } = ledCategories(l.chartBy ? { by: l.chartBy } : l, split, metric);
  const totals = pts.map((p) => ledValue(metric, p));
  // speeds don't add up: a point's column is how fast all its replies
  // wrote, and each one told apart is a mark at its own speed on it (#860)
  const speed = metric === "speed";
  const max = Math.max(0, ...totals, ...(speed ? pts.flatMap((p) => top.map((c) => ledSpeed(p.by?.[split]?.[c.id]))) : []));
  if (!(max > 0)) {
    plot.append(el("div", "none", t(metric === "cost" ? "No known price for these requests" : speed ? "No streamed replies timed in this period" : "Nothing in this period")));
    return;
  }
  const topV = ledAxis(max);
  const selectable = l.bucket === "day" && !!box.chooseDay;
  const g = sv("svg", { viewBox: `0 0 ${W} ${H}`, role: selectable ? "group" : "img" });
  g.setAttribute("aria-label", t("Usage trend"));
  // the side's labels, measured as drawn: the plot starts where the widest
  // ends ("8000 万", "¥1250" are wider than "80M"), so none reaches past the
  // card's padding
  plot.append(g);
  const labs = [0, 1, 2, 3, 4].map((i) => {
    const lab = sv("text", { "text-anchor": "end", class: "axis" });
    lab.textContent = ledFormat(metric, topV * (1 - i / 4));
    g.append(lab);
    return lab;
  });
  const widest = Math.max(0, ...labs.map((lab) => { try { return lab.getComputedTextLength(); } catch { return 0; } }));
  const M = { l: widest > 0 ? Math.ceil(widest) + 6 : compact ? 34 : 44, r: 6, t: 8, b: 22 };
  const pw = W - M.l - M.r, ph = H - M.t - M.b, slot = pw / n, bw = Math.max(2, Math.min(compact ? 14 : 30, slot * 0.68));
  const Y = (v) => M.t + ph - (ph * v) / topV;
  labs.forEach((lab, i) => {
    const y = M.t + (ph * i) / 4;
    lab.setAttribute("x", M.l - 6);
    lab.setAttribute("y", y + 3.5);
    g.insertBefore(sv("line", { x1: M.l, x2: W - M.r, y1: y, y2: y, class: "grid" }), lab);
  });
  const room = Math.max(2, Math.floor(pw / (compact ? 46 : 62))), every = Math.ceil(n / room);
  pts.forEach((p, i) => {
    if (i % every) return;
    const x = sv("text", { x: M.l + slot * (i + 0.5), y: H - 6, "text-anchor": "middle", class: "axis" });
    x.textContent = ledTick(p, l.bucket);
    g.append(x);
  });
  const hot = sv("rect", { class: "hot", y: M.t, height: ph, rx: 3, width: slot }, { display: "none" });
  g.append(hot);
  const selected = pts.findIndex((p) => p.time.slice(0, 10) === l.day);
  if (selectable && selected >= 0) g.append(sv("rect", { class: "hot", x: M.l + slot * selected, y: M.t, height: ph, rx: 3, width: slot }));
  // each column: what the top of the ranking had of it, from the bottom up,
  // and what is left of its total as "Other"
  const segs = [];
  pts.forEach((p, i) => {
    const x = M.l + slot * (i + 0.5) - bw / 2;
    let y0 = 0;
    const draw = (key, color, v) => {
      if (!(v > 0)) return;
      const r = sv("rect", { x, width: bw, y: Y(y0 + v), height: Math.max(0.5, Y(y0) - Y(y0 + v)), rx: 1, class: "col", "data-k": key, "data-day": p.time.slice(0, 10), "data-color": color }, { fill: color });
      g.append(r);
      segs.push(r);
      y0 += v;
    };
    const by = p.by?.[split] || {};
    if (speed) {
      // the point's own speed, as a track the models' marks sit on: not a
      // share of anything, and not the grey "Other" of the stacked charts
      if (totals[i] > 0) { draw("\0all", "var(--usage-pill)", totals[i]); segs.at(-1).classList.add("all"); }
      for (const c of top) {
        const v = ledSpeed(by[c.id]);
        if (!(v > 0)) continue;
        // a model faster than the point's own leaves its mark above the
        // column, in nothing: a leader line ties the two, so the mark reads
        // as this column's rather than as a stray dash (huoranxuanyuan, #860).
        // The colour goes through style, not the attribute: var() substitutes
        // in a style and not in a presentation attribute. The line stops at
        // the column's top when the mark is within a pixel of it, or it would
        // be drawn back down into the column
        if (v > totals[i]) {
          const top = Y(totals[i]);
          const stem = sv("line", {
            x1: x + bw / 2, x2: x + bw / 2, y1: top, y2: Math.min(Y(v) + 1.5, top),
            class: "col stem", "data-k": c.id, "data-day": p.time.slice(0, 10), "data-color": c.color,
          }, { stroke: c.color, strokeWidth: 1, strokeOpacity: ".45" });
          g.append(stem);
          segs.push(stem);
        }
        const r = sv("rect", { x, width: bw, y: Y(v) - 1.5, height: 3, rx: 1.5, class: "col mark", "data-k": c.id, "data-day": p.time.slice(0, 10), "data-color": c.color }, { fill: c.color });
        g.append(r);
        segs.push(r);
      }
      return;
    }
    let stacked = 0;
    for (const c of top) { const v = ledPart(metric, by[c.id] || {}); stacked += v; draw(c.id, c.color, v); }
    draw("\0other", "var(--usage-faint)", Math.max(0, totals[i] - stacked));
  });
  if (selectable) pts.forEach((p, i) => {
    const day = p.time.slice(0, 10);
    const hit = sv("rect", { class: "led-day", x: M.l + slot * i, y: M.t, width: slot, height: ph, fill: "transparent", role: "button", tabindex: 0, "aria-label": ledWhen(p, l.bucket), "aria-pressed": day === l.day, "data-day": day });
    const pick = () => box.chooseDay(day);
    hit.onclick = pick;
    hit.onkeydown = (e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); pick(); } };
    g.append(hit);
  });

  const tip = el("div", "tip");
  tip.hidden = true;
  plot.append(g, tip);
  const name = (c) => t(c.name || c.id || "—");
  const at = (e) => {
    const r = g.getBoundingClientRect();
    const i = Math.max(0, Math.min(n - 1, Math.floor(((e.clientX - r.left) / r.width * W - M.l) / slot)));
    const p = pts[i];
    hot.setAttribute("x", M.l + slot * i);
    hot.style.display = "";
    tip.replaceChildren(el("b", "", ledWhen(p, l.bucket)));
    const by = p.by?.[split] || {};
    const rows = top.map((c) => [name(c), c.color, ledPart(metric, by[c.id] || {}), by[c.id]]);
    if (!speed) rows.push([t("Other"), "var(--usage-faint)", Math.max(0, totals[i] - rows.reduce((a, x) => a + x[2], 0))]);
    for (const [nm, color, v, part] of rows.filter((x) => x[2] > 0).sort((a, b) => b[2] - a[2])) {
      const row = el("div");
      const sw = el("i");
      sw.style.background = color;
      const said = el("em", "", ledFormatLong(metric, v));
      if (speed && ledTTFT(part)) said.append(" · " + t("TTFT {ms}", { ms: ledTook(ledTTFT(part)) }));
      row.append(sw, el("span", "", nm), said);
      tip.append(row);
    }
    const sum = el("div", "sum");
    sum.append(el("span", "", t(speed ? "All" : "Total")), el("em", "", ledFormatLong(metric, totals[i])));
    if (speed && ledTTFT(p)) sum.lastChild.append(" · " + t("TTFT {ms}", { ms: ledTook(ledTTFT(p)) }));
    if (metric !== "calls" && p.calls) sum.lastChild.append(" · " + t(p.calls === 1 ? "{n} request" : "{n} requests", { n: ledNum(p.calls) }));
    tip.append(sum);
    tip.hidden = false;
    const x = M.l + slot * (i + 0.5), w = tip.offsetWidth;
    tip.style.left = Math.max(0, x + 14 + w > W ? x - w - 14 : x + 14) + "px";
  };
  g.onpointermove = at;
  g.onpointerdown = at;
  g.onpointerleave = () => { hot.style.display = "none"; tip.hidden = true; };
  // the ranking asks for one thing to stand out
  box.emphasize = (key) => {
    for (const r of segs) {
      const dimDay = l.day && r.dataset.day !== l.day;
      const color = dimDay ? "var(--usage-faint)" : r.dataset.color;
      r.style.opacity = dimDay || key != null && r.dataset.k !== key ? ".22" : "";
      // a mark's leader line is a line, which is stroked, not filled
      if (r.tagName === "line") r.style.stroke = color;
      else r.style.fill = color;
    }
  };
  box.emphasize(null);
  if (focusDay) box.querySelector(`.led-day[data-day="${CSS.escape(focusDay)}"]`)?.focus({ preventScroll: true });
}

// the ranking: who the requests were of, by the metric, the most first —
// each with its share, its other figures, and a click that lists only its
// requests (a provider or an agent), or those of its model. picked is the
// one the filter has picked, if any; the ones past the chart's colours are
// left out of it and said in a line.
function drawLedRank(box, l, split, metric, picked, choose, compact) {
  // a redraw (a pick, a refresh) leaves the ranking scrolled where it was
  const kept = box.scrollTop;
  box.replaceChildren();
  const { top, rest, list } = ledCategories(l, split, metric);
  if (l.chartBy) {
    const colors = ledCategories({ by: l.chartBy }, split, metric).top;
    for (const x of top) x.color = colors.find((c) => c.id === x.id)?.color || "var(--usage-faint)";
  }
  if (!list.length) return;
  const sum = list.reduce((a, x) => a + ledValue(metric, x), 0) || 1;
  const leader = Math.max(1, ledValue(metric, top[0]));
  const one = (x, color, plain) => {
    const b = el("button", "rk" + (plain ? " plain" : "") + (picked && picked === x.id ? " on" : ""));
    b.type = "button";
    const v = ledValue(metric, x);
    const l1 = el("div", "rk-a");
    const sw = el("i", "rk-sw");
    sw.style.background = color;
    l1.append(sw);
    if (x.icon && split !== "model") l1.append(icon(x.icon));
    const nm = el("span", "rk-nm", t(x.name || x.id || "—"));
    nm.title = t(x.name || x.id || "");
    l1.append(nm, el("span", "rk-val", ledFormatLong(metric, v)));
    const bar = el("div", "rk-bar"), fill = el("i");
    fill.style.width = Math.max(1.5, (100 * v) / leader).toFixed(1) + "%";
    fill.style.background = color;
    bar.append(fill);
    const more = [];
    if (metric !== "calls") more.push(t(x.calls === 1 ? "{n} request" : "{n} requests", { n: ledNum(x.calls) }));
    if (metric !== "tokens") more.push(t("{n} tokens", { n: fmtN(allTokens(x)) }));
    if (metric !== "cost" && x.cost) more.push("≈" + fmtCost(x));
    // how fast its replies came, beside what they cost (#860)
    if (metric !== "speed" && ledSpeed(x)) more.push(t("{n} tok/s", { n: ledNum(Math.round(ledSpeed(x))) }));
    if (ledTTFT(x)) more.push(t("TTFT {ms}", { ms: ledTook(ledTTFT(x)) }));
    const prompt = x.input + x.cache_write + x.cache_read;
    if (prompt && !compact) more.push(t("hit rate {p}", { p: Math.round((100 * x.cache_read) / prompt) + "%" }));
    // a speed is no one's share of all
    if (metric !== "speed") more.push(Math.round((100 * v) / sum) + "%");
    const l2 = el("div", "rk-b");
    // Overview columns keep the three summary figures together. The complete
    // upstream timing/cache details remain in the accessible description and
    // tooltip; the trend's ranking still prints every metric inline.
    const brief = [];
    if (metric !== "calls") brief.push(t(x.calls === 1 ? "{n} request" : "{n} requests", { n: ledNum(x.calls) }));
    if (metric !== "cost" && x.cost) brief.push("≈" + fmtCost(x));
    if (metric !== "speed") brief.push(Math.round((100 * v) / sum) + "%");
    b.title = more.join(" · ");
    b.setAttribute("aria-description", b.title);
    l2.append((compact ? brief : more).join(" · "));
    if (x.errors) l2.append(" · ", Object.assign(el("span", "rk-bad"), { textContent: t("{n} failed", { n: x.errors }) }));
    b.append(l1, bar, l2);
    if (!plain) b.onclick = () => choose(x);
    b.onpointerenter = () => box.chart?.emphasize?.(x.id);
    b.onpointerleave = () => box.chart?.emphasize?.(null);
    return b;
  };
  for (const x of top) box.append(one(x, x.color));
  if (rest.length) {
    const tot = rest.reduce((acc, x) => {
      for (const k of ["calls", "errors", "input", "output", "cache_read", "cache_write", "cost", "timed", "ttft_ms", "decode_ms", "decode_out"]) acc[k] = (acc[k] || 0) + (x[k] || 0);
      return acc;
    }, { id: "\0other", name: t("Other ({n})", { n: rest.length }) });
    box.append(one(tot, "var(--usage-faint)", true));
  }
  box.scrollTop = kept;
  box.onscroll = () => ledRankEdges(box);
  ledRankEdges(box);
}

// a ranking that scrolls fades out at the edge with more beyond it, and its
// rail (the window's) shows how much there is and where it is scrolled to
function ledRankEdges(box) {
  const end = box.scrollHeight - box.clientHeight;
  box.classList.toggle("more-above", end > 1 && box.scrollTop > 1);
  box.classList.toggle("more-below", end > 1 && box.scrollTop < end - 1);
  const rail = box.rail;
  if (!rail) return;
  rail.hidden = !(end > 1) || !box.offsetParent;
  if (rail.hidden) return;
  const h = box.clientHeight, thumb = Math.max(24, (h * h) / box.scrollHeight);
  rail.style.top = box.offsetTop + "px";
  rail.style.height = h + "px";
  rail.firstChild.style.height = thumb + "px";
  rail.firstChild.style.transform = `translateY(${((h - thumb) * box.scrollTop) / end}px)`;
}

// the rail's thumb drags the ranking; a click on the rail takes it there
function ledRail(rail, box) {
  box.rail = rail;
  rail.onpointerdown = (e) => {
    if (e.button !== 0) return;
    e.preventDefault();
    const end = box.scrollHeight - box.clientHeight, thumb = rail.firstChild.offsetHeight, room = rail.clientHeight - thumb;
    if (end <= 0 || room <= 0) return;
    if (e.target !== rail.firstChild) {
      // the thumb's middle to where the rail was clicked
      const y = e.clientY - rail.getBoundingClientRect().top - thumb / 2;
      box.scrollTop = (Math.max(0, Math.min(room, y)) / room) * end;
    }
    const y0 = e.clientY, top0 = box.scrollTop;
    rail.setPointerCapture(e.pointerId);
    rail.classList.add("drag");
    rail.onpointermove = (m) => { box.scrollTop = top0 + ((m.clientY - y0) / room) * end; };
    rail.onpointerup = rail.onpointercancel = () => {
      rail.classList.remove("drag");
      rail.onpointermove = rail.onpointerup = rail.onpointercancel = null;
    };
  };
}

return {drawLedColumns,drawLedRank,ledCategories,ledRail};
}
