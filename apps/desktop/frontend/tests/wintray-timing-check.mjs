// Uses only the in-memory preview and an isolated headless browser.
import {createRequire} from "node:module";
import {mkdir,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
const require=createRequire(import.meta.url);
const {chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||"_temp/wintray-timing");
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});
const report=[],errors=[];
try {
 const page=await browser.newPage();page.on("pageerror",error=>errors.push(String(error)));
 page.on("console",message=>{if(message.type()==="error")errors.push(message.text());});
 // The standalone fixture has no favicon; avoid an unrelated browser request.
 await page.route("**/favicon.ico",route=>route.fulfill({status:204,body:""}));
 for(const language of ["zh-CN","en-US"])for(const width of [800,520]){
  const en=language==="en-US";
  await page.setViewportSize({width,height:1000});
  const url=new URL(process.env.WINONE_LAYOUT_URL||"http://127.0.0.1:43187/tests/wintray-preview.html");url.searchParams.set("language",language);
  url.searchParams.set("friendlyNames","1");await page.goto(url.href);
  const save=page.getByRole("button",{name:en?"Save changes":"保存修改",exact:true});
  await page.getByRole("tab",{name:en?"Startup settings":"启动设置",exact:true}).click();
  const interval=page.getByRole("spinbutton",{name:en?"Delay between programs (seconds)":"程序之间的启动间隔（秒）"});
  for(const value of ["","121","-1","1.5"]){
   await interval.fill(value);
   if(await interval.inputValue()!==value||await interval.getAttribute("aria-invalid")!=="true"||!await save.isDisabled())throw Error(`Invalid input was changed or accepted: ${value}`);
  }
  await interval.fill("8");await save.click();await page.getByText(en?"Saved":"已保存",{exact:true}).waitFor();
  await page.getByRole("tab",{name:en?"Programs":"程序列表",exact:true}).click();
  const delay=page.getByRole("spinbutton",{name:en?"Start delay (minutes)":"延后启动（分钟）"});
  await delay.fill("");
  const schedule=page.getByRole("switch",{name:en?"Custom schedule":"自定义时间",exact:true});
  await schedule.click();if(!await save.isDisabled())throw Error("Hidden unfinished timing was accepted");
  await schedule.click();if(await delay.inputValue()!=="")throw Error("Unfinished input was discarded");
  await delay.fill("25");if(await save.isDisabled())throw Error("Valid correction did not allow save");
  await page.getByRole("tab",{name:en?"More settings":"更多设置",exact:true}).click();
  await page.getByRole("textbox",{name:en?"Name":"名称",exact:true}).fill(en?"LongProgramNameWithoutSpacesForLayoutVerification".repeat(3):"这是一个用于检查换行和布局的很长程序名称".repeat(3));
  const overflow=await page.evaluate(()=>[...document.querySelectorAll(".wintray-workspace,.wintray-program-detail,[data-slot=resizable-panel]")].filter(el=>el.scrollWidth>el.clientWidth+1).map(el=>({width:el.clientWidth,scroll:el.scrollWidth})));
  await page.waitForTimeout(250);
  await page.screenshot({path:resolve(output,`${language}-${width}.png`),fullPage:true});
  report.push({language,width,overflow});
  const name=page.getByRole("textbox",{name:en?"Name":"名称",exact:true});
  await name.fill("");await name.pressSequentially("example0");
  if(await name.inputValue()!=="example0")throw Error("Custom name was replaced by detected metadata");
  await save.click();await page.getByText(en?"Saved":"已保存",{exact:true}).waitFor();
  if(await name.inputValue()!=="example0")throw Error("Saved custom name changed");
  await name.fill("");if(!await save.isDisabled())throw Error("Empty program name was accepted");
  await page.getByRole("tab",{name:en?"General":"常用设置",exact:true}).click();
  await page.getByRole("tab",{name:en?"More settings":"更多设置",exact:true}).click();
  if(await name.inputValue()!=="")throw Error("Tab change restored an unfinished name");
 }
 await writeFile(resolve(output,"report.json"),JSON.stringify({report,errors},null,2));
 if(errors.length||report.some(item=>item.overflow.length))throw Error("Program timing/layout check failed");
 console.log("PASS: bilingual numeric editing, validation, hidden-input recovery, and long program names at 800/520px");
}finally{await browser.close();}
