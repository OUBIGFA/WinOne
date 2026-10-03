import {createRequire} from "node:module";
import {mkdir,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
const require=createRequire(import.meta.url),{chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||"_temp/updates-ui");await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});const report=[],errors=[];
try{
 const page=await browser.newPage();page.on("pageerror",error=>errors.push(String(error)));
 for(const language of ["zh-CN","en-US"])for(const width of [800,520])for(const scenario of ["unconfigured","available","current","failure"]){
  const en=language==="en-US";await page.setViewportSize({width,height:800});
  await page.goto(`${process.env.WINONE_LAYOUT_BASE||"http://127.0.0.1:43187"}/tests/updates-preview.html?language=${language}&case=${scenario}`);
  await page.getByText(en?"Current version: 1.0.0":"当前版本：1.0.0",{exact:true}).waitFor();
  const check=page.getByRole("button",{name:en?"Check for updates":"检查更新",exact:true});
  if(scenario==="unconfigured"){if(!await check.isDisabled())throw Error("Unconfigured check enabled");}
  else{
   await check.click();
   if(scenario==="available"){await page.getByRole("button",{name:en?"View release notes":"查看发行说明",exact:true}).click();if(await page.locator("#opened").textContent()!=="yes")throw Error("Release action failed");}
   if(scenario==="current")await page.getByText(en?"You're up to date":"已是最新版",{exact:true}).waitFor();
   if(scenario==="failure"){await page.getByRole("alert").waitFor();if(await page.getByText(en?"You're up to date":"已是最新版",{exact:true}).count())throw Error("Failure reported success");}
  }
  await page.waitForTimeout(250);await page.screenshot({path:resolve(output,`${language}-${width}-${scenario}.png`),fullPage:true});
  report.push({language,width,scenario,overflow:await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1)});
 }
 await writeFile(resolve(output,"report.json"),JSON.stringify({report,errors},null,2));if(errors.length||report.some(item=>item.overflow))throw Error("Update UI check failed");
 console.log("PASS: update source, available/current/error states and release action across 16 bilingual layouts");
}finally{await browser.close();}
