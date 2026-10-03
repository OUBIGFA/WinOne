import {readNativeState} from "./native-state.mjs";
import {createRequire} from "node:module";
import {readFile,readdir,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
import {clickPrivateCanvas} from "./native-probe-input.mjs";
const require=createRequire(import.meta.url),{chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=process.env.WINONE_NATIVE_PROBE_OUTPUT;if(!output)throw Error("Set WINONE_NATIVE_PROBE_OUTPUT");
const mode=process.env.WINONE_CONFIRM_CASE||"retry";
const browser=await chromium.connectOverCDP(process.env.WINONE_NATIVE_PROBE_CDP||"http://127.0.0.1:43192");
try{
 const page=browser.contexts()[0].pages()[0];
 const root=page.locator("[data-native-window]");await root.waitFor();
 const original=await root.getAttribute("data-native-window");
 if(mode==="retry"){
  await clickPrivateCanvas(output,400,300,"double-message");
  await page.getByText("复制未完成，请重试",{exact:true}).waitFor();
  const failed=(await readNativeState(resolve(output,"state.json")));
  if(failed.copied||failed.closeRequested||failed.nativeWindow!==original)throw Error("Copy failure lost the screenshot");
  const closed=page.waitForEvent("close",{timeout:10000});
  await clickPrivateCanvas(output,400,300,"duplicate-confirm");await closed;
 }else if(mode==="text"||mode==="empty-text"){
  await page.getByRole("button",{name:"文字",exact:true}).click();await clickPrivateCanvas(output,140,150);
  await page.getByRole("textbox",{name:"输入文字",exact:true}).fill(mode==="text"?"确认前提交":"   ");
  const closed=page.waitForEvent("close",{timeout:10000});
  await page.getByRole("button",{name:"确认",exact:true}).click();await closed;
 }else if(mode==="cancel"){
  const closed=page.waitForEvent("close",{timeout:10000});await clickPrivateCanvas(output,400,300,"right-message");await closed;
 }else throw Error("Unknown confirmation case");
 const files=await readdir(output);
 if(files.some(name=>/^capture-.*\.png$/.test(name)))throw Error("Confirm or cancel implicitly saved a screenshot");
 let receiver=null;
 if(mode==="cancel"){if(files.includes("copy-receiver.json"))throw Error("Cancel copied an image");}
 else{
  receiver=JSON.parse(await readFile(resolve(output,"copy-receiver.json"),"utf8"));
  if(receiver.realClipboard||receiver.copyAttempts!==(mode==="retry"?2:1)||receiver.annotations!==(mode==="text"?2:1)||receiver.width!==700||receiver.height!==400)throw Error("Incorrect confirmation output");
 }
 await writeFile(resolve(output,"confirmation-verification.json"),JSON.stringify({mode,receiver,scope:"Private-window messages and isolated image receiver; no system clipboard"},null,2));
 console.log("PASS: "+mode+" confirmation flow");
}finally{await browser.close();}
