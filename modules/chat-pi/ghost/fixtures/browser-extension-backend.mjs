// Actual installed main.js and official Cindy SDK are the browser-side test
// actor. Only OS Registry commands are replaced; bridge, pairing and relay are
// the pinned production slot/native-binding implementations.
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import JSZip from 'jszip';
import {createGhostHost} from '../host.mjs';
import {BrowserExtensionBridge} from '../../cindy/ghost/apps/desktop/src/main/browser-extension/bridge.ts';
import {isolatedRegistry, LocalBrowserClient} from './browser-local.mjs';

export const browserBackendManifest = {
  schemaVersion: 3, minCindyVersion: '0.1.122-beta', id: 'browser-backend-fixture', name: 'Private extension bridge', version: '1.0.0',
  description: 'Local SDK pairing and relay fixture', entry: 'main.js', library: true,
  node: {entry: 'worker.cjs', protocol: 'json-rpc-stdio'},
  browserExtension: {protocol: 'cindy-browser-extension/3', extensionIds: ['a'.repeat(32)], pageControl: true, nativeHost: {name: 'com.winone.fixture.backend', binary: 'native'}},
  tools: ['browser', 'pair', 'stats'].map(name => ({name, description: name, parameters: {type: 'object', properties: {}}})),
};
export const browserBackendMain = `
const statePath='extension.json';
async function state(){const r=await cindy.library({op:'read',path:statePath});return r.ok?JSON.parse(r.content):{pairRequests:0,relays:[],clients:[]};}
async function change(update){return navigator.locks.request('browser-fixture-state',async()=>{const value=await state();const result=await update(value);const r=await cindy.library({op:'write',path:statePath,content:JSON.stringify(value)});if(!r.ok)throw Error('Fixture state did not commit');return result;});}
const request=input=>cindy.send({...input,type:'browser-extension-request'});
cindy.onHostMessage(async message=>{
 if(message.type==='event'&&message.name==='browser-extension-policy-request'){
  const clients=await change(value=>{value.pairRequests++;return value.clients;});if(clients.length)await request({kind:'setTrustedClients',clients});return;
 }
 if(message.type==='event'&&message.name==='browser-extension-relay'){
  await change(value=>value.relays.push({id:message.id,clientId:message.clientId,request:message.request}));
  await request({kind:'relayProgress',id:message.id,progress:{stage:'generating',receivedChars:message.request.text.length}});
  // Real deterministic provider work. No AI model or browser execution is
  // faked as accepted: this fixture's translation is explicitly local text.
  const result={ok:true,text:message.request.text.toUpperCase()};
  await request({kind:'relayResult',id:message.id,result});return;
 }
 if(message.type!=='tool-call')return;
 const reply=result=>cindy.send({type:'tool-result',callId:message.callId,ok:true,result});
 try{
  if(message.tool==='stats')return reply(await state());
  if(message.tool==='pair'){const clients=message.args.clients;const applied=await request({kind:'setTrustedClients',clients});if(applied.ok)await change(value=>value.clients=clients);return reply(applied);}
  if(message.tool==='browser')return reply(await request(message.args));
  throw Error('Unknown fixture tool');
 }catch(error){await cindy.send({type:'tool-result',callId:message.callId,ok:false,errorCode:'FIXTURE_FAILED',message:error.message});}
});`;

const repository = fileURLToPath(new URL('../../../../', import.meta.url));
export async function createLocalBrowserBackend(directory) {
  if (!path.isAbsolute(directory) || !directory.startsWith(path.join(repository, '_temp') + path.sep)) throw Error('Browser fixture needs repository-private _temp');
  await fs.mkdir(directory, {recursive: true});
  const registry = isolatedRegistry(), bridges = [], events = [];
  const nativeOptions = {platform: 'win32', arch: 'x64', home: path.join(directory, 'home'), profiles: [path.join(directory, 'profiles', 'chrome')], env: {SystemRoot: path.join(directory, 'fake-windows')}, run: registry.run};
  const options = {directory, executable: process.env.WINONE_GHOST_SANDBOX, sdkFile: process.env.WINONE_GHOST_SDK,
    mailSDKFile: fileURLToPath(new URL('../../cindy/ghost/apps/desktop/src/main/cindy-brain/runtime/mailComposeRuntime.js', import.meta.url)),
    snapshotWorker: process.env.WINONE_GHOST_SNAPSHOT, runtimeExecutable: process.execPath,
    nodeWorker: fileURLToPath(new URL('../node-worker.mjs', import.meta.url)), masterKey: Buffer.alloc(32, 43).toString('base64'),
    browserExtensionOptions: {nativeOptions, createBridge: options => {const bridge = new BrowserExtensionBridge({...options, ports: [0], pollTimeout: 20, connectWait: 0, jobTimeout: 1500}); bridges.push(bridge); return bridge;}},
    event: event => events.push(event), host: async (method, input) => {
      if (method === 'session-snapshot') return input.sessionId === 'browser-task' ? {workingDir: directory, remoteHostId: null, archived: false, permissionMode: 'bypassPermissions', mcpAppEpoch: 0} : null;
      if (method === 'library-read-root') return 'not-granted';
      throw Error('Unhandled browser fixture boundary: ' + method);
    }};
  let host = createGhostHost(options);
  const archive = new JSZip(); archive.file('ghost.json', JSON.stringify(browserBackendManifest)); archive.file('main.js', browserBackendMain);
  archive.file('worker.cjs', "process.stdin.resume();");
  archive.file('native/win32-x64/cindy-browser-native.exe', 'SANITIZED FIXTURE BINARY; NEVER EXECUTED');
  const file = path.join(directory, 'extension.cindy'); await fs.writeFile(file, await archive.generateAsync({type: 'nodebuffer'}));
  const inspection = await host.call('inspect', {path: file}); const installed = await host.call('install', {token: inspection.token, confirmed: true});
  const session = {id: 'browser-task', workingDir: directory, approved: false};
  const call = (tool, args = {}) => host.call('agent-call', {ghostId: browserBackendManifest.id, namespace: null, tool, args, session, language: 'en'});
  const initial = await call('browser', {kind: 'status'});
  if (!initial.ok || !initial.result.ok || !bridges.length || initial.result.nativeBinding !== 'ready') {
    const diagnostics = events.filter(event => ['state', 'diagnostic'].includes(event.type));
    await host.close();
    throw Error('Actual SDK/original native bridge not ready: ' + JSON.stringify({initial, diagnostics}));
  }
  const token = async port => {
    const registration = JSON.parse(await fs.readFile(path.join(directory, 'browser-native', browserBackendManifest.browserExtension.nativeHost.name + '.json'), 'utf8'));
    if (!registration.path.startsWith(path.join(directory, 'browser-native') + path.sep)) throw Error('Native registration escaped the owned fixture');
    return JSON.parse(await fs.readFile(path.join(path.dirname(registration.path), 'connections', port + '.json'), 'utf8')).token;
  };
  return {directory, registry, bridges, events, installed, call, token, initial: initial.result, get host() {return host;},
    async browser(input) {const result = await call('browser', input);if (!result.ok) throw Error(JSON.stringify(result));return result.result;},
    async stats() {const result = await call('stats');if (!result.ok) throw Error(JSON.stringify(result));return result.result;},
    async client(previous) {
      const status = (await call('browser', {kind: 'status'})).result;
      const client = new LocalBrowserClient(status.port);
      if (previous) for (const key of ['id', 'privateKey', 'publicKey', 'publicKeyText', 'encodedKey', 'headers']) client[key] = previous[key];
      const response = await client.connect(previous ? undefined : await token(status.port));
      return {client, response};
    },
    async reopen() {await host.close();host = createGhostHost(options);},
    close: () => host.close(),
  };
}
