// @ts-nocheck
// Original Magpie ede1e32 context grid and sparkline, internal/gui/assets/context.js.
// MIT. WinOne supplies translation and DOM helpers without replacing the drawing.
export function magpieContextGraphics(t,el,icon,state){
  const PARTS = ["system", "tools", "memory", "files", "results", "chat"];
  const PART_NAMES = {
    system: "System prompt", tools: "Tools", memory: "Memory",
    files: "Files", results: "Tool results", chat: "Conversation",
  };
  const PART_NOTES = {
    system: "The agent's own instructions",
    tools: "The definitions of the tools it can call",
    memory: "Instruction files, memory and reminders",
    files: "Files it read, and images",
    results: "What the other tools it called returned",
    chat: "What you and the model said, turn by turn",
  };
  const CELLS = 400;
  const partName = (k) => t(PART_NAMES[k] || k);

  const fmtK = (n) => {
    n = Math.max(0, Math.round(n || 0));
    const short = (x, d) => x.toFixed(d).replace(/\.0+$/, "");
    if (n >= 1e6) return short(n / 1e6, n >= 1e7 ? 1 : 2) + "M";
    if (n >= 1e3) return short(n / 1e3, n >= 1e5 ? 0 : 1) + "K";
    return String(n);
  };
  const pct = (x) => Math.round(x * 100) + "%";
  const median = (xs) => {
    if (!xs.length) return 0;
    const s = [...xs].sort((a, b) => a - b);
    return s[s.length >> 1];
  };
  const agentMeta = (id) => (state.clients || state.agents || []).find((a) => a.id === id);
  const agentLabel = (id) => agentMeta(id)?.name || id || t("your agent");
  const agentIcon = (id) => icon(agentMeta(id)?.icon || "generic");

  // what an item of the prompt is called: a file's path or a tool's name
  // as it is; magpie's own names for the rest in the reader's language
  const ITEM_NAMES = {
    prompt: "Agent prompt", harness: "Harness messages", billing: "Billing header",
    environment: "Environment notes", context: "Context", skills: "Skills",
    reminders: "System reminders", compacted: "Compacted summary", image: "Image", document: "Document",
  };
  function itemName(it) {
    if (it.tag === "turn") return t("Turn {n}", { n: it.name });
    if (it.tag === "turns") return t("{n} earlier turns", { n: it.n });
    if (it.tag === "more") return t("{n} more", { n: it.n });
    return ITEM_NAMES[it.name] ? t(ITEM_NAMES[it.name]) : it.name;
  }
  // a path's file name first, where it is after it
  function itemLabel(it, kind) {
    const name = itemName(it);
    const e = el("span", "ctx-name");
    if ((kind === "files" || kind === "memory") && name.includes("/")) {
      const at = name.lastIndexOf("/");
      e.append(el("b", "", name.slice(at + 1)), el("small", "", name.slice(0, at + 1)));
    } else e.append(el("b", "", name));
    e.title = name;
    return e;
  }
  const TAG_NAMES = {
    mcp: "MCP", namespace: "namespace", read: "read", shell: "shell", result: "result",
    user: "user", project: "project", local: "local", memory: "memory", skills: "skills",
    reminder: "reminder", image: "image", turn: "", turns: "", more: "", compacted: "",
  };
  const tagOf = (it) => it.tag in TAG_NAMES ? TAG_NAMES[it.tag] : it.tag || "";

  const health = (fill) => fill < 0.75 ? ["ok", "Healthy"] : fill < 0.9 ? ["warn", "Filling up"] : ["bad", "Near full"];

  // the cache a route's vendor read: its last answer's usage
  function cacheOf(r) {
    const u = r.usage?.[r.usage.length - 1];
    if (!u) return null;
    const total = (u.in || 0) + (u.cache_read || 0) + (u.cache_write || 0);
    if (!total) return null;
    return { read: u.cache_read || 0, total };
  }

  // the cells of the grid, each the part and item it shows; a window
  // smaller than the prompt (not known, or overrun) is the prompt
  function cellsOf(p, window) {
    const scale = Math.max(window, p.tokens, 1);
    const used = Math.min(CELLS, Math.round((p.tokens / scale) * CELLS));
    const parts = PARTS.map((k) => p.parts.find((x) => x.kind === k)).filter((x) => x && x.tokens > 0);
    const sum = parts.reduce((s, x) => s + x.tokens, 0) || 1;
    // largest remainder, so the cells add up to used and a part of any
    // size has at least one
    const want = parts.map((x) => (x.tokens / sum) * used);
    const got = want.map((w) => Math.max(1, Math.floor(w)));
    let left = used - got.reduce((s, n) => s + n, 0);
    const order = want.map((w, i) => [w - Math.floor(w), i]).sort((a, b) => b[0] - a[0]);
    for (let j = 0; left > 0 && order.length; j = (j + 1) % order.length, left--) got[order[j][1]]++;
    for (let j = got.length - 1; left < 0 && j >= 0; j--) while (left < 0 && got[j] > 1) { got[j]--; left++; }
    const cells = [];
    parts.forEach((x, pi) => {
      const items = [...(x.items || [])].sort((a, b) => b.tokens - a.tokens);
      const isum = items.reduce((s, it) => s + it.tokens, 0) || 1;
      let at = 0, acc = 0;
      for (let c = 0; c < got[pi]; c++) {
        // the item whose share the cell's middle falls in
        const mid = ((c + 0.5) / got[pi]) * isum;
        while (at < items.length - 1 && acc + items[at].tokens < mid) acc += items[at++].tokens;
        cells.push({ kind: x.kind, item: items.length ? at : -1 });
      }
    });
    while (cells.length < CELLS) cells.push(null);
    return cells.slice(0, CELLS);
  }

  // the headroom: how many more requests fit, at the pace the session's
  // prompts grew
  function headroom(series, free) {
    const steps = [];
    for (let i = 1; i < series.length; i++) {
      const d = series[i].tokens - series[i - 1].tokens;
      if (d > 0 && series[i].tokens >= series[i - 1].tokens * 0.6) steps.push(d);
    }
    const g = median(steps);
    return g > 0 ? Math.floor(free / g) : null;
  }

  // count words a number with the phrase for one, or for many
  const count = (n, one, many) => n === 1 ? t(one) : t(many, { n });

  // spark draws a session's prompts, request by request, against its
  // window; the one the card tells of is marked, and a point clicked opens
  // its request
  function spark(series, at, window, onPoint) {
    const W = 240, H = 34, pad = 3;
    // scaled to the session's peak, so a session that never came near its
    // window still shows how it grew; the window's line shows once it is near
    const peak = Math.max(...series.map((p) => p.tokens), 1);
    const top = window && peak > window * 0.6 ? Math.max(window, peak) : peak * 1.2;
    const x = (i) => series.length < 2 ? W / 2 : pad + (i / (series.length - 1)) * (W - pad * 2);
    const y = (n) => H - pad - (n / top) * (H - pad * 2);
    const box = el("span", "ctx-spark");
    const s = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    s.setAttribute("viewBox", `0 0 ${W} ${H}`);
    s.setAttribute("preserveAspectRatio", "none");
    box.append(s);
    const ns = (tag, attrs) => {
      const e = document.createElementNS("http://www.w3.org/2000/svg", tag);
      for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, v);
      s.append(e);
      return e;
    };
    if (window && window <= top) ns("line", { x1: 0, x2: W, y1: y(window), y2: y(window), class: "win" });
    const pts = series.map((p, i) => `${x(i).toFixed(1)},${y(p.tokens).toFixed(1)}`);
    if (series.length > 1) {
      ns("path", { d: `M${pts[0]} L${pts.slice(1).join(" L")} L${x(series.length - 1).toFixed(1)},${H} L${x(0).toFixed(1)},${H} Z`, class: "area" });
      ns("polyline", { points: pts.join(" "), class: "line" });
    }
    series.forEach((p, i) => {
      const on = p.id === at;
      // the marker is a dot over the stretched svg, so it stays round
      if (on) {
        const dot = el("span", "at");
        dot.style.left = (x(i) / W * 100).toFixed(2) + "%";
        dot.style.top = (y(p.tokens) / H * 100).toFixed(2) + "%";
        box.append(dot);
      }
      if (!onPoint) return;
      const hit = ns("rect", { x: x(i) - Math.max(2, W / series.length / 2), y: 0, width: Math.max(4, W / series.length), height: H, class: "hit" + (on ? " on" : "") });
      const tip = document.createElementNS("http://www.w3.org/2000/svg", "title");
      tip.textContent = t("Request {n}: {tokens} tokens", { n: i + 1, tokens: fmtK(p.tokens) });
      hit.append(tip);
      hit.onclick = (e) => { e.stopPropagation(); onPoint(p); };
    });
    return box;
  }

  // A card's narrow layouts go by its own width, as container queries
  // would, set here as classes (max520: 520px wide or less). Not by
  // container queries: WebKit pulled a view scrolled to its end back up by
  // a card's height each time the card was drawn again (#1249). A card
  // drawn again starts with the width the last one in its place had, so it
  // doesn't lay out wide for a moment and pull the page up as it narrows

return {cellsOf,spark,headroom};
}
