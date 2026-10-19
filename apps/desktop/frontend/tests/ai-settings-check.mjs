import {createRequire} from "node:module";
import {mkdir,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
const require=createRequire(import.meta.url);
const {chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||"_temp/ai-settings-layout");
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});
const report=[],errors=[];
try {
 const page=await browser.newPage();page.on("pageerror",error=>errors.push(String(error)));
 page.on("console",message=>{if(message.type()==="error")errors.push(message.text());});
 for(const language of ["zh-CN","en-US"])for(const theme of ["light","dark"])for(const width of [520,800,1280]){
  const zh=language==="zh-CN";
  await page.setViewportSize({width,height:820});
  const url=new URL(process.env.WINONE_LAYOUT_URL||"http://127.0.0.1:5192/tests/ai-settings-preview.html");
  url.searchParams.set("language",language);url.searchParams.set("theme",theme);url.searchParams.set("manyModels","1");
  await page.goto(url.href);
  const address=page.getByLabel(zh?"API 基础地址":"API base URL",{exact:true});await address.waitFor();
  await address.fill("https://example.invalid/preserved");
  await page.getByRole("tab",{name:zh?"账号供应商":"Account providers",exact:true}).click();
  await page.getByRole("tab",{name:zh?"自定义供应商":"Custom providers",exact:true}).click();
  if(await address.inputValue()!=="https://example.invalid/preserved")throw Error("Tab switch discarded provider input");
  await page.getByRole("button",{name:zh?"保存 AI 设置":"Save AI settings",exact:true}).click();
  for(const source of ["provider","account","defaults","capture","settings"]){
   if(source==="provider"){
    await page.getByRole("button",{name:zh?"获取模型列表":"Fetch model list",exact:true}).click();
    if(await page.getByRole("checkbox",{name:"discovered-model",exact:true}).isChecked())throw Error("Discovery enabled a model without a user selection");
    await page.getByLabel(zh?"搜索或填写模型 ID":"Search or enter a model ID",{exact:true}).fill("manual-model");
    await page.getByRole("button",{name:zh?"添加模型":"Add model",exact:true}).click();
    if(!await page.getByRole("checkbox",{name:"manual-model",exact:true}).isChecked())throw Error("Manual model was not enabled");
    const list=page.getByLabel(zh?"模型列表":"Models",{exact:true});await list.scrollIntoViewIfNeeded();
    const before=await list.boundingBox();
    await page.mouse.move(before.x+before.width-4,before.y+before.height-4);await page.mouse.down();
    await page.mouse.move(before.x+before.width-4,before.y+before.height+100,{steps:10});await page.mouse.up();
    const after=await list.boundingBox();if(after.height<before.height+60)throw Error("Model list could not be resized vertically");
   }
   if(source==="account"){
    await page.getByRole("tab",{name:zh?"账号供应商":"Account providers",exact:true}).click();
    await page.getByRole("button",{name:zh?"登录":"Sign in",exact:true}).click();
    await page.getByText("test@example.invalid",{exact:false}).waitFor();
   }
   if(source==="defaults"){
    await page.getByRole("tab",{name:zh?"默认 AI 服务":"Default AI service",exact:true}).click();
    if(await address.isVisible())throw Error("Default tab still exposes provider connection fields");
    await page.getByRole("switch",{name:zh?"显示思考过程":"Show reasoning",exact:true}).waitFor();
   }
   if(source==="capture")await page.getByRole("button",{name:zh?"截图":"Capture",exact:true}).click();
   if(source==="settings")await page.getByRole("button",{name:zh?"设置":"Settings",exact:true}).click();
   const overflow=await page.evaluate(()=>[...document.querySelectorAll('html,body,[data-slot="page-scroll"]')].filter(e=>e.clientWidth>0&&e.scrollWidth>e.clientWidth+1).map(e=>e.className||e.tagName));
   const clipped=await page.locator('[data-slot="page-content"]').evaluate(root=>[...root.querySelectorAll('button,input,textarea,[role="combobox"]')].filter(e=>{const r=e.getBoundingClientRect();return e.getAttribute("aria-hidden")!=="true"&&r.width>0&&r.height>0&&(r.left<0||r.right>window.innerWidth+1)}).map(e=>e.id||e.textContent));
   if(source==="settings"&&(await page.getByRole("tab").allTextContents()).at(-1)!==(zh?"关于与诊断":"About and diagnostics"))throw Error("Diagnostics is not in the last independent settings category");
   report.push({language,theme,width,source,overflow,clipped});
   await page.evaluate(()=>Promise.all(document.getAnimations().map(animation=>animation.finished.catch(()=>{}))));
   if(width===800){await page.locator('[data-slot="page-scroll"]').evaluate(element=>{element.scrollTop=0;});}if(width===800)await page.screenshot({path:resolve(output,`${language}-${theme}-${source}.png`),fullPage:true});
  }
 }
 await writeFile(resolve(output,"report.json"),JSON.stringify({report,errors},null,2));
 if(errors.length||report.some(r=>r.overflow.length||r.clipped.length))throw Error("AI settings layout failed; inspect report.json");
 console.log("PASS: independent AI services, provider/account paths, manual/discovered model choices and diagnostics; zh/en, light/dark, 520/800/1280px");
} finally {await browser.close();}
