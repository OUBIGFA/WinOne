// Sanitized local mail provider. It uses the real Host-owned Cindy mail SDK;
// draft bodies are in its own Library file, never the SDK's routing KV.
export const localMailManifest = {
  schemaVersion: 3, minCindyVersion: '0.1.122-beta', id: 'local-mail-fixture', name: 'Local mailbox fixture', version: '1.0.0',
  description: 'Private local mail SDK fixture', entry: 'main.js', kind: 'chip', mail: true, library: true,
  mailReader: {tool: 'message_read', hosts: ['mail.example.invalid'], markRead: true},
  tools: ['prime', 'get', 'send', 'mutate', 'stats', 'message_read'].map(name => ({name, description: name, parameters: {type: 'object', properties: {}}})),
};
export const localMailMain = `
const path='mailbox.json';
async function load(){const result=await cindy.library({op:'read',path});return result.ok?JSON.parse(result.content):null;}
async function store(value){const result=await cindy.library({op:'write',path,content:JSON.stringify(value)});if(!result.ok)throw Error('Local mailbox write did not commit');}
async function seeded(){let value=await load();if(!value){value={version:1,headers:{from:'fixture@example.invalid',to:'recipient@example.invalid',cc:'',bcc:'',subject:'Local draft'},body:'one two',quote:'',writes:0,sends:0,markReads:0,history:[]};await store(value);}return value;}
function snapshot(value){return {status:'read_back',draftId:'draft-local',fingerprint:'v'+value.version,locator:{version:value.version},headers:{...value.headers},editor:{editable:true,format:'text',body:value.body,quote:value.quote},attachments:[]};}
const account={ok:true,accountId:'local-account',accountEmail:'fixture@example.invalid'};
const sdk=cindyMail.register({providerName:'Local fixture mailbox',draftStorage:'local',
 selectAccount:async()=>account,
 readDraft:async()=>({ok:true,data:{readBack:snapshot(await seeded())}}),
 writeDraft:async input=>navigator.locks.request('local-mailbox-write',async()=>{
  const value=await seeded();if(input.record.fingerprint!=='v'+value.version)return {ok:false,execution:'not_executed',stale:snapshot(value),message:'Local provider confirmed no stale write'};
  for(const key of ['to','cc','bcc','subject'])value.headers[key]=input.fields[key];value.body=input.fields.body;value.quote=input.fields.quote||'';value.version++;value.writes++;value.history.push({send:input.send===true,at:Date.now(),body:value.body});
  if(input.send){value.sends++;await store(value);return {ok:true,data:{sent:true,receipt:{id:'sent-'+value.sends,body:value.body,subject:value.headers.subject}}};}
  await store(value);return {ok:true,data:{readBack:snapshot(value)}};
 }),
 deleteDraft:async()=>{const value=await seeded();value.deleted=true;await store(value);return {ok:true,data:{deleted:true}};}
});
cindy.onHostMessage(async message=>{
 if(message.type!=='tool-call')return;
 const reply=result=>cindy.send({type:'tool-result',callId:message.callId,ok:true,result});
 if(message.tool==='prime'){const value=await seeded();const presented=await sdk.present(message,account,snapshot(value));return reply({ok:presented,draftId:'draft-local'});}
 if(message.tool==='stats')return reply(await seeded());
 if(message.tool==='mutate'){const value=await seeded();if(message.args.body!==undefined)value.body=message.args.body;if(message.args.subject!==undefined)value.headers.subject=message.args.subject;value.version++;await store(value);return reply(value);}
 if(message.tool==='get'||message.tool==='send')return reply(await sdk.beforeTool({operation:message.tool,account:'local-account',draftId:'draft-local',sourceCallId:message.callId,readOnly:false}));
 if(message.tool==='message_read'){
  const value=await seeded();if(message.args.markRead){value.markReads++;await store(value);}if(message.args.openDraft)await sdk.present(message,account,snapshot(value));
  return reply({accountId:'local-account',draftId:'draft-local',messageId:'message-local',from:value.headers.from,to:value.headers.to,cc:'',bcc:'',subject:value.headers.subject,date:'2026-11-06',bodyText:value.body,bodyHtml:'',attachments:[]});
 }
});`;
