import {createRequire} from "node:module";
import {mkdir,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
const require=createRequire(import.meta.url),{chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||"_temp/capture-single-surface");await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});
const report=[],errors=[];
try{
 for(const action of ["confirm","fast-confirm","cancel"]){
  const page=await browser.newPage({viewport:{width:1280,height:800}});page.on("pageerror",error=>errors.push(String(error)));
  await page.addInitScript(()=>{window.outputs=[];window.addEventListener("capture-fixture-output",event=>window.outputs.push(event.detail));});
  await page.goto(`${process.env.WINONE_LAYOUT_BASE||"http://127.0.0.1:43187"}/tests/capture-editor.html?surface=1&picking=1`);
  const canvas=page.locator(".capture-editor-canvas");await canvas.waitFor();
  await canvas.locator("img").first().evaluate(image=>image.decode());
  await canvas.evaluate(node=>{window.selectionCanvas=node;window.selectionImage=node.querySelector("img");});
  if(await page.getByRole("button",{name:"矩形",exact:true}).isVisible())throw Error("Drawing toolbar appeared before selection");
  if(action!=="cancel"){
   await page.mouse.move(100,120);await page.mouse.down();await page.mouse.move(600,420,{steps:4});await page.mouse.up();
   if(action==="fast-confirm")await page.mouse.dblclick(500,350);
   else{
    await page.getByRole("button",{name:"确认",exact:true}).waitFor();
    if(!await page.evaluate(()=>window.selectionCanvas===document.querySelector(".capture-editor-canvas")&&window.selectionImage===document.querySelector(".capture-editor-canvas img")))throw Error("Selecting replaced the screenshot canvas or image");
    await page.screenshot({path:resolve(output,"selected.png")});
    await page.mouse.dblclick(500,350);
   }
   await page.waitForFunction(()=>window.outputs.length===1);
   const events=await page.evaluate(()=>window.outputs);
   if(events[0].action!=="confirm"||events[0].document.marks.length!==0)throw Error("Double-click saved a file or added a mark");
   const crop=events[0].document.crop;
   if(crop.x!==250||crop.y!==300||crop.width!==1250||crop.height!==750)throw Error(`Wrong selected output: ${JSON.stringify(crop)}`);
  }else await page.mouse.click(500,350,{button:"right"});
  await page.getByRole("button",{name:"打开编辑器",exact:true}).waitFor();
  report.push({action,events:await page.evaluate(()=>window.outputs)});await page.close();
 }
 await writeFile(resolve(output,"report.json"),JSON.stringify({report,errors},null,2));
 if(errors.length)throw Error(errors.join("\n"));
 console.log("PASS: one canvas, toolbar after selection, normal/fast double-click confirm once, right-click cancellation");
}finally{await browser.close();}
