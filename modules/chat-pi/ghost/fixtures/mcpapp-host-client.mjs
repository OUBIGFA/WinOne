// Independent official App SDK used by the real Go Host/PiInbox regression.
// The transport is the native-only HTTP channel issued by createGhostMcpApps;
// this process has no fake Host, virtual MCP server or message acceptance path.
import {App} from '@modelcontextprotocol/ext-apps';
import {once} from 'node:events';
import {Database} from 'bun:sqlite';
import fs from 'node:fs/promises';
import JSZip from 'jszip';
import {localMailManifest, localMailMain} from './mail-local.mjs';

class NativeTransport {
  onmessage; onerror; onclose;
  active = true;
  after = 0;
  constructor(session) {this.session = session;}
  async request(input) {
    const response = await fetch(this.session.bridge.baseUrl + '/bridge', {method: 'POST', headers: {
      Authorization: 'Bearer ' + this.session.bridge.key, 'Content-Type': 'application/json'}, body: JSON.stringify(input)});
    if (!response.ok) throw Error(await response.text());
    return response.json();
  }
  async start() {
    void (async () => {
      while (this.active) {
        try {
          const events = await this.request({op: 'poll', after: this.after});
          for (const event of events) {this.after = event.seq; if (event.message) this.onmessage?.(event.message);}
        } catch (error) {if (this.active) {this.onerror?.(error); await this.close();} return;}
      }
    })();
  }
  async send(message) {await this.request({op: 'message', message});}
  async close() {if (!this.active) return;this.active = false;this.onclose?.();}
}
let app, transport;
const inputs = [], results = [], contexts = [];
let writes = Promise.resolve();
function emit(value) {
  const line = JSON.stringify(value) + '\n';
  writes = writes.then(async () => {if (!process.stdout.write(line)) await once(process.stdout, 'drain');});
  return writes;
}
async function handle(request) {
  const args = request.params ?? {};
  if (request.method === 'pack-mail') {
    const zip = new JSZip();zip.file('ghost.json', JSON.stringify(localMailManifest));zip.file('main.js', localMailMain);
    await fs.writeFile(args.file, await zip.generateAsync({type: 'nodebuffer'}));return {path: args.file};
  }
  if (request.method === 'init') {
    if (app) throw Error('App already initialized');
    const address = new URL(args.session.bridge.baseUrl);
    if (address.protocol !== 'http:' || address.hostname !== '127.0.0.1' || address.username || address.password || !/^[a-f0-9]{64}$/.test(args.session.bridge.key)) throw Error('Native Host descriptor required');
    app = new App({name: 'Independent Host integration App', version: '1'}, {availableDisplayModes: ['inline', 'fullscreen']}, {autoResize: false});
    app.ontoolinput = value => inputs.push(value);
    app.ontoolresult = value => results.push(value);
    app.onhostcontextchanged = value => contexts.push(value);
    app.onteardown = async () => ({});
    transport = new NativeTransport(args.session);
    await app.connect(transport);
    return {connected: true};
  }
  if (request.method === 'diagnostic') {
    const database = new Database(args.file, {readonly: true});
    try {
      const schema = database.query("SELECT name,sql FROM sqlite_master WHERE type='table'").all();
      const rows = [];
      for (const table of schema) {
        if (!/task|entr|memo|doc/i.test(table.name)) continue;
        const values = database.query('SELECT * FROM "' + table.name.replaceAll('"', '""') + '"').all();
        for (const value of values) {
          const text = JSON.stringify(value, (_, item) => typeof item === 'bigint' ? String(item) : item);
          if (text.includes('pi.generation') || text.includes('winone.onebot.input-') || args.requestId && text.includes(args.requestId)) rows.push({table: table.name, value: JSON.parse(text)});
        }
      }
      return {schema, rows};
    } finally {database.close();}
  }
  if (!app) throw Error('App not initialized');
  switch (request.method) {
    case 'message': return app.sendMessage({role: 'user', content: args.content ?? [{type: 'text', text: args.text}]});
    case 'tool': return app.callServerTool({name: args.name, arguments: args.args ?? {}});
    case 'resource': return app.readServerResource({uri: args.uri});
    case 'state': return {inputs, results, contexts, hostContext: app.getHostContext()};
    case 'close': await app.close(); return {closed: true};
    default: throw Error('Unknown App fixture command');
  }
}
let buffer = '';
process.stdin.setEncoding('utf8');
process.stdin.on('data', chunk => {
  buffer += chunk;
  for (;;) {
    const offset = buffer.indexOf('\n'); if (offset < 0) break;
    const line = buffer.slice(0, offset); buffer = buffer.slice(offset + 1);
    let request;try {request = JSON.parse(line);} catch {process.exit(2);}
    void handle(request).then(value => emit({type: 'response', id: request.id, ok: true, value}), error => emit({type: 'response', id: request.id, ok: false, error: error.message}));
  }
});
process.stdin.once('end', async () => {await app?.close(); await writes;process.exit(0);});
void emit({type: 'ready', protocol: 1});
