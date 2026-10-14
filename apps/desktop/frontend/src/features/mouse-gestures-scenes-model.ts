import {strokeId,type StrokeAction,type StrokePackage,type StrokePreferences,type StrokeScope} from "./mouse-gestures-types";

export function orderedStrokeScopes(scopes:StrokeScope[]){
 // Presentation order must not rewrite imported rules or their stored order.
 return [...scopes].sort((a,b)=>Number(b.kind==="explorer-folders")-Number(a.kind==="explorer-folders"));
}

export function strokeSceneActions(preferences:StrokePreferences,scope:StrokeScope){
 const packages=preferences.actionPackages.filter(pack=>scope.packages.includes(pack.id));
 const winners=new Map<string,StrokeAction>();
 // This is GestureEngine.Find's order: later packages win, and the first
 // enabled action within a package wins. Keep shadowed rows visible to edit.
 for(const pack of [...packages].reverse())for(const action of pack.actions){
  if(action.enabled&&!winners.has(action.gesture))winners.set(action.gesture,action);
 }
 const entries=packages.flatMap(pack=>pack.actions.map(action=>({pack,action,overriddenBy:action.enabled&&winners.get(action.gesture)!==action?winners.get(action.gesture)?.name:undefined})));
 // Older installations keep their original packages, including “parent” in
 // Explorer. Recognize only shipped action orders; a reordered/extended user
 // package keeps its explicit order. Display sorting never changes the worker's
 // package precedence, action scripts, bindings, or the saved configuration.
 const navigation=["back","forward","parent"];
 const shippedOrders=new Set([
  "copy,paste,cut,select-all,undo,redo,back,forward",
  "back,forward,parent,copy,paste,cut,select-all,undo,redo",
  "refresh,fullscreen,new-folder,parent,new-window"
 ]);
 if(packages.some(pack=>pack.actions.some(action=>navigation.includes(action.id))&&!shippedOrders.has(pack.actions.map(action=>action.id).join(","))))return entries;
 const rank=(id:string)=>{const index=navigation.indexOf(id);return index<0?navigation.length:index;};
 return entries.sort((a,b)=>rank(a.action.id)-rank(b.action.id));
}

function packageName(preferences:StrokePreferences,name:string){
 // The existing settings format limits names to 240 UTF-8 bytes. Sixty code
 // points also leave room for a short suffix without cutting a surrogate pair.
 const base=Array.from(name).slice(0,54).join("");
 let result=base,index=2;
 while(preferences.actionPackages.some(pack=>pack.name===result))result=base+" ("+index+++")";
 return result;
}

function editScenePackage(preferences:StrokePreferences,scopeId:string,packageId:string,edit:(actions:StrokeAction[])=>StrokeAction[]){
 const scope=preferences.scopes.find(item=>item.id===scopeId),pack=preferences.actionPackages.find(item=>item.id===packageId);
 if(!scope||!pack||!scope.packages.includes(pack.id))throw Error("missingSceneAction");
 const shared=preferences.scopes.some(item=>item.id!==scopeId&&item.packages.includes(packageId));
 if(shared&&preferences.actionPackages.length>=32)throw Error("packageLimit");
 const nextId=shared?strokeId():packageId;
 const next:StrokePackage={...pack,id:nextId,name:shared?packageName(preferences,scope.name+" · "+pack.name):pack.name,actions:edit(pack.actions)};
 // Use the existing package format so import/export, scripts and the worker
 // remain compatible. Copy on edit only; simply visiting a scene stays clean.
 const actionPackages=preferences.actionPackages.flatMap(item=>item.id===packageId?(shared?[item,next]:[next]):[item]);
 const scopes=shared?preferences.scopes.map(item=>item.id===scopeId?{...item,packages:item.packages.map(id=>id===packageId?nextId:id)}:item):preferences.scopes;
 return {preferences:{...preferences,actionPackages,scopes},packageId:nextId};
}

export function updateStrokeSceneAction(preferences:StrokePreferences,scopeId:string,packageId:string,actionId:string,patch:Partial<StrokeAction>){
 return editScenePackage(preferences,scopeId,packageId,actions=>{
  if(!actions.some(action=>action.id===actionId))throw Error("missingSceneAction");
  return actions.map(action=>action.id===actionId?{...action,...patch}:action);
 });
}

export function removeStrokeSceneAction(preferences:StrokePreferences,scopeId:string,packageId:string,actionId:string){
 return editScenePackage(preferences,scopeId,packageId,actions=>actions.filter(action=>action.id!==actionId));
}

export function addStrokeSceneAction(preferences:StrokePreferences,scopeId:string,action:Omit<StrokeAction,"id">){
 const scope=preferences.scopes.find(item=>item.id===scopeId);
 if(!scope)throw Error("missingSceneAction");
 // New assignments use the last package only when it belongs exclusively to
 // this scene; otherwise create one at the end to make precedence explicit.
 const last=preferences.actionPackages.at(-1);
 const own=last&&scope.packages.includes(last.id)&&!preferences.scopes.some(item=>item.id!==scopeId&&item.packages.includes(last.id))&&last.actions.length<256;
 if(!own&&preferences.actionPackages.length>=32)throw Error("packageLimit");
 const nextAction={...action,id:strokeId()};
 if(own)return editScenePackage(preferences,scopeId,last.id,actions=>[...actions,nextAction]);
 const pack={id:strokeId(),name:packageName(preferences,scope.name),actions:[nextAction]};
 return {preferences:{...preferences,actionPackages:[...preferences.actionPackages,pack],scopes:preferences.scopes.map(item=>item.id===scopeId?{...item,packages:[...item.packages,pack.id]}:item)},packageId:pack.id};
}
