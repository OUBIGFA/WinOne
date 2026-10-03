// Delayed in-memory picker response; never opens a system file dialog.
import {createRequire} from "node:module";
import {mkdir,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
const require=createRequire(import.meta.url),{chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||"_temp/capture-folder");await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});const report=[],errors=[];
try{
 const page=await browser.newPage();page.on("pageerror",error=>errors.push(String(error)));
 for(const language of ["zh-CN","en-US"])for(const width of [800,520])for(const outcome of ["selected","cancelled","failed"]){
  const en=language==="en-US";await page.setViewportSize({width,height:1000});await page.goto(`${process.env.WINONE_LAYOUT_BASE||"http://127.0.0.1:43187"}/tests/capture-settings-preview.html?folderTest=1&language=${language}`);
  const choose=page.getByRole("button",{name:en?"Choose folder":"选择目录",exact:true}),save=page.getByRole("button",{name:en?"Save settings":"保存设置",exact:true});
  await choose.click();if(!await choose.isDisabled()||!await save.isDisabled())throw Error("Directory selection did not block concurrent actions");
  await choose.evaluate(button=>button.click());if(await page.evaluate(()=>document.documentElement.dataset.folderCalls)!=="1")throw Error("Duplicate picker opened");
  await page.evaluate(outcome=>document.dispatchEvent(new CustomEvent("fixture:folder-result",{detail:outcome==="selected"?{path:"D:\\Pictures\\WinOne"}:outcome==="failed"?{error:"Folder selection unavailable"}:{path:""}})),outcome);
  await page.waitForFunction(()=>!document.querySelector('button[aria-describedby="capture-path"]').matches(":disabled"));
  const path=await page.locator("#capture-path").textContent();
  if(outcome==="selected"){if(path!=="D:\\Pictures\\WinOne"||await save.isDisabled())throw Error("Selected folder was not retained as an unsaved edit");}
  else if(path!=="C:\\Pictures\\WinOne"||!await save.isDisabled())throw Error("Cancelled/failed picker changed preferences");
  if(outcome==="failed")await page.getByRole("alert").waitFor();
  await page.waitForTimeout(250);await page.screenshot({path:resolve(output,`${language}-${width}-${outcome}.png`),fullPage:true});
  report.push({language,width,outcome,overflow:await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1)});
 }
 await writeFile(resolve(output,"report.json"),JSON.stringify({report,errors},null,2));if(errors.length||report.some(item=>item.overflow))throw Error("Folder interaction check failed");
 console.log("PASS: folder selection, cancellation, failure and duplicate protection in 12 bilingual layouts");
}finally{await browser.close();}
