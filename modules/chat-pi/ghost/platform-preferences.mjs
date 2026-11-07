// One upstream configuration store for task creation, errands and text.oneshot.
// The manifest's explicit opt-out suppresses reads, not just the visible card.
import fs from 'node:fs';
import path from 'node:path';
import {readPluginTaskConfig,writePluginTaskConfig,validatePluginTaskConfig,setPluginTaskConfigValidator} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/pluginTaskPrefsStore.ts';
import {readGhostCindyInflightLimit} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/cindyPrefsStore.ts';

export function createGhostPreferences({root,find,host}){
 let migrated=false;
 const show=id=>find(id)?.manifest.showTaskPreferences!==false;
 function migrate(){
  if(migrated)return;
  const file=path.join(root,'task-preferences.json');
  if(fs.existsSync(file)){
   const old=JSON.parse(fs.readFileSync(file,'utf8'));
   for(const [id,value] of Object.entries(old)){
    if(value&&typeof value==='object'&&Object.keys(readPluginTaskConfig(id)).length===0){
     const {route,...rest}=value;writePluginTaskConfig(id,{...rest,...route});
    }
   }
  }
  // Preserve the old file as recovery evidence; the upstream store is now the
  // sole writer. Reopening never overwrites an existing explicit preference.
  migrated=true;
 }
 const read=id=>{if(!show(id))return undefined;migrate();return readPluginTaskConfig(id);};
 async function resolve(config={},sourceSessionId){
  if(config.agentKind&&config.agentKind!=='pi')throw Error('WinOne 插件任务仅使用 Pi');
  if(config.fastMode==='ultrafast')throw Error('Pi 不支持 ultrafast，请选择当前模型提供的模式');
  const base=config.model?{agentKind:'pi',providerId:config.providerId??'',model:config.model,effort:'',fastMode:false}
   :await host('task',{op:'plugin-native',method:'resolve-route',sourceSessionId});
  const route={...base};
  for(const key of ['agentKind','providerId','model','effort','fastMode'])if(config[key]!==undefined)route[key]=config[key];
  return host('task',{op:'plugin-native',method:'resolve-route',route});
 }
 setPluginTaskConfigValidator(async config=>{await resolve(config);});
 return {show,read,resolve,inflight:readGhostCindyInflightLimit,
  async validate(id,value){
   if(!show(id))throw Error('此插件不使用宿主任务配置');
   await validatePluginTaskConfig(value);
   if(value?.workingDir)await host('task',{op:'plugin-native',method:'validate-directory',pluginId:id,workingDir:value.workingDir});
  },
  write(id,value){migrate();return writePluginTaskConfig(id,value);},
 };
}
