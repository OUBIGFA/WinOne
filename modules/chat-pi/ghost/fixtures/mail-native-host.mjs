// Explicit private-desktop native fixture. The actual installed provider,
// official SDK and original MailCardService/Reader execute in createGhostHost.
// This server is only the native transport/preview boundary; it never invents
// a provider receipt or a successful mail operation.
import fs from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import {once} from 'node:events';
import {randomBytes} from 'node:crypto';
import {fileURLToPath} from 'node:url';
import {createLocalMailBackend, MAIL_PNG, MAIL_SOURCE} from './mail-backend.mjs';

const repository = fileURLToPath(new URL('../../../../', import.meta.url));
const directory = path.resolve(process.argv[2] ?? '');
if (process.env.WINONE_MAIL_NATIVE !== '1' || !directory.startsWith(path.join(repository, '_temp') + path.sep)) throw Error('Use the explicit repository-private mail native profile');
const fixture = await createLocalMailBackend(directory), key = randomBytes(32).toString('hex');
const calls = [], media = new Map(); let externalRequests = 0;
const server = http.createServer(async (req, res) => {
  try {
    if (req.url === '/external.png') {
      externalRequests++;res.writeHead(200, {'Content-Type': 'image/png', 'Cache-Control': 'no-store'});res.end(MAIL_PNG);return;
    }
    if (req.url.startsWith('/media/' + key + '/')) {
      const file = media.get(req.url);if (!file) {res.writeHead(404);res.end();return;}
      res.writeHead(200, {'Content-Type': 'image/png', 'Cache-Control': 'no-store'});res.end(await fs.readFile(file));return;
    }
    if (req.headers.authorization !== 'Bearer ' + key) {res.writeHead(403);res.end();return;}
    let result;
    if (req.method === 'POST' && req.url === '/call') {
      const chunks = [];for await (const chunk of req) chunks.push(chunk);
      const request = JSON.parse(Buffer.concat(chunks).toString()); const method = request.method.replace(/^chat\.ghosts\./, ''), params = request.params ?? {};
      calls.push({method, params});
      if (method === 'pending') {
        const events = fixture.events.map((event, index) => ({event, sequence: index + 1})).filter(item => item.sequence > (params.after ?? 0) && item.event.type === 'push');
        result = {cursor: fixture.events.length, prompts: [], events: events.map(({event, sequence}) => ({sequence, channel: event.channel, payload: event.payload}))};
      } else if (method === 'media-preview') {
        const {path: file} = await fixture.host.call('media-path', params);
        if (!(await fs.realpath(file)).startsWith(directory + path.sep)) throw Error('Preview left the actual fixture media store');
        const route = '/media/' + key + '/' + media.size;media.set(route, file);
        result = {kind: 'image', url: 'http://127.0.0.1:' + server.address().port + route, name: path.basename(file), mimeType: 'image/png', size: (await fs.stat(file)).size};
      } else result = await fixture.host.call(method, params);
    } else if (req.url === '/state') {
      const state = await fixture.call('stats');if (!state.ok) throw Error('Actual provider stats failed: ' + JSON.stringify(state));
      result = {provider: state.result, calls, externalRequests};
    } else if (req.url === '/selection') {
      const file = path.join(directory, 'native-selected.png');await fs.writeFile(file, MAIL_PNG);fixture.selections.push(file);result = {};
    } else if (req.url === '/late') {result = await fixture.call('mode', {mode: 'late'});}
    else if (req.url === '/settle') {result = await fixture.call('settle');}
    else {res.writeHead(404);res.end();return;}
    res.writeHead(200, {'Content-Type': 'application/json'});res.end(JSON.stringify(result));
  } catch (error) {res.writeHead(500, {'Content-Type': 'application/json'});res.end(JSON.stringify({error: error.message}));}
}).listen(0, '127.0.0.1');
await once(server, 'listening');
const externalURL = 'http://127.0.0.1:' + server.address().port + '/external.png';
const source = MAIL_SOURCE + '<img src="' + externalURL + '" alt="External fixture image"><script>globalThis.mailAuthoredScriptRan=true</script><a href="javascript:globalThis.mailAuthoredLinkRan=true">unsafe link</a><p onclick="globalThis.mailAuthoredEventRan=true">Inert authored event</p>';
await fixture.call('mutate', {bodyHtml: source});const primed = await fixture.call('prime');
if (!primed.ok || !primed.result.presented || !primed.result.xdt_card_id) throw Error('Actual provider did not present native draft');
process.stdout.write(JSON.stringify({baseURL: 'http://127.0.0.1:' + server.address().port, key, sessionID: 'mail-task', callID: primed.result.xdt_card_id, source}) + '\n');
process.stdin.resume();
process.stdin.once('end', async () => {
  await fixture.close();server.closeIdleConnections();await new Promise(resolve => server.close(resolve));process.exit(0);
});
