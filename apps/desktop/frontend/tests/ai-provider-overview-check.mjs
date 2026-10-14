import {createRequire} from "node:module";
import {mkdir,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
const require=createRequire(import.meta.url);
const {chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||"_temp/ai-provider-overview");
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,...(process.env.WINONE_CHROMIUM?{executablePath:process.env.WINONE_CHROMIUM}:{channel:'msedge'})});
const report=[],errors=[];
try{
 const page=await browser.newPage();page.on("pageerror",error=>errors.push(String(error)));
 for(const language of ["zh-CN","en-US"])for(const theme of ["light","dark"])for(const width of [520,800,1280])for(const many of [false,true]){
  const zh=language==="zh-CN";
  await page.setViewportSize({width,height:820});
  const url=new URL(process.env.WINONE_LAYOUT_URL||"http://127.0.0.1:5191/tests/ai-settings-preview.html");
  url.searchParams.set("language",language);url.searchParams.set("theme",theme);if(many)url.searchParams.set("manyProviders","1");
  await page.goto(url.href);
  const overview=page.getByRole("region",{name:zh?"已启用供应商":"Enabled providers",exact:true});await overview.waitFor();
  const layout=await overview.evaluate(root=>{
   const bounds=root.getBoundingClientRect();
   const buttons=[...root.querySelectorAll("button")].map(button=>{const rect=button.getBoundingClientRect();return {label:button.getAttribute("aria-label"),left:rect.left,right:rect.right,height:rect.height};});
   return {height:bounds.height,left:bounds.left,right:bounds.right,buttons,overflow:document.documentElement.scrollWidth>innerWidth+1};
  });
  if(layout.overflow||layout.buttons.some(button=>button.left<layout.left-1||button.right>layout.right+1||button.height<30||button.height>38))throw Error(`Provider summary controls overflow: ${JSON.stringify({language,theme,width,many,layout})}`);
  if(!many&&layout.height>120)throw Error(`Two-provider summary uses excessive height: ${JSON.stringify({language,theme,width,layout})}`);
  if(!many&&width===1280&&layout.height>50)throw Error(`Desktop provider summary should fit one compact row: ${JSON.stringify({language,theme,width,layout})}`);
  const tabs=page.getByRole("tablist",{name:zh?"AI 服务":"AI services",exact:true});const before=await tabs.boundingBox();
  await page.getByRole("tab",{name:zh?"默认 AI 服务":"Default AI service",exact:true}).click();
  const after=await tabs.boundingBox();if(Math.abs(before.y-after.y)>1)throw Error("Changing service tabs moved the provider summary/navigation");
  await overview.getByRole("button").first().click();
  await page.getByLabel(zh?"API 基础地址":"API base URL",{exact:true}).waitFor();
  report.push({language,theme,width,many,...layout});
  if(language==="zh-CN"&&width===800){await page.locator('[data-slot="page-scroll"]').evaluate(element=>{element.scrollTop=0;});await page.screenshot({path:resolve(output,`${theme}-${many?"many":"two"}.png`)});}
 }
 await writeFile(resolve(output,"report.json"),JSON.stringify({report,errors},null,2));
 if(errors.length)throw Error(errors.join("\n"));
 console.log(`PASS: ${report.length} provider summary layouts, stable tabs and provider navigation`);
}finally{await browser.close();}
