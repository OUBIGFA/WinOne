// Result links are resolved against the child task, never against the parent
// companion's workspace. Cross-task opening goes through WinOne's draft guard.
import type {ReactNode} from 'react';
import {ChatFileNavigation,useChatFileNavigation} from '../../chat-file-navigation';
import {LOCAL_FILE_ORIGIN} from '../group-shared/renderer/lib/sessionFileOrigin';
export function useChatSessionFile(){const value=useChatFileNavigation();return {workingDir:value.cwd,sessionId:value.id||undefined,origin:LOCAL_FILE_ORIGIN};}
export function useChatSessionFileValue(sessionId?: string, workingDir='', _remoteHostId?: string | null) {return {sessionId,workingDir,origin:LOCAL_FILE_ORIGIN};}
// Route through the existing Chat navigation guard; the target conversation
// owns the file location, and its current draft remains protected.
export async function openFileInSidebarFileBrowser(sessionId: string, path: string, line?: number) {window.dispatchEvent(new CustomEvent('winone-open-chat', {detail: {id: sessionId, file: {path,line}}}));}
export async function openExternalFileInSidebarFileBrowser(sessionId: string, path: string) {return openFileInSidebarFileBrowser(sessionId,path);}
export async function openDirInSidebarFileBrowser(sessionId: string, path: string) {window.dispatchEvent(new CustomEvent('winone-open-chat', {detail: {id: sessionId, file: {path,directory:true}}}));}
export async function openUrlInSidebarBrowser(sessionId: string, url: string) {window.dispatchEvent(new CustomEvent('winone-open-chat', {detail: {id: sessionId, url}}));}
export function pathToFileUrl(path: string) {const value=path.replaceAll(String.fromCharCode(92),'/');return 'file:///'+value.split('/').map((part,index)=>index===0?part:encodeURIComponent(part)).join('/');}
export function ChatSessionFileProvider({value,children}:{value:{workingDir:string;sessionId?:string};children:ReactNode}){
 const parent=useChatFileNavigation();
 const open=(path:string,line?:number,directory=false)=>{
  if(value.sessionId===parent.id){if(directory&&parent.onDirectory){parent.onDirectory(path);return;}if(!directory&&parent.onOpen){parent.onOpen(path,line);return;}}
  if(value.sessionId)window.dispatchEvent(new CustomEvent('winone-open-chat',{detail:{id:value.sessionId,file:{path,line,directory}}}));
 };
 return <ChatFileNavigation id={value.sessionId} cwd={value.workingDir} onOpen={(path,line)=>open(path,line)} onDirectory={path=>open(path,undefined,true)}>{children}</ChatFileNavigation>;
}
