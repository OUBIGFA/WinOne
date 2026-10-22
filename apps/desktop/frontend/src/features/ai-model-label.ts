import type {AIProvider} from "./ai-configuration";
import {modelTells} from "./magpie-model-tells";

type LabeledProvider=Pick<AIProvider,"modelNames"|"modelFree">&Partial<Pick<AIProvider,"availableModels"|"models">>;
// React provider snapshots are immutable. Compute once per complete provider,
// not per visible search result, so a 4000-model picker stays linear and a
// filtered single row keeps the same disambiguator it had in the full list.
const labels=new WeakMap<LabeledProvider,Map<string,string>>();
const baseLabel=(p:LabeledProvider,id:string)=>{const name=p.modelNames?.[id]||id;return name.replace(/\s*[(（](?:free|免费)[)）]\s*$/i,"")||name;};

// Keep Magpie's two positive signals: vendor metadata, or "free" as a whole
// word in the ID/name. Unknown pricing and names such as Freedom stay unmarked.
export function isFreeModel(provider:Pick<AIProvider,"modelNames"|"modelFree">,id:string):boolean{
 return provider.modelFree?.[id]===true||[id,provider.modelNames?.[id]].some(name=>/(^|[^a-z])free($|[^a-z])|[(（]免费[)）]/i.test(name??""));
}
export function modelLabel(provider:LabeledProvider,id:string,_freeLabel?:string):string{
 let cached=labels.get(provider);
 if(!cached){
  const ids=[...new Set([...(provider.availableModels??[]),...(provider.models??[]),...Object.keys(provider.modelNames??{})])];
  const tells=modelTells(ids,value=>baseLabel(provider,value),value=>value);
  cached=new Map(ids.map(value=>[value,baseLabel(provider,value)+(tells.has(value)?` · ${tells.get(value)}`:"")]));
  labels.set(provider,cached);
 }
 return cached.get(id)??baseLabel(provider,id);
}
