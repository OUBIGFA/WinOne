import {createRequire} from "node:module";
import {mkdir,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||"_temp/account-states");await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});
const report=[],errors=[];
try{
 for(const width of [520,1100])for(const theme of ["light","dark"])for(const language of ["zh-CN","en-US"]){
  const page=await browser.newPage({viewport:{width,height:900},reducedMotion:"reduce"});page.on("pageerror",e=>errors.push(String(e)));
  const en=language==="en-US",base=process.env.WINONE_LAYOUT_BASE||"http://127.0.0.1:5198";
  for(const state of ["new","code","callback","failure"]){
   await page.goto(base+"/tests/ai-settings-preview.html?language="+language+"&theme="+theme+"&login="+state);
   await page.getByRole("tab",{name:en?"Account providers":"账号供应商",exact:true}).click();
   if(state==="new"){
    await page.getByRole("button",{name:en?"Add account":"添加账号",exact:true}).click();
    const picker=page.getByRole("dialog",{name:en?"Choose account type":"选择账号类型",exact:true});
    await picker.getByRole("button",{name:"ChatGPT / Codex",exact:true}).click();
    await page.getByRole("button",{name:en?"Sign in":"登录",exact:true}).click();
    await page.locator('.settings-state-panel').waitFor();
    await page.locator('.settings-state-panel').scrollIntoViewIfNeeded();
   }else{
    await page.getByRole("button",{name:en?"Sign in":"登录",exact:true}).click();
    await page.locator('.settings-state-panel').waitFor();
    await page.locator('.settings-state-panel').scrollIntoViewIfNeeded();
   }
   const overflow=await page.locator('[data-slot="page-scroll"]').evaluate(e=>e.scrollWidth>e.clientWidth+1);
   const gap=await page.locator('.ai-account-section > h3').evaluateAll(es=>es.map(e=>e.nextElementSibling.getBoundingClientRect().top-e.getBoundingClientRect().bottom));
   report.push({width,theme,language,state,overflow,gap});
   await page.screenshot({path:resolve(output,[width,theme,language,state].join('-')+'.png')});
  }
  await page.close();
 }
 await writeFile(resolve(output,"report.json"),JSON.stringify({report,errors},null,2));
 if(errors.length||report.some(r=>r.overflow||r.gap.some(g=>g<15)))throw Error("Account state hierarchy failed");
 console.log('PASS: '+report.length+' account creation/sign-in states; labels, feedback and actions remain separated');
}finally{await browser.close();}
