import {readNativeState} from "./native-state.mjs";
import net from "node:net";
import {readFile,writeFile,rename} from "node:fs/promises";
import {resolve} from "node:path";
import {clickPrivateCanvas} from "./native-probe-input.mjs";
import {execFile} from "node:child_process";
import {promisify} from "node:util";
const output=process.env.WINONE_NATIVE_PROBE_OUTPUT;
if(!output)throw Error("Set WINONE_NATIVE_PROBE_OUTPUT");
const ready=JSON.parse(await readFile(resolve(output,"worker-ready.json"),"utf8"));
let sequence=0;
async function request(method,params={}){
 const id=String(++sequence),payload=Buffer.from(JSON.stringify({version:1,id,method,params}));
 const prefix=Buffer.alloc(4);prefix.writeUInt32LE(payload.length);
 return await new Promise((resolve,reject)=>{
  const socket=net.createConnection("\\\\.\\pipe\\"+ready.pipe);let bytes=Buffer.alloc(0);
  socket.setTimeout(5000,()=>socket.destroy(new Error("Worker request timed out")));
  socket.on("error",reject);socket.on("connect",()=>socket.write(Buffer.concat([prefix,payload])));
  socket.on("data",chunk=>{
   bytes=Buffer.concat([bytes,chunk]);if(bytes.length<4)return;
   const length=bytes.readUInt32LE(0);if(length>1024*1024){socket.destroy(new Error("Oversized response"));return;}
   if(bytes.length<4+length)return;
   try{const response=JSON.parse(bytes.subarray(4,4+length));if(response.id!==id)throw Error("Mismatched response");socket.end();resolve(response);}catch(error){socket.destroy();reject(error);}
  });
 });
}
async function call(method,params){const r=await request(method,params);if(r.error)throw Error(r.error.message);return r.result;}
async function waitState(index){for(let i=0;i<150;i++){try{const s=(await readNativeState(resolve(output,`session-${index}/state.json`)));if(s.toolbarReadyMs!==null)return s;}catch(error){if(error.code!=="ENOENT")throw error;}await new Promise(r=>setTimeout(r,100));}throw Error("Native session did not appear");}
async function terminal(id){for(let i=0;i<100;i++){const job=await call("jobs.get",{id});if(job.state!=="running")return job;await new Promise(r=>setTimeout(r,50));}throw Error("Native job did not finish");}
const before=await call("snapshot");
const first=await call("capture.unified",{mode:"window",language:"zh-CN"});
const visible=await waitState(1);
if(visible.nativeProcess!==before.processId||visible.initializationStartedMs!==null)throw Error("Capture escaped its native worker");
if(process.env.WINONE_PROBE_EMPTY_SELECTION==="1"){
 if(visible.hasSelection||visible.nativeToolbarVisible)throw Error("Empty capture exposed output controls");
 if(process.env.WINONE_PROBE_TEST_WINDOW_REGIONS==="1"){
  const session=resolve(output,"session-1");
  await clickPrivateCanvas(session,200,200,"move");
  await clickPrivateCanvas(session,260,240,"move");
  let candidates;
  for(let i=0;i<100;i++){candidates=(await readNativeState(resolve(session,"state.json"))).windowCandidates;if(candidates[0]?.Width===250)break;await new Promise(r=>setTimeout(r,50));}
  if(candidates[0]?.Width!==250)throw Error("Native pointer did not refine interior regions");
  await clickPrivateCanvas(session,101,200,"move");
  candidates=(await readNativeState(resolve(session,"state.json"))).windowCandidates;
  if(candidates.length!==1||candidates[0].Width!==700)throw Error("Native pointer did not restore whole-window selection");
 }
 await clickPrivateCanvas(resolve(output,"session-1"),400,300,"double-message");
 let rejected;
 for(let i=0;i<100;i++){rejected=(await readNativeState(resolve(output,"session-1/state.json")));if(rejected.error)break;await new Promise(r=>setTimeout(r,50));}
 if(rejected.error!=="请先选择截图区域"||!rejected.selectionHintVisible||rejected.copied||rejected.saved)throw Error("Empty capture was exported or rejected without feedback");
 if((await call("jobs.get",{id:first.id})).state!=="running")throw Error("Empty confirmation ended the session");
 await clickPrivateCanvas(resolve(output,"session-1"),400,300,"right-message");
 if((await terminal(first.id)).state!=="cancelled")throw Error("Empty capture could not cancel");
 await call("shutdown");
 await writeFile(resolve(output,"empty-selection-verification.json"),JSON.stringify({controlsHidden:true,emptyOutputRejected:true,feedbackVisible:true,cancelled:true}));
 console.log("PASS: empty selection hides controls, rejects output and cancels");
}else{
let nextTheme;
if(process.env.WINONE_PROBE_TEST_THEME_SWITCH==="1"){
 if(!before.configurationManaged||visible.nativeTheme!==process.env.WINONE_PROBE_THEME)throw Error("Native UI ignored managed theme");
 nextTheme=visible.nativeTheme==="dark"?"light":"dark";
 const projection=resolve(output,"data/managed-settings.json"),temporary=projection+".next";
 await writeFile(temporary,JSON.stringify({...before.settings,revision:before.settings.revision+1,theme:nextTheme}));
 await rename(temporary,projection);await call("settings.reload");
 const active=(await readNativeState(resolve(output,"session-1/state.json")));
 if(active.nativeTheme!==visible.nativeTheme)throw Error("Reload changed the active screenshot theme");
}
if(!(await request("capture.unified",{mode:"window"})).error)throw Error("Concurrent capture was accepted");
if(!(await request("shutdown")).error)throw Error("Worker exited during editing");
await clickPrivateCanvas(resolve(output,"session-1"),400,300,"double-message");
const completed=await terminal(first.id);
if(completed.state!=="completed"||completed.result?.copied!==true)throw Error("Native completion was not reported");
const second=await call("capture.unified",{mode:"window"});const secondState=await waitState(2);
if(nextTheme&&secondState.nativeTheme!==nextTheme)throw Error("Next screenshot did not pick up managed theme");
await clickPrivateCanvas(resolve(output,"session-2"),400,300,"right-message");
const cancelled=await terminal(second.id),after=await call("snapshot");
if(cancelled.state!=="cancelled"||after.busy||after.processId!==before.processId)throw Error("Cancellation or worker reuse failed");
if((await call("jobs.get",{id:first.id})).state!=="completed")throw Error("New session lost previous result");
const copy=JSON.parse(await readFile(resolve(output,"copy.json"),"utf8"));
if(copy.copies!==1||copy.realClipboard)throw Error("Cancel changed copy output");
if(process.env.WINONE_PROBE_TEST_PIN==="1"){
 const pinSamples=[]; const pinCount=Number(process.env.WINONE_PROBE_PIN_COUNT||5);
 if(!Number.isInteger(pinCount)||pinCount<1||pinCount>100)throw Error("Invalid pin count");
 for(let iteration=0;iteration<pinCount;iteration++){
 const pinJob=await call("capture.unified",{mode:"window"}),pinState=await waitState(3+iteration);
 const button=pinState.nativeButtons.find(item=>item.command==="pin");
 if(!button?.enabled)throw Error("Pin action is unavailable");
 const target=resolve(output,"pin-target.json");await writeFile(target,JSON.stringify({...pinState,nativeWindow:button.window}));
 await promisify(execFile)("powershell.exe",["-NoProfile","-File",resolve("tests/capture-host-probe/click-private-window.ps1"),"-StateFile",target,"-X",String(button.x),"-Y",String(button.y)],{windowsHide:true});
 const pinned=await terminal(pinJob.id),snapshot=await call("snapshot");
 if(pinned.state!=="completed"||!pinned.result?.pinned||pinned.result.copied||snapshot.pins!==1||snapshot.busy)throw Error("Pin did not survive completion of the capture window");
 const closing=await call("pins.close");if((await terminal(closing.id)).state!=="completed"||(await call("snapshot")).pins!==0)throw Error("Pin did not release");
 if(JSON.parse(await readFile(resolve(output,"copy.json"),"utf8")).copies!==1)throw Error("Pin copied unexpectedly");
 pinSamples.push(JSON.parse((await promisify(execFile)("powershell.exe",["-NoProfile","-File",resolve("tests/capture-host-probe/measure-private-worker.ps1"),"-ReadyFile",resolve(output,"worker-ready.json")],{windowsHide:true})).stdout));
 }
 await writeFile(resolve(output,"pin-lifetime.json"),JSON.stringify(pinSamples));
 if(pinCount>=20&&new Set(pinSamples.slice(-8).map(item=>item.gdi)).size!==1)throw Error("Pin GDI objects did not stabilize");
 await writeFile(resolve(output,"pin-verification.json"),JSON.stringify({pinned:true,workerRetained:true,closed:true,clipboardUsed:false}));
}
const repeats=Number(process.env.WINONE_WORKER_REPEAT||0);
if(!Number.isInteger(repeats)||repeats<0||repeats>100)throw Error("Invalid repeat count");
if(repeats&&process.env.WINONE_PROBE_TEST_PIN==="1")throw Error("Run pin and repeated-capture scenarios separately");
const metrics=async index=>({...JSON.parse((await promisify(execFile)("powershell.exe",["-NoProfile","-File",resolve("tests/capture-host-probe/measure-private-worker.ps1"),"-ReadyFile",resolve(output,"worker-ready.json")],{windowsHide:true})).stdout),...JSON.parse(await readFile(resolve(output,`session-${index}/managed-metrics.json`),"utf8"))});
const observations=[];
async function persistObservations(complete){
 const temporary=resolve(output,"repeat-observations.next");
 await writeFile(temporary,JSON.stringify({complete,observations,scope:"Post-session worker counters; managed size is an estimate without forced GC; synthetic image excludes actual capture"},null,2));
 await rename(temporary,resolve(output,"repeat-observations.json"));
}
if(repeats){
 observations.push({session:2,...await metrics(2)});await persistObservations(false);
 for(let i=0;i<repeats;i++){
  const number=i+3,job=await call("capture.unified",{mode:"window"}),shown=await waitState(number);
  const cancel=i%2===1;
  await clickPrivateCanvas(resolve(output,`session-${number}`),400,300,cancel?"right-message":"double-message");
  const result=await terminal(job.id);
  if(result.state!==(cancel?"cancelled":"completed"))throw Error("Repeated capture did not complete");
  observations.push({session:number,firstPaintMs:shown.firstPaintMs,...await metrics(number)});await persistObservations(false);
 }
 const finalCopy=JSON.parse(await readFile(resolve(output,"copy.json"),"utf8"));
 if(finalCopy.copies!==1+Math.ceil(repeats/2))throw Error("Repeated captures duplicated or lost a copy");
 await persistObservations(true);
}
await call("shutdown");
await writeFile(resolve(output,"worker-verification.json"),JSON.stringify({processId:before.processId,completed:completed.state,cancelled:cancelled.state,copy,scope:"Real worker pipe and STA lifecycle; synthetic image and private-window messages"},null,2));
console.log("PASS: real worker native jobs, exclusivity, confirm, cancel, reuse and shutdown");
}
