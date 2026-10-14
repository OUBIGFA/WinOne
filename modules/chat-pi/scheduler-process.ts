import type {ChildProcess} from 'node:child_process';

export function capAppend(current:string,chunk:string,cap:number):string{return current+chunk.slice(0,Math.max(0,cap-current.length));}

/** This worker is assigned to its own Windows Job Object before init. The
 * Host closes that job before returning any terminal result, so descendants
 * are reaped by handle ownership, never by a reusable numeric PID. */
export function killProcessTree(_pid:number|undefined,child:ChildProcess,onSettled?:()=>void):void{
 if(child.exitCode===null&&child.signalCode===null)child.kill('SIGKILL');
 onSettled?.();
}

export function commandEnvironment(source=process.env):NodeJS.ProcessEnv{
 const allowed=new Set(['APPDATA','COMSPEC','HOME','LANG','LC_ALL','LC_CTYPE','LOCALAPPDATA','LOGNAME','PATH','PATHEXT','SHELL','SYSTEMROOT','TEMP','TMP','TMPDIR','USER','USERNAME','USERPROFILE','WINDIR']);
 return {...Object.fromEntries(Object.entries(source).filter(([name,value])=>value!==undefined&&allowed.has(name.toUpperCase()))),PYTHONUTF8:'1'};
}
