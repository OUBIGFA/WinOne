// Private Host-owned JSON-lines process. Public plugin messages never enter
// this command namespace: the sandbox adapter dispatches them separately.
import {once} from 'node:events';
import {createGhostHost} from './host.mjs';

let host, closing = false, initializing = false, writes = Promise.resolve(), queuedBytes = 0;
const pending = new Map();
const MAX_LINE = 1 << 20;
function emit(value) {
  const line = Buffer.from(JSON.stringify(value) + '\n');
  if (line.length > MAX_LINE || queuedBytes + line.length > 4 * MAX_LINE) return Promise.reject(Error('Plugin host output exceeded'));
  queuedBytes += line.length;
  const work = writes.then(async () => { if (!process.stdout.write(line)) await once(process.stdout, 'drain'); });
  writes = work.finally(() => { queuedBytes -= line.length; });
  return writes;
}
async function shutdown(error) {
  if (closing) return;
  closing = true;
  try { await host?.close(); await Promise.allSettled([...pending.values()]); await writes; }
  catch (failure) { error ??= failure; }
  if (error) console.error('Plugin host shutdown failed');
  process.exit(error ? 1 : 0);
}
async function handle(request) {
  if (request.method === 'init') {
    if (host || initializing) throw Error('Plugin host is already initialized');
    initializing = true;
    try {
      const params = request.params ?? {};
      // Test-only OS transports are constructor dependencies, never data in
      // the Host command protocol or fields a plugin/model can supply.
      if(params.browserExtensionOptions!==undefined)throw Error('Browser transport dependencies cannot be serialized');
      const base = new URL(params.baseUrl);
      if (base.protocol !== 'http:' || base.hostname !== '127.0.0.1' || base.username || base.password || !params.key) throw Error('Explicit private Host bridge is required');
      const callHost = async (method, input, signal) => {
        const deadline = AbortSignal.timeout(method === 'ui' ? 15 * 60_000 : method === 'task' ? 30 * 60_000 : 120_000);
        const response = await fetch(params.baseUrl + '/ghost-request', {method: 'POST', headers: {'Content-Type': 'application/json', Authorization: 'Bearer ' + params.key}, body: JSON.stringify({method, params: input}), signal: signal ? AbortSignal.any([signal, deadline]) : deadline});
        const result = await response.json();
        if (!response.ok || result.ok !== true) throw Object.assign(Error(result.message ?? 'Plugin Host request failed'), {name: result.name ?? 'Error', code: result.code, retryable: result.retryable === true});
        return result.value;
      };
      host = createGhostHost({...params, host: callHost, event: event => { void emit({type: 'ghost_event', event}).catch(shutdown); }});
      return {ready: true};
    } finally { initializing = false; }
  }
  if (!host || closing) throw Error('Plugin host is not ready');
  return host.call(request.method, request.params);
}
function accept(line) {
  let request;
  try { request = JSON.parse(line); } catch (error) { void shutdown(error); return; }
  if (!request || typeof request.id !== 'string' || !request.id || request.id.length > 128 || typeof request.method !== 'string' || pending.has(request.id) || pending.size >= 128) { void shutdown(Error('Invalid plugin host request')); return; }
  const work = handle(request).then(value => emit({type: 'response', id: request.id, ok: true, value}), error => emit({type: 'response', id: request.id, ok: false, error: '[' + (error.code ?? 'INTERNAL') + '] ' + error.message}));
  pending.set(request.id, work);
  work.then(() => pending.delete(request.id), error => { pending.delete(request.id); void shutdown(error); });
}
let buffer = Buffer.alloc(0);
process.stdin.on('data', chunk => {
  if (closing) return;
  buffer = Buffer.concat([buffer, chunk]);
  for (;;) {
    const newline = buffer.indexOf(10); if (newline < 0) break;
    const line = buffer.subarray(0, newline); buffer = buffer.subarray(newline + 1);
    if (line.length > MAX_LINE) { void shutdown(Error('Plugin host input exceeded')); return; }
    accept(line);
  }
  if (buffer.length > MAX_LINE) void shutdown(Error('Plugin host input exceeded'));
});
process.stdin.on('end', () => { void shutdown(buffer.length ? Error('Incomplete plugin host input') : undefined); });
process.stdin.on('error', shutdown); process.stdout.on('error', shutdown);
void emit({type: 'ready', protocol: 1}).catch(shutdown);
