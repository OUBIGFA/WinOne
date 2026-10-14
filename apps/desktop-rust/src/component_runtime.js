// Compatibility transport for the current UI. The pages and components keep
// their existing interfaces; only the native desktop message boundary changes.
(() => {
  if (window !== window.top) return;
  let sequence = 0;
  const pending = new Map();
  const listeners = new Map();
  const call = (method, args) => new Promise((resolve, reject) => {
    // The existing frontend calls this after its settings and home catalog
    // have committed. Record readiness independently of debugger connection
    // time; the next frame marks a paint opportunity, not input-to-photon.
    if (method === "StartupProbeNonce" && performance.getEntriesByName("winone-ui-ready").length === 0) {
      performance.mark("winone-ui-ready");
      requestAnimationFrame(() => requestAnimationFrame(() => performance.mark("winone-ui-ready-frame")));
    }
    const id = ++sequence;
    pending.set(id, {resolve, reject});
    try { window.ipc.postMessage(JSON.stringify({id, method, args})); }
    catch (error) { pending.delete(id); reject(error); }
  });
  Object.defineProperty(window, "__winoneReply", {value: reply => {
    const waiter = pending.get(reply.id);
    if (!waiter) return;
    pending.delete(reply.id);
    if (reply.error) waiter.reject(new Error(reply.error));
    else waiter.resolve(reply.result);
  }});
  Object.defineProperty(window, "__winoneEvent", {value: name => {
    for (const listener of listeners.get(name) || []) listener();
  }});
  window.go = {main: {App: new Proxy({}, {get: (_target, method) =>
    typeof method === "string" ? (...args) => call(method, args) : undefined})}};
  window.runtime = {EventsOn: (name, listener) => {
    if (!listeners.has(name)) listeners.set(name, new Set());
    listeners.get(name).add(listener);
    return () => listeners.get(name)?.delete(listener);
  }};
  window.wails = {flags: {enableResize: !location.pathname.endsWith('/capture.html'), borderThickness: 6, defaultCursor: ""}};
  // Preserve the existing titlebar's drag contract without adding UI markup.
  addEventListener("mousedown", event => {
    if (event.button !== 0 || event.detail !== 1) return;
    if (window.wails.flags.enableResize && window.wails.flags.resizeEdge) {
      void call("BeginWindowResize", [window.wails.flags.resizeEdge]).catch(console.error);
      return;
    }
    for (let element = event.target; element instanceof Element; element = element.parentElement) {
      const drag = getComputedStyle(element).getPropertyValue("--wails-draggable").trim();
      if (drag === "no-drag") return;
      if (drag === "drag") { void call("BeginWindowDrag", []).catch(console.error); return; }
    }
  });
})();
