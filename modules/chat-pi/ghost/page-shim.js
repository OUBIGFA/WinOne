// Loaded first in a plugin's panel, main view, task view and settings pages. Cindy
// keeps these pages and the plugin's logic in one browser partition; here the logic
// runs in its own hidden process, so this script carries what that sharing gave:
// same-origin calls under the private prefix, cindy-ghost:// addresses, and a
// BroadcastChannel that reaches the logic page through the sandbox's local server.
(() => {
  const root = globalThis.__winonePluginScope ?? (document.currentScript?.src ? new URL(document.currentScript.src).pathname.split('/').slice(0, 2).join('/') : '');
  const source = crypto.randomUUID();
  const native = globalThis.fetch.bind(globalThis);
  const local = address => {
    const text = String(address);
    const scheme = /^cindy-ghost:\/\/[^/]+(\/.*)$/.exec(text);
    if (scheme) return root + scheme[1];
    const url = new URL(text, location.href);
    if (url.origin !== location.origin) return null;
    return url.pathname.startsWith(root + '/') ? url.href : root + url.pathname + url.search;
  };
  globalThis.fetch = (input, init) => {
    const target = local(typeof input === 'string' || input instanceof URL ? input : input.url);
    return target === null ? Promise.reject(Error('Direct network access is unavailable; use cindy.fetch')) : native(target, init);
  };
  const rewrite = element => {
    for (const name of ['src', 'href']) {
      const value = element.getAttribute?.(name);
      if (value?.startsWith('cindy-ghost://')) element.setAttribute(name, local(value));
    }
  };
  new MutationObserver(records => {
    for (const record of records) {
      if (record.type === 'attributes') rewrite(record.target);
      for (const node of record.addedNodes ?? []) {
        if (node.nodeType !== 1) continue;
        rewrite(node);
        node.querySelectorAll?.('[src^="cindy-ghost:"],[href^="cindy-ghost:"]').forEach(rewrite);
      }
    }
  }).observe(document, {subtree: true, childList: true, attributes: true, attributeFilter: ['src', 'href']});

  const channels = new Map();
  let stream;
  const listen = () => {
    if (stream) return;
    stream = new EventSource(root + '/__bc/stream');
    stream.onmessage = event => {
      const message = JSON.parse(event.data);
      // Native BroadcastChannel excludes the posting object. Sibling objects
      // in this document were already notified locally; skip our server echo.
      if (message.source === source) return;
      const {channel, data} = message;
      for (const open of channels.get(channel) ?? []) open.dispatchEvent(new MessageEvent('message', {data}));
    };
  };
  globalThis.BroadcastChannel = class extends EventTarget {
    constructor(name) {
      super();
      this.name = String(name);
      this.onmessage = null;
      this.addEventListener('message', event => this.onmessage?.(event));
      if (!channels.has(this.name)) channels.set(this.name, new Set());
      channels.get(this.name).add(this);
      listen();
    }
    postMessage(data) {
      for (const open of channels.get(this.name) ?? []) if (open !== this) queueMicrotask(() => open.dispatchEvent(new MessageEvent('message', {data})));
      void native(root + '/__bc/post', {method: 'POST', body: JSON.stringify({channel: this.name, source, data})});
    }
    close() { channels.get(this.name)?.delete(this); }
  };
})();
