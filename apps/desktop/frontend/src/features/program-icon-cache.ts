export interface ProgramIconMetadata {name:string;icon?:string}
const key='winone.program-icons.v1';
// This is a disposable local display cache, never exported as user settings.
export function readProgramIconCache():Record<string,ProgramIconMetadata>{
 try{const raw=localStorage.getItem(key);if(!raw||raw.length>4_000_000)return {};const value=JSON.parse(raw);if(!value||Array.isArray(value)||typeof value!=='object')return {};
 return Object.fromEntries(Object.entries(value).filter(([,v])=>!!v&&typeof v==='object'&&typeof (v as ProgramIconMetadata).name==='string'&&typeof (v as ProgramIconMetadata).icon==='string'&&/^[A-Za-z0-9+/]+=*$/.test((v as ProgramIconMetadata).icon!))) as Record<string,ProgramIconMetadata>;
 }catch{return {}}
}
export function saveProgramIconCache(value:Record<string,ProgramIconMetadata>){
 try{const entries=Object.entries(value).filter(([,v])=>!!v.icon).slice(-256);const raw=JSON.stringify(Object.fromEntries(entries));if(raw.length<=4_000_000)localStorage.setItem(key,raw);}catch(error){console.warn('Program icon display cache unavailable',error)}
}
