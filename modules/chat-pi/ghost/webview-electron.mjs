// Browser-only replacement for Cindy ghostPreload's Electron calls. Requests
// use the existing random, same-origin plugin endpoint: no Node API, generic Go
// binding or public network bypass enters the guest. Large media never travel
// through repeated native Eval/stdio JSON copies or a 1 MiB queue.
const listeners = new Map();
const rpcURL = globalThis.__winonePluginRpc;
const invokeFetch = globalThis.fetch.bind(globalThis);
let sequence = 0, pending = 0;
export const ipcRenderer = {
  on(channel, callback) {
    if (channel !== 'ghost-pipe:message') throw Error('Unsupported guest channel');
    listeners.set(channel, callback);
  },
  async invoke(channel, payload) {
    if (!['ghost-pipe:ping', 'ghost-pipe:send'].includes(channel)) throw Error('Unsupported guest channel');
    if (pending >= 128) throw Error('Plugin request limit reached');
    if (typeof rpcURL !== 'string') throw Error('Plugin endpoint is unavailable');
    pending++;
    try {
      const response = await invokeFetch(rpcURL, {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({id: String(++sequence), channel, payload})});
      const result = await response.json();
      if (!response.ok || result.error) throw Error(result.error ?? 'Plugin request failed');
      return result.value;
    } finally { pending--; }
  },
};
export const contextBridge = {
  exposeInMainWorld(name, value) {
    if (name !== 'cindy') throw Error('Unsupported guest binding');
    Object.defineProperty(globalThis, name, {value, configurable: false, writable: false});
  },
};
Object.defineProperty(globalThis, '__winoneGhostDeliver', {value: payload => listeners.get('ghost-pipe:message')?.(null, payload)});
