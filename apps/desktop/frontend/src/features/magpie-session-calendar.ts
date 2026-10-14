// @ts-nocheck
// Magpie ede1e32 app.js: session activity calendar and quartile levels (MIT).
export function magpieSessionCalendar(language,t){
const el=(tag,cls='',text)=>{const e=document.createElement(tag);e.className=cls;if(text!=null)e.textContent=text;return e;};
const intlLang=()=>language,SESS_WEEK=20,SESS_WEEK_MAX=24;
const sessDay=day=>day.toLocaleDateString(language);
// sessLevels grades values 0 (none) to 4 by the quartiles of those above 0,
// as a calendar of contributions does
function sessLevels(values) {
  const nz = values.filter((v) => v > 0).sort((a, b) => a - b);
  if (!nz.length) return () => 0;
  const q = (p) => nz[Math.min(nz.length - 1, Math.floor(p * nz.length))];
  const [a, b, c, top] = [q(0.25), q(0.5), q(0.75), nz[nz.length - 1]];
  return (v) => v <= 0 ? 0 : v >= top ? 4 : v <= a ? 1 : v <= b ? 2 : v <= c ? 3 : 4;
}

function sessLegend() {
  const l = el("span", "sess-legend");
  l.append(el("span", "", t("Less")));
  for (let i = 0; i <= 4; i++) l.append(el("i", "l" + i));
  l.append(el("span", "", t("More")));
  return l;
}

// the short names of the days of the week, Monday first
function sessWeekdays() {
  const loc = intlLang() || "en";
  return Array.from({ length: 7 }, (_, i) => new Date(2024, 0, 1 + i).toLocaleDateString(loc, { weekday: "short" }));
}

// sessCalendar is the range as weeks of days, Monday on top, each day as
// dark as it is busy among the others, after pad empty weeks before it
function sessCalendar(days, value, tip, pad = 0) {
  const lv = sessLevels(days.map(value));
  const lead = (days[0].day.getDay() + 6) % 7 + 7 * pad;
  const weeks = Math.ceil((lead + days.length) / 7);
  const cal = el("div", "sess-cal");
  cal.dataset.pad = pad;
  cal.style.gridTemplateColumns = `auto repeat(${weeks}, minmax(0, 1fr))`;
  cal.style.maxWidth = `calc(2.6em + ${weeks * (pad ? SESS_WEEK_MAX : SESS_WEEK)}px)`;
  const names = sessWeekdays();
  for (const i of [0, 2, 4]) {
    const l = el("span", "wd", names[i]);
    l.style.gridArea = `${i + 2} / 1`;
    cal.append(l);
  }
  const loc = intlLang() || "en";
  let month = -1, labelAt = -9;
  // the days before the range, from the Monday it is laid out from: their
  // months named as the range's are, the days themselves left blank
  const before = Array.from({ length: pad ? lead : 0 }, (_, i) => {
    const d = new Date(days[0].day);
    d.setDate(d.getDate() - lead + i);
    return { day: d, before: true };
  });
  [...before, ...days].forEach((x, i) => {
    const k = (pad ? 0 : lead) + i, w = Math.floor(k / 7), wd = k % 7;
    if ((wd === 0 || i === 0) && x.day.getMonth() !== month) {
      month = x.day.getMonth();
      // a month mostly gone when the weeks before begin is left unnamed, so
      // as not to crowd out the next one's name
      if (w - labelAt >= 3 && !(x.before && x.day.getDate() > 14)) {
        const m = el("span", "mo", x.day.toLocaleDateString(loc, month === 0 && weeks > 20 ? { month: "short", year: "numeric" } : { month: "short" }));
        m.style.gridArea = `1 / ${w + 2}`;
        cal.append(m);
        labelAt = w;
      }
    }
    const c = x.before ? el("s") : el("i", "l" + lv(value(x)));
    c.style.gridArea = `${wd + 2} / ${w + 2}`;
    if (!x.before) c.title = tip(x, sessDay(x.day));
    cal.append(c);
  });
  return cal;
}
return {sessCalendar,sessLegend,sessLevels,sessWeekdays};
}
