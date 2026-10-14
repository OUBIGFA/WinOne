// Only launched by test-desktop-components.ps1 on its private desktop/profile.
import {createRequire} from "node:module";
import {writeFile,access} from "node:fs/promises";
import {resolve} from "node:path";
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2);
let browser,page;
try {
 const deadline=Date.now()+25000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(resolve=>setTimeout(resolve,100));}}
 if(!browser)throw Error("Isolated desktop did not become available");
 while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(page=>page.url().includes("winone.localhost"));if(page)break;await new Promise(resolve=>setTimeout(resolve,100));}
 if(!page)throw Error("Desktop document did not load");
 const checked=[];
 for(const [name,tab] of [["截图","截图后"],["划词助手","常规"],["置顶功能","常规设置"],["程序管理","程序列表"]]){
  await page.getByRole("button",{name,exact:true}).click();
  await page.getByRole("tab",{name:tab,exact:true}).waitFor();
  if(await page.getByRole("tab",{name:"诊断日志",exact:true}).count())throw Error(`Independent diagnostics tab remains: ${name}`);
  checked.push(name);
 }
 await page.getByRole("button",{name:"设置",exact:true}).click();
 await page.getByRole("tab",{name:"诊断日志",exact:true}).click();
 await page.getByRole("heading",{name:"WinOne 日志",exact:true}).waitFor();
 await page.getByRole("button",{name:"刷新日志",exact:true}).click();
 await page.getByLabel("WinOne 日志",{exact:true}).waitFor();
 const text=await page.getByLabel("WinOne 日志",{exact:true}).textContent();
 if(!text.includes('"source":"host"'))throw Error("Global log did not read the actual Host event");
 const profile=process.env.WINONE_COMPONENTS_PROFILE;
 for(const file of ["modules/capture/worker.log","modules/always-on-top/worker.log","modules/wintray/worker.log","modules/wintray/wintray.log","modules/selection/worker.log"]){
  let exists=true;try{await access(resolve(profile,file));}catch(error){if(error.code!=="ENOENT")throw error;exists=false;}
  if(exists)throw Error(`Independent diagnostic file created: ${file}`);
 }
 await page.getByLabel("WinOne 日志",{exact:true}).scrollIntoViewIfNeeded();
 await page.screenshot({path:resolve(output,"unified-logs.png")});
 await writeFile(resolve(output,"result.json"),JSON.stringify({moduleTabsRemoved:checked,globalLogVisible:true,hostEventRead:true,independentLogFilesAbsent:true},null,2));
 await page.evaluate(()=>{void window.go.main.App.WindowCommand("close");}).catch(error=>{if(!String(error).includes("closed"))throw error;});
}catch(error){await writeFile(resolve(output,"node-error.txt"),String(error.stack||error));if(page)await page.screenshot({path:resolve(output,"failure.png")}).catch(()=>{});throw error;}
finally{if(browser)await browser.close();}
