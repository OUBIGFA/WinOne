import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import {ScheduleStore} from './scheduler-store.mjs';
import {Scheduler} from '../../third_party/cindy/scheduler/engine/scheduler.ts';

const definition={name:'Daily review',prompt:'Review the workspace',kind:'cron',cronExpr:'0 9 * * *',timezone:'Asia/Shanghai',recurring:true,manual:false,agentKind:'pi',workspaceKind:'dialogue',useWorktree:false,permissionMode:'ask',notify:{desktop:false,feishu:false}};
async function fixture(){const root=await fs.mkdtemp(path.join(process.env.WINONE_CHAT_TEST_DIRECTORY??os.tmpdir(),'scheduler-'));return {root,file:path.join(root,'automations.db')};}

test('Cindy task definitions, runs and pause state survive closing the database',async()=>{
 const f=await fixture();let store=new ScheduleStore(f.file);const clock={now:()=>Date.parse('2026-10-10T00:00:00Z')};
 const scheduler=new Scheduler({storage:store,runner:{fire:async()=>({sessionId:'conversation',resultText:'Completed review'})},clock,generateId:()=>crypto.randomUUID(),passive:true});
 await scheduler.start();const task=await scheduler.create(definition);const execution=await scheduler.runNow(task.id);assert.ok(execution.runId);
 const runs=await scheduler.listRuns(task.id);assert.equal(runs[0].status,'success');assert.equal(runs[0].resultText,'Completed review');
 await scheduler.pause(task.id);await scheduler.stop();store.close();
 store=new ScheduleStore(f.file);assert.equal((await store.get(task.id)).status,'paused');assert.equal((await store.listRuns(task.id))[0].sessionId,'conversation');store.close();
});

test('two storage connections cannot claim the same scheduled fire',async()=>{
 const f=await fixture(),a=new ScheduleStore(f.file),b=new ScheduleStore(f.file);
 await a.insert({...definition,id:'one',status:'active',nextFireAt:100,createdAt:0,updatedAt:0});
 assert.ok(await a.claimDueFire('one',100));assert.equal(await b.claimDueFire('one',100),null);
 a.close();b.close();
});

test('stale-run cleanup preserves a live heartbeat and this process active runs',async()=>{
 const f=await fixture(),store=new ScheduleStore(f.file);
 await store.insertRun({id:'stale',scheduleId:'one',status:'running',firedAt:1,heartbeatAt:1});
 await store.insertRun({id:'live',scheduleId:'two',status:'running',firedAt:1,heartbeatAt:100});
 await store.insertRun({id:'own',scheduleId:'three',status:'running',firedAt:1,heartbeatAt:1});
 store.releaseExecution('one',false); // Its process relinquished the OS lock without settling the run.
 assert.deepEqual(await store.markRunningAsInterrupted(50,['own']),['one']);assert.equal(await store.hasRunningRuns('two'),true);assert.equal(await store.hasRunningRuns('three'),true);store.close();
});

test('a reused live PID cannot block takeover after the actual execution lock is gone',async()=>{
 const f=await fixture(),oldOwner=new ScheduleStore(f.file),nextOwner=new ScheduleStore(f.file);
 try{
  await oldOwner.insert({...definition,id:'one',status:'active',nextFireAt:100,createdAt:0,updatedAt:0});
  await oldOwner.insertRun({id:'old-run',scheduleId:'one',status:'running',firedAt:1,heartbeatAt:1});
  oldOwner.releaseExecution('one',false); // PID remains this live test process; the owner lock is gone.
  assert.deepEqual(await nextOwner.recoverAbandonedClaims(200),['one']);assert.equal((await nextOwner.getRun('old-run')).status,'interrupted');
 }finally{oldOwner.close();nextOwner.close();}
});
