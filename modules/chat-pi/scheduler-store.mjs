import {Database} from 'bun:sqlite';
import fs from 'node:fs';
import path from 'node:path';
import {createHash,randomUUID} from 'node:crypto';

const parse=row=>{if(!row)return null;const value=JSON.parse(row.data);if(value.nextFireAt===null)delete value.nextFireAt;return value;};
/** Cindy's storage interface, backed by the Bun runtime already shipped with
 * WinOne. Claiming uses one SQLite compare-and-swap across app instances. */
export class ScheduleStore{
 constructor(file){
  this.owner=randomUUID();this.locks=new Map();this.lockDirectory=path.join(path.dirname(file),'execution-locks');fs.mkdirSync(this.lockDirectory,{recursive:true});
  this.db=new Database(file,{create:true});
  this.db.exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA busy_timeout=5000;
   CREATE TABLE IF NOT EXISTS schedules(id TEXT PRIMARY KEY,status TEXT NOT NULL,next_fire INTEGER,data TEXT NOT NULL);
   CREATE TABLE IF NOT EXISTS runs(id TEXT PRIMARY KEY,schedule_id TEXT NOT NULL,status TEXT NOT NULL,fired INTEGER NOT NULL,heartbeat INTEGER,data TEXT NOT NULL);
   CREATE INDEX IF NOT EXISTS runs_schedule ON runs(schedule_id,fired DESC);
   CREATE TABLE IF NOT EXISTS deleted_records(kind TEXT NOT NULL,id TEXT NOT NULL,deleted_at INTEGER NOT NULL,data TEXT NOT NULL,PRIMARY KEY(kind,id));
   CREATE TABLE IF NOT EXISTS schedule_claims(schedule_id TEXT PRIMARY KEY,owner TEXT NOT NULL,owner_pid INTEGER NOT NULL,run_id TEXT,automatic INTEGER NOT NULL,heartbeat INTEGER NOT NULL);
   PRAGMA user_version=1;`);
 }
 close(){for(const id of this.locks.keys())this.releaseExecution(id);this.db.close();}
 // Hold a write transaction in a separate tiny SQLite database for the whole
 // execution. OS file locking, not a reusable PID, proves the owner is alive.
 // Separate files keep long runs from locking normal schedule/history writes.
 acquireExecution(id){
  if(this.locks.has(id))return true;let lock;
  try{lock=new Database(path.join(this.lockDirectory,createHash('sha256').update(id).digest('hex')+'.db'),{create:true});lock.exec('PRAGMA busy_timeout=0; CREATE TABLE IF NOT EXISTS execution_owner(id INTEGER PRIMARY KEY); BEGIN IMMEDIATE;');this.locks.set(id,lock);return true;}
  catch(error){lock?.close();if(error.code==='SQLITE_BUSY'||error.code==='SQLITE_LOCKED'||/database is locked/i.test(error.message))return false;throw error;}
 }
 releaseExecution(id,removeClaim=true){
  const lock=this.locks.get(id);if(!lock)return;
  if(removeClaim)this.db.query('DELETE FROM schedule_claims WHERE schedule_id=? AND owner=?').run(id,this.owner);
  this.locks.delete(id);try{lock.exec('ROLLBACK');}finally{lock.close();}
 }
 async list(filter={}){return this.db.query('SELECT data FROM schedules').all().map(parse).filter(value=>(!filter.status||value.status===filter.status)&&(!filter.source||value.source===filter.source));}
 async listActive(){return this.db.query("SELECT data FROM schedules WHERE status='active'").all().map(parse);}
 async get(id){return parse(this.db.query('SELECT data FROM schedules WHERE id=?').get(id));}
 async insert(value){this.db.query('INSERT INTO schedules(id,status,next_fire,data) VALUES(?,?,?,?)').run(value.id,value.status,value.nextFireAt??null,JSON.stringify(value));return structuredClone(value);}
 async update(id,patch){let release=false;const result=this.db.transaction(()=>{
  const old=parse(this.db.query('SELECT data FROM schedules WHERE id=?').get(id));if(!old)return null;
  const claim=this.db.query('SELECT * FROM schedule_claims WHERE schedule_id=?').get(id),effective={...patch};
  // A second engine's startup must not turn an active claim back into a due
  // schedule. Other edited metadata is retained for the owning run to reread.
  if(claim&&claim.owner!==this.owner)delete effective.nextFireAt;
  const value={...old,...effective};this.db.query('UPDATE schedules SET status=?,next_fire=?,data=? WHERE id=?').run(value.status,value.nextFireAt??null,JSON.stringify(value),id);
  release=claim?.owner===this.owner&&(!claim.run_id||!this.db.query("SELECT 1 FROM runs WHERE id=? AND status='running'").get(claim.run_id))&&(Object.hasOwn(effective,'nextFireAt')||value.status!=='active');
  return value;
 })();if(release)this.releaseExecution(id);return result;}
 async delete(id){this.db.transaction(()=>{
  const row=this.db.query('SELECT data FROM schedules WHERE id=?').get(id);if(!row)return;
  this.db.query("INSERT OR REPLACE INTO deleted_records VALUES('schedule',?,?,?)").run(id,Date.now(),row.data);
  this.db.query("INSERT OR REPLACE INTO deleted_records(kind,id,deleted_at,data) SELECT 'run',id,?,data FROM runs WHERE schedule_id=?").run(Date.now(),id);
  this.db.query('DELETE FROM runs WHERE schedule_id=?').run(id);
  this.db.query('DELETE FROM schedules WHERE id=?').run(id);
 })();this.releaseExecution(id);}
 async listDeleted(){return this.db.query("SELECT d.data,d.deleted_at FROM deleted_records d LEFT JOIN schedules s ON s.id=d.id WHERE d.kind='schedule' AND s.id IS NULL ORDER BY d.deleted_at DESC").all().map(row=>({...parse(row),deletedAt:row.deleted_at}));}
 async restoreSchedule(id){return this.db.transaction(()=>{
  if(this.db.query('SELECT 1 FROM schedules WHERE id=?').get(id))throw Error('An automation with this identity already exists.');
  const row=this.db.query("SELECT data FROM deleted_records WHERE kind='schedule' AND id=?").get(id);if(!row)throw Error('Deleted automation no longer exists.');
  const value={...parse(row),status:'paused',nextFireAt:undefined,updatedAt:Date.now()};
  this.db.query('INSERT INTO schedules(id,status,next_fire,data) VALUES(?,?,NULL,?)').run(id,value.status,JSON.stringify(value));
  for(const saved of this.db.query("SELECT data FROM deleted_records WHERE kind='run' AND json_extract(data,'$.scheduleId')=?").all(id)){
   let run=parse(saved);if(run.status==='running')run={...run,status:'interrupted',finishedAt:Date.now(),errorMsg:'The automation was deleted while this run was active.'};this.db.query('INSERT INTO runs(id,schedule_id,status,fired,heartbeat,data) VALUES(?,?,?,?,?,?)').run(run.id,run.scheduleId,run.status,run.firedAt,run.heartbeatAt??null,JSON.stringify(run));
  }
  return value;
 })();}
 async claimDueFire(id,expectedNextFireAt){
  if(this.locks.has(id)||!this.acquireExecution(id))return null;
  const claimed=this.db.transaction(()=>{
   if(this.db.query('SELECT 1 FROM schedule_claims WHERE schedule_id=?').get(id))return null;
   const row=this.db.query("UPDATE schedules SET next_fire=NULL,data=json_remove(data,'$.nextFireAt') WHERE id=? AND status='active' AND next_fire=? RETURNING data").get(id,expectedNextFireAt);
   if(row)this.db.query('INSERT INTO schedule_claims VALUES(?,?,?,NULL,1,?)').run(id,this.owner,process.pid,Date.now());return parse(row);
  })();if(!claimed)this.releaseExecution(id);return claimed;
 }
 async insertRun(value){
  if(this.locks.has(value.scheduleId)&&this.db.query('SELECT 1 FROM schedule_claims WHERE schedule_id=? AND run_id IS NOT NULL').get(value.scheduleId))throw Error('This automation already owns an execution.');
  if(!this.acquireExecution(value.scheduleId))throw Error('This automation is already executing in another process.');
  try{return this.db.transaction(()=>{
   const claim=this.db.query('SELECT * FROM schedule_claims WHERE schedule_id=?').get(value.scheduleId);
   if(claim&&(claim.owner!==this.owner||claim.run_id))throw Error('This automation has an execution claim that has not settled.');
   this.db.query('INSERT INTO runs(id,schedule_id,status,fired,heartbeat,data) VALUES(?,?,?,?,?,?)').run(value.id,value.scheduleId,value.status,value.firedAt,value.heartbeatAt??null,JSON.stringify(value));
   this.db.query('INSERT OR REPLACE INTO schedule_claims VALUES(?,?,?,?,?,?)').run(value.scheduleId,this.owner,process.pid,value.id,claim?.automatic??0,Date.now());return structuredClone(value);
  })();}catch(error){this.releaseExecution(value.scheduleId,false);throw error;}
 }
 async updateRun(id,patch){let release;const result=this.db.transaction(()=>{
  const old=parse(this.db.query('SELECT data FROM runs WHERE id=?').get(id));if(!old)return null;
  const value={...old,...patch};this.db.query('UPDATE runs SET status=?,heartbeat=?,data=? WHERE id=?').run(value.status,value.heartbeatAt??null,JSON.stringify(value),id);
  if(value.status!=='running'&&this.db.query('SELECT 1 FROM schedule_claims WHERE schedule_id=? AND owner=? AND automatic=0').get(value.scheduleId,this.owner))release=value.scheduleId;
  return value;
 })();if(release)this.releaseExecution(release);return result;}
 async ownedStoppedSchedules(){return this.db.query("SELECT c.schedule_id FROM schedule_claims c LEFT JOIN schedules s ON s.id=c.schedule_id WHERE c.owner=? AND (s.id IS NULL OR s.status!='active')").all(this.owner).map(row=>row.schedule_id);}
 async recoverAbandonedClaims(now=Date.now()){
  const recovered=[];
  for(const claim of this.db.query('SELECT * FROM schedule_claims').all()){
   if(this.locks.has(claim.schedule_id)||!this.acquireExecution(claim.schedule_id))continue;
   try{this.db.transaction(()=>{
    const current=this.db.query('SELECT * FROM schedule_claims WHERE schedule_id=?').get(claim.schedule_id);if(!current||current.owner!==claim.owner)return;
    const run=current.run_id?parse(this.db.query('SELECT data FROM runs WHERE id=?').get(current.run_id)):null;
    if(run?.status==='running'){const ended={...run,status:'interrupted',finishedAt:now,errorMsg:'The execution owner exited before this run completed.'};this.db.query("UPDATE runs SET status='interrupted',data=? WHERE id=?").run(JSON.stringify(ended),run.id);}
    this.db.query('DELETE FROM schedule_claims WHERE schedule_id=?').run(claim.schedule_id);recovered.push(claim.schedule_id);
   })();}finally{this.releaseExecution(claim.schedule_id,false);}
  }
  return recovered;
 }
 async listRuns(id,limit=100,before,beforeId=''){
  const count=Math.max(1,Math.min(limit,500));
  return (before===undefined?this.db.query('SELECT data FROM runs WHERE schedule_id=? ORDER BY fired DESC,id DESC LIMIT ?').all(id,count):this.db.query('SELECT data FROM runs WHERE schedule_id=? AND (fired<? OR fired=? AND id<?) ORDER BY fired DESC,id DESC LIMIT ?').all(id,before,before,beforeId,count)).map(parse);
 }
 async deleteRun(id,options={}){return this.db.transaction(()=>{
  const row=this.db.query('SELECT data,schedule_id FROM runs WHERE id=?').get(id);if(!row)return null;
  if(options.excludeBotSchedules){const schedule=parse(this.db.query('SELECT data FROM schedules WHERE id=?').get(row.schedule_id));if(schedule?.source==='bot')return null;}
  this.db.query("INSERT OR REPLACE INTO deleted_records VALUES('run',?,?,?)").run(id,Date.now(),row.data);
  this.db.query('DELETE FROM runs WHERE id=?').run(id);return parse(row);
 })();}
 async markRunningAsInterrupted(staleBefore,excludeRunIds=[],opts={}){return this.db.transaction(()=>{
  const changed=new Set();const excluded=new Set(excludeRunIds);
  for(const row of this.db.query("SELECT id,schedule_id,data,heartbeat,fired FROM runs WHERE status='running'").all()){
   if(excluded.has(row.id))continue;
   if(row.heartbeat===null?row.fired>=(opts.legacyStaleBefore??staleBefore):row.heartbeat>=staleBefore)continue;
   const claim=this.db.query('SELECT 1 FROM schedule_claims WHERE schedule_id=?').get(row.schedule_id);
   if(claim){if(this.locks.has(row.schedule_id)||!this.acquireExecution(row.schedule_id))continue;this.releaseExecution(row.schedule_id,false);}
   const value={...parse(row),status:'interrupted',finishedAt:Date.now(),errorMsg:'WinOne stopped before this run completed.'};
   this.db.query("UPDATE runs SET status='interrupted',data=? WHERE id=? AND status='running'").run(JSON.stringify(value),row.id);changed.add(row.schedule_id);
  }
  return [...changed];
 })();}
 async touchRunHeartbeats(ids,now){this.db.transaction(()=>{for(const id of ids){this.db.query("UPDATE runs SET heartbeat=?,data=json_set(data,'$.heartbeatAt',?) WHERE id=? AND status='running'").run(now,now,id);this.db.query('UPDATE schedule_claims SET heartbeat=? WHERE run_id=? AND owner=?').run(now,id,this.owner);}})();}
 async hasRunningRuns(id){return !!(id?this.db.query("SELECT 1 FROM runs WHERE status='running' AND schedule_id=? LIMIT 1").get(id):this.db.query("SELECT 1 FROM runs WHERE status='running' LIMIT 1").get());}
 async getRun(id){return parse(this.db.query('SELECT data FROM runs WHERE id=?').get(id));}
 async unreadCounts(id){return this.db.query("SELECT COUNT(*) AS unreadRunCount,COALESCE(SUM(status IN ('failed','interrupted')),0) AS unreadFailureCount FROM runs WHERE schedule_id=? AND status IN ('success','failed','interrupted') AND json_extract(data,'$.readAt') IS NULL").get(id);}
}
