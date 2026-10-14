import {createRequire} from "node:module";
import {mkdir,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
const require=createRequire(import.meta.url);
const {chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||"_temp/selection-layout");await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});
const report=[],errors=[];
try{
 const page=await browser.newPage();page.on("pageerror",error=>errors.push(String(error)));
 for(const language of ["zh-CN","en-US"])for(const theme of ["light","dark"])for(const width of [520,800,1280]){
  const zh=language==="zh-CN";await page.setViewportSize({width,height:900});
  const url=new URL(process.env.WINONE_LAYOUT_URL||"http://127.0.0.1:5195/tests/selection-preview.html");url.searchParams.set("language",language);url.searchParams.set("theme",theme);await page.goto(url.href);
  await page.getByLabel(zh?"排除应用":"Excluded apps",{exact:true}).waitFor();
  await page.getByRole("tab",{name:zh?"工具栏":"Toolbar",exact:true}).click();
  const positions=page.getByRole("combobox",{name:zh?"位置":"Position",exact:true});await positions.first().click();await page.getByRole("option",{name:"3",exact:true}).click();
  if(!await page.locator(".selection-action-trigger").first().getAttribute("aria-label").then(label=>label===(zh?"编辑 翻译":"Edit Translate")))throw Error("Action reorder failed");
  const names=page.getByLabel(zh?"功能名称":"Action name",{exact:true});if(await names.count())throw Error("Action editors should start collapsed");
  await page.getByRole("button",{name:zh?"添加自定义功能":"Add custom action"}).click();await names.last().fill(zh?"邮件回复":"Email reply");await page.getByLabel(zh?"AI 指令":"AI instruction",{exact:true}).last().fill("Write a concise reply to the selected text.");
  await page.getByRole("button",{name:zh?"保存设置":"Save settings",exact:true}).click();await page.getByText(zh?"已保存":"Saved",{exact:true}).waitFor();
  const overflow=await page.evaluate(()=>[...document.querySelectorAll('html,body,[data-slot="page-scroll"],.module-section,.selection-action-editor')].filter(e=>e.clientWidth>0&&e.scrollWidth>e.clientWidth+1).map(e=>e.className||e.tagName));
  const rowGeometry=await page.locator(".selection-action-row").evaluateAll(rows=>rows.map(row=>{const r=row.getBoundingClientRect();const name=row.querySelector(".selection-action-heading").getBoundingClientRect();const controls=row.querySelector(".selection-action-controls").getBoundingClientRect();return {height:r.height,overlap:name.right>controls.left+1&&name.bottom>controls.top+1};}));
  if(rowGeometry.some(row=>row.height<56||row.overlap))throw Error("Action names and controls must have separate space");
  // Base UI keeps 1 px form inputs offscreen with inset(50%) clipping; these
  // submit values but are not visible controls or user-facing overflow.
  const clipped=await page.locator(".module-sections").evaluate(root=>[...root.querySelectorAll('button,input,textarea,[role="combobox"]')].filter(e=>{const r=e.getBoundingClientRect(),style=getComputedStyle(e);if(e.tagName==="INPUT"&&style.clipPath==="inset(50%)")return false;return r.width>0&&r.height>0&&(r.left<0||r.right>window.innerWidth+1)}).map(e=>e.id||e.textContent));
  report.push({language,theme,width,overflow,clipped});
  if(width===800)await page.screenshot({path:resolve(output,`${language}-${theme}.png`),fullPage:true});
 }
 await writeFile(resolve(output,"report.json"),JSON.stringify({report,errors},null,2));
 if(errors.length||report.some(item=>item.overflow.length||item.clipped.length))throw Error("Selection layout failed; see report");
 console.log("PASS: selection settings in two languages, two themes and three widths; action ordering and custom instructions saved");
}finally{await browser.close();}
