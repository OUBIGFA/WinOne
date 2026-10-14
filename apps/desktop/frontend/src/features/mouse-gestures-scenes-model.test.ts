import {expect,it} from "vitest";
import defaults from "../../../../../modules/stroke/defaults.json";
import {addStrokeSceneAction,removeStrokeSceneAction,strokeSceneActions,updateStrokeSceneAction} from "./mouse-gestures-scenes-model";
import type {StrokePreferences} from "./mouse-gestures-types";

function preferences(){return {...structuredClone(defaults),revision:7} as StrokePreferences;}

function legacyPreferences(){
 const value=preferences(),global=value.actionPackages.find(pack=>pack.id==="global")!,explorer=value.actionPackages.find(pack=>pack.id==="explorer")!;
 const parent=global.actions.find(action=>action.id==="parent")!;
 global.actions=["copy","paste","cut","select-all","undo","redo","back","forward"].map(id=>global.actions.find(action=>action.id===id)!);
 explorer.actions.splice(3,0,parent);
 return value;
}

it("shows navigation first for an upgraded legacy default without rewriting scripts or precedence",()=>{
 const value=legacyPreferences(),scene=value.scopes.find(scope=>scope.id==="explorer-folders")!;
 value.actionPackages[0].actions[0].code="custom copy script";
 const before=JSON.stringify(value);
 expect(strokeSceneActions(value,scene).slice(0,3).map(entry=>entry.action.id)).toEqual(["back","forward","parent"]);
 expect(JSON.stringify(value)).toBe(before);
 const reopened=JSON.parse(before) as StrokePreferences;
 expect(strokeSceneActions(reopened,reopened.scopes.find(scope=>scope.id===scene.id)!).slice(0,3).map(entry=>entry.action.id)).toEqual(["back","forward","parent"]);
});

it("keeps explicit custom action ordering and additions in place",()=>{
 const value=legacyPreferences(),global=value.actionPackages[0];
 global.actions.reverse();
 global.actions.splice(2,0,{id:"mine",name:"Custom",gesture:"#0",code:"custom();",enabled:true});
 const scene=value.scopes.find(scope=>scope.id==="global")!;
 expect(strokeSceneActions(value,scene).map(entry=>entry.action.id)).toEqual(["forward","back","mine","redo","undo","select-all","cut","paste","copy"]);
});

it("retains a scene-specific assignment after serialization without copying the package again",()=>{
 const original=preferences(),before=JSON.stringify(original);
 const assigned=updateStrokeSceneAction(original,"explorer-folders","global","copy",{gesture:"#0"});
 const reopened=JSON.parse(JSON.stringify(assigned.preferences)) as StrokePreferences;
 const updated=updateStrokeSceneAction(reopened,"explorer-folders",assigned.packageId,"copy",{enabled:false});
 expect(updated.packageId).toBe(assigned.packageId);
 expect(updated.preferences.actionPackages).toHaveLength(9);
 const copyCode=defaults.actionPackages[0].actions.find(action=>action.id==="copy")!.code;
 expect(updated.preferences.actionPackages.find(pack=>pack.id===assigned.packageId)?.actions.find(action=>action.id==="copy")).toMatchObject({gesture:"#0",enabled:false,code:copyCode});
 expect(updated.preferences.actionPackages.find(pack=>pack.id==="global")?.actions.find(action=>action.id==="copy")).toMatchObject({gesture:"up-then-left",enabled:false});
 expect(JSON.stringify(original)).toBe(before);
});

it("adds an existing action to a scene with its complete script and explicit package precedence",()=>{
 const original=preferences();
 original.actionPackages.find(pack=>pack.id==="global")!.actions.find(action=>action.id==="copy")!.enabled=true;
 const source=original.actionPackages.find(pack=>pack.id==="browser")!.actions[0];
 const assigned=addStrokeSceneAction(original,"explorer-folders",{...source,name:"网页刷新",gesture:"up-then-left",enabled:true}).preferences;
 const scene=assigned.scopes.find(scope=>scope.id==="explorer-folders")!;
 const last=assigned.actionPackages.at(-1)!;
 expect(last.actions[0].code).toBe(source.code);
 expect(last.actions[0].id).not.toBe(source.id);
 expect(scene.packages.at(-1)).toBe(last.id);
 expect(strokeSceneActions(assigned,scene).find(entry=>entry.action.id==="copy")?.overriddenBy).toBe("网页刷新");
 expect(assigned.scopes.find(scope=>scope.id==="browser")?.packages).toEqual(["browser"]);
});

it("removes an action only from the selected scene when its package was shared",()=>{
 const original=preferences();
 const next=removeStrokeSceneAction(original,"explorer-folders","global","copy");
 const globalCount=defaults.actionPackages[0].actions.length;
 expect(next.preferences.actionPackages.find(pack=>pack.id==="global")?.actions).toHaveLength(globalCount);
 expect(next.preferences.actionPackages.find(pack=>pack.id===next.packageId)?.actions).toHaveLength(globalCount-1);
 expect(next.preferences.scopes.find(scope=>scope.id==="global")?.packages).toEqual(["global"]);
});

it("rejects a copy at the format limit without mutating the source settings",()=>{
 const original=preferences();
 for(let index=0;index<24;index++)original.actionPackages.push({id:"extra-"+index,name:"Extra "+index,actions:[]});
 const before=JSON.stringify(original);
 expect(()=>updateStrokeSceneAction(original,"explorer-folders","global","copy",{gesture:"#0"})).toThrow("packageLimit");
 expect(JSON.stringify(original)).toBe(before);
});

it("shows the first enabled action in the last matching package as the winner",()=>{
 const original=preferences();
 original.actionPackages=[{id:"first",name:"First",actions:[{id:"copy",name:"Copy",gesture:"up",code:"",enabled:true}]},{id:"last",name:"Last",actions:[{id:"off",name:"Disabled",gesture:"up",code:"",enabled:false},{id:"winner",name:"Winner",gesture:"up",code:"",enabled:true},{id:"later",name:"Later in package",gesture:"up",code:"",enabled:true}]}];
 const scope={id:"scene",name:"Scene",kind:"global" as const,enabled:true,packages:["last","first"]};
 expect(strokeSceneActions(original,scope).map(entry=>[entry.action.id,entry.overriddenBy])).toEqual([["copy","Winner"],["off",undefined],["winner",undefined],["later","Winner"]]);
});
