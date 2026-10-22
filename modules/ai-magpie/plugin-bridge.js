// Authenticated standalone document: WinOne supplies native picking and
// navigation; plugin pages retain the upstream scripts-only sandbox.
const mode = "window", params = new URLSearchParams(location.search);
let view = "plugins", state = {agents: [], profiles: [], settings: {}};
let winOnePluginParent = "";
const winOnePluginPending = new Map();
const winOnePluginPrefix = location.pathname.match(/^\/session\/[^/]+\//)?.[0] || "/";
window.winOnePluginURL = value => {
  const target = new URL(value, location.href);
  if (target.origin === location.origin && !target.pathname.startsWith(winOnePluginPrefix)) target.pathname = winOnePluginPrefix + target.pathname.replace(/^\//, "");
  return target.href;
};
function winOnePluginNative(method, data = {}) {
  if (!winOnePluginParent) return Promise.reject(new Error("WinOne page bridge is not ready"));
  const id = crypto.randomUUID();
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      winOnePluginPending.delete(id);
      reject(new Error(document.documentElement.lang.startsWith("zh") ? "WinOne 未响应，请重新打开插件页后重试" : "WinOne did not respond. Reopen the plugins page and retry."));
    }, 120000);
    winOnePluginPending.set(id, {resolve, reject, timer});
    parent.postMessage({type: "winone-plugin-native", id, method, ...data}, winOnePluginParent);
  });
}
addEventListener("message", event => {
  if (event.source !== parent) return;
  if (event.data?.type === "winone-plugin-theme") {
    if (winOnePluginParent && event.origin !== winOnePluginParent) return;
    winOnePluginParent = event.origin;
    window.winOnePluginTheme?.(event.data);
  } else if (event.origin === winOnePluginParent && event.data?.type === "winone-plugin-native-result") {
    const pending = winOnePluginPending.get(event.data.id);
    if (!pending) return;
    clearTimeout(pending.timer);
    winOnePluginPending.delete(event.data.id);
    event.data.error ? pending.reject(new Error(event.data.error)) : pending.resolve(event.data.result);
  }
});
(() => {
  const original = fetch.bind(window);
  window.fetch = async (input, init) => {
    const target = new URL(typeof input === "string" ? input : input.url || input, location.href);
    if (target.origin === location.origin && target.pathname.endsWith("/api/plugins/choose")) {
      return new Response(JSON.stringify({dir: await winOnePluginNative("choose")}), {headers: {"Content-Type": "application/json"}});
    }
    const response = await original(input instanceof Request ? new Request(window.winOnePluginURL(target), input) : window.winOnePluginURL(target), init);
    // Only successful registry mutations invalidate the Host's account cache.
    const method = init?.method || (input instanceof Request ? input.method : "GET");
    if (response.ok && method.toUpperCase() === "POST" && /\/api\/plugins\/(add|off|options|update|upgrade|remove|prefer)$/.test(target.pathname) && winOnePluginParent) {
      parent.postMessage({type: "winone-plugin-changed"}, winOnePluginParent);
    }
    return response;
  };
})();
