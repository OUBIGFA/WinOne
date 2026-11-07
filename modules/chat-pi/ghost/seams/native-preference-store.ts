// Electron-store's host-facing get/set surface over Cindy's existing atomic
// override-settings implementation. The source modules retain their original
// schema, normalization, owner path and lifecycle; no Electron profile is opened.
import path from 'node:path';
import {ghostDataRoot} from './app-session.ts';
import {createOverrideSettingsFile} from '../../cindy/ghost/apps/desktop/src/main/maker-host/override-settings-file.ts';

type Options<T>={name:string;cwd?:string;defaults?:T;clearInvalidConfig?:boolean;schema?:unknown};
export default class NativePreferenceStore<T extends Record<string,any>> {
 private readonly delegate;
 constructor(options:Options<T>){
  const root=options.cwd??ghostDataRoot();
  this.delegate=createOverrideSettingsFile<T>({filePath:()=>path.join(root,options.name+'.json'),defaults:options.defaults??{} as T,
   normalize:raw=>raw&&typeof raw==='object'&&!Array.isArray(raw)?raw as T:{} as T,
   preserveUnreadableFile:options.clearInvalidConfig!==true,label:options.name,
   logLoadedValue:false,logReadErrorDetails:false,log:{info(){},warn(){}}});
 }
 get<K extends keyof T>(key:K,fallback?:T[K]):T[K]{return this.delegate.read()[key]??fallback as T[K];}
 set<K extends keyof T>(key:K,value:T[K]):void{this.delegate.writePatch({[key]:value} as Partial<T>);}
}
