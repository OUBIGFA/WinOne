// Synthetic keyboard events exercise React/portal routing without modifying the
// system clipboard. The fixture records image-copy requests only in memory.
import {createRequire} from "node:module";
import {mkdir,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
const require=createRequire(import.meta.url);
const {chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||"_temp/capture-text-copy");
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});
const report=[],errors=[];
try {
 const page=await browser.newPage({viewport:{width:1400,height:900}});
 page.on("pageerror",error=>errors.push(String(error)));
 await page.addInitScript(()=>{window.__captureOutputs=[];window.addEventListener("capture-fixture-output",event=>window.__captureOutputs.push(event.detail));});
 const message="Copy this diagnostic text";
 const url=new URL(process.env.WINONE_LAYOUT_URL||"http://127.0.0.1:5188/tests/capture-editor.html");
 url.searchParams.set("feedback",message);
 await page.goto(url.toString());
 await page.waitForFunction(()=>{const image=document.querySelector(".capture-editor-canvas > img");return image?.complete&&image.naturalWidth>0;});
 const feedback=page.getByText(message,{exact:true});await feedback.waitFor();
 const selected=await feedback.evaluate(node=>{
  const popup=node.closest('[data-slot="popover-content"]');popup.focus();
  const selection=document.getSelection(),range=document.createRange();range.selectNodeContents(node);selection.removeAllRanges();selection.addRange(range);
  const allowed=popup.dispatchEvent(new KeyboardEvent("keydown",{key:"c",ctrlKey:true,bubbles:true,cancelable:true}));
  return {selected:selection.toString(),defaultAllowed:allowed};
 });
 report.push({step:"text",...selected});
 if(selected.selected!==message||!selected.defaultAllowed)throw Error("Editor intercepted text copying");
 await page.waitForTimeout(150);
 if(await page.evaluate(()=>window.__captureOutputs.length)!==0)throw Error("Text selection triggered image copy");
 await page.screenshot({path:resolve(output,"selected-text.png")});
 const imageCopy=await feedback.evaluate(node=>{
  document.getSelection().removeAllRanges();
  return node.closest('[data-slot="popover-content"]').dispatchEvent(new KeyboardEvent("keydown",{key:"c",ctrlKey:true,bubbles:true,cancelable:true}));
 });
 if(imageCopy)throw Error("Image-copy shortcut was not handled");
 await page.waitForFunction(()=>window.__captureOutputs.length===1);
 const result=await page.evaluate(()=>window.__captureOutputs[0]);
 if(result.action!=="copy")throw Error("Wrong image action");
 report.push({step:"image",action:result.action});
 if(errors.length)throw Error(errors.join("\n"));
 console.log("PASS: selected popup text retains native copy; clearing the selection restores image copy");
} finally {
 await writeFile(resolve(output,"report.json"),JSON.stringify({report,errors},null,2));
 await browser.close();
}
