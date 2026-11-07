// Cindy's original receipt store over a small, owner-local Bun SQLite database.
// It contains attribution/receipts, not another copy of the Chat transcript.
import {Database} from 'bun:sqlite';
import {drizzle} from 'drizzle-orm/bun-sqlite';
import path from 'node:path';
import {createRuntimeTable} from '../runtime-schema.mjs';
import {createOneBotPaths} from '../onebot/content-service.mjs';
import {sessions,pluginTaskRequests,sessionPrRefs,mediaBlobs,mediaRefs,orcaTeams,orcaWorkers,orcaWorkerCreationReservations} from '../cindy/tasks/db/schema.ts';
import {createOneBotPrService} from '../onebot/pr-service.mjs';
import {createPluginTaskStore} from '../cindy/ghost/apps/desktop/src/main/maker-ipc/pluginTaskStore.ts';

export function openGhostTaskLedger(directory){
 const paths=createOneBotPaths(directory),file=path.join(paths.root,'plugin-tasks.db');
 paths.assertPath(file);
 const sqlite=new Database(file,{create:true});
 try{
  const version=sqlite.query('PRAGMA user_version').get().user_version;
  if(version<0||version>4)throw Error('Unsupported plugin receipt database version');
  sqlite.exec('PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON;');
  if(version===0)sqlite.transaction(()=>{
   createRuntimeTable(sqlite,pluginTaskRequests);
   // Creation tombstones remain after the real Host task is removed. The
   // upstream discard-before-INSERT guard uses only this table's id column.
   createRuntimeTable(sqlite,sessions);
   sqlite.exec('PRAGMA user_version=1');
  })();
  if(version<2)sqlite.transaction(()=>{createRuntimeTable(sqlite,sessionPrRefs);sqlite.exec('PRAGMA user_version=2');})();
  if(version<3)sqlite.transaction(()=>{createRuntimeTable(sqlite,mediaBlobs,true);createRuntimeTable(sqlite,mediaRefs,true);sqlite.exec('PRAGMA user_version=3');})();
  if(version<4)sqlite.transaction(()=>{createRuntimeTable(sqlite,orcaTeams,true);createRuntimeTable(sqlite,orcaWorkers,true);createRuntimeTable(sqlite,orcaWorkerCreationReservations,true);sqlite.exec('PRAGMA user_version=4');})();
  // Cindy WorktreeManager metadata is an owner-local index; the actual Worker
  // session remains the Host authority and is never duplicated here.
  sqlite.exec('CREATE TABLE IF NOT EXISTS bot_worktrees (session_id TEXT PRIMARY KEY, data TEXT NOT NULL)');
 }catch(error){sqlite.close(true);throw error;}
 const db={drizzle:drizzle(sqlite)};
 return {db,sqlite,store:createPluginTaskStore(db),prs:createOneBotPrService(db),
  created:taskId=>{const now=Date.now();return db.drizzle.insert(sessions).values({id:taskId,createdAt:now,updatedAt:now}).onConflictDoNothing().run();},
  close:()=>sqlite.close(true)};
}
