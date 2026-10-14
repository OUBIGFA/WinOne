// WinOne transport adaptation for Cindy's Apache-2.0 browser comment preload.
// This narrow channel reports page evidence only; it exposes no host operations.
const listeners=new Map<string,(_:unknown,payload:unknown)=>void>();
export const ipcRenderer={
 on:(channel:string,handler:(_:unknown,payload:unknown)=>void)=>{listeners.set(channel,handler);},
 sendToHost:(channel:string,payload?:unknown)=>{
  if(!channel.startsWith("browser-comment:"))return;
  (window as unknown as {chrome?:{webview?:{postMessage:(value:string)=>void}}}).chrome?.webview?.postMessage(JSON.stringify({kind:"comment",channel,payload}));
 }
};
Object.defineProperty(window,"__winoneBrowserComment",{value:(channel:string,payload:unknown)=>listeners.get(channel)?.(null,payload)});
