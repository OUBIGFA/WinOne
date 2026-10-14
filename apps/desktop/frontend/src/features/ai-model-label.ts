import type {AIProvider} from "./ai-configuration";

// Keep Magpie's two positive signals: vendor metadata, or "free" as a whole
// word in the ID/name. Unknown pricing and names such as Freedom stay unmarked.
export function isFreeModel(provider:Pick<AIProvider,"modelNames"|"modelFree">,id:string):boolean{
 return provider.modelFree?.[id]===true||[id,provider.modelNames?.[id]].some(name=>/(^|[^a-z])free($|[^a-z])|[(（]免费[)）]/i.test(name??""));
}
export function modelLabel(provider:Pick<AIProvider,"modelNames"|"modelFree">,id:string,_freeLabel?:string):string{
 const name=provider.modelNames?.[id]||id;
 return name.replace(/\s*[(（](?:free|免费)[)）]\s*$/i,"")||name;
}
