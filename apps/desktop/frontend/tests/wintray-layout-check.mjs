// Explicit, isolated browser check; never connects to the user's browser.
// Provide WINONE_PLAYWRIGHT_MODULE if Playwright is installed outside this repo.
import {createRequire} from "node:module";
import {mkdir,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
const require=createRequire(import.meta.url);
const {chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||"_temp/wintray-layout");
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});
try{
 const page=await browser.newPage();const report=[];const errors=[];
 page.on("pageerror",error=>errors.push(String(error)));
 for(const width of [1200,800,520]){
  await page.setViewportSize({width,height:1100});
  await page.goto(process.env.WINONE_LAYOUT_URL||"http://127.0.0.1:5188/tests/wintray-preview.html");
  const row=page.getByRole("listitem",{name:"音乐播放器"});await row.waitFor();await row.click({position:{x:8,y:8}});
  if(await row.getAttribute("aria-current")!=="true")throw Error("Row background is not selectable");
  const actions=page.getByRole("group",{name:"程序操作"});const before=await actions.boundingBox();
  await page.getByRole("tab",{name:"更多设置",exact:true}).click();
  // Wait for shadcn's focus/selection transitions before measuring layout.
  await page.waitForTimeout(250);const after=await actions.boundingBox();
  if(!before||!after||Math.abs(before.y-after.y)>1)throw Error("Program actions moved between tabs");
  const overflow=await page.evaluate(()=>[...document.querySelectorAll(".wintray-workspace,.wintray-program-detail,[data-slot=resizable-panel]")].filter(element=>element.scrollWidth>element.clientWidth+1).map(element=>({width:element.clientWidth,scroll:element.scrollWidth})));
  report.push({width,overflow,before,after});await page.screenshot({path:resolve(output,`${width}.png`),fullPage:true});
 }
 await writeFile(resolve(output,"report.json"),JSON.stringify({report,errors},null,2));
 if(errors.length||report.some(result=>result.overflow.length))throw Error("Layout check failed; see report.json");
 console.log("PASS: whole-row selection, stable actions, and no horizontal overflow at 1200/800/520px");
}finally{await browser.close();}
