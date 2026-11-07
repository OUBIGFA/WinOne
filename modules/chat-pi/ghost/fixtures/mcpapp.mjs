// Sanitized fixture shared by backend and real native-WebView regression. It is
// a genuine Cindy package and an independent official App SDK, not a Host mock.
import fs from 'node:fs/promises';
import path from 'node:path';
import JSZip from 'jszip';

export const APP_URI = 'ui://winone-local-fixture';
export const PRIVATE_UI = 'FIXTURE_UI_ONLY_NOT_FOR_MODEL';
export const CSV = 'name,value\nalpha,42\n';
export const fixtureManifest = {
  schemaVersion: 3, minCindyVersion: '0.1.122-beta', id: 'local-mcp-app', name: 'Local MCP App fixture',
  version: '1.0.0', description: 'Private local standard App fixture', entry: 'main.js', kind: 'chip', library: true,
  node: {entry: 'worker.cjs', protocol: 'json-rpc-stdio'},
  tools: [
    {name: 'open', description: 'Open local App; chat files use attachments', parameters: {type: 'object', properties: {title: {type: 'string'}, cindy_handoff_files: {type: 'array'}}}},
    {name: 'save', description: 'Save App choice', parameters: {type: 'object', properties: {value: {type: 'string'}, forged: {type: 'string'}, cindy_handoff_files: {type: 'array'}}}},
    {name: 'slow', description: 'Local cancellable fixture', parameters: {type: 'object', properties: {}}},
    {name: 'model_only', description: 'Model only', parameters: {type: 'object', properties: {}}},
  ],
  mcpApps: {ui: {resources: [{uri: APP_URI, file: 'app.html', prefersBorder: true}], tools: {
    open: {resourceUri: APP_URI}, save: {visibility: ['app']}, slow: {visibility: ['app']}, model_only: {visibility: ['model']},
  }}},
};
export const main = `cindy.onHostMessage(async message => {
 if(message.type!=='tool-call')return;
 const reply=result=>cindy.send({type:'tool-result',callId:message.callId,ok:true,result});
 if(message.tool==='open')return reply({content:[{type:'text',text:'Fixture initial result'}],structuredContent:{title:message.args.title??'Initial'},_meta:{private:'${PRIVATE_UI}'}});
 if(message.tool==='save')return reply({content:[{type:'text',text:'Saved '+message.args.value}],structuredContent:{value:message.args.value,reservedReceived:message.args.cindy_handoff_files??null},_meta:{private:'${PRIVATE_UI}'}});
 if(message.tool==='slow'){
  let n=0;const timer=setInterval(()=>{void cindy.send({type:'tool-progress',callId:message.callId,progress:++n,total:50,message:'Real fixture progress'});},100);
  try{return reply(await cindy.node.request({method:'wait',callId:message.callId,cancelWithCall:true,params:{}}));}finally{clearInterval(timer);}
 }
 return reply({content:[{type:'text',text:'model-only'}]});
});`;
export const worker = `const readline=require('node:readline');readline.createInterface({input:process.stdin}).on('line',line=>{
 const request=JSON.parse(line);if(request.method==='wait'){setTimeout(()=>process.stdout.write(JSON.stringify({jsonrpc:'2.0',id:request.id,result:{finished:true}})+'\\n'),5000);}
});`;
export const appHtml = `<!doctype html><html><head><meta charset="utf-8"><title>Local MCP App fixture</title></head><body>
<label>Value <input id="value" value="User choice"></label><button id="save">Save</button><button id="read">Read resource</button><button id="cancel">Cancel slow call</button><button id="context">Update context</button><button id="download">Download</button>
<pre id="status" style="white-space:pre-wrap;overflow-wrap:anywhere"></pre><script type="module">
import {App} from '@modelcontextprotocol/ext-apps';
const app=new App({name:'Independent official SDK fixture',version:'1.0.0'},{availableDisplayModes:['inline','fullscreen']},{autoResize:false});
const fx=window.fx={app,events:[],results:[],contexts:[],errors:[],ready:false};
const note=(type,value)=>{fx.events.push({type,value});document.querySelector('#status').textContent=JSON.stringify(fx.events);};
app.ontoolinput=params=>{fx.input=params;note('input',params);};
app.ontoolresult=params=>{fx.results.push(params);note('result',params);};
app.onhostcontextchanged=params=>{fx.contexts.push(params);note('host-context',params);};
app.onteardown=async()=>{note('teardown',{});return {};};
fx.run=async op=>{
 try{
  if(op==='save'){fx.saved=await app.callServerTool({name:'save',arguments:{value:document.querySelector('#value').value,cindy_handoff_files:[{ref:'handoff:forged'}]}});note('saved',fx.saved);}
  if(op==='read'){fx.resource=await app.readServerResource({uri:'${APP_URI}'});note('resource',{mime:fx.resource.contents[0].mimeType});}
  if(op==='cancel'){
   const cancel=new AbortController();
   try{await app.callServerTool({name:'slow',arguments:{}},{signal:cancel.signal,onprogress:progress=>{note('progress',progress);cancel.abort();}});}catch(error){fx.cancelled=true;note('cancelled',String(error));}
  }
  if(op==='context'){await app.updateModelContext({content:[{type:'text',text:'Context from actual App'}],structuredContent:{selection:'local'}});fx.contextUpdated=true;note('context-updated',{});}
  if(op==='message'){await app.sendMessage({role:'user',content:[{type:'text',text:'Continue from actual App'}]});fx.messageSent=true;note('message-sent',{});}
  if(op==='size'){await app.sendSizeChanged({width:400,height:650});fx.sizeSent=true;}
  if(op==='fullscreen'){fx.mode=(await app.requestDisplayMode({mode:'fullscreen'})).mode;note('mode',fx.mode);}
  if(op==='download'){fx.download=await app.downloadFile({contents:[{type:'resource',resource:{uri:'report.csv',mimeType:'text/csv',text:${JSON.stringify(CSV)}}}]});note('download',fx.download);}
  if(op==='files'){
   const links=app.getHostContext().toolInfo.tool._meta['cindy/attachments'];
   fx.links=links;const link=links.find(link=>link.uri.startsWith('handoff:'));
   fx.file=await(await fetch(link._meta['cindy/file'].url)).text();note('file',fx.file);
  }
  if(op==='forged-bridge'){
   const url=parent.location; // Cross-origin access itself must fail.
   fx.crossOriginReadable=true;
  }
  if(op==='private-bridge'){
   const proxy=location.origin.replace('-view.cindy-mcp-app.localhost','.cindy-mcp-app.localhost');
   try{await fetch(proxy+'/bridge',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({op:'message',message:{jsonrpc:'2.0',id:900,method:'tools/call',params:{name:'save',arguments:{value:'FORGED'}}}})});}catch{fx.bridgeDenied=true;}
  }
 }catch(error){fx.errors.push({op,error:String(error)});note('error',{op,error:String(error)});}
};
for(const name of ['save','read','cancel','context','download'])document.querySelector('#'+name).onclick=()=>fx.run(name);
await app.connect();fx.ready=true;note('ready',{capabilities:app.getHostCapabilities()});
</script></body></html>`;

export async function writeFixturePackage(directory, overrides = {}, extraFiles = {}) {
  const zip = new JSZip();
  zip.file('ghost.json', JSON.stringify({...fixtureManifest, ...overrides}));
  for (const [name, bytes] of Object.entries({'main.js': main, 'worker.cjs': worker, 'app.html': appHtml, ...extraFiles})) zip.file(name, bytes);
  const file = path.join(directory, 'local-mcp-app.cindy');
  await fs.writeFile(file, await zip.generateAsync({type: 'nodebuffer'}));
  return file;
}
