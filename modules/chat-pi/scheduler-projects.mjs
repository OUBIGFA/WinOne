import fs from 'node:fs/promises';
import path from 'node:path';
import {randomUUID} from 'node:crypto';
import {nextCronOrMonthlyFire} from '../../third_party/cindy/scheduler/engine/monthlyClamp.ts';

// Cindy project-file schema and promote/clone/demote semantics; the existing
// Pi host supplies execution. User task pause state survives reconciliation.
const fields=['name','prompt','cronExpr','timezone','recurring','manual','intervalMs','model','providerId','effort','fastMode','useWorktree','persistentSession','silentWhenIdle','preRunHook','notify','executionMode','scriptConfig','permissionMode'];
const samePath=(a,b)=>path.resolve(a??'.').toLowerCase()===path.resolve(b).toLowerCase();
export function createProjectAutomations({scheduler,storage,normalize}){
 const queues=new Map();
 async function location(directory){if(!path.isAbsolute(directory))throw Error('Choose an absolute project directory.');const root=await fs.realpath(directory);if(!(await fs.stat(root)).isDirectory())throw Error('Project path is not a directory.');return {root,file:path.join(root,'.cindy','automations','schedules.json')};}
 async function read(directory){
  const {root,file}=await location(directory);let raw,source=file;
  try{raw=await fs.readFile(file,'utf8');}catch(error){if(error.code!=='ENOENT')throw error;source=path.join(root,'.xdmaker','automations','schedules.json');try{raw=await fs.readFile(source,'utf8');}catch(old){if(old.code!=='ENOENT')throw old;return {file,source:file,exists:false,config:{version:1,schedules:[]}};}}
  const config=JSON.parse(raw);validate(config,root);return {file,source,exists:true,config};
 }
 function input(config,directory){const result={...config,source:'project',projectConfigId:config.id,kind:'cron',agentKind:'pi',modelAgentKind:config.model?'pi':undefined,workspaceKind:'project',workingDir:directory,timezone:config.timezone??'Asia/Shanghai',recurring:config.recurring??true,manual:config.manual??false,useWorktree:config.useWorktree??false,persistentSession:config.persistentSession??false,silentWhenIdle:config.silentWhenIdle??false,notify:{desktop:true,feishu:false,...config.notify}};delete result.id;return normalize(result);}
 function validate(config,directory){if(config?.version!==1||!Array.isArray(config.schedules))throw Error('Project configuration must contain version: 1 and schedules.');const ids=new Set();for(const schedule of config.schedules){if(!schedule||typeof schedule.id!=='string'||!(/^[a-z0-9][a-z0-9-]*$/).test(schedule.id)||ids.has(schedule.id))throw Error('Project schedule IDs must be unique kebab-case names.');ids.add(schedule.id);const value=input(schedule,directory);nextCronOrMonthlyFire(value.cronExpr,Date.now(),value.timezone);if((value.executionMode??'agent')==='agent'&&!value.prompt?.trim())throw Error('Project agent prompt is required.');}}
 async function reconcile(directory){
  const {root}=await location(directory);let loaded;
  try{loaded=await read(root);}catch(error){for(const current of await storage.list())if(current.source==='project'&&samePath(current.workingDir,root))await scheduler.pause(current.id);throw error;}
  const existing=(await storage.list()).filter(schedule=>schedule.source==='project'&&samePath(schedule.workingDir,root)),byID=new Map(existing.map(schedule=>[schedule.projectConfigId,schedule]));
  const result={inserted:0,updated:0,deleted:0};
  for(const config of loaded.config.schedules){const next=input(config,root),old=byID.get(config.id);if(!old){await scheduler.create(next);result.inserted++;}else{const patch={...next};for(const key of [...fields,'modelAgentKind'])if(!(key in patch))patch[key]=undefined;const changed=Object.keys(patch).some(key=>JSON.stringify(old[key])!==JSON.stringify(patch[key]));if(changed){await scheduler.update(old.id,patch);result.updated++;}byID.delete(config.id);}}
  for(const old of byID.values()){await scheduler.delete(old.id);result.deleted++;}return {...loaded,...result};
 }
 async function write(directory,config){
  const {root,file}=await location(directory);validate(config,root);const parent=path.dirname(file);await fs.mkdir(parent,{recursive:true});const resolved=await fs.realpath(parent);if(!samePath(resolved,parent))throw Error('Project automation configuration cannot be written through a junction.');
  const temporary=file+'.'+randomUUID()+'.tmp';await fs.writeFile(temporary,JSON.stringify(config,null,2)+'\n','utf8');await fs.rename(temporary,file);return reconcile(root);
 }
 async function upsert(schedule,remove=false){const loaded=await read(schedule.workingDir),id=schedule.projectConfigId??'task-'+randomUUID();const config={id};for(const key of fields)if(schedule[key]!==undefined)config[key]=schedule[key];const rows=loaded.config.schedules.filter(value=>value.id!==id);if(!remove)rows.push(config);return write(schedule.workingDir,{version:1,schedules:rows});}
 function serial(directory,work){const key=path.resolve(directory);const pending=(queues.get(key)??Promise.resolve()).catch(()=>{}).then(work);queues.set(key,pending);void pending.finally(()=>{if(queues.get(key)===pending)queues.delete(key)}).catch(()=>{});return pending;}
 const personal=schedule=>({...Object.fromEntries([...fields,'kind','agentKind','modelAgentKind','workspaceKind','workingDir','targetSessionId','onceAt'].filter(key=>schedule[key]!==undefined).map(key=>[key,schedule[key]])),source:'user'});
 return {
  read,reconcile:directory=>serial(directory,()=>reconcile(directory)),write:(directory,config)=>serial(directory,()=>write(directory,config)),
  async update(schedule,patch){await serial(schedule.workingDir,()=>upsert({...schedule,...patch}));return scheduler.get(schedule.id);},
  async remove(schedule,keepUser=false){if(keepUser)await scheduler.create(personal(schedule));await serial(schedule.workingDir,()=>upsert(schedule,true));return {deleted:true};},
  async promote(schedule){await serial(schedule.workingDir,()=>upsert(schedule));await scheduler.delete(schedule.id);return {promoted:true};},
  clone:schedule=>scheduler.create(personal(schedule)),
 };
}
