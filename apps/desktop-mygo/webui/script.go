package webui

// preload runs in the page before its own scripts, once window.mygo is
// there. It gives the React code the host it had in the Rust window
// (apps/desktop-rust/src/component_runtime.js): window.go.main.App, whose
// every method calls the bound App service, and window.runtime.EventsOn.
// Go's window events arrive as winone:dispatch.
//
// Files dropped from outside reach the page twice: MyGo hands their paths
// to Go and to this trusted page (mygo:file-drop), and the page gets the
// DOM drop, which carries no paths. That drop stops here, so React's drop
// zones do not report a drop without paths; the paths then go where the
// Rust window sent them (component_ui.rs): the drop zone under the point,
// else, outside dialogs, the Chat page (winone:chat-files-drop), which in
// this page is the file library or, when it declines, the native Chat
// page's attachments.
const preload = `(() => {
  if (window !== window.top || !window.mygo) return;
  const listeners = new Map();
  window.go = {main: {App: new Proxy({}, {get: (_target, method) =>
    typeof method === "string" ? (...args) => mygo.call("App." + method, ...args) : undefined})}};
  window.runtime = {EventsOn: (name, listener) => {
    if (!listeners.has(name)) listeners.set(name, new Set());
    listeners.get(name).add(listener);
    return () => listeners.get(name)?.delete(listener);
  }};
  mygo.on("winone:dispatch", ({name, detail, runtime}) => {
    if (runtime) { for (const listener of [...(listeners.get(name) || [])]) listener(); return; }
    window.dispatchEvent(new CustomEvent(name, {detail}));
  });
  let dragging = false, point = null;
  addEventListener("dragstart", () => { dragging = true; }, true);
  addEventListener("dragend", () => { dragging = false; }, true);
  addEventListener("drop", event => {
    if (dragging || !event.dataTransfer || !Array.from(event.dataTransfer.types).includes("Files")) return;
    point = {x: event.clientX, y: event.clientY};
    event.preventDefault();
    event.stopImmediatePropagation();
  }, true);
  mygo.on("mygo:file-drop", ({paths}) => {
    const position = point || {x: innerWidth / 2, y: innerHeight / 2};
    point = null;
    const element = document.elementFromPoint(position.x, position.y);
    if (element && !element.dispatchEvent(new CustomEvent("winone:paths-drop", {bubbles: true, cancelable: true, detail: {paths, position}}))) return;
    if (document.querySelector("[role=dialog]")) return;
    window.dispatchEvent(new CustomEvent("winone:chat-files-drop", {detail: {paths, position}}));
  });
  // The file library takes a drop over it in the capture phase; one that
  // reaches here goes to the conversation's attachments.
  addEventListener("winone:chat-files-drop", event => {
    mygo.call("App.Embed", "drop", JSON.stringify(event.detail.paths)).catch(() => {});
  });
})();`
