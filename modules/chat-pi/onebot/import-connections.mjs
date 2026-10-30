// Cindy's original companion-connections MCP runs in-process behind the private
// Host bridge. Pi still reviews every tool with its ordinary permission policy;
// this adapter changes only transport/ownership, never exposes imported secrets.
import {Client} from '@modelcontextprotocol/sdk/client/index.js';
import {InMemoryTransport} from '@modelcontextprotocol/sdk/inMemory.js';
import {createCompanionConnectionsProvider} from '../cindy/import/main/bot-import/connectionProvider.ts';
import {closeInvalidImportedConnections} from '../cindy/import/main/bot-import/connections.ts';
import {readCompanionSessionDiscovery} from './import-host.mjs';
import {withIMScope} from './im-scope.mjs';
import {oneBotError} from './content-service.mjs';

export function createOneBotImportedConnections(owner){
 const active=new Map();let closed=false;
 async function call(method,params){
  const key=params.sessionId+':'+params.callId;
  if(method==='cancel'){active.get(key)?.abort.abort();return {ok:true};}
  if(closed)throw oneBotError('NOT_READY','导入连接正在关闭');
  const scope=await readCompanionSessionDiscovery(params.sessionId);
  if(!scope)return method==='list'?[]:Promise.reject(oneBotError('NOT_FOUND','当前会话没有导入连接'));
  if(active.has(key))throw oneBotError('BUSY','导入工具正在执行');
  const abort=new AbortController(),client=new Client({name:'winone-onebot',version:'1.0.0'});
  const server=createCompanionConnectionsProvider().toClaudeSdkConfig({sessionId:params.sessionId}).instance;
  const [clientTransport,serverTransport]=InMemoryTransport.createLinkedPair();
  const work=(async()=>{
   try{
    await server.connect(serverTransport);await client.connect(clientTransport);
    if(method==='list')return (await client.listTools({}, {signal:abort.signal,timeout:120000})).tools;
    if(method!=='call')throw oneBotError('UNKNOWN_METHOD','不支持的导入工具操作');
    return await client.callTool({name:params.name,arguments:params.args??{}},undefined,{signal:abort.signal,timeout:125000});
   }finally{await client.close();await server.close();}
  })();
  active.set(key,{abort,work});
  try{return await work;}finally{active.delete(key);}
 }
 return {call,async close(){
  closed=true;owner.importsClosing=true;
  for(const operation of active.values())operation.abort.abort();
  await Promise.allSettled([...active.values()].map(operation=>operation.work));
  await withIMScope(owner,()=>closeInvalidImportedConnections());
 }};
}
