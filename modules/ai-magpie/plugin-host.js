// Complete plugin operations in WinOne's settings layout. The upstream owns
// registry and sandbox semantics; WinOne owns visual hierarchy and navigation.
magpie.strings({zh:{"Install a plugin":"安装插件","About plugins":"插件说明"}});
async function loadProviders() { providers = await api("providers"); }
function openProvider(id) {
  const provider = providers?.plugins?.find(value => value.id === id);
  void winOnePluginNative("provider", {provider: provider?.pid || id, name: provider?.name, plugin: provider?.spec}).catch(error => status(error.message, "err"));
}
const pluginSignIn = openProvider;
function renderProviders() {}
function fit() { publishPluginHeight(); }
// No Magpie application sidebar or Settings page is imported with a surface.
function navSubs() {}
let surfaceMounted = false, actionDelivered = false;
window.winOnePluginLoaded = async list => {
  const slot = params.get("slot"), key = params.get("page");
  if (slot && ["agents", "usage", "settings", "provider"].includes(slot)) {
    document.documentElement.classList.add("winone-plugin-surface");
    document.getElementById("view-plugins").hidden = true;
    if (!surfaceMounted) {
      surfaceMounted = true;
      const context = slot === "provider" ? {provider: {id: params.get("item-id") || "", name: params.get("item-name") || ""}} : {};
      document.getElementById("plugin-content").append(magpie.slot(slot, context));
    }
    publishPluginHeight();
    return;
  }
  const entry = list.find(item => item.key === key);
  if (!entry || surfaceMounted) return;
  surfaceMounted = true;
  if (params.get("surface") === "1") document.documentElement.classList.add("winone-plugin-surface");
  const id = params.get("action"), menu = params.get("menu");
  if (id && !actionDelivered && entry.actions?.some(action => action.id === id && action.menu === menu) && ["provider", "agent"].includes(menu)) {
    actionDelivered = true;
    // Never forward an entire provider object. Upstream's act retains its
    // hello/late-onAction queue, permissions and local failure handling.
    await window.winOnePluginAction(key, id, {[menu]: {id: params.get("item-id") || "", name: params.get("item-name") || ""}});
  } else await show(key);
};
// The standalone host has no application's periodic refresh. Bring newly
// installed/disabled page tabs into the same successful manager refresh.
function renderPluginDot() { void window.loadPluginPages?.(); }
window.renderPluginDot = renderPluginDot;
let pluginHeightFrame = 0;
function publishPluginHeight() {
  cancelAnimationFrame(pluginHeightFrame);
  pluginHeightFrame = requestAnimationFrame(() => {
    if (!winOnePluginParent) return;
    parent.postMessage({type: "winone-plugin-height", height: Math.ceil(document.body.getBoundingClientRect().height)}, winOnePluginParent);
  });
}
async function show(key) {
  if (key !== "plugins" && !magpie.page(key)) return;
  if (view !== key) { magpie.call(view, "close"); closeModal(); }
  view = key;
  for (const main of document.querySelectorAll("#plugin-content > .view")) main.hidden = main.id !== "view-" + key;
  for (const button of document.querySelectorAll("#nav button[data-view]")) {
    const current = button.dataset.view === key;
    button.classList.toggle("on", current);
    if (current) button.setAttribute("aria-current", "page"); else button.removeAttribute("aria-current");
  }
  await magpie.call(key, "open");
  publishPluginHeight();
}
window.winOnePluginTheme = value => {
	const previousLocale = locale;
  if (value.language) { setLocale(value.language.startsWith("zh") ? "zh" : "en"); document.documentElement.lang = value.language; }
  if (value.theme) {
    document.documentElement.dataset.theme = value.theme;
    document.documentElement.classList.toggle("dark", value.theme === "dark");
    document.documentElement.style.colorScheme = value.theme;
  }
  translateStatic();
  document.getElementById("nav").setAttribute("aria-label", locale === "zh" ? "插件页面" : "Plugin pages");
  if (locale !== previousLocale) { void window.loadPlugins?.(); void window.loadPluginPages?.(); }
  publishPluginHeight();
};
const pluginManagerButton = el("button", "on", t("Plugins"));
pluginManagerButton.dataset.view = "plugins";
pluginManagerButton.dataset.en = "Plugins";
pluginManagerButton.setAttribute("data-t", "");
pluginManagerButton.onclick = () => void show("plugins");
document.getElementById("nav").append(pluginManagerButton);
// The same upstream modal and escape handling used by its manager.
document.addEventListener("keydown", event => {
  if (event.key === "Escape" && !document.getElementById("modal").hidden) closeModal();
});
const pluginModal = document.getElementById("modal"), pluginBackdrop = backdropClick(pluginModal);
pluginModal.addEventListener("click", event => { if (pluginBackdrop(event)) closeModal(); });
addEventListener("DOMContentLoaded", () => {
  magpie.start();
  if (params.has("slot") || params.get("surface") === "1") document.documentElement.classList.add("winone-plugin-surface");
  window.winOnePluginTheme({language: window.bootPrefs?.lang, theme: window.bootPrefs?.theme});
  new ResizeObserver(publishPluginHeight).observe(document.body);
});
