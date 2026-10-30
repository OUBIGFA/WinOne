// @ts-nocheck
// Generated from yetone/magpie 42ec2700bd2a4cf8d4ceb76a95767ac221e283e5; MIT.
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
export function proxyDisplay(value) {
  // Also mask an older backend's proxyNow, and environment/system proxies.
  return (value || "").replace(/^(\w+:\/\/)?[^/?#]*@/, "$1");
}
