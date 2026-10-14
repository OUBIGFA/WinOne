import fs from 'node:fs/promises';
import path from 'node:path';
import {McpServerConnection,createDefaultTransport,McpOAuthCredentialStore} from './node_modules/@earendil-works/pi-coding-agent/dist/extensions/mcp/runtime.js';
import {FileAuthStorageBackend} from './node_modules/@earendil-works/pi-coding-agent/dist/core/auth-storage.js';

export const SCRIPT_METHODS=[
 {method:'host.capabilities',capability:null},
 {method:'jira.get',capability:'jira.read'},
 {method:'jira.search_jql',capability:'jira.read'},
 {method:'jira.add_comment',capability:'jira.comment'},
 {method:'feishu.recent_chats',capability:'feishu.read'},
 {method:'feishu.recent_messages',capability:'feishu.read'},
 {method:'sessions.dispatch',capability:'sessions.dispatch'},
];
const fail=(code,message)=>{throw Object.assign(Error(message),{code});};
const required=(params,key)=>{if(typeof params[key]!=='string'||!params[key].trim())fail('INVALID_ARGS',key+' is required');};
const count=(params,key,max)=>{if(params[key]!==undefined&&(!Number.isInteger(params[key])||params[key]<1||params[key]>max))fail('INVALID_ARGS',key+' must be 1–'+max);};
function validate(method,params){
 if(method==='jira.get'||method==='jira.add_comment')required(params,'issue_key');
 if(method==='jira.search_jql'){required(params,'jql');count(params,'max_results',100);if(params.next_page_token!==undefined)required(params,'next_page_token');}
 if(method==='jira.get'||method==='jira.search_jql')if(params.fields!==undefined&&(!Array.isArray(params.fields)||params.fields.some(value=>typeof value!=='string')))fail('INVALID_ARGS','fields must be a string array');
 if(method==='jira.add_comment'){
  if((params.body_text!==undefined)===(params.body_adf!==undefined))fail('INVALID_ARGS','Provide exactly one of body_text or body_adf');
  if(params.body_text!==undefined)required(params,'body_text');
  if(params.body_adf!==undefined&&(!params.body_adf||typeof params.body_adf!=='object'||Array.isArray(params.body_adf)))fail('INVALID_ARGS','body_adf must be an object');
 }
 if(method.startsWith('feishu.'))count(params,'count',50);
 if(method==='feishu.recent_messages'){required(params,'chat_id');if(params.start_time!==undefined&&!['string','number'].includes(typeof params.start_time))fail('INVALID_ARGS','Invalid start_time');}
 if(method==='sessions.dispatch')required(params,'message');
}
const inside=(root,target)=>{const relative=path.relative(root,target);return relative===''||relative!=='..'&&!relative.startsWith('..'+path.sep)&&!path.isAbsolute(relative);};
async function saveResult(directory,name,value){
 if(typeof name!=='string'||!name.trim()||path.isAbsolute(name)||name.includes('\0')||name.includes(':'))fail('INVALID_ARGS','out_file must be a relative file path');
 const root=await fs.realpath(directory),target=path.resolve(root,name);if(!inside(root,target)||target===root)fail('INVALID_ARGS','out_file must stay within the project');
 // Existing parent and target are resolved to reject junction/symlink escapes.
 const parent=await fs.realpath(path.dirname(target));if(!inside(root,parent))fail('INVALID_ARGS','out_file parent leaves the project');
 try{if(!inside(root,await fs.realpath(target)))fail('INVALID_ARGS','out_file leaves the project');}catch(error){if(error.code!=='ENOENT')throw error;}
 await fs.writeFile(target,JSON.stringify(value,null,2),'utf8');return {saved_to:name};
}

/** Explicit mappings reuse the user's existing MCP transport and OAuth store.
 * No method guesses, credentials in script stdin, or successful placeholders. */
export function createScriptBroker({settings={},mcpServers=[],mcpAuthDirectory,cwd,dispatch,connect}){
 const connections=new Map();let closed=false;
 const configured=method=>method==='sessions.dispatch'||!!settings.methods?.[method]?.server&&mcpServers.some(server=>server.name===settings.methods[method].server&&server.enabled!==false);
 const toolCall=async(method,params,signal)=>{
  if(closed)fail('CLOSED','This script run has ended');const mapping=settings.methods?.[method];
  if(!mapping?.server||!mapping.tool)fail('NOT_CONFIGURED',method+' has no MCP method mapping. Configure automation connections.');
  const configuredServer=mcpServers.find(server=>server.name===mapping.server&&server.enabled!==false);if(!configuredServer)fail('NOT_CONFIGURED','MCP server is missing or disabled: '+mapping.server);
  let connection=connections.get(mapping.server);
  if(!connection){
   if(connect)connection=await connect(configuredServer);
   else{
    const {name,...config}=configuredServer;
    const credentials=new McpOAuthCredentialStore(new FileAuthStorageBackend(path.join(mcpAuthDirectory,'mcp-auth.json')),mcpAuthDirectory);
    connection=new McpServerConnection({entry:{name,config,source:'WinOne automations'},cwd,credentials,createTransport:createDefaultTransport,onTools:()=>{},onChange:()=>{}});
   }
   connections.set(mapping.server,connection);
  }
  const args={};for(const [key,value]of Object.entries(params)){if(key!=='out_file')args[mapping.parameterMap?.[key]??key]=value;}
  Object.assign(args,mapping.arguments??{});
  const result=await connection.callTool(mapping.tool,args,{signal});
  if(result.isError)fail('REMOTE_ERROR',(result.content??[]).filter(part=>part.type==='text').map(part=>part.text).join('\n')||'MCP tool failed');
  return result.structuredContent??((result.content??[]).length===1&&result.content[0].type==='text'?parseText(result.content[0].text):result);
 };
 return {
  async call(request,granted,{schedule},signal){
   const entry=SCRIPT_METHODS.find(item=>item.method===request.method);if(!entry)fail('METHOD_NOT_FOUND','Unknown script method: '+request.method);
   if(entry.capability&&!granted.has(entry.capability))fail('CAPABILITY_DENIED','Capability not granted: '+entry.capability);
   if(request.method==='host.capabilities')return {protocol:'cindy-script/1',granted:[...granted].sort(),methods:SCRIPT_METHODS.map(item=>({...item,available:!item.capability||granted.has(item.capability)&&configured(item.method),configured:!item.capability||configured(item.method)}))};
   const params=request.params??{};if(!params||typeof params!=='object'||Array.isArray(params))fail('INVALID_ARGS','params must be an object');validate(request.method,params);
   if(request.method==='sessions.dispatch')return dispatch(params,schedule);
   const result=await toolCall(request.method,params,signal);
   if(params.out_file!==undefined)return saveResult(cwd,params.out_file,result);
   return result;
  },
  async close(){closed=true;await Promise.all([...connections.values()].map(connection=>connection.close()));},
  notify:(channel,text,signal)=>toolCall('notify.'+channel,{text},signal),
 };
}
function parseText(text){try{return JSON.parse(text);}catch{return text;}}
