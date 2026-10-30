// The original coordinator queries its original schema. Profile/session rows
// are Host-owned projections; delegation and timeline receipts live here.
import {Database} from 'bun:sqlite';
import {drizzle} from 'drizzle-orm/bun-sqlite';
import {eq} from 'drizzle-orm';
import {getTableConfig,SQLiteSyncDialect} from 'drizzle-orm/sqlite-core';
import * as schema from '../cindy/tasks/db/schema.ts';
import * as transactions from '../cindy/tasks/db/transactions.ts';
import * as groupTransactions from '../cindy/tasks/db/botGroupsTx.ts';
import * as wechatTransactions from '../cindy/wechat/main/localDb/worker/opHandlers/wechatTx.ts';
import {createOneBotPaths,oneBotError} from './content-service.mjs';
import path from 'node:path';

// Tables added by each user_version: 1 the task coordinator, 2 PR references,
// 3 Cindy's teammate direct messages (send_to_agent threads) and the runtime
// snapshots an @teammate reference reports (WinOne records none: "unverified"),
// 4 Cindy's local group chats: groups, ordered members, the timeline and 分工 plans.
const SCHEMA_ADDITIONS={
 1:[schema.sessions,schema.botProfiles,schema.botProfileVersions,schema.botSessionLinks,schema.botDelegations,schema.messages],
 2:[schema.sessionPrRefs],
 3:[schema.botDirectMessageThreads,schema.botDirectMessages,schema.botRuntimeSnapshots],
 4:[schema.botGroups,schema.botGroupMembers,schema.botGroupMessages,schema.botGroupPlans,schema.botGroupPlanSteps],
 5:[schema.mediaBlobs,schema.mediaRefs,schema.wechatSyncState,schema.wechatInbox,schema.wechatOutbox,schema.wechatFileAttachments],
};
// Group tables keep Cindy's composite keys and cascades: deleting a group (or a
// member's profile) removes what belongs to it. Earlier versions' DDL is unchanged.
const RELATIONAL_VERSIONS=new Set([4,5]);
const LATEST_VERSION=5;
// Cindy transactions that archive a group's lanes return the archived session
// ids; the profile store archives the same links in the same tick (onArchive).
const ARCHIVING_TRANSACTIONS=new Set(['botGroups.setMembers','botGroups.delete','botGroups.archiveLanes']);
const transactionMethods={
 ...Object.fromEntries(Object.entries(wechatTransactions).filter(([name])=>name.startsWith('wechat'))),
 'bots.createDelegation':transactions.botsCreateDelegation,
 'bots.finishDelegation':transactions.botsFinishDelegation,
 'bots.reopenDelegation':transactions.botsReopenDelegation,
 'botGroups.create':groupTransactions.botGroupsCreate,
 'botGroups.setMembers':groupTransactions.botGroupsSetMembers,
 'botGroups.delete':groupTransactions.botGroupsDelete,
 'botGroups.archiveLanes':groupTransactions.botGroupsArchiveLanes,
 'botGroups.appendMessage':groupTransactions.botGroupsAppendMessage,
 'botGroups.createPlan':groupTransactions.botGroupsCreatePlan,
 'botGroups.settleStep':groupTransactions.botGroupsSettleStep,
 'botGroups.removePlanStep':groupTransactions.botGroupsRemovePlanStep,
};
const quote=name=>'"'+name.replaceAll('"','""')+'"';

export function createOneBotTaskStore(directory,{onArchive=()=>{}}={}){
 const paths=createOneBotPaths(directory),file=path.join(paths.root,'tasks.db');paths.assertPath(file);
 const sqlite=new Database(file,{create:true}),dialect=new SQLiteSyncDialect();
 try{
  const version=sqlite.query('PRAGMA user_version').get().user_version;
  if(!Object.hasOwn(SCHEMA_ADDITIONS,version)&&version!==0)throw oneBotError('UNSUPPORTED_VERSION','OneBot 任务库版本不兼容');
  // Foreign keys apply only to tables created with them (the group tables).
  sqlite.exec('PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON;');
  if(version<LATEST_VERSION)sqlite.transaction(()=>{
   // Each version only adds tables from the pinned schema; existing task and
   // timeline rows are untouched and all DDL comes from that schema.
   for(let next=version+1;next<=LATEST_VERSION;next++)for(const table of SCHEMA_ADDITIONS[next]){
    const config=getTableConfig(table),relational=RELATIONAL_VERSIONS.has(next);
    const columns=config.columns.map(column=>{
     let definition=quote(column.name)+' '+column.getSQLType()+(column.primary?' PRIMARY KEY':'')+(column.notNull?' NOT NULL':'');
     if(column.default!==undefined){
      const value=column.default;
      if(value&&typeof value==='object'&&typeof value.getSQL==='function')definition+=' DEFAULT ('+dialect.sqlToQuery(value.inlineParams()).sql+')';
      else{const stored=column.mapToDriverValue(value);definition+=' DEFAULT '+(typeof stored==='string'?"'"+stored.replaceAll("'","''")+"'":stored===null?'NULL':Number(stored));}
     }
     return definition;
    });
    if(relational){
     for(const key of config.primaryKeys)columns.push('PRIMARY KEY ('+key.columns.map(column=>quote(column.name)).join(',')+')');
     for(const key of config.foreignKeys){
      const reference=key.reference();
      columns.push('FOREIGN KEY ('+reference.columns.map(column=>quote(column.name)).join(',')+') REFERENCES '+quote(getTableConfig(reference.foreignTable).name)
       +' ('+reference.foreignColumns.map(column=>quote(column.name)).join(',')+')'+(key.onDelete?' ON DELETE '+key.onDelete.toUpperCase():''));
     }
    }
    sqlite.exec('CREATE TABLE '+quote(config.name)+' ('+columns.join(',')+')');
    for(const {config:index} of config.indexes){
     const names=index.columns.map(column=>quote(column.name)).join(',');
     const where=index.where?' WHERE '+dialect.sqlToQuery(index.where.inlineParams()).sql:'';
     sqlite.exec('CREATE '+(index.unique?'UNIQUE ':'')+'INDEX '+quote(index.name)+' ON '+quote(config.name)+' ('+names+')'+where);
    }
   }
   sqlite.exec('PRAGMA user_version='+LATEST_VERSION);
  })();
 }catch(error){sqlite.close(true);throw error;}
 // Cindy's transactions are written for better-sqlite3, whose run().changes
 // counts the statement's own rows. Bun also counts cascaded rows, so the
 // adapter reports SQLite changes() (direct rows only) instead.
 const statement=sql=>{const query=sqlite.query(sql);return {get:(...args)=>query.get(...args),all:(...args)=>query.all(...args),
  pluck:()=>({get:(...args)=>{const row=query.get(...args);return row?Object.values(row)[0]:undefined;},all:(...args)=>query.all(...args).map(row=>Object.values(row)[0])}),
  run:(...args)=>({...query.run(...args),changes:sqlite.query('SELECT changes() AS n').get().n})};};
 const db=drizzle(sqlite),native={prepare:statement,transaction:operation=>sqlite.transaction(operation)};
 let closed=false;
 const assertOpen=()=>{if(closed)throw oneBotError('NOT_READY','OneBot 任务库已关闭');paths.assertPath(file);};
 return {
  drizzle:db,
  // Trusted upstream services use parameterized read queries alongside Drizzle.
  query(sql,args=[]){assertOpen();return statement(sql).all(...args);},
  queryOne(sql,args=[]){assertOpen();return statement(sql).get(...args)??null;},
  exec(sql,args=[]){assertOpen();return statement(sql).run(...args);},
  transaction(operation){assertOpen();return sqlite.transaction(operation)();},
  tx(name,args){
   assertOpen();const operation=transactionMethods[name];if(!operation)throw oneBotError('UNKNOWN_METHOD','不支持的 OneBot 任务事务');
   const result=operation(native,args);
   if(ARCHIVING_TRANSACTIONS.has(name)&&result.archivedSessionIds.length)onArchive(result.archivedSessionIds,args.now);
   return result;
  },
  syncProfile(profile,links){
   assertOpen();
   sqlite.transaction(()=>{
    const {capabilities,identitySource,name,...data}=profile;
    db.insert(schema.botProfiles).values(data).onConflictDoUpdate({target:schema.botProfiles.id,set:data}).run();
    const version={id:profile.id+':'+profile.currentVersion,botId:profile.id,version:profile.currentVersion,identitySource,capabilitiesJson:JSON.stringify(capabilities),createdAt:profile.updatedAt};
    db.insert(schema.botProfileVersions).values(version).onConflictDoNothing().run();
    for(const link of links){
     const row={id:profile.id+':'+link.id,botId:profile.id,sessionId:link.id,role:link.role,routeKey:link.routeKey,profileVersion:profile.currentVersion,createdAt:profile.createdAt,archivedAt:link.archivedAt??(link.role==='history'?profile.updatedAt:null)};
     db.insert(schema.botSessionLinks).values(row).onConflictDoUpdate({target:schema.botSessionLinks.id,set:row}).run();
    }
   })();
  },
  syncSession(row){assertOpen();db.insert(schema.sessions).values(row).onConflictDoUpdate({target:schema.sessions.id,set:row}).run();},
  session(id){assertOpen();return db.select().from(schema.sessions).where(eq(schema.sessions.id,id)).get();},
  close(){if(!closed){closed=true;sqlite.close(true);}},
 };
}
