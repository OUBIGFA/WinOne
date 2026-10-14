// Exercise the shipped components in a real WebView-compatible browser.
import fs from "node:fs/promises";
import path from "node:path";
import {createRequire} from "node:module";
import {execFileSync} from "node:child_process";
const require=createRequire(import.meta.url),{chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE);
const [baseURL,outputArg]=process.argv.slice(2),url=new URL(baseURL);
if(url.hostname!=="127.0.0.1"||!outputArg)throw Error("Use a local Vite URL and an ignored output directory");
const output=path.resolve(outputArg);execFileSync("git",["check-ignore","--quiet","--no-index",path.join(output,"layout.probe")]);await fs.mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,...(process.env.WINONE_BROWSER_EXE?{executablePath:process.env.WINONE_BROWSER_EXE}:{channel:process.env.WINONE_LAYOUT_BROWSER||"msedge"})}),results=[];
try{
 for(const motion of ["reduce","no-preference"]){
  const page=await browser.newPage({viewport:{width:900,height:800},reducedMotion:motion}),errors=[];page.on("pageerror",error=>errors.push(error.message));
  await page.goto(new URL("/tests/chat-process-preview.html",url).href);await page.waitForFunction(()=>typeof window.renderProcessPreview==="function");
  for(const width of [360,720]){
   const value={id:`${motion}-${width}`,width,lines:180,running:width===360,dark:width===720,language:width===720?"en-US":"zh-CN"};
   await page.evaluate(value=>window.renderProcessPreview(value),value);
   await page.locator('[data-slot="cindy-work"]').click();await page.locator('[data-process-long="true"]').waitFor();
   const action=page.locator(".process-collapse-action");await action.waitFor();
   const gap=await page.evaluate(()=>{const process=document.querySelector('.chat-work-process').getBoundingClientRect(),answer=document.querySelector('.chat-answer-content > .chat-markdown > :first-child').getBoundingClientRect();return answer.top-process.bottom;});
   if(Math.abs(gap-16)>.6)throw Error(`Work process answer gap was ${gap}px, expected a total of 16px`);
   if(value.running){
    const viewport=page.locator('[data-preview-transcript]');await viewport.evaluate(node=>{node.scrollTop=120;});
    await page.evaluate(value=>window.renderProcessPreview({...value,lines:220}),value);
    if(Math.abs(await viewport.evaluate(node=>node.scrollTop)-120)>2)throw Error('Streaming moved the reader away from the selected process position');
   }
   const before=await action.boundingBox();if(!before||before.y+before.height>800)throw Error("Collapse action was outside the viewport");
   await page.locator("[data-preview-transcript]").evaluate(node=>{node.scrollTop=node.scrollHeight;});
   const after=await action.boundingBox();if(!after||Math.abs(before.y-after.y)>.6)throw Error("Collapse action moved with the transcript");
   await action.click();await page.locator(".chat-expanded-thinking").waitFor({state:"hidden"});await action.waitFor({state:"hidden"});
   await page.evaluate(value=>window.renderProcessPreview({...value,id:value.id+"-short",lines:1}),value);
   await page.locator('[data-slot="cindy-work"]').click();await page.locator(".process-body").waitFor();
   await page.waitForFunction(()=>{const viewport=document.querySelector(".process-body");return viewport&&viewport.clientHeight>0&&viewport.scrollHeight<=viewport.clientHeight+1;});
   if(await page.locator(".process-collapse-action").count())throw Error("A short process had a persistent collapse action");
   results.push({motion,width,running:value.running,gap,fixedAction:true,shortAction:false});
  }
  if(errors.length)throw Error(errors.join("\n"));await page.close();
 }
 const page=await browser.newPage({viewport:{width:800,height:800},reducedMotion:"reduce"});const errors=[];page.on("pageerror",error=>errors.push(error.message));
 await page.goto(new URL("/tests/chat-process-preview.html",url).href);await page.waitForFunction(()=>typeof window.renderProcessPreview==="function");
 await page.evaluate(()=>window.renderProcessPreview({id:"avatar",width:780,lines:1,running:false,dark:false,language:"zh-CN",avatar:true}));
 const picture=await fs.readFile(path.resolve("internal/branding/chat-welcome.png"));const maximum=Buffer.alloc(1<<20);picture.copy(maximum);
 await page.locator('input[type="file"]').setInputFiles({name:"avatar.png",mimeType:"image/png",buffer:maximum});
 await page.waitForFunction(()=>document.querySelector('[data-icon^="file:"] img')?.naturalWidth>0);
 const reference=await page.locator("#avatar-reference").textContent();if(!reference?.startsWith("file:"))throw Error("Uploaded avatar not retained");
 await page.getByRole("button",{name:"图片链接",exact:true}).click();await page.getByRole("textbox",{name:"图片链接"}).fill("https://example.invalid/avatar.png");await page.getByRole("button",{name:"使用图片",exact:true}).click();
 await page.getByRole("button",{name:"内置图标",exact:true}).click();await page.getByRole("textbox",{name:"搜索图标"}).fill("openai");await page.getByRole("button",{name:"openai",exact:true}).click();
 if(await page.locator("#avatar-reference").textContent()!=="openai")throw Error("Built-in choice not applied");
 await page.getByRole("button",{name:"恢复默认",exact:true}).click();if(await page.locator("#avatar-reference").textContent()!=="")throw Error("Avatar reset failed");
 await page.screenshot({path:path.join(output,"avatar-controls.png")});if(errors.length)throw Error(errors.join("\n"));await page.close();
 await fs.writeFile(path.join(output,"checks.json"),JSON.stringify({process:results,avatar:{maximumUpload:true,url:true,builtin:true,reset:true}},null,2));
 console.log(JSON.stringify({processCases:results.length,maximumAvatarBytes:1<<20,browserErrors:0}));
}finally{await browser.close();}
