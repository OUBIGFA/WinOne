// Isolated browser check of the real page components with in-memory fixtures.
import {createRequire} from "node:module";
import {mkdir,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
const require=createRequire(import.meta.url);
const {chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||"_temp/module-layout");
const base=process.env.WINONE_LAYOUT_BASE||"http://127.0.0.1:5188";
const theme=process.env.WINONE_LAYOUT_THEME||"light";
if(!["light","dark"].includes(theme))throw Error("WINONE_LAYOUT_THEME must be light or dark");
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});
const report=[],errors=[];
try {
 const page=await browser.newPage();
 await page.emulateMedia({colorScheme:theme});
 const navigate=async url=>{await page.goto(url);await page.evaluate(theme=>{document.documentElement.dataset.theme=theme;},theme);};
 page.on("pageerror",error=>errors.push(String(error)));
 const pages=[
  ["home","shell-preview","首页","Home"],
  ["capture","capture-settings-preview","截图","Capture"],
  ["pin","module-preview","置顶功能","Always on Top"],
  ["programs","wintray-preview","程序管理","Program management"],
 ];
 for(const language of ["zh-CN","en-US"])for(const width of [1280,800,520])for(const [name,path,zh,en]of pages) {
  await page.setViewportSize({width,height:1000});
  await navigate(`${base}/tests/${path}.html?language=${language}`);
  await page.getByRole("heading",{name:language==="zh-CN"?zh:en,exact:true}).first().waitFor();
  await page.evaluate(()=>document.fonts.ready);
  await page.waitForTimeout(300);
  const result=await page.evaluate(()=>({
   language:document.documentElement.lang,
   overflow:document.documentElement.scrollWidth>innerWidth+1,
   tabs:[...document.querySelectorAll('[role="tablist"]')].map(node=>{const rect=node.getBoundingClientRect();return {labels:node.textContent,right:rect.right,overflow:rect.right>innerWidth+1};}),
   alerts:[...document.querySelectorAll('[role="alert"]')].map(node=>node.textContent),
  }));
  if(result.language!==language)throw Error(`Wrong fixture language: ${name}/${language}`);
  if(name==="home") {
   const overlapping=await page.locator('.dashboard-tools [data-slot="item"]').evaluateAll(items=>items.filter(item=>{
    const title=item.querySelector('[data-slot="item-title"]')?.getBoundingClientRect();
    const actions=item.querySelector('[data-slot="item-actions"]')?.getBoundingClientRect();
    return title&&actions&&Math.min(title.right,actions.right)-Math.max(title.left,actions.left)>1&&Math.min(title.bottom,actions.bottom)-Math.max(title.top,actions.top)>1;
   }).map(item=>item.textContent));
   if(overlapping.length)throw Error(`Home title overlaps its actions: ${language}/${width}: ${overlapping.join(", ")}`);
  }
  if(name==="home"&&width===1280) {
   const handle=page.locator('.dashboard-panels [role="separator"]');
   const first=page.locator('.dashboard-panels > [data-slot="resizable-panel"]').first();
   const before=await first.boundingBox();await handle.focus();await page.keyboard.press("ArrowRight");
   await page.waitForTimeout(150);const after=await first.boundingBox();
   result.resized=!!before&&!!after&&Math.abs(before.width-after.width)>1;
   if(!result.resized)throw Error("Home keyboard resize failed");
  }
  report.push({name,width,...result});
  await page.screenshot({path:resolve(output,`${name}-${language}-${width}.png`),fullPage:true});
 }
 for(const language of ["zh-CN","en-US"])for(const width of [1280,800]) {
  await page.setViewportSize({width,height:1000});
  await navigate(`${base}/tests/permission-settings-preview.html?language=${language}`);
  await page.getByRole("heading",{name:language==="zh-CN"?"设置":"Settings",exact:true}).first().waitFor();
  await page.evaluate(()=>document.fonts.ready);
  await page.waitForTimeout(300);
  const result=await page.evaluate(()=>({overflow:document.documentElement.scrollWidth>innerWidth+1,language:document.documentElement.lang,tabs:[],alerts:[...document.querySelectorAll('[role="alert"]')].map(node=>node.textContent)}));
  if(result.overflow)throw Error("Global settings overflow the supported viewport");
  report.push({name:"settings",width,...result});
  await page.screenshot({path:resolve(output,`settings-${language}-${width}.png`),fullPage:true});
  await page.locator('[data-slot="page-scroll"]').evaluate(node=>{node.scrollTop=node.scrollHeight;});
  await page.screenshot({path:resolve(output,`settings-bottom-${language}-${width}.png`),fullPage:true});
 }
 for(const language of ["zh-CN","en-US"])for(const [name,path]of pages.slice(1)) {
  await page.setViewportSize({width:520,height:1000});
  const error=(language==="zh-CN"?"无法读取设置：":"Could not read settings: ")+"C:\\WinOne\\"+"configuration".repeat(24)+".json";
  await navigate(`${base}/tests/${path}.html?language=${language}&failure=1&error=${encodeURIComponent(error)}`);
  const alert=page.getByRole("alert");await alert.waitFor();
  await page.evaluate(()=>document.fonts.ready);
  await page.waitForTimeout(300);
  const recovery=alert.getByRole("button",{name:language==="zh-CN"?"重试":"Retry",exact:true});
  const recoveryBounds=await recovery.boundingBox();
  if(!recoveryBounds||recoveryBounds.x<0||recoveryBounds.x+recoveryBounds.width>521)throw Error("Recovery button is outside the page");
  if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1))throw Error("Long error text overflows the page");
  await page.screenshot({path:resolve(output,`${name}-error-${language}.png`),fullPage:true});
  await page.getByRole("tab",{name:language==="zh-CN"?"诊断日志":"Diagnostic logs",exact:true}).click();
  await page.locator(".diagnostics").getByRole("button",{name:language==="zh-CN"?"刷新日志":"Refresh logs",exact:true}).waitFor();
  await page.getByRole("alert").waitFor({state:"hidden"});
  await page.waitForTimeout(250);
  const result={...await page.evaluate(()=>({overflow:document.documentElement.scrollWidth>innerWidth+1,tabs:[]})),alerts:await page.getByRole("alert").allTextContents()};
  report.push({name:`${name}-failure`,width:520,language,...result});
  await page.screenshot({path:resolve(output,`${name}-failure-${language}.png`),fullPage:true});
 }
 for(const language of ["zh-CN","en-US"])for(const width of [1280,800,520]) {
  const warning=language==="zh-CN"?"启动记录未能写入诊断日志。\n请检查日志文件夹。":"The startup diagnostic log could not be written.\nCheck the log folder.";
  await page.setViewportSize({width,height:1000});
  await navigate(`${base}/tests/shell-preview.html?language=${language}&warning=${encodeURIComponent(warning)}`);
  await page.getByRole("status").filter({hasText:warning}).waitFor();
  await page.evaluate(()=>document.fonts.ready);await page.waitForTimeout(200);
  const result={...await page.evaluate(()=>({overflow:document.documentElement.scrollWidth>innerWidth+1,tabs:[]})),alerts:await page.getByRole("alert").allTextContents()};
  report.push({name:"home-warning",width,language,...result});
  await page.screenshot({path:resolve(output,`home-warning-${language}-${width}.png`),fullPage:true});
 }
 if(errors.length||report.some(item=>item.overflow||item.tabs.some(tab=>tab.overflow)||item.alerts.length))throw Error("Layout check failed; see report.json");
 console.log("PASS: 28 bilingual layouts, 6 long-error recovery routes and 6 nonblocking warning layouts");
} finally {
 await writeFile(resolve(output,"report.json"),JSON.stringify({theme,report,errors},null,2));
 await browser.close();
}
