interface ResizeFlags { enableResize:boolean; borderThickness:number; resizeEdge?:string; defaultCursor?:string }
type DesktopWindow = Window & { wails?:{flags:ResizeFlags} };

// Wails handles the native sizing loop. Its bundled hit test uses outerWidth,
// which need not equal the WebView's CSS viewport at mixed DPI or browser zoom.
// Keep its edge and our visible cursor in the same client-coordinate space.
export function installWindowResizeFeedback(maximized:boolean) {
 const desktop=window as DesktopWindow, root=document.documentElement;
 // Chromium's scrollbars do not participate in DOM mouse hit testing or
 // inherit element cursors. Invisible edge targets keep the existing Wails
 // resize contract reachable over scrollbars without changing page layout.
 const handles=document.createElement("div");
 handles.dataset.windowResizeHandles="";handles.setAttribute("aria-hidden","true");
 Object.assign(handles.style,{position:"fixed",inset:"0",zIndex:"2147483647",pointerEvents:"none"});
 const flags=desktop.wails?.flags;
 if(flags?.enableResize&&!maximized){
  const thickness=`${flags.borderThickness}px`;
  for(const direction of ["n","ne","e","se","s","sw","w","nw"]){
   const handle=document.createElement("div");handle.dataset.windowResizeHandle=direction;
   Object.assign(handle.style,{position:"absolute",pointerEvents:"auto",cursor:`${direction}-resize`});
   const horizontal=direction.includes("e")||direction.includes("w");
   const vertical=direction.includes("n")||direction.includes("s");
   if(horizontal){handle.style.width=thickness;handle.style[direction.includes("w")?"left":"right"]="0";}
   else{handle.style.left=thickness;handle.style.right=thickness;}
   if(vertical){handle.style.height=thickness;handle.style[direction.includes("n")?"top":"bottom"]="0";}
   else{handle.style.top=thickness;handle.style.bottom=thickness;}
   handles.append(handle);
  }
  root.append(handles);
 }
 const restore=()=>{
  root.removeAttribute("data-window-resizing");root.style.removeProperty("--window-resize-cursor");
  const flags=desktop.wails?.flags;
  if(flags){flags.resizeEdge=undefined;root.style.cursor=flags.defaultCursor??"";}
 };
 const update=(event:MouseEvent)=>{
  const flags=desktop.wails?.flags;
  if(!flags)return;
  if(maximized||!flags.enableResize){restore();return;}
  const {clientX:x,clientY:y}=event, edge=flags.borderThickness;
  const horizontal=x<edge?"w":window.innerWidth-x<edge?"e":"";
  const vertical=y<edge?"n":window.innerHeight-y<edge?"s":"";
  const cursor=x>=0&&y>=0&&x<window.innerWidth&&y<window.innerHeight&&(horizontal||vertical)?`${vertical}${horizontal}-resize`:undefined;
  flags.resizeEdge=cursor;
  if(!cursor){restore();return;}
  root.dataset.windowResizing="true";
  root.style.setProperty("--window-resize-cursor",cursor);root.style.cursor=cursor;
 };
 // Installed after the runtime's move listener; capture recalculates on the
 // actual press as well, so resize cannot use a stale edge after maximizing.
 window.addEventListener("mousemove",update);
 window.addEventListener("mousedown",update,true);
 window.addEventListener("blur",restore);
 document.addEventListener("mouseleave",restore);
 restore();
 return()=>{window.removeEventListener("mousemove",update);window.removeEventListener("mousedown",update,true);window.removeEventListener("blur",restore);document.removeEventListener("mouseleave",restore);handles.remove();restore();};
}
