// @ts-nocheck
// Direct Magpie ede1e32 app.js request heatmap. MIT; lifecycle, DOM scope,
// currency and persistence key are adapted to WinOne; geometry and interactions remain upstream.
import {magpieSessionCalendar} from "./magpie-session-calendar";
export function magpieUsageHeatmap(root,language,t){
const $=selector=>selector==="#ledHeat"?root:root.querySelector(selector);
const locale=language,intlLang=()=>language;
const el=(tag,cls="",value)=>{const e=document.createElement(tag);e.className=cls;if(value!=null)e.textContent=value;return e;};
const ledNum=value=>new Intl.NumberFormat(language).format(value||0),fmtN=value=>new Intl.NumberFormat(language,{notation:"compact",maximumFractionDigits:1}).format(value||0);
const {sessLevels,sessWeekdays,sessLegend}=magpieSessionCalendar(language,t);
const sessISO=d=>[d.getFullYear(),String(d.getMonth()+1).padStart(2,"0"),String(d.getDate()).padStart(2,"0")].join("-");
const currency="usd",fx={rate:0};
function fmtCost(t) {
  if (!t.cost && t.unpriced) return "";
  let c = t.cost, sign = "$";
  if (currency === "cny" && fx.rate > 0) { c = c * fx.rate; sign = "¥"; }
  const s = c >= 100 ? c.toFixed(0) : c >= 1 ? c.toFixed(2) : c.toFixed(3);
  return sign + s + (t.unpriced ? "+" : "");
}
let ledHeat={data:null},ledHeatMetric="tokens";
try{const saved=localStorage.getItem("winone.usage.heatmapMetric");if(["tokens","calls","cost"].includes(saved))ledHeatMetric=saved;}catch{}
const LED_HEAT=[["tokens","Tokens"],["calls","Requests"],["cost","Cost"]];
const ledHeatDate = (s) => { const [y, m, d] = String(s).split("-").map(Number); return new Date(y, m - 1, d); };
function ledHeatTip(x) {
  const loc = intlLang() || "en";
  const tip = el("div");
  tip.append(el("b", "", x.day.toLocaleDateString(loc, { year: "numeric", month: "short", day: "numeric", weekday: "short" })));
  if (!x.calls) {
    tip.append(el("span", "", t("No requests")));
    return tip;
  }
  const parts = [t("{n} tokens", { n: fmtN(x.tokens) }), t(x.calls === 1 ? "{n} request" : "{n} requests", { n: ledNum(x.calls) })];
  const c = fmtCost(x);
  if (c) parts.push("≈" + c);
  const order = { tokens: 0, calls: 1, cost: 2 }[ledHeatMetric];
  if (order && parts[order]) parts.unshift(...parts.splice(order, 1)); // the metric shown first
  tip.append(el("span", "", parts.join(" · ")));
  return tip;
}
function drawLedHeat() {
  const box = $("#ledHeat"), h = ledHeat?.data;
  const at = new Map((h?.days || []).map((d) => [d.date, d]));
  box.hidden = !h?.from || !h?.to || ![...at.values()].some((d) => d.calls > 0);
  if (box.hidden) return;
  const first = ledHeatDate(h.from), last = ledHeatDate(h.to);
  const days = [];
  for (const d = new Date(first); d <= last; d.setDate(d.getDate() + 1)) {
    const x = at.get(sessISO(d)) || { calls: 0, tokens: 0, cost: 0 };
    days.push({ ...x, day: new Date(d) });
  }
  const value = (x) => ledHeatMetric === "cost" ? x.cost : ledHeatMetric === "calls" ? x.calls : x.tokens;
  const lv = sessLevels(days.map(value));
  const seg = $("#ledHeatMetric");
  seg.replaceChildren();
  for (const [id, name] of LED_HEAT) {
    const b = el("button", "opt" + (id === ledHeatMetric ? " on" : ""), t(name));
    b.type = "button";
    b.onclick = () => {
      if (id === ledHeatMetric) return;
      ledHeatMetric = id;
      try { localStorage.setItem("winone.usage.heatmapMetric", id); } catch {}
      drawLedHeat();
    };
    seg.append(b);
  }
  for (const button of seg.children) button.setAttribute("aria-pressed", String(button.classList.contains("on")));
  const scroll = $("#ledHeatScroll");
  // the same days in the same language keep their cells, coloured again, so
  // a refresh moves nothing and the scroll stays where the reader left it
  const shape = [h.from, h.to, locale].join("|");
  let grid = scroll.querySelector(".heat-grid");
  if (!grid || grid.dataset.shape !== shape) {
    grid = el("div", "heat-grid");
    grid.dataset.shape = shape;
    grid.setAttribute("role", "img");
    const names = sessWeekdays();
    // every row has its place at the edge, named every other day
    for (let i = 0; i < 7; i++) {
      const l = el("span", "wd", i % 2 ? "" : names[i]);
      l.style.gridArea = `${i + 2} / 1`;
      grid.append(l);
    }
    const loc = intlLang() || "en";
    const weeks = Math.ceil(days.length / 7), labels = [];
    let month = -1;
    days.forEach((x, i) => {
      const w = Math.floor(i / 7), wd = i % 7;
      if (wd === 0 && x.day.getMonth() !== month) {
        month = x.day.getMonth();
        // a month mostly gone in the first week is left unnamed, so as not
        // to crowd out the next one's name, and so is one begun in the last
        // week, with no room for its name
        if ((!labels.length || w - labels.at(-1).w >= 3) && !(w === 0 && x.day.getDate() > 14) && w < weeks - 1) {
          const m = el("span", "mo", x.day.toLocaleDateString(loc, { month: "short" }));
          labels.push({ m, w });
          grid.append(m);
        }
      }
      const c = el("i");
      c.dataset.date = sessISO(x.day);
      c.style.gridArea = `${wd + 2} / ${w + 2}`;
      grid.append(c);
    });
    // a month's name spans its weeks up to the next name, cut there, so a
    // long one never widens the grid past its last week
    labels.forEach(({ m, w }, i) => { m.style.gridArea = `1 / ${w + 2} / 2 / ${(labels[i + 1]?.w ?? weeks) + 2}`; });
    grid.addEventListener("pointerover", (e) => { if (e.target.dataset?.date) ledHeatShow(e.target); });
    grid.addEventListener("pointerleave", () => { $("#ledHeatTip").hidden = true; });
    scroll.replaceChildren(grid);
    if (!scroll.onscroll) {
      // the newest week in sight, by the card's own scroll, never the page's:
      // kept there as the card is laid out or resized, until the reader
      // scrolls it back, and again once they scroll to the end
      scroll.onscroll = () => {
        $("#ledHeatTip").hidden = true;
        // its own scroll to the end, seen after the card narrowed, is not the reader's
        if (scroll.clientWidth && scroll.scrollLeft !== ledHeatSetAt) ledHeatAtEnd = scroll.scrollLeft >= scroll.scrollWidth - scroll.clientWidth - 2;
      };
    }
    ledHeatSized.disconnect();
    ledHeatSized.observe(scroll);
    ledHeatSized.observe(grid);
    ledHeatKeepEnd();
  }
  grid.setAttribute("aria-label", t("Usage heatmap"));
  const cells = grid.querySelectorAll("i");
  days.forEach((x, i) => {
    const c = cells[i];
    c.className = "l" + lv(value(x));
    c.day = x;
  });
  const legend = box.querySelector(".heat-foot");
  legend.replaceChildren(sessLegend());
  const tip = $("#ledHeatTip");
  if (!tip.hidden && tip.cell?.isConnected) ledHeatShow(tip.cell);
  else tip.hidden = true;
}
let ledHeatAtEnd = true, ledHeatSetAt = -1;
const ledHeatSized = new ResizeObserver(() => ledHeatKeepEnd());
function ledHeatKeepEnd() {
  const scroll = $("#ledHeatScroll");
  if (!ledHeatAtEnd || !scroll.clientWidth || scroll.scrollWidth <= scroll.clientWidth) return;
  scroll.scrollLeft = scroll.scrollWidth;
  ledHeatSetAt = scroll.scrollLeft;
}
// the tip over a day, above it, kept inside the card
function ledHeatShow(cell) {
  const box = $("#ledHeat"), tip = $("#ledHeatTip");
  if (!cell.day) return;
  tip.replaceChildren(...ledHeatTip(cell.day).children); // the date over the figures
  tip.cell = cell;
  tip.hidden = false;
  const b = box.getBoundingClientRect(), c = cell.getBoundingClientRect();
  const w = tip.offsetWidth, hgt = tip.offsetHeight;
  const left = Math.max(6, Math.min(b.width - w - 6, c.left - b.left + c.width / 2 - w / 2));
  let top = c.top - b.top - hgt - 6;
  if (top < 4) top = c.bottom - b.top + 6;
  tip.style.left = left + "px";
  tip.style.top = top + "px";
}

return {update(data){ledHeat={data};drawLedHeat();},dispose(){ledHeatSized.disconnect();}};
}
