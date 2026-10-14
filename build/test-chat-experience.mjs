// End-to-end renderer checks using production components and an isolated bridge.
import fs from "node:fs/promises";
import path from "node:path";
import {createRequire} from "node:module";
import {execFileSync} from "node:child_process";
const require=createRequire(import.meta.url),{chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE);
const [baseURL,outputArg]=process.argv.slice(2),url=new URL(baseURL),output=path.resolve(outputArg);
if(url.hostname!=="127.0.0.1")throw Error("Use the isolated local preview");
execFileSync("git",["check-ignore","--quiet","--no-index",path.join(output,"probe")]);await fs.mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,...(process.env.WINONE_BROWSER_EXE?{executablePath:process.env.WINONE_BROWSER_EXE}:{channel:process.env.WINONE_LAYOUT_BROWSER||"msedge"})}),results=[];
try{
 for(const width of [640,1120])for(const language of ["zh-CN","en-US"])for(const theme of ["light","dark"]){
  const page=await browser.newPage({viewport:{width,height:900},reducedMotion:"reduce"}),errors=[];page.on("pageerror",error=>errors.push(error.message));
  const target=new URL("/tests/chat-experience-preview.html",url);target.searchParams.set("language",language);target.searchParams.set("theme",theme);
  await page.goto(target.href);await page.locator(".chat-markdown h2").waitFor();
  await page.locator('[data-slot="cindy-work"]').first().click();
  await page.locator('[data-process-long="true"]').waitFor();
  const metrics=await page.evaluate(()=>{
   const body=document.querySelector('.process-body'),main=document.querySelector('.chat-transcript-viewport');
   // Upstream code blocks and tables deliberately scroll horizontally instead
   // of breaking source tokens or squeezing table columns into vertical text.
   const horizontal=[...document.querySelectorAll('*')].filter(node=>node instanceof HTMLElement&&!node.matches('.desktop-ai-markdown pre,.desktop-ai-markdown table')&&node.clientWidth>0&&node.scrollWidth>node.clientWidth+1&&/auto|scroll/.test(getComputedStyle(node).overflowX)).map(node=>({tag:node.tagName,classes:node.className,width:node.clientWidth,scroll:node.scrollWidth}));
   return {horizontal,processViewports:document.querySelectorAll('.process-viewport').length,processHeight:body.clientHeight,processScrollHeight:body.scrollHeight,processOverflow:getComputedStyle(body).overflowY,mainHeight:main.clientHeight,bodyOverflow:document.documentElement.scrollWidth-innerWidth};
  });
  if(metrics.horizontal.length||metrics.bodyOverflow>1||metrics.processViewports!==0||metrics.processHeight<1000||metrics.processOverflow!=='visible'||Math.abs(metrics.processHeight-metrics.processScrollHeight)>1)throw Error(JSON.stringify({width,language,theme,metrics}));
  if(await page.locator('.process-resize-handle').count())throw Error('Work process still has a resize handle');
  const collapse=page.locator('.process-collapse-action'),before=await collapse.boundingBox();
  await page.locator('.chat-transcript-viewport').evaluate(node=>{node.scrollTop=node.scrollHeight/2;});
  const after=await collapse.boundingBox();if(!before||!after||Math.abs(before.y-after.y)>1||after.y<0||after.y+after.height>900)throw Error('The collapse control moved out of reach');
  const retained=await page.locator('.desktop-conversation-message--assistant').elementHandle(),draft=await page.getByRole('textbox',{name:language==='zh-CN'?'消息':'Message'}).inputValue();
  const model=page.getByRole('button',{name:language==='zh-CN'?'模型':'Model',exact:true});await model.click();await page.getByRole('option',{name:/Another Model/}).click();
  await page.waitForFunction(()=>document.querySelector('.chat-model-trigger')?.textContent.includes('Another Model'));
  if(!await retained.evaluate(node=>node.isConnected))throw Error('Model selection replaced the transcript');
  if(await page.getByRole('textbox',{name:language==='zh-CN'?'消息':'Message'}).inputValue()!==draft)throw Error('Model selection replaced the draft');
  if(await model.locator('[data-model-badge]').count()!==3)throw Error('Selected model lost a badge');
  const clipped=await model.locator('[data-model-badge]').evaluateAll(nodes=>nodes.some(node=>{const range=document.createRange();range.selectNodeContents(node);const text=range.getBoundingClientRect(),box=node.getBoundingClientRect();return text.width>box.width+1;}));if(clipped)throw Error('A model badge was clipped');
  if(errors.length)throw Error(errors.join('\n'));
  if(language==='zh-CN'){await page.screenshot({path:path.join(output,`chat-${width}-${theme}.png`)});}
  results.push({width,language,theme,...metrics,fullProcess:true,fixedCollapse:true,modelRetained:true,modelBadges:3});await page.close();
 }
 await fs.writeFile(path.join(output,'report.json'),JSON.stringify({cases:results.length,results},null,2));console.log(JSON.stringify({cases:results.length}));
}finally{await browser.close();}
