// Sanitized provider fixture for the actual Cindy mail SDK. It owns a local
// Library mailbox; the SDK's KV contains routing/receipts, never copied bodies.
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {createHash, randomUUID} from 'node:crypto';
import JSZip from 'jszip';
import {createGhostHost} from '../host.mjs';

export const MAIL_PNG = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/l1sAAAAASUVORK5CYII=', 'base64');
export const MAIL_SOURCE = '<style>.signature{color:#234567}td{padding:4px}</style><div style="background:white"><p>Hello <b>one two</b> <a href="https://example.invalid/reference">reference</a></p><table class="signature"><tr><td>Signed</td><td><img src="cid:original@fixture" alt="Inline original"></td></tr></table></div>';
export const mailBackendManifest = {
  schemaVersion: 3, minCindyVersion: '0.1.122-beta', id: 'mail-backend-fixture', name: 'Private local mailbox', version: '1.0.0',
  description: 'Deterministic local mail provider, no external mailbox', entry: 'main.js', mail: true, library: true, agent: {},
  cindy: {media: ['deposit']}, node: {entry: 'files.cjs', protocol: 'json-rpc-stdio'},
  mailReader: {tool: 'mail_read', hosts: ['mail.example.invalid'], markRead: true},
  tools: ['prime', 'get', 'send', 'update', 'delete', 'mutate', 'stats', 'mode', 'settle', 'mail_read', 'routing', 'probe'].map(name => ({name, description: name, parameters: {type: 'object', properties: {}}})),
};

export const mailBackendMain = `
const mailboxPath='mailbox.json', waiters=[], selectedAccounts=[];
const account={ok:true,accountId:'fixture-account',accountEmail:'sender@example.invalid'};
async function load(){const r=await cindy.library({op:'read',path:mailboxPath});if(r.ok)return JSON.parse(r.content);if(r.errorCode==='NOT_FOUND')return null;throw Error('Mailbox read failed: '+r.errorCode);}
async function store(value){const r=await cindy.library({op:'write',path:mailboxPath,content:JSON.stringify(value)});if(!r.ok)throw Error('Mailbox did not commit');}
async function seeded(){let value=await load();if(!value){
 const media=await cindy.send({type:'cindy-request',kind:'deposit_media',data:${JSON.stringify(MAIL_PNG.toString('base64'))}});if(!media.ok)throw Error('Owned inline image was not deposited');
 value={version:1,headers:{from:'Sender <sender@example.invalid>',to:'"Recipient, One" <one@example.invalid>',cc:'Copy <copy@example.invalid>',bcc:'',subject:'Private rich draft'},body:'Hello one two reference\\n\\nSigned',quote:'Earlier message',bodyHtml:${JSON.stringify(MAIL_SOURCE)},quoteHtml:'<blockquote><p>Earlier <i>message</i></p></blockquote>',attachments:[{key:'original',filename:'inline.png',bytes:${MAIL_PNG.length},contentId:'<original@fixture>',mimeType:'image/png',previewUrl:media.url}],writes:0,sends:0,deletes:0,attempts:0,markReads:0,reads:[],selected:[],history:[],writeMode:'normal'};
 await store(value);
}return value;}
function snapshot(value){return {status:'read_back',draftId:'fixture-draft',fingerprint:'v'+value.version,locator:{version:value.version},headers:{...value.headers},editor:{editable:true,format:'html',body:value.body,quote:value.quote,bodyHtml:value.bodyHtml,quoteHtml:value.quoteHtml},attachments:value.attachments,images:value.attachments.filter(a=>a.contentId&&a.previewUrl).map(a=>({cid:a.contentId.replace(/^<|>$/g,''),url:a.previewUrl}))};}
const sdk=cindyMail.register({providerName:'Fixture mailbox',draftStorage:'local',richText:true,
 selectAccount:async selected=>{selectedAccounts.push(selected||'default');return !selected||selected===account.accountId?account:{ok:false,message:'Unknown fixture account'};},
 readDraft:async()=>{const value=await seeded();return value.deleted?{ok:false,execution:'not_executed',message:'Draft deleted'}:{ok:true,data:{readBack:snapshot(value)}};},
 writeDraft:async input=>{
  let value=await seeded();value.attempts++;
  if(input.record.fingerprint!=='v'+value.version){await store(value);return {ok:false,execution:'not_executed',stale:snapshot(value),message:'Provider confirmed no stale execution'};}
  const additions=[];
  for(const attachment of input.attachments||[]){
   const inspected=await cindy.node.request({method:'inspect',params:{path:attachment.path}});
   if(!inspected.ok||inspected.result.bytes!==attachment.bytes||inspected.result.sha256!==attachment.sha256)return {ok:false,execution:'not_executed',message:'Snapshot bytes did not match'};
   let previewUrl;const mimeType=inspected.result.mimeType;
   if(mimeType==='image/png'){const media=await cindy.send({type:'cindy-request',kind:'deposit_media',data:inspected.result.data});if(!media.ok)throw Error('Added image was not deposited');previewUrl=media.url;}
   additions.push({key:'added-'+(value.writes+1)+'-'+additions.length,filename:attachment.filename,bytes:attachment.bytes,sha256:attachment.sha256,path:attachment.path,mimeType,...(attachment.contentId?{contentId:attachment.contentId}:{}),...(previewUrl?{previewUrl}:{})});
  }
  value=await seeded();
  if(input.record.fingerprint!=='v'+value.version)return {ok:false,execution:'not_executed',stale:snapshot(value),message:'Provider confirmed no stale execution'};
  for(const name of ['to','cc','bcc','subject'])value.headers[name]=input.fields[name];
  for(const name of ['body','quote','bodyHtml','quoteHtml'])value[name]=input.fields[name]||'';
  const items=JSON.parse(input.fields.attachmentItems||'[]');value.attachments=[...items.map(item=>({...value.attachments.find(a=>a.key===item.key),...item})),...additions];
  value.version++;value.writes++;if(input.send)value.sends++;
  const mode=value.writeMode;value.history.push({at:Date.now(),send:input.send===true,fields:{...input.fields},additions:additions.map(a=>({filename:a.filename,path:a.path,sha256:a.sha256,bytes:a.bytes,contentId:a.contentId})),mode});await store(value);
  const receipt={id:'fixture-sent-'+value.sends,subject:value.headers.subject,bodyHtml:value.bodyHtml};
  if(input.send&&mode==='late')await new Promise(resolve=>waiters.push(resolve));
  if(mode==='unknown')return {ok:false,execution:'unknown',message:'Provider committed; receipt was lost, inspect mailbox before further work'};
  if(mode==='incomplete'&&input.send)return {ok:true,data:{sent:true}};
  return input.send?{ok:true,data:{sent:true,receipt}}:{ok:true,data:{readBack:snapshot(await seeded())}};
 },
 deleteDraft:async()=>{const value=await seeded();value.deleted=true;value.deletes++;await store(value);return {ok:true,data:{deleted:true}};}
});
cindy.onHostMessage(async message=>{
 if(message.type!=='tool-call')return;
 const reply=result=>cindy.send({type:'tool-result',callId:message.callId,ok:true,result});
 try{
  if(message.tool==='prime'){const value=await seeded();return reply({presented:await sdk.present(message,account,snapshot(value))});}
  if(message.tool==='stats')return reply({...await seeded(),selected:selectedAccounts.slice()});
  if(message.tool==='routing'){const r=await fetch('/kv');return reply(await r.json());}
  if(message.tool==='probe')return reply(await cindy.send(message.args.payload));
  if(message.tool==='mode'){const value=await seeded();value.writeMode=message.args.mode;await store(value);return reply({mode:value.writeMode});}
  if(message.tool==='settle'){for(const done of waiters.splice(0))done();return reply({released:true});}
  if(message.tool==='mutate'){const value=await seeded();for(const name of ['body','quote','bodyHtml','quoteHtml'])if(message.args[name]!==undefined)value[name]=message.args[name];if(message.args.subject!==undefined)value.headers.subject=message.args.subject;value.version++;await store(value);return reply({version:value.version});}
  if(['get','send','update','delete'].includes(message.tool))return reply(await sdk.beforeTool({operation:message.tool,account:account.accountId,draftId:'fixture-draft',sourceCallId:message.callId,readOnly:message.args.readOnly===true}));
  if(message.tool==='mail_read'){
   const value=await seeded(),target=new URL(message.args.url),messageId=target.pathname.slice(1)||'selected';
   value.reads.push({messageId,markRead:message.args.markRead===true,openDraft:message.args.openDraft===true});if(message.args.markRead)value.markReads++;await store(value);
   if(message.args.openDraft&&messageId==='draft'&&!value.deleted)await sdk.present(message,account,snapshot(value));
   const large=messageId==='large',bodyHtml=large?'<div><p>'+('Long reader body '.repeat(24000))+'</p></div>':value.bodyHtml,bodyText=large?'Long reader body '.repeat(24000):value.body;
   return reply({accountId:account.accountId,...(messageId==='draft'?{draftId:'fixture-draft'}:{}),messageId,from:value.headers.from,to:value.headers.to,cc:value.headers.cc,bcc:value.headers.bcc,subject:value.headers.subject,date:'2026-11-06T12:00:00Z',bodyText,bodyHtml,attachments:value.attachments,
    ...(!large&&messageId!=='draft'?{conversation:{threadId:'fixture-thread',messages:Array.from({length:8},(_,i)=>({messageId:i===4?'selected':'older-'+i,url:'https://mail.example.invalid/'+(i===4?'selected':'older-'+i),from:value.headers.from,date:'2026-11-06T'+String(i+1).padStart(2,'0')+':00:00Z',subject:value.headers.subject,snippet:'Message '+i}))}}:{})});
  }
  throw Error('Unknown fixture tool');
 }catch(error){await cindy.send({type:'tool-result',callId:message.callId,ok:false,errorCode:'FIXTURE_FAILED',message:error.message});}
});`;

export const mailBackendFiles = `
const fs=require('node:fs/promises'),path=require('node:path'),crypto=require('node:crypto'),readline=require('node:readline');
readline.createInterface({input:process.stdin}).on('line',async line=>{const m=JSON.parse(line);try{
 if(m.method!=='inspect'||!path.isAbsolute(m.params.path)||!m.params.path.replace(/\\\\/g,'/').includes('/_temp/'))throw Error('Only owned fixture snapshots');
 const bytes=await fs.readFile(m.params.path),png=bytes.subarray(0,8).equals(Buffer.from([137,80,78,71,13,10,26,10]));
 process.stdout.write(JSON.stringify({jsonrpc:'2.0',id:m.id,result:{bytes:bytes.length,sha256:crypto.createHash('sha256').update(bytes).digest('hex'),mimeType:png?'image/png':'text/plain',data:bytes.toString('base64')}})+'\\n');
}catch(error){process.stdout.write(JSON.stringify({jsonrpc:'2.0',id:m.id,error:{code:-32000,message:error.message}})+'\\n');}});`;

const repository = fileURLToPath(new URL('../../../../', import.meta.url));
export async function createLocalMailBackend(directory) {
  if (!path.isAbsolute(directory) || !directory.startsWith(path.join(repository, '_temp') + path.sep)) throw Error('Mail fixture requires repository-private _temp directory');
  await fs.mkdir(directory, {recursive: true});
  const events = [], continuations = [], snapshots = [], selections = [];
  const options = {directory, executable: process.env.WINONE_GHOST_SANDBOX, sdkFile: process.env.WINONE_GHOST_SDK,
    mailSDKFile: fileURLToPath(new URL('../../cindy/ghost/apps/desktop/src/main/cindy-brain/runtime/mailComposeRuntime.js', import.meta.url)),
    snapshotWorker: process.env.WINONE_GHOST_SNAPSHOT, runtimeExecutable: process.execPath,
    nodeWorker: fileURLToPath(new URL('../node-worker.mjs', import.meta.url)), masterKey: Buffer.alloc(32, 42).toString('base64'),
    event: value => events.push(value), host: async (method, input) => {
      if (method === 'session-snapshot') return input.sessionId === 'mail-task' ? {workingDir: directory, remoteHostId: null, archived: false, sharedTask: false, permissionMode: 'bypassPermissions', mcpAppEpoch: 0} : null;
      if (method === 'library-read-root') return 'not-granted';
      if (method === 'models') return [];
      if (method === 'media-config') return {};
      if (method === 'task' && input.op === 'agent-turn') {
        continuations.push(input.request);
        return {ok: false, errorCode: 'HOST_NOT_READY', message: 'No model runner in this local provider test'};
      }
      if (method === 'ui' && input.kind === 'pick-files') return {paths: selections.length ? [selections.shift()] : []};
      if (method === 'mail-attachment-snapshot') {
        const source = await fs.realpath(input.path), staging = await fs.realpath(path.join(directory, 'mail-staging'));
        if (input.sessionId !== 'mail-task' || !source.startsWith(staging + path.sep)) throw Error('Snapshot is not this Host mail staging');
        const bytes = await fs.readFile(source), target = path.join(directory, 'session-attachments', randomUUID());
        await fs.mkdir(path.dirname(target), {recursive: true}); await fs.writeFile(target, bytes, {flag: 'wx'});
        snapshots.push({source, path: target, sha256: createHash('sha256').update(bytes).digest('hex'), bytes: bytes.length});
        return {path: target};
      }
      throw Error('Unhandled mail backend boundary: ' + method + (input?.op ? '/' + input.op : ''));
    }};
  let host = createGhostHost(options);
  const archive = new JSZip(); archive.file('ghost.json', JSON.stringify(mailBackendManifest)); archive.file('main.js', mailBackendMain); archive.file('files.cjs', mailBackendFiles);
  const file = path.join(directory, 'mail-fixture.cindy'); await fs.writeFile(file, await archive.generateAsync({type: 'nodebuffer'}));
  const inspection = await host.call('inspect', {path: file}); const installed = await host.call('install', {token: inspection.token, confirmed: true});
  const session = {id: 'mail-task', workingDir: directory, approved: false};
  const call = (tool, args = {}) => host.call('agent-call', {ghostId: mailBackendManifest.id, namespace: null, tool, args, session, language: 'en'});
  const primed = await call('prime');
  if (!primed.ok || !primed.result.presented || !primed.result.xdt_card_id) throw Error('Actual SDK did not publish its Host mail card: ' + JSON.stringify(primed));
  const firstCallId = primed.result.xdt_card_id;
  return {directory, installed, events, continuations, snapshots, selections, firstCallId, get host() {return host;}, call,
    async reopen() {await host.close();host = createGhostHost(options);}, close: () => host.close(),
    async card(callId = firstCallId) {return host.call('card-get', {callId});},
    async action(name, values, callId = firstCallId) {
      const card = await host.call('card-get', {callId});
      return host.call('card-action', {callId, action: 'mail:' + name + ':' + card.mail.key, ...(values ? {formValues: values} : {}), ...(name === 'change' ? {change: true} : {})});
    },
  };
}
