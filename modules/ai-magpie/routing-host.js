// Complete Magpie routing behavior in a WinOne settings document. The parent
// owns navigation; the design adapter owns the presentation, never the data.
magpie.strings({zh:{"Request activity":"请求动态","Turn on routing groups to create or edit custom groups. Automatic groups remain available.":"开启路由组后，可新建或修改自己的组。自动发现的组仍可正常使用。"}});
let winOneParentOrigin = "", winOneLastHeight = 0, winOneMeasureFrame = 0;
function publishRoutingHeight() {
  cancelAnimationFrame(winOneMeasureFrame);
  winOneMeasureFrame = requestAnimationFrame(() => {
    const page = document.getElementById("view-routing");
    if (!page || !winOneParentOrigin) return;
    const height = Math.ceil(page.getBoundingClientRect().height);
    if (height > 0 && height !== winOneLastHeight) {
      winOneLastHeight = height;
      parent.postMessage({ type: "winone-routing-height", height }, winOneParentOrigin);
    }
  });
}
function applyWinOneTheme(value) {
  if (value.language) { setLocale(value.language.startsWith("zh") ? "zh" : "en"); document.documentElement.lang = value.language; }
  if (value.theme) {
    document.documentElement.classList.toggle("dark", value.theme === "dark");
    document.documentElement.dataset.theme = value.theme;
    document.documentElement.style.colorScheme = value.theme;
  }
  winOneLastHeight = 0;
  publishRoutingHeight();
}
window.addEventListener("message", event => {
  if (event.source !== parent || event.data?.type !== "winone-routing-theme") return;
  if (winOneParentOrigin && winOneParentOrigin !== event.origin) return;
  winOneParentOrigin = event.origin;
  applyWinOneTheme(event.data);
});
window.addEventListener("winone-routing-layout", publishRoutingHeight);
window.addEventListener("beforeunload", event => {
  if (magpie.call("routing", "dirty")) { event.preventDefault(); event.returnValue = ""; }
});
// Context details borrow the upstream modal. Keep dismissal and focus handling
// here, without loading the unrelated provider editor's event handlers.
document.addEventListener("keydown", event => {
  const modal = document.getElementById("modal");
  if (event.key === "Escape" && !pick) {
    if (confirmAsk) { event.preventDefault(); closeConfirmAsk(); }
    else if (!modal.hidden) { event.preventDefault(); closeModal(); }
  }
  if (event.key !== "Tab" || modal.hidden) return;
  const fields = [...modal.querySelectorAll('button:not([disabled]),input:not([disabled]),textarea,[tabindex="0"]')].filter(node => node.getClientRects().length);
  const first = fields[0], last = fields.at(-1);
  if (first && (event.shiftKey ? document.activeElement === first : document.activeElement === last)) {
    event.preventDefault(); (event.shiftKey ? last : first).focus();
  }
});
const modal = document.getElementById("modal");
const onBackdrop = backdropClick(modal);
modal.addEventListener("click", event => { if (onBackdrop(event)) confirmAsk ? closeConfirmAsk() : closeModal(); });

async function startWinOneRouting() {
  try {
    await load();
    const page = document.getElementById("view-routing");
    page.hidden = false;
    page.tabIndex = -1;
    magpie.start();
    translateStatic();
    new ResizeObserver(publishRoutingHeight).observe(page);
    publishRoutingHeight();
    void loadQuotas();
  } catch (error) {
    status(error.message, "err", 0);
    const retry = el("button", "text", t("Try again"));
    retry.onclick = () => { retry.remove(); void startWinOneRouting(); };
    document.getElementById("status").append(retry);
  }
}
applyWinOneTheme({ language: window.bootPrefs?.lang, theme: window.bootPrefs?.theme });
void startWinOneRouting();
