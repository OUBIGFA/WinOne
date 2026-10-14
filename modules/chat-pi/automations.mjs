import fs from 'node:fs/promises';
import path from 'node:path';
import {randomUUID} from 'node:crypto';
import {Scheduler} from '../../third_party/cindy/scheduler/engine/scheduler.ts';
import {nextCronOrMonthlyFire} from '../../third_party/cindy/scheduler/engine/monthlyClamp.ts';
import {cronToHuman} from '../../third_party/cindy/scheduler/engine/cron.ts';
import {ScheduleStore} from './scheduler-store.mjs';
import {BUILTIN_TEMPLATES,TEMPLATE_CATEGORIES} from '../../third_party/cindy/scheduler/builtin-templates.ts';
import {applyTemplateParams} from '../../third_party/cindy/scheduler/engine/template.ts';
import enTemplates from '../../third_party/cindy/scheduler/en-templates.json' with {type:'json'};
import {createProjectAutomations} from './scheduler-projects.mjs';

function localizedTemplate(template,language){
 const translation=language?.startsWith('en')&&template.source==='builtin'?enTemplates.items[template.id]:undefined;
 return translation?{...template,name:translation.name,description:translation.description,prompt:translation.prompt,parameters:template.parameters?.map(parameter=>({...parameter,...translation.params?.[parameter.key]}))}:template;
}

const clearedFields=new Set(['fastMode','effort','modelAgentKind','intervalMs','model','providerId','targetSessionId','workingDir','onceAt','scriptConfig','preRunHook']);
function normalized(input,now,current){
 const patch={...input};for(const key of clearedFields)if(patch[key]===null)patch[key]=undefined;
 const value={...current,...patch};
 if(typeof value.name!=='string'||!value.name.trim()||value.name.length>160)throw Error('Enter an automation name within 160 characters.');
 if(value.agentKind!=='pi'||value.modelAgentKind&&value.modelAgentKind!=='pi')throw Error('WinOne automations use the configured Pi agent.');
 if(value.effort!==undefined&&!['off','minimal','low','medium','high','xhigh','max','ultra'].includes(value.effort))throw Error('Invalid reasoning effort.');
 if(value.fastMode!==undefined&&typeof value.fastMode!=='boolean')throw Error('Invalid priority service choice.');
 if(!['ask','auto','bypassPermissions'].includes(value.permissionMode??'ask'))throw Error('Invalid automation permission mode.');
 if(value.prompt?.length>100000||value.workingDir&&(!path.isAbsolute(value.workingDir)||value.workingDir.includes('\0')))throw Error('Invalid prompt or working directory.');
 if(value.intervalMs!==undefined&&(!Number.isSafeInteger(value.intervalMs)||value.intervalMs<1000))throw Error('The automation interval must be at least one second.');
 const timingChanged=!current||['onceAt','intervalMs','cronExpr','timezone','recurring','manual'].some(key=>Object.hasOwn(patch,key)&&patch[key]!==current[key]);
 if(value.onceAt!==undefined&&timingChanged){
  if(!Number.isSafeInteger(value.onceAt)||value.onceAt<=now)throw Error('Choose a future time for a one-time automation.');
  // The original Cindy engine owns the timer and recurring semantics. Keep
  // the chosen calendar instant as authority when adapting its interval mode.
  patch.intervalMs=value.onceAt-now;patch.recurring=false;patch.manual=false;patch.expireAt=value.onceAt+60000;
  if(current?.status==='expired'){patch.status='active';patch.lastFiredAt=undefined;patch.lastFinishedAt=undefined;}
 }else if(Object.hasOwn(patch,'onceAt')&&patch.onceAt===undefined)patch.expireAt=undefined;
 return patch;
}

export async function createAutomationRuntime({directory,projectDirectories=[],dispatch,notify=async()=>{},event=()=>{},clock={now:()=>Date.now()},passive=false,validateTarget=async()=>{}}){
 if(!path.isAbsolute(directory))throw Error('Absolute automation storage directory is required.');
 await fs.mkdir(directory,{recursive:true});
 const storage=new ScheduleStore(path.join(directory,'automations.db'));
 const pending=new Set(),manualStarting=new Set();let closed=false;
 const track=promise=>{pending.add(promise);void promise.finally(()=>pending.delete(promise)).catch(()=>{});return promise;};
 const logger={warn:(message,detail)=>event({type:'diagnostic',message,detail}),error:(message,detail)=>event({type:'diagnostic',message,detail})};
 const dispatchRun=async(schedule,context)=>{
  try{return await dispatch(schedule,{...context,onSessionBound:async id=>{
   await context.onSessionBound?.(id);
   if(schedule.persistentSession&&schedule.targetSessionId!==id&&!context.signal.aborted)await scheduler.update(schedule.id,{targetSessionId:id});
  }});}catch(error){
   if(error.code==='target_unavailable'&&schedule.targetSessionId&&!schedule.persistentSession&&!context.signal.aborted){
    // Preserve the failed run while preventing an endless stream of failures.
    // Exempt this run so pausing cannot wait on the very callback doing it.
    await scheduler.pause(schedule.id,{exemptRunId:context.runId});
   }
   throw error;
  }
 };
 const scheduler=new Scheduler({storage,clock,passive,generateId:randomUUID,maxConcurrentRuns:2,processId:process.pid,logger,validateTargetSession:validateTarget,runner:{fire:(schedule,context)=>track(dispatchRun(schedule,context))}});
 for(const type of ['fired','completed','failed','silenced','notified','deferred','skipped','session-bound','changed','runtime-state'])scheduler.on(type,entry=>{
  event(entry);
  if(entry.type==='completed'&&!entry.silenced||entry.type==='failed')void track((async()=>{
   const schedule=await storage.get(entry.scheduleId),run=await storage.getRun(entry.runId);if(!schedule||!run)return;
   try{await notify(schedule,run);}catch(error){await storage.updateRun(run.id,{notificationError:String(error.message??error)});event({type:'diagnostic',message:'Notification delivery failed: '+String(error.message??error)});event({type:'changed',scheduleId:schedule.id});}
  })()).catch(error=>logger.error('Notification result could not be saved',String(error)));
 });
 storage.db.exec('CREATE TABLE IF NOT EXISTS automation_templates(id TEXT PRIMARY KEY,data TEXT NOT NULL)');
 const projects=createProjectAutomations({scheduler,storage,normalize:value=>normalized(value,clock.now())});
 for(const workingDir of new Set([...projectDirectories,...(await storage.list()).filter(value=>value.source==='project'&&value.workingDir).map(value=>value.workingDir)])){try{await projects.reconcile(workingDir);}catch(error){logger.warn('Project automation configuration needs attention',String(error));}}
 await storage.recoverAbandonedClaims(clock.now());
 // One-time calendar choices must not turn into a fresh full interval after
 // a restart. Missed dates expire instead of running at a different date.
 for(const schedule of await storage.listActive())if(schedule.onceAt!==undefined){
  const now=clock.now();await storage.update(schedule.id,schedule.onceAt<=now?{status:'expired',nextFireAt:undefined}:{intervalMs:schedule.onceAt-schedule.createdAt,nextFireAt:schedule.onceAt});
 }
 await scheduler.start();
 let recovering=false,recovery=Promise.resolve();
 const recover=async()=>{
  if(recovering||closed)return;recovering=true;
  try{
   for(const id of await storage.ownedStoppedSchedules()){
    const current=await storage.get(id);if(current)await scheduler.pause(id);else await scheduler.delete(id);
   }
   for(const id of await storage.recoverAbandonedClaims(clock.now())){
    const current=await storage.get(id);if(!current)continue;
    if(current.onceAt!==undefined){const now=clock.now();await storage.update(id,current.onceAt<=now?{status:'expired',nextFireAt:undefined}:{nextFireAt:current.onceAt});await scheduler.update(id,{});}
    else await scheduler.update(id,{manual:current.manual,intervalMs:current.intervalMs});
   }
  }finally{recovering=false;}
 };
 const recoveryTimer=passive?undefined:setInterval(()=>{recovery=recover().catch(error=>logger.error('Automation execution-owner recovery failed',String(error)));},1000);
 const withCounts=async schedule=>schedule?{...schedule,...await storage.unreadCounts(schedule.id)}:null;
 const call=async(method,params={})=>{
  if(closed)throw Error('Automation scheduler is closing.');
  switch(method){
   case 'project.read':return projects.read(params.workingDir);
   case 'project.write':return projects.write(params.workingDir,params.config);
   case 'project.reconcile':return projects.reconcile(params.workingDir);
   case 'project.promote':case 'project.clone':case 'project.demote':{
    const schedule=await scheduler.get(params.id);if(!schedule)throw Error('Automation no longer exists.');
    if(method==='project.promote')return projects.promote(schedule);
    if(method==='project.clone')return projects.clone(schedule);
    return projects.remove(schedule,true);
   }
   case 'templates':return {categories:TEMPLATE_CATEGORIES.map(category=>params.language?.startsWith('en')?{...category,name:enTemplates.categories[category.id]??category.name}:category),templates:[...BUILTIN_TEMPLATES.map(template=>localizedTemplate(template,params.language)),...storage.db.query('SELECT data FROM automation_templates').all().map(row=>JSON.parse(row.data))]};
   case 'template.save':{
    const input=params.template;if(!input||typeof input.name!=='string'||!input.name.trim()||typeof input.prompt!=='string')throw Error('Template name and prompt are required.');
    const value={...input,id:params.id??randomUUID(),source:'user'};if(BUILTIN_TEMPLATES.some(template=>template.id===value.id))throw Error('Built-in templates cannot be overwritten.');
    storage.db.query('INSERT OR REPLACE INTO automation_templates(id,data) VALUES(?,?)').run(value.id,JSON.stringify(value));return value;
   }
   case 'template.delete':storage.db.query('DELETE FROM automation_templates WHERE id=?').run(params.id);return {deleted:true};
   case 'template.apply':{
    const template=BUILTIN_TEMPLATES.find(template=>template.id===params.id)??JSON.parse(storage.db.query('SELECT data FROM automation_templates WHERE id=?').get(params.id)?.data??'null');if(!template)throw Error('Template no longer exists.');
    const localized=localizedTemplate(template,params.language);return {...localized,prompt:applyTemplateParams(localized.prompt,params.parameters??{},localized.parameters)};
   }
   case 'list':return Promise.all((await scheduler.list(params.filter)).map(withCounts));
   case 'get':return withCounts(await scheduler.get(params.id));
   case 'create':return withCounts(await scheduler.create(normalized(params.input,clock.now())));
   case 'update':{
    const existing=await scheduler.get(params.id);if(!existing)throw Error('Automation no longer exists.');
    if(existing.source==='project')return withCounts(await projects.update(existing,normalized(params.patch,clock.now(),existing)));
    return withCounts(await scheduler.update(params.id,normalized(params.patch,clock.now(),existing)));
   }
   case 'pause':return withCounts(await scheduler.pause(params.id));
   case 'retire':return withCounts(await scheduler.pause(params.id,{exemptRunId:params.runId}));
   case 'policy':{
    const runId=scheduler.resolveInflightRunForSession(params.sessionId);
    // A fast model can call its policy tool before the execute-start frame
    // reaches this process. Host already checked its accepted request; the
    // saved early binding provides the same ownership check during that gap.
    const early=runId?undefined:await storage.getRun(params.requestId);
    if(runId!==params.requestId&&!(early?.status==='running'&&early.sessionId===params.sessionId))throw Error('This conversation is not executing that automation turn.');
    return {applied:params.action==='silence'?scheduler.silenceRun(params.requestId):scheduler.notifyRun(params.requestId)};
   }
   case 'resume':{
    const existing=await scheduler.get(params.id);if(!existing)throw Error('Automation no longer exists.');
    if(existing.onceAt===undefined)return withCounts(await scheduler.resume(params.id));
    const now=clock.now();
    // Cindy's interval resume starts a fresh countdown. Calendar tasks instead
    // retain their absolute instant, including while the application is closed.
    await storage.update(params.id,existing.onceAt<=now?{status:'expired',nextFireAt:undefined,updatedAt:now}:{status:'active',nextFireAt:existing.onceAt,intervalMs:existing.onceAt-existing.createdAt,expireAt:existing.onceAt+60000,updatedAt:now});
    return withCounts(await scheduler.update(params.id,{}));
   }
   case 'delete':{const current=await scheduler.get(params.id);if(current?.source==='project')return projects.remove(current);await scheduler.delete(params.id);return {deleted:true};}
   case 'deleted':return storage.listDeleted();
   case 'restore':await storage.restoreSchedule(params.id);return withCounts(await scheduler.update(params.id,{}));
   case 'runs':return storage.listRuns(params.id,params.limit,params.before,params.beforeId);
   case 'deleteRun':await scheduler.deleteRun(params.runId);return {deleted:true};
   case 'markRead':{
    const run=await storage.updateRun(params.runId,{readAt:clock.now()});if(!run)throw Error('Run no longer exists.');event({type:'changed',scheduleId:run.scheduleId});return run;
   }
   case 'runtime':return scheduler.getRuntimeSnapshot();
   case 'preview':return {nextFireAt:nextCronOrMonthlyFire(params.cronExpr,params.from??clock.now(),params.timezone),description:cronToHuman(params.cronExpr)};
   case 'silence':return {silenced:scheduler.silenceRun(params.runId)};
   case 'notify':return {notified:scheduler.notifyRun(params.runId)};
   case 'run':{
    if(manualStarting.has(params.id)||scheduler.getInflightCount(params.id))throw Error('This automation is already running.');
    manualStarting.add(params.id);
    return new Promise((resolve,reject)=>{
     const listener=entry=>{if(entry.scheduleId===params.id){scheduler.off('fired',listener);resolve({runId:entry.runId});}};
     scheduler.on('fired',listener);
     const running=track(scheduler.runNow(params.id));
     void running.then(resolve,reject).finally(()=>{scheduler.off('fired',listener);manualStarting.delete(params.id);}).catch(()=>{});
    });
   }
   default:throw Error('Unsupported automation action.');
  }
 };
 return {call,async settled(){await Promise.allSettled([...pending]);},async close(){closed=true;if(recoveryTimer)clearInterval(recoveryTimer);await recovery;await scheduler.stop();await Promise.allSettled([...pending]);storage.close();}};
}
