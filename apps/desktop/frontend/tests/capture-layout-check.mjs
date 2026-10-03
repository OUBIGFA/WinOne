// Explicit headless check of the production editor with synthetic images and
// simulated output actions. Never connects to a user's browser or clipboard.
import {createRequire} from "node:module";
import {mkdir,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
const require=createRequire(import.meta.url);
const {chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||"_temp/capture-layout");
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});
const report=[],errors=[];
try {
 const page=await browser.newPage();
 page.on("pageerror",error=>errors.push(String(error)));
 for(const language of ["zh-CN","en-US"])for(const width of [1280,800,520]) {
  await page.setViewportSize({width,height:800});
  await page.goto(process.env.WINONE_LAYOUT_URL||"http://127.0.0.1:5188/tests/capture-editor.html?long=1");
  const labels=await page.evaluate(async language=>{
   const {setApplicationLanguage,i18n}=await import("/src/i18n.ts");
   await setApplicationLanguage(language);
   return Object.fromEntries(["saveImage","saveAs","copy","pin","undo","redo","retake","moreActions","moreTools","customizeToolbar","toolOptions"].map(key=>[key,i18n.t(`capture.${key}`)]));
  },language);
  await page.waitForFunction(()=>{
   const image=document.querySelector(".capture-editor-canvas > img");
   return image?.complete&&image.naturalWidth>0&&!document.querySelector(".capture-effect-status");
  });
  await page.evaluate(()=>document.fonts.ready);
  await page.waitForTimeout(250);
  const rows=await page.evaluate(()=>[...document.querySelectorAll(".capture-toolbar-row")].map(row=>({
   height:row.getBoundingClientRect().height,
   clipped:[...row.querySelectorAll("button")].filter(button=>!button.closest("[inert]")).filter(button=>{
    const bounds=button.getBoundingClientRect();
    if(bounds.left<0||bounds.top<0||bounds.right>innerWidth+1||bounds.bottom>innerHeight+1)return true;
    for(let parent=button.parentElement;parent;parent=parent.parentElement){
     if(!["hidden","clip","auto","scroll"].includes(getComputedStyle(parent).overflowX))continue;
     const limit=parent.getBoundingClientRect();
     if(bounds.left<limit.left-1||bounds.right>limit.right+1)return true;
    }
    return false;
   }).map(button=>button.textContent)
  })));
  if(rows.length!==2||rows.some(row=>row.height>40||row.clipped.length))throw Error(`Toolbar overflow: ${language}/${width}`);
  const actions=page.locator(".capture-toolbar-row").first();
  const reachable=new Set(await actions.getByRole("button").allTextContents());
  const menus=[];
  for(const key of ["moreActions","moreTools","customizeToolbar","toolOptions"]) {
   const trigger=page.getByRole("button",{name:labels[key],exact:true});
   if(!await trigger.count())continue;
   await trigger.focus();await page.keyboard.press("Enter");
   const panel=page.locator("[data-slot=dropdown-menu-content],[data-slot=popover-content]").filter({visible:true});
   await panel.waitFor();await page.waitForTimeout(200);
   const bounds=await panel.boundingBox();
   if(!bounds||bounds.x<0||bounds.y<0||bounds.x+bounds.width>width+1||bounds.y+bounds.height>801)throw Error(`Menu overflow: ${key}/${language}/${width}`);
   if(key==="moreActions")for(const label of await panel.getByRole("menuitem").allTextContents())reachable.add(label.trim());
   await page.keyboard.press("Escape");
   await page.waitForFunction(()=>!document.querySelector('[data-slot="dropdown-menu-content"],[data-slot="popover-content"]'));
   if(!await trigger.evaluate(button=>document.activeElement===button))throw Error(`Focus was not restored: ${key}`);
   menus.push({key,bounds});
  }
  for(const key of ["saveImage","saveAs","copy","pin","undo","redo","retake"]){
   if(!reachable.has(labels[key]))throw Error(`Missing action: ${key}/${language}/${width}`);
  }
  report.push({language,width,rows,menus,actions:[...reachable]});
  await page.screenshot({path:resolve(output,`${language}-${width}.png`)});
 }
 if(errors.length)throw Error(errors.join("\n"));
 console.log("PASS: full toolbar actions, two rows, menu bounds and keyboard focus at 1280/800/520px in Chinese and English");
} finally {
 await writeFile(resolve(output,"report.json"),JSON.stringify({report,errors},null,2));
 await browser.close();
}
