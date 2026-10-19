// WinOne's document boundary; page and controls come from the routing plugin.
const mode = "window";
let view = "routing";
let state = { agents: [], profiles: [], settings: {} };
let winOneRoutingParent = "";
const winOneRoutingPending = new Map();
addEventListener("message", event => {
  if (event.source !== parent) return;
  if (event.data?.type === "winone-routing-theme") {
    if (!winOneRoutingParent || winOneRoutingParent === event.origin) winOneRoutingParent = event.origin;
  } else if (winOneRoutingParent && event.origin === winOneRoutingParent && event.data?.type === "winone-routing-native-result") {
    const pending = winOneRoutingPending.get(event.data.id);
    if (!pending) return;
    winOneRoutingPending.delete(event.data.id);
    event.data.error ? pending.reject(new Error(event.data.error)) : pending.resolve();
  }
});
(() => {
  const prefix = location.pathname.match(/^\/session\/[^/]+\//)?.[0];
  if (!prefix) return;
  const original = window.fetch.bind(window);
  window.fetch = async (input, init) => {
    const target = new URL(typeof input === "string" ? input : input.url || input, location.href);
    const method = init?.method || (input instanceof Request ? input.method : "GET");
    if (target.origin === location.origin && target.pathname.endsWith("/api/plugins/builtin") && method.toUpperCase() === "POST") {
      const body = JSON.parse(init?.body || (input instanceof Request ? await input.clone().text() : "{}"));
      if (body.id !== "groups" || body.on !== true || !winOneRoutingParent) throw new Error("Unsupported routing plugin action");
      const id = crypto.randomUUID();
      await new Promise((resolve, reject) => {
        winOneRoutingPending.set(id, {resolve, reject});
        parent.postMessage({type: "winone-routing-native", id, method: "enable-groups"}, winOneRoutingParent);
      });
      return new Response(JSON.stringify(state.settings.builtins.map(value => value.id === "groups" ? {...value, on: true, chosen: true} : value)), {headers: {"Content-Type": "application/json"}});
    }
    if (target.origin === location.origin && !target.pathname.startsWith(prefix)) target.pathname = prefix + target.pathname.replace(/^\//, "");
    return original(input instanceof Request ? new Request(target, input) : target, init);
  };
})();

// A settings write owns its new value until a read that began after it.
let winOneRead = 0;
async function load() {
  const read = ++winOneRead, writes = prefsWrites;
  const [nextState, nextProviders] = await Promise.all([api("state"), api("providers")]);
  if (read !== winOneRead) return;
  if (!prefsSettled(writes)) nextState.settings = state.settings;
  state = nextState;
  prefs = state.settings;
  applyFeatures(prefs.builtins);
  providers = nextProviders;
  quotaLeft = !!prefs.quotaLeft;
  chineseUnits = !!prefs.chineseUnits;
  currency = prefs.currency || "USD";
  window.hideAccounts?.follow(!!prefs.redactPersonal);
  redrawNoNames();
  fit();
}
const loadProviders = load;
function fit() { window.dispatchEvent(new Event("winone-routing-layout")); }
