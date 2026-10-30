// Pi owns its append-only transcript. Search reads a stable snapshot into a
// non-persisting SessionManager and uses Cindy's visible-text/ranking rules.
import fs from 'node:fs/promises';
import path from 'node:path';
import {Database} from 'bun:sqlite';
import {SessionManager} from '@earendil-works/pi-coding-agent';
import {oneBotError} from './content-service.mjs';
import {cjkSeg} from '../cindy/bots/search/cjkSeg.ts';
import {buildMessagesFtsMatch,fuseRRF} from '../cindy/bots/search/chatHistorySearch.pure.ts';
import {fuzzyTitleMatch,mergeConversationSearchResults,normalizeConversationContentPreview,visibleMessageTextForConversationSearch} from '../cindy/bots/search/conversationSearch.pure.ts';

function within(root,file){const relative=path.relative(root,file);return relative!==''&&!relative.startsWith('..'+path.sep)&&relative!=='..'&&!path.isAbsolute(relative);}
/** A stable branch snapshot of a Host-listed conversation transcript. */
export async function readBranch(session){
 if(!session.sessionFile)return [];
 if(!path.isAbsolute(session.sessionDirectory)||!path.isAbsolute(session.sessionFile)||!within(session.sessionDirectory,session.sessionFile))throw oneBotError('INVALID_PATH','历史文件不在该会话保存目录内');
 const [root,file]=await Promise.all([fs.realpath(session.sessionDirectory),fs.realpath(session.sessionFile)]);
 if(!within(root,file))throw oneBotError('INVALID_PATH','历史文件链接超出会话保存目录');
 const handle=await fs.open(file,'r');
 try{
  const before=await handle.stat();
  if(!before.isFile()||before.size>128*1024*1024)throw oneBotError('HISTORY_UNAVAILABLE','历史文件不可读取或超过 128 MiB');
  const text=await handle.readFile('utf8'),after=await handle.stat();
  if(before.size!==after.size||before.mtimeMs!==after.mtimeMs)throw oneBotError('HISTORY_UNAVAILABLE','历史正在写入，请稍后重试');
  // Pi's tolerant file loader skips broken lines. A search must instead report
  // an incomplete snapshot, so it never presents missing messages as no hits.
  let entries;
  try{entries=text.split(/\r?\n/).filter(line=>line.trim()).map(line=>JSON.parse(line));}
  catch{throw oneBotError('HISTORY_UNAVAILABLE','历史正在写入或格式无效，请稍后重试');}
  if(entries[0]?.type!=='session')throw oneBotError('HISTORY_UNAVAILABLE','历史文件缺少会话头');
  const seen=new Set();
  for(const entry of entries.slice(1)){
   if(typeof entry.id!=='string'||seen.has(entry.id)||entry.parentId&&!seen.has(entry.parentId))throw oneBotError('HISTORY_UNAVAILABLE','历史分支标识无效');
   seen.add(entry.id);
  }
  return SessionManager.inMemory(session.directory,undefined,entries).getBranch();
 }finally{await handle.close();}
}

export async function searchOneBotHistory(profiles,params){
 const profile=profiles.get(params.botId);
 if(typeof params.query!=='string'||params.query.length>500)throw oneBotError('INVALID_PARAMS','搜索内容无效或超过 500 字符');
 if(!Array.isArray(params.sessions))throw oneBotError('INVALID_SCOPE','历史范围必须由 Host 提供');
 const links=new Map(profiles.sessions(profile.id).map(link=>[link.id,link]));
 const selected=new Set();
 for(const session of params.sessions){
  const link=links.get(session.id);
  if(!link||['channel','group'].includes(link.role)||selected.has(session.id))throw oneBotError('INVALID_SCOPE','历史范围不属于此 OneBot 的可见任务');
  selected.add(session.id);
 }
 const query=params.query.trim(),limit=Number.isInteger(params.limit)?Math.max(1,Math.min(50,params.limit)):20;
 const empty={query,results:[],vectorUsed:false,vectorSkipReason:null,poolCapped:false};
 if(!query||selected.size===0)return empty;
 const database=new Database(':memory:');
 try{
  // The derived FTS index lives for this query only. It never becomes another
  // transcript store or persists private conversation text after deletion.
  database.exec('CREATE VIRTUAL TABLE messages_fts USING fts5(text, messageId UNINDEXED, clientId UNINDEXED, sessionId UNINDEXED, role UNINDEXED, createdAt UNINDEXED, visible UNINDEXED, tokenize="unicode61")');
  const insert=database.query('INSERT INTO messages_fts VALUES(?,?,?,?,?,?,?)');
  const summaries=new Map(),titleMatches=[];
  for(const session of params.sessions){
   const link=links.get(session.id),summary={id:session.id,title:session.title,workingDir:session.directory,workspaceKind:'dialogue',agentKind:'pi',status:session.archived?'archived':'active',
    companion:{id:profile.id,name:profile.name,role:link.role,readOnly:link.role==='history'},userSendAt:null,createdAt:session.createdAt,updatedAt:session.updatedAt,_count:{messages:0}};
   summaries.set(session.id,summary);
   const match=fuzzyTitleMatch(session.title,query);
   if(match)titleMatches.push({session:summary,...match});
   let input;
   const branch=await readBranch(session);
   database.transaction(()=>{
    for(const entry of branch){
     if(entry.type==='custom'&&entry.customType==='winone.input')input=entry.data;
     if(entry.type!=='message'||!['user','assistant'].includes(entry.message?.role))continue;
     const message=entry.message,receipt=message.role==='user'?input:undefined;
     if(message.role==='user')input=undefined;
     const content=typeof receipt?.text==='string'?receipt.text:message.content;
     const visible=visibleMessageTextForConversationSearch(message.role,content);
     if(!visible)continue;
     summary._count.messages++;
     if(message.role==='user')summary.userSendAt=entry.timestamp;
     insert.run(cjkSeg(visible)??visible,entry.id,receipt?.id??entry.id,session.id,message.role,entry.timestamp,visible);
    }
   })();
  }
  titleMatches.sort((a,b)=>b.score-a.score||Date.parse(b.session.updatedAt)-Date.parse(a.session.updatedAt));
  const expression=buildMessagesFtsMatch(query);
  const rows=expression?database.query('SELECT rowid,* FROM messages_fts WHERE messages_fts MATCH ? ORDER BY bm25(messages_fts),createdAt DESC,messageId LIMIT 151').all(expression):[];
  const byId=new Map(rows.map(row=>[String(row.rowid),row]));
  const contentHits=fuseRRF(rows.slice(0,150).map(row=>String(row.rowid)),[]).map(rank=>{
   const row=byId.get(rank.messageId),preview=normalizeConversationContentPreview(row.role,row.visible,query);
   return {session:summaries.get(row.sessionId),hit:{messageId:row.messageId,messageClientId:row.clientId,role:row.role,createdAt:row.createdAt,preview:preview.preview,snippet:preview.snippet,score:rank.score,ftsRank:rank.ftsRank,vectorRank:null}};
  });
  return {query,results:mergeConversationSearchResults({titleMatches,contentHits,limit}),vectorUsed:false,
   vectorSkipReason:'聊天记录语义索引未启用, 本次仅用 FTS 全文检索。',poolCapped:rows.length>150};
 }finally{database.close(true);}
}
