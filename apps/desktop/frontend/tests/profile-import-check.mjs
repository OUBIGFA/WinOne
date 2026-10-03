import {createRequire} from "node:module";
import {mkdir,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
const require=createRequire(import.meta.url);
const {chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||"_temp/profile-import");
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});
const report=[],errors=[];
try{
 const page=await browser.newPage();page.on("pageerror",error=>errors.push(String(error)));
 for(const language of ["zh-CN","en-US"])for(const width of [1000,520]){
  const en=language==="en-US";await page.setViewportSize({width,height:1100});
  await page.goto(`${process.env.WINONE_LAYOUT_BASE||"http://127.0.0.1:43187"}/tests/profile-import-preview.html?language=${language}`);
  const choose=page.getByRole("button",{name:en?"Import configuration":"导入配置",exact:true}),confirm=page.getByRole("button",{name:en?"Confirm import":"确认导入",exact:true});
  await choose.click();if(!await confirm.isDisabled())throw Error("Unmapped import enabled");
  await page.getByRole("button",{name:en?"Set up local paths":"设置本机路径",exact:true}).click();
  await page.getByRole("button",{name:en?"Choose Capture folder":"选择截图保存目录",exact:true}).click();
  await page.getByText("D:\\Pictures\\WinOne",{exact:true}).waitFor();
  await page.getByRole("tab",{name:en?"Settings details":"详细设置",exact:true}).click();
  if(await page.getByRole("combobox").count()!==1)throw Error("Module details selector missing");
  await page.getByRole("tab",{name:en?"Import contents":"导入内容",exact:true}).click();
  const global=page.getByRole("checkbox",{name:en?"Theme, language, and sign-in settings":"主题、语言和登录设置",exact:true});
  await global.click();await page.getByRole("checkbox",{name:en?"Always on Top":"置顶功能",exact:true}).click();
  if(await page.getByText(en?/Disabled after import/:/导入后关闭/).count())throw Error("Unselected module still promises an import change");
  await choose.click();await page.getByText(/Cannot read selected file/).waitFor();
  if(await global.isChecked()||!await confirm.isDisabled())throw Error("Failed replacement lost choices or allowed unreviewed import");
  await page.getByText("C:\\Backups\\Home.winone",{exact:true}).waitFor();
  await page.waitForTimeout(250);await page.screenshot({path:resolve(output,`${language}-${width}.png`),fullPage:true});
  const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1);
  await page.getByRole("button",{name:en?"Review again":"重新预览",exact:true}).click();
  await confirm.click();await page.getByText(en?"Configuration imported":"配置已导入",{exact:true}).waitFor();
  const result=JSON.parse(await page.locator("#result").textContent());
  if(result.path!=="C:\\Backups\\Home.winone"||result.global!==false||result.modules.join()!=="capture"||result.bindings["capture.output"]!=="D:\\Pictures\\WinOne")throw Error("Confirmation applied different choices");
  report.push({language,width,overflow});
 }
 await writeFile(resolve(output,"report.json"),JSON.stringify({report,errors},null,2));
 if(errors.length||report.some(item=>item.overflow))throw Error("Import browser check failed");
 console.log("PASS: mapped import, details, retained review after replacement failure, and exact confirmation in 4 bilingual layouts");
}finally{await browser.close();}
