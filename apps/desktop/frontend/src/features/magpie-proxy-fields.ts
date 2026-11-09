// @ts-nocheck
// Generated from yetone/magpie c9d253e4a2e7add1e31578244eda52bfdce11258; MIT.
// Only module exports are added. See third_party/magpie-proxy-source.json.
export function proxyFields(value) {
  const [, scheme = "", auth, rest] = /^([a-z][\w+.-]*:\/\/)?([^/?#]*)(.*)$/is.exec((value || "").trim());
  const at = auth.lastIndexOf("@"), info = at < 0 ? "" : auth.slice(0, at), c = info.indexOf(":");
  const eu = c < 0 ? info : info.slice(0, c), ep = c < 0 ? "" : info.slice(c + 1);
  // non-UTF-8 escapes are kept as saved until edited
  const dec = (v) => { try { return decodeURIComponent(v); } catch { return v; } };
  return { address: scheme + auth.slice(at + 1) + rest, username: dec(eu), password: dec(ep), eu, ep };
}
// each typed part percent-encoded; one left as loaded keeps its escapes
export function proxyAddress(a, f, user, pass) {
  a = (a || "").trim();
  if (!a || !(user || pass)) return a;
  const enc = (t, l, e) => t === l && e !== undefined ? e : encodeURIComponent(t);
  const [, scheme = "", rest] = /^([a-z][\w+.-]*:\/\/)?(.*)$/is.exec(a);
  return scheme + enc(user, f.username, f.eu) + (pass ? ":" + enc(pass, f.password, f.ep) : "") + "@" + rest;
}
// proxyLoginRow: a proxy login's fields for the proxy rows outside
// Settings (#3382), split and joined as Settings' row does (proxyFields,
// proxyAddress), so setURL gets the address Settings would save.
export function proxyLoginRow(addr, setURL, changed) {
  const fields = {};
  const auth = el("div", "proxy-auth proxy-login");
  const username = input("", t("Username (optional)"));
  const password = input("", t("Password (optional)"), "password");
  for (const f of [username, password]) {
    f.className = "proxy-credential";
    f.setAttribute("aria-label", f.placeholder);
  }
  auth.append(username, password);
  const join = () => {
    let v = addr.value;
    try {
      v = proxyAddress(addr.value.trim(), fields, username.value, password.value);
    } catch {} // a half-typed address is kept as it was typed
    setURL(v);
  };
  const split = (quiet) => {
    const parsed = proxyFields(addr.value.trim());
    if (parsed.username || parsed.password) {
      Object.assign(fields, { username: parsed.username, password: parsed.password, eu: parsed.eu, ep: parsed.ep });
      username.value = parsed.username;
      password.value = parsed.password;
      addr.value = parsed.address;
    }
    if (!quiet) { join(); if (changed) changed(); }
  };
  username.oninput = password.oninput = join;
  addr.oninput = () => split();
  split(true);
  return auth;
}
export function proxyDisplay(value) {
  // Also mask an older backend's proxyNow, and environment/system proxies.
  return (value || "").replace(/^(\w+:\/\/)?[^/?#]*@/, "$1");
}
