// Real browser input on isolated fixtures; no native settings are changed.
import {createRequire} from "node:module";
import {mkdir,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
const require=createRequire(import.meta.url);
const {chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||"_temp/module-number-fields");
const base=process.env.WINONE_LAYOUT_BASE||"http://127.0.0.1:43187";
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});
const report=[],errors=[];
try {
 const page=await browser.newPage();page.on("pageerror",error=>errors.push(String(error)));
 for(const language of ["zh-CN","en-US"])for(const width of [800,520])for(const module of ["capture","pin"]){
  const en=language==="en-US";
  await page.setViewportSize({width,height:1000});
  await page.goto(`${base}/tests/${module==="capture"?"capture-settings-preview":"module-preview"}.html?language=${language}`);
  if(module==="capture")await page.getByRole("tab",{name:en?"Scrolling capture settings":"滚动截图设置",exact:true}).click();
  const input=page.locator(module==="capture"?"#capture-scroll-delay":"#aot-thickness");
  await input.fill("");
  if(await input.inputValue()!==""||await input.getAttribute("aria-invalid")!=="true")throw Error("Empty numeric input was not retained and marked");
  const errorId=await input.getAttribute("aria-describedby");
  if(!errorId||!await page.locator(`[id="${errorId}"]`).isVisible())throw Error("Field error is not associated or visible");
  const save=page.getByRole("button",{name:en?"Save settings":"保存设置",exact:true});
  if(!await save.isDisabled())throw Error("Invalid number can be saved");
  await page.waitForTimeout(250);await page.screenshot({path:resolve(output,`${module}-${language}-${width}.png`),fullPage:true});
  if(module==="capture"){
   await page.getByRole("tab",{name:en?"After capture":"截图后",exact:true}).click();
   await page.getByRole("button",{name:en?"Check scroll interval":"检查滚动间隔",exact:true}).click();
   await page.waitForFunction(()=>document.activeElement?.id==="capture-scroll-delay");
  }
  await input.fill(module==="capture"?"500":"4");
  if(await save.isDisabled()||await input.getAttribute("aria-invalid")!=="false")throw Error("Correction did not restore saving");
  await save.click();await page.getByText(en?"Saved":"已保存",{exact:true}).waitFor();
  const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1);
  report.push({module,language,width,overflow});
 }
 await writeFile(resolve(output,"report.json"),JSON.stringify({report,errors},null,2));
 if(errors.length||report.some(item=>item.overflow))throw Error("Numeric form check failed");
 console.log("PASS: capture and pin numeric fields, associated errors, correction focus, and save in 8 bilingual layouts");
}finally{await browser.close();}
