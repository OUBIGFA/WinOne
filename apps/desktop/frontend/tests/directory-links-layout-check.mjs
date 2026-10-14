import {createRequire} from "node:module";
import {mkdir,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||"../../../_temp/directory-links-layout");await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});
const report=[],errors=[];let page;
const check=(ok,message)=>{if(!ok)throw Error(message);};
try{
 const context=await browser.newContext({permissions:["clipboard-read","clipboard-write"]});page=await context.newPage();page.on("pageerror",error=>errors.push(String(error)));
 for(const language of ["zh-CN","en-US"])for(const theme of ["light","dark"])for(const [width,height] of [[1180,920],[820,720],[1440,1000]]){
  const zh=language==="zh-CN";await page.setViewportSize({width,height});
  const url=new URL(process.env.WINONE_LAYOUT_URL||"http://127.0.0.1:5198/tests/directory-links-preview.html");url.searchParams.set("language",language);url.searchParams.set("theme",theme);await page.goto(url.href);
  await page.getByText("existing-hardlink",{exact:true}).waitFor();await page.evaluate(()=>document.fonts.ready);
  const source=page.getByRole("region",{name:zh?"源文件夹":"Source folder",exact:true}),target=page.getByRole("region",{name:zh?"目标文件夹":"Destination folder",exact:true});
  await source.getByRole("textbox").fill("C:\\source");await source.getByRole("textbox").press("Enter");await source.getByRole("checkbox",{name:zh?"选择来源：document-1.txt":"Select source: document-1.txt",exact:true}).waitFor();
  await target.getByRole("textbox").fill("C:\\links");await target.getByRole("textbox").press("Enter");await target.getByText("existing-hardlink",{exact:true}).waitFor();
  await page.getByRole("button",{name:zh?"添加来源":"Add sources",exact:true}).click();await page.getByRole("menuitem",{name:zh?"选择文件…":"Choose files…",exact:true}).click();
  await target.getByText("document-2.txt",{exact:true}).waitFor();
  const layout=await page.evaluate(()=>{
   const rect=element=>{const r=element.getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height,bottom:r.bottom,right:r.right};};
   const panes=document.querySelector(".directory-links-panes"),bar=document.querySelector(".directory-links-create-bar"),left=document.querySelector('[data-directory-links-drop="source"]'),right=document.querySelector('[data-directory-links-drop="target"]');
   const main=bar.querySelector('button[data-slot="button"][class*="bg-primary"]');
   const badges=[...right.querySelectorAll(".directory-links-kind")].map(e=>({text:e.textContent,background:getComputedStyle(e).backgroundColor,color:getComputedStyle(e).color}));
   const green=right.querySelector(".directory-links-linked");
   return {left:rect(left),right:rect(right),bar:rect(bar),panes:rect(panes),primary:main?rect(main):null,rootWidth:document.documentElement.scrollWidth,green:green?{background:getComputedStyle(green).backgroundColor,color:getComputedStyle(green).color}:null,badges,sourceList:rect(left.querySelector(".directory-links-file-list"))};
  });
  check(layout.bar.y>=0&&layout.bar.bottom<=height+1,`Create bar leaves viewport: ${JSON.stringify({language,theme,width,height,layout})}`);
  check(layout.left.x+layout.left.width<=layout.right.x+1&&Math.abs(layout.left.y-layout.right.y)<1,"Folder panes are not side by side");
  check(layout.rootWidth<=width+1,"Workspace overflows horizontally");
  check(layout.sourceList.height>=80,"Source list has no usable scrolling area");
  check(new Set(layout.badges.map(b=>b.background)).size===4,"Link kinds do not have four distinct color blocks");
  check(layout.green?.background===(theme==="dark"?"rgb(30, 58, 43)":"rgb(228, 246, 236)"),"Linked badge does not reuse Magpie green-soft");
  await page.screenshot({path:resolve(output,`links-${language}-${theme}-${width}x${height}.png`)});
  const beforeY=layout.bar.y;await source.locator(".directory-links-file-list").evaluate(e=>e.scrollTop=e.scrollHeight);
  const afterY=(await page.locator(".directory-links-create-bar").boundingBox()).y;check(Math.abs(beforeY-afterY)<1,"List scrolling moved create controls");
  await page.getByRole("button",{name:zh?"创建链接":"Create links",exact:true}).click();await target.getByText("document-1.txt",{exact:true}).waitFor();
  check(await target.getByText(zh?"已链接":"Linked",{exact:true}).count()===6,"Created links are not marked in actual destination contents");
  const calls=await page.evaluate(()=>window.directoryLinksFixture.calls.map(call=>call.method));check(calls.indexOf("previewBatch")>=0&&calls.indexOf("previewBatch")<calls.indexOf("createBatch"),"Direct create bypassed safety preview");
  if(width===1180&&theme==="light"){
   await page.evaluate(()=>navigator.clipboard.writeText('"C:\\source\\clipboard.txt"\r\n"C:\\source\\clipboard.txt"'));
   await page.getByRole("button",{name:zh?"添加来源":"Add sources",exact:true}).click();await page.getByRole("menuitem",{name:zh?"粘贴路径":"Paste paths",exact:true}).click();await source.getByText("clipboard.txt",{exact:true}).waitFor();
   await page.getByLabel(zh?"链接名称":"Link name",{exact:true}).fill("custom-alias.txt");
   await page.getByRole("button",{name:zh?"创建链接":"Create links",exact:true}).click();await target.getByText("custom-alias.txt",{exact:true}).waitFor();
   const custom=await page.evaluate(()=>window.directoryLinksFixture.calls.find(call=>call.method==="previewBatch"&&call.args.linkName==="custom-alias.txt"));check(!!custom,"Custom name was not passed to safety preview");
  }
  report.push({language,theme,width,height,...layout,directCreate:true,scrollKeepsControls:true});
 }
 // Both panes retain one preferred height. Resizing the window clamps the
 // rendered size, without losing the user's choice or hiding creation.
 await page.setViewportSize({width:1180,height:1000});await page.goto(process.env.WINONE_LAYOUT_URL||"http://127.0.0.1:5198/tests/directory-links-preview.html");
 const handle=page.getByRole("separator",{name:"调整左右视窗高度"}),panes=page.locator(".directory-links-panes");await handle.waitFor();const initial=await panes.boundingBox();
 await handle.press("ArrowUp");const keyboard=await panes.boundingBox();check(keyboard.height<initial.height,"Pane height did not respond to keyboard");
 const grip=await handle.boundingBox();await page.mouse.move(grip.x+grip.width/2,grip.y+grip.height/2);await page.mouse.down();await page.mouse.move(grip.x+grip.width/2,grip.y-70,{steps:5});await page.mouse.up();const resized=await panes.boundingBox();check(resized.height<keyboard.height-40,"Pane height did not follow pointer");
 const heights=await page.locator(".directory-links-pane").evaluateAll(nodes=>nodes.map(node=>node.getBoundingClientRect().height));check(Math.abs(heights[0]-heights[1])<1,"Source and destination heights diverged");
 await page.reload();await handle.waitFor();check(Math.abs((await panes.boundingBox()).height-resized.height)<1,"Pane height was not remembered");
 await page.setViewportSize({width:1180,height:760});await page.waitForTimeout(100);check((await page.locator(".directory-links-create-bar").boundingBox()).y+(await page.locator(".directory-links-create-bar").boundingBox()).height<=761,"Resizing window hid the create controls");
 await handle.press("Home");check(await page.evaluate(()=>localStorage.getItem("winone.directory-links.pane-height.v1"))===null,"Automatic height reset failed");
 check(errors.length===0,errors.join("\n"));await writeFile(resolve(output,"layout-result.json"),JSON.stringify({layouts:report,errors},null,2));console.log(`Verified ${report.length} layouts, direct creation, native multi-file selection and clipboard imports.`);
}catch(error){await writeFile(resolve(output,"layout-failure.json"),JSON.stringify({error:String(error.stack||error),report,errors},null,2));if(page)await page.screenshot({path:resolve(output,"layout-failure.png")});throw error;}finally{await browser.close();}
