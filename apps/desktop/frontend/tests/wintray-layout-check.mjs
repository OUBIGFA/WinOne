// Explicit, isolated browser check; never connects to the user's browser.
// Provide WINONE_PLAYWRIGHT_MODULE if Playwright is installed outside this repo.
import {createRequire} from "node:module";
import {mkdir,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
const require=createRequire(import.meta.url);
const {chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||"_temp/wintray-layout");
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});
try{
 const page=await browser.newPage();const report=[];const errors=[];
 page.on("pageerror",error=>errors.push(String(error)));
 async function inspectLayout(width,language,state){
  const overflow=await page.evaluate(()=>[...document.querySelectorAll("html,body,[data-slot=page-scroll],.wintray-workspace,.wintray-program-list,.wintray-program-detail,[data-slot=resizable-panel]")].filter(element=>element.clientWidth>0&&element.scrollWidth>element.clientWidth+1).map(element=>({element:element.className||element.tagName,width:element.clientWidth,scroll:element.scrollWidth,overflowX:getComputedStyle(element).overflowX})));
  const clippedControls=await page.locator('.wintray-program-detail').evaluate(element=>{const box=element.getBoundingClientRect();return [...element.querySelectorAll('button,[role=switch],[role=checkbox],input:not([type=hidden])')].filter(control=>{const rect=control.getBoundingClientRect();return rect.width>1&&rect.height>1&&(rect.left<box.left-1||rect.right>box.left+element.clientWidth+1);}).map(control=>control.id||control.getAttribute('data-slot'));});
  report.push({width,language,state,overflow,clippedControls});
 }
 for(const language of ['zh-CN','en-US']) for(const width of [390,520,640,800,960,1100,1200,1600]){
  await page.setViewportSize({width,height:700});
  const url=new URL(process.env.WINONE_LAYOUT_URL||"http://127.0.0.1:5188/tests/wintray-preview.html?many");
  url.searchParams.set('language',language);url.searchParams.set('longContent','');url.searchParams.set('many','');
  await page.goto(url.href);
  const row=page.locator('.wintray-program-row').first();await row.waitFor();await row.click({position:{x:8,y:8}});
  if(await row.getAttribute("aria-current")!=="true")throw Error("Row background is not selectable");
  const actions=page.locator('.wintray-program-detail [role=group]');const before=await actions.boundingBox();
  const scrollBefore=await page.locator('.wintray-program-detail').evaluate(element=>element.scrollTop);
  await inspectLayout(width,language,'common');
  await page.locator('.wintray-program-detail [role=tab]').nth(1).click();
  // Wait for shadcn's focus/selection transitions before measuring layout.
  await page.waitForTimeout(250);const after=await actions.boundingBox();
  const scrollAfter=await page.locator('.wintray-program-detail').evaluate(element=>element.scrollTop);
  if(!before||!after||Math.abs(before.y+scrollBefore-after.y-scrollAfter)>1)throw Error("Program actions moved between tabs");
  const list = page.locator('.wintray-program-list');
  const geometry=await list.evaluate(element=>({height:element.clientHeight,scroll:element.scrollHeight,document:document.documentElement.scrollHeight,viewport:window.innerHeight}));
  if(geometry.height<100||geometry.scroll<=geometry.height||geometry.document>geometry.viewport+1){await page.screenshot({path:resolve(output,`failed-${language}-${width}.png`)});throw Error(`The program list does not own scrolling at ${width}px (${language}): `+JSON.stringify(geometry));}
  const wrapped=await row.evaluate(element=>{const bounds=element.getBoundingClientRect();return [...element.querySelectorAll('button,img')].some(control=>{const box=control.getBoundingClientRect();return box.top<bounds.top||box.bottom>bounds.bottom||box.right>bounds.right;});});
  if(wrapped)throw Error(`Program row wraps or clips its controls at ${width}px (${language})`);
  await list.evaluate(element=>{element.scrollTop=element.scrollHeight;});
  if(await list.evaluate(element=>element.scrollTop)<=0)throw Error('Program list did not scroll');
  await inspectLayout(width,language,'advanced');
  const divider=page.locator('.wintray-panels > [data-slot=resizable-handle]');
  if(await divider.isVisible()){
   const box=await divider.boundingBox();
   await page.mouse.move(box.x+box.width/2,box.y+box.height/2);await page.mouse.down();await page.mouse.move(box.x+300,box.y+box.height/2,{steps:8});await page.mouse.up();
   await page.locator('.wintray-program-detail [role=tab]').nth(0).click();await inspectLayout(width,language,'common-narrow-panel');
  }
  await page.screenshot({path:resolve(output,`${language}-${width}.png`),fullPage:true});
 }
 await writeFile(resolve(output,"report.json"),JSON.stringify({report,errors},null,2));
 if(errors.length||report.some(result=>result.overflow.length||result.clippedControls.length))throw Error("Layout check failed; see report.json");
 console.log("PASS: common/advanced settings, long content, narrowed panels, Chinese/English, 390-1600px; no horizontal overflow or clipped controls; single-line rows and independent scrolling");
}finally{await browser.close();}
