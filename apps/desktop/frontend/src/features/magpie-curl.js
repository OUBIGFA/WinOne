// Magpie 7660399a121af67db4efd56ecb6bd4fb04dc53f1 / MIT. Extracted by build/prepare-ai-curl.mjs.
function respellURL(u, api) {
  u = u.trim().replace(/\/+$/, "");
  if (api === "anthropic") return u.replace(/\/v1$/, "");
  if (api === "decide") u = u.replace(/\/systemone$/, "");
  if (api === "gemini") return /^https?:\/\/[^/]+\/v1$/.test(u) ? u + "beta" : /^https?:\/\/[^/]+$/.test(u) ? u + "/v1beta" : u;
  return /\/v\d+[a-z]*$/.test(u) || !/^https?:\/\/[^/]+$/.test(u) ? u : u + "/v1";
}
function curlOf(text) {
  let s = String(text || "").trim();
  const head = /^curl(?:\.exe)?(?=\s)/i.exec(s);
  if (!head) return null;
  const cmd = /\^[ \t]*\r?\n/.test(s), ps = /`[ \t]*\r?\n/.test(s) || /^curl\.exe/i.test(s);
  s = s.slice(head[0].length).replace(/(?:\\|\^|`)[ \t]*\r?\n/g, " ");
  // the words as the shell hands them to curl: quotes taken off, escapes read
  const words = [];
  let w = null;
  for (let i = 0; i < s.length;) {
    const c = s[i];
    if (/\s/.test(c)) { if (w !== null) words.push(w); w = null; i++; continue; }
    w ??= "";
    if (c === "'") {
      // PowerShell doubles a quote inside single quotes
      let j = i + 1;
      for (;;) {
        const k = s.indexOf("'", j);
        if (k < 0) { w += s.slice(j); i = s.length; break; }
        if (ps && s[k + 1] === "'") { w += s.slice(j, k) + "'"; j = k + 2; continue; }
        w += s.slice(j, k); i = k + 1; break;
      }
    } else if (c === '"') {
      for (i++; i < s.length && s[i] !== '"'; i++) {
        const esc = (s[i] === "\\" && /["\\$`]/.test(s[i + 1] || "")) || (ps && s[i] === "`");
        if (esc) i++;
        w += s[i] ?? "";
      }
      i++;
    } else if ((c === "\\" && !cmd && !ps) || (c === "^" && cmd)) { w += s[i + 1] ?? ""; i += 2; }
    else { w += c; i++; }
  }
  if (w !== null) words.push(w);
  // the options curl reads a value after; the others stand alone
  const valued = /^(-[HdXuoAeFbcmT]|--(header|data|data-raw|data-binary|data-ascii|data-urlencode|json|request|url|user|output|user-agent|referer|form|cookie|max-time|connect-timeout|proxy|upload-file|retry|cert|key|cacert))$/;
  let url = "", body = "";
  const headers = [];
  for (let i = 0; i < words.length; i++) {
    const x = words[i];
    let opt = x, val = null;
    if (/^-[Hd]./.test(x)) { opt = x.slice(0, 2); val = x.slice(2); }
    else if (valued.test(x)) val = words[++i] ?? "";
    else if (x.startsWith("-")) continue;
    else { if (!url) url = x; continue; }
    if (opt === "-H" || opt === "--header") headers.push(val);
    else if (opt === "--url") url = val;
    else if (/^(-d|--data(-raw|-binary|-ascii)?|--json)$/.test(opt) && !val.startsWith("@")) body = val;
  }
  const out = { url, api: "", base: "", urlHole: "", key: "", placeholder: "", model: "" };
  const at = /^(https?:\/\/[^?#]*?)\/*(\/chat\/completions|\/messages|\/responses)\/*(?:[?#].*)?$/i.exec(url);
  if (at) {
    out.api = { "/chat/completions": "openai", "/messages": "anthropic", "/responses": "responses" }[at[2].toLowerCase()];
    out.base = respellURL(at[1], out.api);
    out.urlHole = (/\{[^}]*\}|<[^>]*>|\$\{?\w+\}?|%\w+%/.exec(out.base) || [""])[0];
  }
  for (const h of headers) {
    const m = /^\s*(authorization|x-api-key|api-key)\s*:\s*(.*?)\s*$/i.exec(h);
    if (!m) continue;
    const v = m[1].toLowerCase() === "authorization" ? m[2].replace(/^bearer\s+/i, "") : m[2];
    if (!v) continue;
    if (/^\$|^%\w+%$|^<.*>$|^\{\{.*\}\}$|^\[.*\]$|your[_-]?|api[_-]?key|x{6,}|\.\.\.|…/i.test(v)) out.placeholder = v;
    else out.key = v;
    break;
  }
  // a body for PowerShell 5 often escapes its quotes for curl.exe: '{\"model\": …}'
  for (const b of [body, body.replace(/\\"/g, '"')]) {
    try { const j = JSON.parse(b); if (typeof j?.model === "string") out.model = j.model.trim(); break; } catch {}
  }
  return out;
}
export {curlOf};
