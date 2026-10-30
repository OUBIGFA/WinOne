// Persistence and native-host facts for Cindy's unchanged lifecycle and
// invitation services, projected onto the OneBot profile store.
import {oneBotOwnerScope} from './im-scope.mjs';
import {oneBotError} from './content-service.mjs';
import {queryLearningRows} from './learning-data.mjs';

const field=(table,name)=>Object.freeze({table,name});
export const botProfiles={table:'profiles',id:field('profiles','id')};
export const botProfileVersions={table:'versions',botId:field('versions','botId'),version:field('versions','version')};
export const botSessionLinks={table:'sessions',botId:field('sessions','botId'),sessionId:field('sessions','sessionId')};
export const botLifecycleEvents={table:'events'};
// Chat rows the invitation worker checks: Pi owns the transcript and the Host
// owns the conversation index, so these read through the private bridge.
export const messages={table:'messages',id:field('messages','id'),sessionId:field('messages','sessionId'),role:field('messages','role'),
 clientId:field('messages','clientId'),content:field('messages','content'),rewindAt:field('messages','rewindAt')};
export const sessions={table:'chat-sessions',id:field('chat-sessions','id'),agentKind:field('chat-sessions','agentKind')};
export const eq=(column,value)=>({column,value});
export const and=(...conditions)=>({and:conditions.flatMap(condition=>condition.and??[condition])});

// Only the reads and transactions that the vendored lifecycle and invitation
// services issue are projected; any other query fails instead of guessing.
function equalities(table,where){
 const values={};
 for(const {column,value} of where.and??[where]){
  if(!column||column.table!==table.table)throw oneBotError('UNSUPPORTED_QUERY','不支持的 OneBot 档案查询');
  values[column.name]=value;
 }
 return values;
}
const only=(values,...names)=>Object.keys(values).length===names.length&&names.every(name=>Object.hasOwn(values,name));
async function readRows(owner,table,where){
 const store=owner.profiles,values=equalities(table,where);
 if(table===botProfiles&&only(values,'id')){
  try{return [store.get(values.id)];}catch(error){if(error.code==='NOT_FOUND')return [];throw error;}
 }
 if(table===botProfileVersions&&only(values,'botId','version')){
  let rows;
  try{rows=store.versions(values.botId);}catch(error){if(error.code==='NOT_FOUND')return [];throw error;}
  return rows.filter(row=>row.version===values.version).map(row=>({botId:row.botId,version:row.version,identitySource:row.identitySource,capabilitiesJson:JSON.stringify(row.capabilities),createdAt:row.createdAt}));
 }
 if(table===botSessionLinks&&only(values,'botId'))return store.sessions(values.botId).map(row=>({sessionId:row.id}));
 if(table===messages&&only(values,'sessionId','role')){
  if(!owner.host)throw oneBotError('NOT_READY','OneBot 会话记录尚未就绪');
  return owner.host.transcriptRows(values.sessionId,values.role);
 }
 // The reply-quote note rereads the quoted original from its own chat.
 if(table===messages&&only(values,'sessionId','clientId')){
  if(!owner.host)throw oneBotError('NOT_READY','OneBot 会话记录尚未就绪');
  return owner.host.messageRow(values.sessionId,values.clientId);
 }
 // Every OneBot conversation runs on Pi; a missing link has no row.
 if(table===sessions&&only(values,'id'))return store.sessionOwnerOrNull(values.id)?[{id:values.id,agentKind:'pi'}]:[];
 throw oneBotError('UNSUPPORTED_QUERY','不支持的 OneBot 档案查询');
}

// Cindy uses client identity as an owner-boundary token in invitations and
// creation drafts. Rebuilding a wrapper on every read falsely cancels them.
const clients = new WeakMap();
export function getDbClient(){
 const owner=oneBotOwnerScope(),store=owner.profiles;
 if(!store)throw oneBotError('NOT_READY','OneBot 生命周期存储尚未就绪');
 if(clients.has(store))return clients.get(store);
 const client = {
  query: (sql, args) => queryLearningRows(owner, sql, args),
  queryOne: (sql, args) => queryLearningRows(owner, sql, args).then(rows => rows[0] ?? null),
  tx:(name,args)=>name==='bots.updateProfile'?store.txUpdateProfile(args):store.lifecycleTransaction(name,args),
  drizzle:{
   select(selection){return {from(table){return {where(where){
    const read=async()=>{
     const rows=await readRows(owner,table,where);
     return selection?rows.map(row=>Object.fromEntries(Object.entries(selection).map(([key,column])=>[key,row[column.name]]))):rows;
    };
    return {limit:n=>read().then(rows=>rows.slice(0,n)),then:(resolve,reject)=>read().then(resolve,reject)};
   }}}};},
   insert(table){if(table!==botLifecycleEvents)throw oneBotError('UNSUPPORTED_QUERY','不支持的生命周期事件表');return {values:row=>store.recordLifecycleEvent(row)};},
  },
 };
 clients.set(store,client);
 return client;
}

export async function resolveBotCanonicalSession(botId){const session=oneBotOwnerScope().profiles.findSession(botId);return session?{status:'resolved',sessionId:session.sessionId}:{status:'missing'};}
export async function deleteBotProfileAndDetachSessionsInDb(botId,sessionIds,keepHistory){
 const owner=oneBotOwnerScope();
 if(sessionIds.length){if(!owner.lifecycle.deleteSessions)throw oneBotError('NOT_READY','OneBot 会话清理尚未就绪');await owner.lifecycle.deleteSessions(botId,sessionIds,keepHistory);}
 owner.profiles.removeProfile(botId);
}
// A Bot Home contains user-authored content. Only the Host can recycle the
// registered path; the worker never accepts a path from the renderer.
export async function removeBotProfileFolder(_root,botId){
 const owner=oneBotOwnerScope();
 if(!owner.lifecycle.recycleHome)throw oneBotError('NOT_READY','OneBot 回收站清理尚未就绪');
 await owner.lifecycle.recycleHome(botId);
 owner.profiles.cleanupCompleted(botId,'homePending');
}

export async function retryDeletedBotCleanup(botId){
 const owner=oneBotOwnerScope();
 const pending=owner.profiles.pendingCleanup().find(row=>row.botId===botId);
 if(!pending)throw oneBotError('NOT_FOUND','没有待恢复的 OneBot 清理记录');
 if(owner.profiles.owner(botId))throw oneBotError('PRECONDITION_FAILED','现有 OneBot 不能作为删除残留清理');
 const warnings=[];
 if(pending.imPending){
  try{await owner.lifecycle.onDeleted?.(botId,()=>{if(owner.closed)throw oneBotError('NOT_READY','OneBot 服务已关闭');});owner.profiles.cleanupCompleted(botId,'imPending');}
  catch{warnings.push('IMPORTED_ENVIRONMENT_CLEANUP_PENDING');}
 }
 if(pending.homePending){
  try{await removeBotProfileFolder(owner.paths.root,botId);}
  catch{warnings.push('HOME_CLEANUP_PENDING');}
 }
 return {warnings};
}

export const app={getPath(name){if(name!=='userData')throw oneBotError('INVALID_PARAMS','不支持的系统目录');return oneBotOwnerScope().paths.root;}};
export const BrowserWindow={getAllWindows(){const owner=oneBotOwnerScope();return [{isDestroyed:()=>owner.closed,webContents:{send:(channel,payload)=>owner.event({type:channel==='routines:changed'?'routines-changed':channel===MAKER_PUSH.BOT_WORKBENCH_CHANGED?'workbench-changed':'lifecycle-changed',channel,...payload,payload})}}];}};
export const ipcMain={handle(){throw oneBotError('UNSUPPORTED_OPERATION','OneBot 由原生 Host 注册接口');}};
export function assertTrustedAppRendererEvent(){throw oneBotError('UNSUPPORTED_OPERATION','OneBot 不接受 Electron 窗口调用');}
export const MAKER_INVOKE={BOT_LIFECYCLE_ACTION:'maker:bot-lifecycle:action'};
export const MAKER_PUSH={BOT_LIFECYCLE_CHANGED:'maker:bot-lifecycle:changed',BOT_WORKBENCH_CHANGED:'maker:bot-workbench:changed'};
export function broadcastBotRemoteResourceChanged(botId){oneBotOwnerScope().event({type:'profile-changed',botId});}
