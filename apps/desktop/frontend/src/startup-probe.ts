interface ProbePort {nonce():Promise<string>;complete(nonce:string):Promise<void>;surface?(nonce:string):Promise<void>}

// Only a maintenance probe loads the editor chunk here. Normal startup keeps
// the editor lazy, while the probe waits for the real shell and local assets.
export async function finishStartupProbe(port:ProbePort,doc:Document,loadEditor:()=>Promise<unknown>=()=>import("./features/capture-editor")) {
 const nonce=await port.nonce();
 if(!nonce)return;
 const shell=doc.querySelector<HTMLElement>(".shell");
 const logo=doc.querySelector<HTMLImageElement>(".mark");
 if(!shell||!logo||!doc.querySelector('nav [aria-current="page"]')||doc.defaultView?.getComputedStyle(shell).display!=="flex")throw Error("Startup layout is not ready");
 const controls=doc.querySelectorAll<HTMLButtonElement>('[data-slot="window-controls"] button');
 if(controls.length!==3||Array.from(controls).some(button=>button.disabled))throw Error("Window controls are not ready");
 await logo.decode();
 if(logo.naturalWidth===0)throw Error("Startup logo is unavailable");
 await loadEditor();
 await port.surface?.(nonce);
 await port.complete(nonce);
}
