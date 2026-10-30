// @ts-nocheck
// Generated from yetone/magpie 42ec2700bd2a4cf8d4ceb76a95767ac221e283e5. MIT.
// Run node build/sync-magpie-usage.mjs --write; provenance: third_party/magpie-usage-source.json.
export function magpieLedgerCharts(translate, language, options = {}) {
 const t=(key,values={})=>translate(key,values);
 const intlLang=()=>language;
 const numbers=new Intl.NumberFormat(language), compactNumbers=new Intl.NumberFormat(language,{notation:'compact',maximumFractionDigits:1});
 const ledNum=value=>numbers.format(value||0);
 const fmtN=value=>compactNumbers.format(value||0);
 const ledMoney=value=>new Intl.NumberFormat(language,{style:'currency',currency:'USD',maximumFractionDigits:2}).format(value||0);
 const fmtCost=value=>new Intl.NumberFormat(language,{style:'currency',currency:'USD',maximumFractionDigits:6}).format(value.cost||0);
 const ledTook=value=>(value/1000).toFixed(2)+' s';
 const tipLines=(...values)=>values.filter(Boolean).join('\n');
 const el=(tag,cls='',text)=>{const e=document.createElement(tag);e.className=cls;if(text!=null)e.textContent=text;return e;};
 const icon=()=>el('span');
const LED_METRICS = [["tokens", "Tokens"], ["cost", "Cost"], ["calls", "Requests"]];
const LED_SHOWN = Math.max(7, options.shown || 7); // told apart in a chart; the rest are "Other"
const allTokens = (x) => x.input + x.output + x.cache_read + x.cache_write;
// a total's cache hit rate, leaving out cache_unknown (calls whose vendor
// counts no cache); null when nothing is left (#3464)
const cacheHitOf = (x) => {
  const prompt = (x.input || 0) + (x.cache_read || 0) + (x.cache_write || 0) - (x.cache_unknown || 0);
  return prompt > 0 ? (x.cache_read || 0) / prompt : null;
};
// what a hit rate leaves out, when it leaves anything out
const cacheLeftOut = (x) => x.cache_unknown_calls
  ? t(x.cache_unknown_calls === 1 ? "{n} request left out: its vendor doesn't count the cache" : "{n} requests left out: their vendor doesn't count the cache", { n: ledNum(x.cache_unknown_calls) })
  : "";
// how fast the timed replies of a point, a share or a part wrote, in tokens
// a second after their first (usage.Totals.Speed): 0 for none timed
const ledSpeed = (x) => x?.decode_ms > 0 ? (1000 * x.decode_out) / x.decode_ms : 0;
// the least the replies of a group that has no first token wrote at
// (usage.Totals.Floor, ledFloorSpeed's for each): 0 for none
const ledFloor = (x) => x?.floor_ms > 0 ? (1000 * x.floor_out) / x.floor_ms : 0;
// a group's speed as every ranking and the Output speed card say it, by one
// rule (Fallmonkey on Discord): with replies whose speed was measured, their
// speed and how many they are, "125 tok/s · 1 timed reply", not beside "260
// requests" alone; with replies read from a session file that tell the least
// they wrote at, that floor and how many, "≥100 tok/s · 30 replies"; with
// both, both, the one of more replies first, "≥71 tok/s · 54 replies; 62
// tok/s · 2 timed replies", as two replies are too few to stand for 56; else
// nothing. Counts are words, not a bare "(2)" only its tooltip explains. The
// Requests tab says each row's alone: "125 tok/s" or "≥100 tok/s". v is the
// figure a speed ranking orders by and bars, the first one's; main is the
// first, other the second ("" with one).
function ledGroupSpeed(x) {
  const v = ledSpeed(x), f = ledFloor(x), kv = x?.decode_n || 0, kf = x?.floor_n || 0;
  if (!v && !f) return null;
  const tok = (s) => t("{n} tok/s", { n: ledNum(Math.round(s)) });
  const count = (k, timed) => !k ? "" : t(timed ? (k === 1 ? "{k} timed reply" : "{k} timed replies") : (k === 1 ? "{k} reply" : "{k} replies"), { k: ledNum(k) });
  const vTitle = !v || !kv ? "" : f ? t(kv === 1 ? "Measured on {k} reply, from its first token to its end" : "Measured on {k} replies, from their first token to their end", { k: ledNum(kv) })
    : t(kv === 1 ? "Measured on {k} reply, from its first token to its end; the others tell no speed" : "Measured on {k} replies, from their first token to their end; the others tell no speed", { k: ledNum(kv) });
  const fTitle = !f || !kf ? "" : t(kf === 1 ? "At least: {k} reply read from a session file, which doesn't say when its answer began, so time before that is counted too" : "At least: {k} replies read from session files, which don't say when their answers began, so time before that is counted too", { k: ledNum(kf) });
  // each figure as its speed and its count, kept apart so a narrow card can
  // put the count under the speed rather than run past its edge (#1623); the
  // card sets the count as a caption, with no "·" to lead a wrapped line
  const measured = v ? { s: v, head: tok(v), count: count(kv, true), title: vTitle } : null;
  const floor = f ? { s: f, head: "≥" + tok(f), count: count(kf, false), title: fTitle } : null;
  // the larger sample first; a tie keeps the measured one first
  const [a, b] = measured && floor && kf > kv ? [floor, measured] : [measured || floor, measured && floor];
  const say = (p) => p.count ? p.head + " · " + p.count : p.head;
  const main = say(a), other = b ? say(b) : "";
  return { v: a.s, text: other ? t("{a}; {b}", { a: main, b: other }) : main, main, mainParts: a.count ? [a.head, " " + a.count] : [a.head], other, title: tipLines(a.title, b?.title) };
}
// their mean wait for the first token, 0 for none
const ledTTFT = (x) => x?.timed > 0 ? x.ttft_ms / x.timed : 0;
// a metric's value of a point or a share, as the chart counts it
const ledValue = (m, x) => m === "speed" ? ledSpeed(x) : m === "cost" ? +x.cost || 0 : m === "calls" ? +x.calls || 0 : allTokens(x);
// of a share in the ranking: a speed is what its row says first, the one of
// more replies (ledGroupSpeed)
const ledRankValue = (m, x) => m === "speed" ? ledGroupSpeed(x)?.v || 0 : ledValue(m, x);
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
  const list = (l.by?.[split] || []).slice().sort((a, b) => ledRankValue(metric, b) - ledRankValue(metric, a) || allTokens(b) - allTokens(a));
  const top = list.slice(0, LED_SHOWN).map((x, i) => ({ ...x, color: `var(--usage-c${i % 7 + 1})` }));
  return { top, rest: list.slice(LED_SHOWN), list };
}

// when a point is: the hour, the day or the week it stands for
function ledWhen(p, bucket) {
  const d = new Date(p.time), lang = intlLang();
  if (bucket === "hour") return d.toLocaleString(lang, { year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false });
  const day = d.toLocaleDateString(lang, { year: "numeric", month: "2-digit", day: "2-digit" });
  return bucket === "week" ? t("week of {label}", { label: day }) : day;
}
function ledTick(p, bucket) {
  const d = new Date(p.time);
  if (bucket === "hour") return String(d.getHours()).padStart(2, "0") + ":00";
  return d.toLocaleDateString(intlLang(), { month: "numeric", day: "numeric" });
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
  const g = sv("svg", { viewBox: `0 0 ${W} ${H}`, role: selectable ? "group" : "img", "aria-label": t("Usage trend") });
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
      segs.push(g.appendChild(sv("rect", { x, width: bw, y: Y(y0 + v), height: Math.max(0.5, Y(y0) - Y(y0 + v)), rx: 1, class: "col", "data-k": key, "data-day": p.time.slice(0, 10), "data-color": color }, { fill: color })));
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
          segs.push(g.appendChild(sv("line", {
            x1: x + bw / 2, x2: x + bw / 2, y1: top, y2: Math.min(Y(v) + 1.5, top),
            class: "col stem", "data-k": c.id, "data-day": p.time.slice(0, 10), "data-color": c.color,
          }, { stroke: c.color, strokeWidth: 1, strokeOpacity: ".45" })));
        }
        segs.push(g.appendChild(sv("rect", { x, width: bw, y: Y(v) - 1.5, height: 3, rx: 1.5, class: "col mark", "data-k": c.id, "data-day": p.time.slice(0, 10), "data-color": c.color }, { fill: c.color })));
      }
      return;
    }
    let stacked = 0;
    for (const c of top) { const v = ledPart(metric, by[c.id] || {}); stacked += v; draw(c.id, c.color, v); }
    draw("\0other", "var(--usage-faint)", Math.max(0, totals[i] - stacked));
  });
  // the Usage page's lines take the columns' place (ledLines)
  box.drawn?.(g, Y, M.l + slot / 2, slot, segs);
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
    // speeds are read across: a model's name in full, its speed and its
    // first token each in a column of their own (#2343)
    tip.classList.toggle("speed", speed);
    const by = p.by?.[split] || {};
    const rows = top.map((c) => [name(c), c.color, ledPart(metric, by[c.id] || {}), by[c.id]]);
    if (!speed) rows.push([t("Other"), "var(--usage-faint)", Math.max(0, totals[i] - rows.reduce((a, x) => a + x[2], 0))]);
    for (const [nm, color, v, part] of rows.filter((x) => x[2] > 0).sort((a, b) => b[2] - a[2])) {
      const row = el("div");
      const sw = el("i");
      sw.style.background = color;
      row.append(sw, el("span", "", nm), el("em", "", ledFormatLong(metric, v)));
      if (speed) row.append(el("em", "ttft", ledTTFT(part) ? t("TTFT {ms}", { ms: ledTook(ledTTFT(part)) }) : ""));
      tip.append(row);
    }
    const sum = el("div", "sum");
    const calls = metric !== "calls" && p.calls ? t(p.calls === 1 ? "{n} request" : "{n} requests", { n: ledNum(p.calls) }) : "";
    if (speed) {
      sum.append(el("span", "", t("All") + (calls ? " · " + calls : "")), el("em", "", ledFormatLong(metric, totals[i])),
        el("em", "ttft", ledTTFT(p) ? t("TTFT {ms}", { ms: ledTook(ledTTFT(p)) }) : ""));
    } else {
      sum.append(el("span", "", t("Total")), el("em", "", ledFormatLong(metric, totals[i]) + (calls ? " · " + calls : "")));
    }
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
      // a leader line or a line chart's series is stroked, not filled
      r.style[r.tagName == "rect" ? "fill" : "stroke"] = color;
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
  const leader = Math.max(1, ledRankValue(metric, top[0]));
  const one = (x, color, plain) => {
    const b = el("button", "rk" + (plain ? " plain" : "") + (picked && picked === x.id ? " on" : ""));
    b.type = "button";
    const v = ledRankValue(metric, x), gs = ledGroupSpeed(x);
    const l1 = el("div", "rk-a");
    const sw = el("i", "rk-sw");
    sw.style.background = color;
    l1.append(sw);
    if (x.icon && split !== "model") l1.append(icon(x.icon));
    const nm = el("span", "rk-nm", t(x.name || x.id || "—"));
    nm.title = t(x.name || x.id || "");
    const val = el("span", "rk-val", metric === "speed" ? gs?.text || "—" : ledFormatLong(metric, v));
    if (metric === "speed" && gs?.title) val.title = gs.title;
    l1.append(nm, val);
    const bar = el("div", "rk-bar"), fill = el("i");
    fill.style.width = Math.max(1.5, (100 * v) / leader).toFixed(1) + "%";
    fill.style.background = color;
    bar.append(fill);
    const more = [];
    if (metric !== "calls") more.push(t(x.calls === 1 ? "{n} request" : "{n} requests", { n: ledNum(x.calls) }));
    if (metric !== "tokens") more.push(t("{n} tokens", { n: fmtN(allTokens(x)) }));
    if (metric !== "cost" && x.cost) more.push("≈" + fmtCost(x));
    if (x.credited) more.push(ledCredits(x));
    // how fast its replies came, beside what they cost (#860)
    if (metric !== "speed" && gs) more.push(Object.assign(el("span", "rk-speed", gs.text), { title: gs.title }));
    if (ledTTFT(x)) more.push(t("TTFT {ms}", { ms: ledTook(ledTTFT(x)) }));
    const hit = cacheHitOf(x);
    if (hit !== null && !compact) more.push(t("hit rate {p}", { p: Math.round(100 * hit) + "%" }));
    // a speed is no one's share of all
    if (metric !== "speed") more.push(Math.round((100 * v) / sum) + "%");
    const l2 = el("div", "rk-b");
    b.title = more.map(m => typeof m === "string" ? m : m.textContent).join(" · ");
    b.setAttribute("aria-description", b.title);
    const brief = [];
    if (metric !== "calls") brief.push(t(x.calls === 1 ? "{n} request" : "{n} requests", { n: ledNum(x.calls) }));
    if (metric !== "cost" && x.cost) brief.push("≈" + fmtCost(x));
    if (metric !== "speed") brief.push(Math.round((100 * v) / sum) + "%");
    (compact ? brief : more).forEach((m, i) => l2.append(...(i ? [" · ", m] : [m])));
    if (x.errors) l2.append(" · ", Object.assign(el("span", "rk-bad"), { textContent: t("{n} failed", { n: x.errors }) }));
    // of those, the replies magpie ended stuck in a loop (#1858)
    if (x.loops) l2.append(" · ", Object.assign(el("span", "rk-bad rk-loop"), { textContent: t("{n} looped", { n: ledNum(x.loops) }), title: t("Replies magpie ended because the model kept repeating itself") }));
    b.append(l1, bar, l2);
    if (!plain) b.onclick = () => choose(x);
    b.onpointerenter = () => box.chart?.emphasize?.(x.id);
    b.onpointerleave = () => box.chart?.emphasize?.(null);
    return b;
  };
  for (const x of top) box.append(one(x, x.color));
  if (rest.length) {
    const tot = rest.reduce((acc, x) => {
      for (const k of ["calls", "errors", "loops", "input", "output", "cache_read", "cache_write", "cache_unknown", "cache_unknown_calls", "cost", "credits", "credited", "timed", "ttft_ms", "decode_ms", "decode_out", "decode_n", "floor_out", "floor_ms", "floor_n"]) acc[k] = (acc[k] || 0) + (x[k] || 0);
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

function ledCredits(x) {
  if (!x?.credited) return null;
  const n = x.credits || 0;
  const s = el("span", "rk-credits", t("{n} credits", { n: n > 0 && n < 0.01 ? "<0.01" : (+n.toFixed(2)).toLocaleString(intlLang()) }));
  s.title = t("What {n} of these requests cost in their plan's credits, as the replies said", { n: ledNum(x.credited) });
  return s;
}

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

// The chart as lines (drawLedColumns' box.drawn): the columns drawn on its
// axis are taken away and each one told apart becomes a line through its
// points. Tokens, cost and requests stay stacked, each line where its part
// of the column ended, with its band softly filled under it, so the top
// line is the total as the column's top was; speeds don't add up, so each
// model's line is at its own speed, broken where it had none, over the
// point's own speed dashed. The tooltip, the hover, a day's pick and the
// ranking's emphasis are drawLedColumns' own, over the same slots.
// A smooth line through points [x, y], as d3's curveMonotoneX draws it
// (Fritsch–Carlson): it bends between points but never past one, so a
// quiet hour isn't drawn dipping under zero or a peak over its value.
// "from" starts it ("M", or "L" to go on from where a path is).
function ledCurve(ps, from = "M") {
  const n = ps.length, f = (v) => v.toFixed(1);
  if (!n) return "";
  let d = from + f(ps[0][0]) + " " + f(ps[0][1]);
  if (n === 1) return d;
  const s = [], m = [];
  for (let i = 0; i < n - 1; i++) s.push((ps[i + 1][1] - ps[i][1]) / (ps[i + 1][0] - ps[i][0]));
  for (let i = 1; i < n - 1; i++) {
    const h0 = ps[i][0] - ps[i - 1][0], h1 = ps[i + 1][0] - ps[i][0], p = (s[i - 1] * h1 + s[i] * h0) / (h0 + h1);
    m[i] = s[i - 1] * s[i] <= 0 ? 0 : (Math.sign(s[i - 1]) + Math.sign(s[i])) * Math.min(Math.abs(s[i - 1]), Math.abs(s[i]), Math.abs(p) / 2);
  }
  m[0] = n > 2 ? (3 * s[0] - m[1]) / 2 : s[0];
  m[n - 1] = n > 2 ? (3 * s[n - 2] - m[n - 2]) / 2 : s[0];
  for (let i = 0; i < n - 1; i++) {
    const [xa, ya] = ps[i], [xb, yb] = ps[i + 1], dx = (xb - xa) / 3;
    d += "C" + f(xa + dx) + " " + f(ya + dx * m[i]) + " " + f(xb - dx) + " " + f(yb - dx * m[i + 1]) + " " + f(xb) + " " + f(yb);
  }
  return d;
}

function ledLines(l, split, metric, g, Y, x0, slot, segs) {
  const pts = l?.series || [];
  for (const r of segs) r.remove();
  segs.length = 0;
  if (!l) return;
  const speed = metric === "speed";
  const { top } = ledCategories(l.chartBy ? { by: l.chartBy } : l, split, metric);
  const X = (i) => x0 + slot * i, dot = slot >= 12 ? 4.5 : 3;
  const at = (i, v) => [X(i), Y(v)];
  // stacked, each one goes under those before it, so a band never covers
  // the line under it
  let head = null;
  const totals = pts.map((p) => ledValue(metric, p));
  // one series: its line, the dots at its points and the band under it
  const series = (key, color, vals, base, dashed) => {
    const s = sv("g", { class: "col ln", "data-k": key, "data-day": l.day || "", "data-color": color }, { stroke: color });
    // each unbroken run is one smooth curve; a point alone in its run has
    // no line to sit on, so it is a dot
    let line = "", dots = "", run = [];
    const end = () => {
      if (run.length === 1 && !dashed) dots += "M" + run[0][0].toFixed(1) + " " + run[0][1].toFixed(1) + "h0";
      line += ledCurve(run);
      run = [];
    };
    vals.forEach((v, i) => { if (v == null) end(); else run.push(at(i, v)); });
    end();
    if (!line) return;
    if (base) {
      const up = ledCurve(vals.map((v, i) => at(i, v)));
      const down = ledCurve(base.map((v, i) => at(i, v)).reverse(), "L");
      s.append(sv("path", { d: up + down + "Z", class: "band" }, { fill: color }));
    }
    s.append(sv("path", { d: line, class: "line" + (dashed ? " all" : "") }));
    if (dots) s.append(sv("path", { d: dots, class: "dots" }, { strokeWidth: dot }));
    head = g.insertBefore(s, speed ? null : head);
    segs.push(s);
  };
  if (speed) {
    series("\0all", "var(--usage-muted)", totals.map((v) => (v > 0 ? v : null)), null, true);
    for (const c of top) series(c.id, c.color, pts.map((p) => ledSpeed(p.by?.[split]?.[c.id]) || null));
    return;
  }
  // from the bottom up: each one's part on what is under it, "Other" last
  let under = pts.map(() => 0);
  const parts = top.map((c) => [c.id, c.color, pts.map((p) => ledPart(metric, p.by?.[split]?.[c.id] || {}))]);
  parts.push(["\0other", "var(--usage-faint)", pts.map((p, i) => Math.max(0, totals[i] - parts.reduce((a, x) => a + x[2][i], 0)))]);
  for (const [key, color, vs] of parts) {
    const over = under.map((u, i) => u + vs[i]);
    if (vs.some((v) => v > 0)) series(key, color, over, under);
    under = over;
  }
}
return {
 drawLedColumns:(...args)=>{args[0].drawn=null;return drawLedColumns(...args);},
 drawLedLines:(box,l,split,metric,compact,chooseDay,focusDay)=>{box.drawn=(...args)=>ledLines(l,split,metric,...args);return drawLedColumns(box,l,split,metric,compact,chooseDay,focusDay);},
 drawLedRank,ledCategories,ledRail,cacheHitOf,ledGroupSpeed
};
}
