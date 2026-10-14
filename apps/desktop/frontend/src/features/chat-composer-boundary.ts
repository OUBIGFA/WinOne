import {useLayoutEffect,type RefObject} from "react";

// The process viewport belongs to the transcript. Its available height follows
// the composer, while the independent files/resources panel remains full-height.
export function useComposerBoundary(root:RefObject<HTMLDivElement|null>,key:string){
 useLayoutEffect(()=>{
  const workspace=root.current,composer=workspace?.querySelector<HTMLElement>(".chat-composer");
  if(!workspace||!composer)return;
  const update=()=>{const transcript=workspace.querySelector<HTMLElement>(".chat-transcript");const available=transcript?.getBoundingClientRect().height??workspace.getBoundingClientRect().height;workspace.style.setProperty("--chat-process-available",`${Math.max(80,available-84)}px`);};
  const observer=new ResizeObserver(update);observer.observe(workspace);observer.observe(composer);
  const area=composer.closest(".chat-input-area,.chat-welcome-stage");if(area)observer.observe(area);
  workspace.addEventListener("scroll",update,true);update();
  return()=>{observer.disconnect();workspace.removeEventListener("scroll",update,true);};
 },[root,key]);
}
