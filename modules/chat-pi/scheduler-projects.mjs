import fs from 'node:fs/promises';
import path from 'node:path';
import {createHash,randomUUID} from 'node:crypto';
import {nextCronOrMonthlyFire} from '../../third_party/cindy/scheduler/engine/monthlyClamp.ts';

// Cindy project-file schema and promote/clone/demote semantics; the existing
// Pi host supplies execution. User task pause state survives reconciliation.
const fields=['name','prompt','cronExpr','timezone','recurring','manual','intervalMs','model','providerId','effort','fastMode','useWorktree','persistentSession','silentWhenIdle','preRunHook','preRunPlugin','notify','executionMode','scriptConfig','permissionMode'];
const samePath=(a,b)=>path.resolve(a??'.').toLowerCase()===path.resolve(b).toLowerCase();
export function createProjectAutomations({scheduler,storage,normalize,event=()=>{}}){
 const queues=new Map();
 // Projects whose file was read here; a pending import there is still known.
 const seen=new Set();
 async function location(directory){if(!path.isAbsolute(directory))throw Error('Choose an absolute project directory.');const root=await fs.realpath(directory);if(!(await fs.stat(root)).isDirectory())throw Error('Project path is not a directory.');return {root,file:path.join(root,'.cindy','automations','schedules.json')};}
 async function read(directory){
  const {root,file}=await location(directory);let raw,source=file;
  try{raw=await fs.readFile(file,'utf8');}catch(error){if(error.code!=='ENOENT')throw error;source=path.join(root,'.xdmaker','automations','schedules.json');try{raw=await fs.readFile(source,'utf8');}catch(old){if(old.code!=='ENOENT')throw old;return {file,source:file,exists:false,config:{version:1,schedules:[]}};}}
  const config=JSON.parse(raw);validate(config,root);return {file,source,exists:true,config};
 }
 function input(config,directory){const result={...config,source:'project',projectConfigId:config.id,kind:'cron',agentKind:'pi',modelAgentKind:config.model?'pi':undefined,workspaceKind:'project',workingDir:directory,timezone:config.timezone??'Asia/Shanghai',recurring:config.recurring??true,manual:config.manual??false,useWorktree:config.useWorktree??false,persistentSession:config.persistentSession??false,silentWhenIdle:config.silentWhenIdle??false,notify:{desktop:true,feishu:false,...config.notify}};delete result.id;return normalize(result);}
 function validate(config,directory){if(config?.version!==1||!Array.isArray(config.schedules))throw Error('Project configuration must contain version: 1 and schedules.');const ids=new Set();for(const schedule of config.schedules){if(!schedule||typeof schedule.id!=='string'||!(/^[a-z0-9][a-z0-9-]*$/).test(schedule.id)||ids.has(schedule.id))throw Error('Project schedule IDs must be unique kebab-case names.');ids.add(schedule.id);const value=input(schedule,directory);nextCronOrMonthlyFire(value.cronExpr,Date.now(),value.timezone);if((value.executionMode??'agent')==='agent'&&!value.prompt?.trim())throw Error('Project agent prompt is required.');}}
 // Cindy 2d8d3c3 ProjectAutomationLoader: repository changes are previews until
 // the user confirms that exact snapshot (approvedHash). A missing or invalid
 // file leaves saved schedules alone; repository contents never mutate them in
 // the background.
 // Values the scheduler stores for an omitted field count as unchanged.
 const defaults={executionMode:'agent'};
 const hashOf=config=>createHash('sha256').update(JSON.stringify(config)).digest('hex');
 function patchFor(old,config,root){const next=input(config,root),patch={...next};for(const key of [...fields,'modelAgentKind'])if(!(key in patch)&&key!=='permissionMode')patch[key]=undefined;return Object.keys(patch).some(key=>JSON.stringify(old[key]??defaults[key])!==JSON.stringify(patch[key]??defaults[key]))?patch:null;}
 const projectRows=async root=>(await storage.list()).filter(schedule=>schedule.source==='project'&&samePath(schedule.workingDir,root));
 async function reconcile(directory,approvedHash){
  const {root}=await location(directory);
  const loaded=await read(root);
  if(!loaded.exists)return {...loaded,workingDir:root,inserted:0,updated:0,deleted:0,skipped:'no-file'};
  seen.add(root);
  const existing=await projectRows(root),byID=new Map(existing.map(schedule=>[schedule.projectConfigId,schedule]));
  const ids=new Set(loaded.config.schedules.map(config=>config.id));
  const removed=existing.filter(schedule=>!schedule.projectConfigId||!ids.has(schedule.projectConfigId));
  const hash=hashOf(loaded.config);
  const changed=removed.length>0||loaded.config.schedules.some(config=>{const old=byID.get(config.id);return !old||!!patchFor(old,config,root);});
  if(changed&&approvedHash!==hash){
   const pendingImport={workingDir:root,configHash:hash,schedules:loaded.config.schedules,removedNames:removed.map(schedule=>schedule.name)};
   event({type:'import-required',pendingImport});
   return {...loaded,workingDir:root,inserted:0,updated:0,deleted:0,skipped:null,pendingImport};
  }
  const result={inserted:0,updated:0,deleted:0};
  for(const config of loaded.config.schedules){const old=byID.get(config.id);if(!old){await scheduler.create(input(config,root));result.inserted++;continue;}const patch=patchFor(old,config,root);if(patch){await scheduler.update(old.id,patch);result.updated++;}}
  for(const old of removed){await scheduler.delete(old.id);result.deleted++;}
  // The renderer shows a toast only when the file content changed since the last reconcile.
  const previous=storage.projectHash(root);
  storage.setProjectHash(root,hash);
  event({type:'reconciled',workingDir:root,...result,isFirstTime:previous===null,hashChanged:previous!==hash});
  return {...loaded,workingDir:root,...result,skipped:null};
 }
 // Saving in the editor approves that one definition, not other repository data.
 async function applyOne(root,config,file){
  // The saved file's hash only dedupes toasts; it is not an approval of other entries.
  storage.setProjectHash(root,hashOf(file));
  const old=(await projectRows(root)).find(schedule=>schedule.projectConfigId===config.id);
  if(!old){await scheduler.create(input(config,root));event({type:'reconciled',workingDir:root,inserted:1,updated:0,deleted:0,isFirstTime:true,hashChanged:true});return {workingDir:root,inserted:1,updated:0,deleted:0,skipped:null};}
  const patch=patchFor(old,config,root);if(patch)await scheduler.update(old.id,patch);
  event({type:'reconciled',workingDir:root,inserted:0,updated:patch?1:0,deleted:0,isFirstTime:false,hashChanged:true});
  return {workingDir:root,inserted:0,updated:patch?1:0,deleted:0,skipped:null};
 }
 async function removeOne(root,id,file){
  if(file)storage.setProjectHash(root,hashOf(file));
  const selected=(await projectRows(root)).filter(schedule=>schedule.projectConfigId===id);
  for(const schedule of selected)await scheduler.delete(schedule.id);
  return {workingDir:root,inserted:0,updated:0,deleted:selected.length,skipped:null};
 }
 async function save(directory,config){
  const {root,file}=await location(directory);validate(config,root);const parent=path.dirname(file);await fs.mkdir(parent,{recursive:true});const resolved=await fs.realpath(parent);if(!samePath(resolved,parent))throw Error('Project automation configuration cannot be written through a junction.');
  const temporary=file+'.'+randomUUID()+'.tmp';await fs.writeFile(temporary,JSON.stringify(config,null,2)+'\n','utf8');await fs.rename(temporary,file);return root;
 }
 // Writing the whole file is the user's explicit save of every entry in it.
 async function write(directory,config){const root=await save(directory,config);return reconcile(root,hashOf(config));}
 async function upsert(schedule,remove=false){const loaded=await read(schedule.workingDir),id=schedule.projectConfigId??'task-'+randomUUID();const config={id};for(const key of fields)if(schedule[key]!==undefined)config[key]=schedule[key];const rows=loaded.config.schedules.filter(value=>value.id!==id);if(!remove)rows.push(config);const file={version:1,schedules:rows},root=await save(schedule.workingDir,file);return remove?removeOne(root,id,file):applyOne(root,config,file);}
 function serial(directory,work){const key=path.resolve(directory);const pending=(queues.get(key)??Promise.resolve()).catch(()=>{}).then(work);queues.set(key,pending);void pending.finally(()=>{if(queues.get(key)===pending)queues.delete(key)}).catch(()=>{});return pending;}
 const personal=schedule=>({...Object.fromEntries([...fields,'kind','agentKind','modelAgentKind','workspaceKind','workingDir','targetSessionId','onceAt'].filter(key=>schedule[key]!==undefined).map(key=>[key,schedule[key]])),source:'user'});
 // Cindy projectAutomation.upsertSchedule / removeSchedule: one entry by id.
 async function upsertConfig(directory,config){const loaded=await read(directory);if(!config||typeof config.id!=='string')throw Error('Project schedule id is required.');const rows=loaded.config.schedules.filter(value=>value.id!==config.id);rows.push(config);const file={version:1,schedules:rows};return applyOne(await save(directory,file),config,file);}
 async function removeConfig(directory,id){if(typeof id!=='string'||!id)throw Error('Project schedule id is required.');const loaded=await read(directory);const file=loaded.exists?{version:1,schedules:loaded.config.schedules.filter(value=>value.id!==id)}:null;const root=file?await save(directory,file):(await location(directory)).root;return removeOne(root,id,file);}
 return {
  upsertConfig:(directory,config)=>serial(directory,()=>upsertConfig(directory,config)),removeConfig:(directory,id)=>serial(directory,()=>removeConfig(directory,id)),
  read,reconcile:(directory,approvedHash)=>serial(directory,()=>reconcile(directory,approvedHash)),write:(directory,config)=>serial(directory,()=>write(directory,config)),
  // Cindy listPendingImports: every known project whose file awaits confirmation.
  async pendingImports(directories){const pending=[],roots=new Set(seen);for(const directory of directories){try{roots.add((await location(directory)).root);}catch{}}for(const directory of roots){try{const result=await serial(directory,()=>reconcile(directory));if(result.pendingImport)pending.push(result.pendingImport);}catch{}}return pending;},
  async update(schedule,patch){await serial(schedule.workingDir,()=>upsert({...schedule,...patch}));return scheduler.get(schedule.id);},
  async remove(schedule,keepUser=false){if(keepUser)await scheduler.create(personal(schedule));await serial(schedule.workingDir,()=>upsert(schedule,true));return {deleted:true};},
  async promote(schedule){await serial(schedule.workingDir,()=>upsert(schedule));await scheduler.delete(schedule.id);return {promoted:true};},
  clone:schedule=>scheduler.create(personal(schedule)),
 };
}
