import {captureCall} from "../bridge";
import type {AIAgent} from "./ai-configuration";
import type {AIServicePreset} from "./ai-provider-presets";

export type AccountState={accounts:{agent:string;user:string;plan?:string;lapsed?:string;icon?:string}[]|null;methods?:{type:"oauth"|"api";label:string;placeholder?:string}[]};
export type AccountRequest={agent:string;provider?:string;draft?:{agent:string;agentBackend?:string}};
export type AccountReader=ReturnType<typeof createAccountReader>;

// A settings page owns only public account summaries. Reusing a pending read
// avoids duplicate workers when tabs/providers change; successful summaries
// expire after 30 seconds. Refresh and sign-in explicitly invalidate them.
export function createAccountReader(){
 const entries=new Map<string,{promise:Promise<AccountState>;expires:number;pending:boolean}>();
 let catalog:Promise<{agents:AIAgent[];presets?:AIServicePreset[]}>|undefined;
 function loadCatalog():Promise<{agents:AIAgent[];presets?:AIServicePreset[]}>{
  if(!catalog){
   const pending:Promise<{agents:AIAgent[];presets?:AIServicePreset[]}>=captureCall<{agents:AIAgent[];presets?:AIServicePreset[]}>("ai.accounts.catalog",{}).then(
    // An installation may invalidate discovery while it is running. Existing
    // callers must receive the current list, not overwrite it with old data.
    value=>catalog===pending?value:loadCatalog(),
    error=>{if(catalog!==pending)return loadCatalog();catalog=undefined;throw error;},
   );
   catalog=pending;
  }
  return catalog;
 }
 return {
  // Share discovery until an explicit plugin mutation changes account types.
  catalog:loadCatalog,
  invalidateCatalog(){catalog=undefined;},
  read(request:AccountRequest,refresh=false):Promise<AccountState>{
   const key=JSON.stringify([request.provider,request.agent,request.draft?.agent,request.draft?.agentBackend]);
   const previous=entries.get(key);
   if(previous&&(previous.pending||!refresh&&previous.expires>Date.now()))return previous.promise;
   const requestPromise=captureCall<AccountState>("ai.accounts.list",request);
   const entry={promise:requestPromise,expires:0,pending:true};
   entry.promise=requestPromise.then(value=>{
    entry.pending=false;entry.expires=Date.now()+30_000;return value;
   },error=>{if(entries.get(key)===entry)entries.delete(key);throw error;});
   entries.set(key,entry);return entry.promise;
  },
  invalidate(){entries.clear();},
 };
}
