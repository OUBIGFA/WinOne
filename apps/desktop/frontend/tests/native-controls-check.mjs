import {readNativeState} from "./native-state.mjs";
import {readFile,writeFile,readdir} from "node:fs/promises";
import {resolve,basename} from "node:path";
import {spawnSync} from "node:child_process";
import {clickPrivateCanvas} from "./native-probe-input.mjs";
const output=process.env.WINONE_NATIVE_PROBE_OUTPUT;
if(!output)throw Error("Set WINONE_NATIVE_PROBE_OUTPUT");
const state=async()=>(await readNativeState(resolve(output,"state.json")));
async function until(predicate){for(let i=0;i<150;i++){const s=await state();if(predicate(s))return s;await new Promise(r=>setTimeout(r,100));}throw Error("Native state did not reach expected condition");}
async function button(command){
 const s=await state(),b=s.nativeButtons?.find(b=>b.command===command);
 if(!b?.enabled||b.window==="0")throw Error("Native button unavailable: "+command);
 const file=resolve(output,"button-target.json");await writeFile(file,JSON.stringify({...s,nativeWindow:b.window}));
 const child=spawnSync("powershell.exe",["-NoProfile","-File",resolve("tests/capture-host-probe/click-private-window.ps1"),"-StateFile",file,"-X",String(b.x),"-Y",String(b.y)],{encoding:"utf8",windowsHide:true});
 if(child.status!==0)throw Error(child.stderr||child.stdout);
}
const initial=await state();
if(!initial.nativeControls||initial.initializationStartedMs!==null||initial.toolbarRoot!==initial.nativeWindow)throw Error("Toolbar still depends on browser startup or another window");
const gripFile=resolve(output,"grip-target.json");
await writeFile(gripFile,JSON.stringify({...initial,nativeWindow:initial.nativeGrip.window}));
const drag=spawnSync("powershell.exe",["-NoProfile","-File",resolve("tests/capture-host-probe/click-private-window.ps1"),"-StateFile",gripFile,"-X",String(initial.nativeGrip.x),"-Y",String(initial.nativeGrip.y),"-Action","drag"],{encoding:"utf8",windowsHide:true});
if(drag.status!==0)throw Error(drag.stderr||drag.stdout);
await until(s=>s.toolbar.x===initial.toolbar.x+50&&s.toolbar.y===initial.toolbar.y+20);
for(const tool of ["ellipse","pen","cover","rectangle"]){await button(tool);await until(s=>s.selectedTool===tool);}
await button("undo");await until(s=>s.annotations===0);
await button("redo");await until(s=>s.annotations===1);
await button("save");await until(s=>s.saved===1);
await button("text");await until(s=>s.textTool);
await clickPrivateCanvas(output,140,150);
await until(s=>s.nativeTextWindow);
async function textMessage(action,text=""){
 const s=await state();if(!s.nativeTextWindow)throw Error("Native input unavailable");
 const file=resolve(output,"text-target.json");await writeFile(file,JSON.stringify({...s,nativeWindow:s.nativeTextWindow}));
 const child=spawnSync("powershell.exe",["-NoProfile","-File",resolve("tests/capture-host-probe/click-private-window.ps1"),"-StateFile",file,"-X","0","-Y","0","-Action",action,"-Text",text],{encoding:"utf8",windowsHide:true});
 if(child.status!==0)throw Error(child.stderr||child.stdout);
}
await textMessage("set-text",Array(30).fill("超出选区的文字").join("\r\n"));
await button("save");
const rejected=await until(s=>s.error==="文字超出截图范围，请缩短内容");
if(!rejected.nativeTextWindow||rejected.saved!==1||rejected.annotations!==1)throw Error("Overflow silently saved or lost the edit");
await textMessage("set-text","原生工具栏\r\n直接编辑");
await until(s=>s.nativeTextValue==="原生工具栏\r\n直接编辑");
await button("save");await until(s=>s.annotations===2&&s.saved===2&&!s.nativeTextWindow);
if(process.env.WINONE_PROBE_SAVE_AS_CASES==="1"){
 await button("saveAs");await until(s=>s.saveAsAttempts===1&&s.savedAs===0&&!s.error);
 await button("saveAs");const failed=await until(s=>s.saveAsAttempts===2&&s.error);
 if(failed.savedAs!==0||failed.annotations!==2||failed.closeRequested)throw Error("Failed save-as lost the screenshot");
 await button("saveAs");await until(s=>s.savedAs===1&&!s.error);
 const result=JSON.parse(await readFile(resolve(output,"save-as-output.json"),"utf8"));
 if(result.copied||result.pinned||basename(result.path)!=="chosen-copy.png")throw Error("Save-as repeated another action");
}
await button("text");await clickPrivateCanvas(output,145,155);
await until(s=>s.nativeTextValue==="原生工具栏\r\n直接编辑");
await textMessage("set-text","取消此次修改");await textMessage("escape-key");await until(s=>!s.nativeTextWindow);
await button("undo");await until(s=>s.annotations===1);
await button("redo");await until(s=>s.annotations===2);
await button("confirm");const final=await until(s=>s.closeRequested);
const receiver=JSON.parse(await readFile(resolve(output,"copy-receiver.json"),"utf8"));
if(receiver.annotations!==2||receiver.copyAttempts!==1||receiver.realClipboard||final.initializationStartedMs!==null||final.nativeWindow!==initial.nativeWindow)throw Error("Native confirmation or browser-free session failed");
if(process.env.WINONE_PROBE_SHARED_OUTPUT==="1"){
 const outputs=JSON.parse(await readFile(resolve(output,"backend-output.json"),"utf8"));
 if(outputs.length!==2||new Set(outputs.map(item=>item.path)).size!==2)throw Error("Product output count or unique naming failed");
 for(const item of outputs){
  if(!/^WinOne-\d{14}-OK(?:-\d+)?\.png$/.test(basename(item.path))||item.width!==700||item.height!==400)throw Error("Product naming or dimensions were not used");
  const png=await readFile(item.path);
  if(png.readUInt32BE(16)!==700||png.readUInt32BE(20)!==400)throw Error("Saved PNG dimensions differ from metadata");
 }
 if((await readdir(output)).some(name=>/^capture-.*\.png$/.test(name)))throw Error("UI used its test-only save path");
}
await writeFile(resolve(output,"native-controls-verification.json"),JSON.stringify({nativeWindow:initial.nativeWindow,toolbarRoot:initial.toolbarRoot,launchToToolbarReadyMs:initial.launchToToolbarReadyMs,browserInitialized:false,receiver,scope:"Private native child-window messages and isolated receiver; no hardware input or IME"},null,2));
console.log("PASS: browser-free native toolbar, text, edit cancellation, history and confirmation");
