// Keyboard integration against the production editor and native fixture renderer.
// Output callbacks are simulated; this never saves to the user's image folder.
import {createRequire} from "node:module";
import {mkdir,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
const require=createRequire(import.meta.url);
const {chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||"_temp/capture-save-shortcuts");
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});
const report=[],errors=[];
try {
 const page=await browser.newPage({viewport:{width:1400,height:900}});
 page.on("pageerror",error=>errors.push(String(error)));
 await page.addInitScript(()=>{
  window.__captureOutputs=[];
  window.addEventListener("capture-fixture-output",event=>window.__captureOutputs.push(event.detail));
 });
 for(const [action,chord]of [["save","Control+s"],["saveAs","Control+Shift+s"]]) {
  const renders=[];
  const observe=request=>{if(request.url().endsWith("/fixture/render")&&request.method()==="POST")renders.push(request.postDataJSON());};
  page.on("request",observe);
  await page.goto(process.env.WINONE_LAYOUT_URL||"http://127.0.0.1:5188/tests/capture-editor.html");
  await page.waitForFunction(()=>{const image=document.querySelector(".capture-editor-canvas > img");return image?.complete&&image.naturalWidth>0;});
  await page.getByRole("button",{name:"文字",exact:true}).click();
  await page.getByRole("group",{name:"标注画布"}).click({position:{x:100,y:80}});
  const input=page.getByRole("textbox",{name:"输入文字"});
  const text=`${action} 第一行\n第二行`;
  await input.fill(text);
  await page.screenshot({path:resolve(output,`${action}-typing.png`)});
  await page.keyboard.press(chord);
  await page.waitForFunction(()=>window.__captureOutputs.length===1);
  const saved=await page.evaluate(()=>window.__captureOutputs[0]);
  report.push({action,chord,text,renderCount:renders.length,document:saved.document,renders});
  if(saved.action!==action||saved.document.marks[0]?.text!==text)throw Error("Keyboard output omitted the active text");
  // The raster request upgrades the schema and omits the crop mask; compare
  // image coordinates and marks, which must remain identical in saved output.
  if(!renders.some(document=>document.width===saved.document.width&&document.height===saved.document.height&&JSON.stringify(document.marks)===JSON.stringify(saved.document.marks)))throw Error("Saved marks differ from the native render request");
  await page.getByRole("button",{name:"打开编辑器"}).waitFor();
  page.off("request",observe);
 }
 if(errors.length)throw Error(errors.join("\n"));
 console.log("PASS: Ctrl+S and Ctrl+Shift+S include active multiline text and wait for the native renderer");
} finally {
 await writeFile(resolve(output,"report.json"),JSON.stringify({report,errors},null,2));
 await browser.close();
}
