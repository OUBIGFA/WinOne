// Production Chat layout with an isolated Host fixture; no real model/files.
// Usage: node build/test-chat-content-boundaries.mjs <ignored output directory>
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {pathToFileURL} from 'node:url';
import {execFileSync} from 'node:child_process';
const output=resolve(process.argv[2]||'_temp/chat-resume/content-boundaries');
execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);
await mkdir(output,{recursive:true});
const frontend=resolve('apps/desktop/frontend');
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const {createServer}=await import(pathToFileURL(join(frontend,'node_modules/vite/dist/node/index.js')).href);
const vite=await createServer({root:frontend,cacheDir:join(output,'vite-cache'),server:{host:'127.0.0.1',port:0},logLevel:'warn'});await vite.listen();
const base=`http://127.0.0.1:${vite.httpServer.address().port}`;
const browser=await chromium.launch({headless:true,...(process.env.WINONE_BROWSER_EXE?{executablePath:process.env.WINONE_BROWSER_EXE}:{channel:'msedge'})});
let page;const report=[];
const settle=()=>page.evaluate(async()=>{await document.fonts.ready;await new Promise(done=>requestAnimationFrame(()=>requestAnimationFrame(done)));});
const geometry=()=>page.evaluate(()=>{
 const v=document.querySelector('.chat-transcript-viewport'),footer=document.querySelector('.chat-input-area'),nav=document.querySelector('.chat-scroll-controls');
 return {scrollTop:v.scrollTop,scrollHeight:v.scrollHeight,viewportHeight:v.clientHeight,footerTop:footer.getBoundingClientRect().top,navClear:(()=>{const box=nav.getBoundingClientRect(),surface=document.querySelector('.chat-input-content')?.getBoundingClientRect();return !surface||box.right<=surface.left+1||surface.right<=box.left+1||box.bottom<=surface.top+1||surface.bottom<=box.top+1;})(),spacerTop:v.querySelector('.chat-transcript-end-space').getBoundingClientRect().top,navBottom:nav.getBoundingClientRect().bottom,headerNav:nav.classList.contains('chat-scroll-controls--header'),horizontal:document.documentElement.scrollWidth-innerWidth};
});
async function jumpToBottom(label){
 const button=page.getByRole('button',{name:label,exact:true}),before=await geometry();
 // ResizeObserver publishes the new button state before the next paint. Wait
 // for that enabled state through click(), rather than skipping a fresh jump.
 if(Math.abs(before.spacerTop-before.footerTop+32)>=2)await button.click();
 await page.waitForFunction(()=>Math.abs(document.querySelector('.chat-transcript-end-space').getBoundingClientRect().top-document.querySelector('.chat-input-area').getBoundingClientRect().top+32)<2);
 await settle();const value=await geometry();assert(value.headerNav||value.navClear,'Navigation covers the input content');assert(value.horizontal<=1,'Page overflows horizontally');
 // Navigation has its own stable reading-gutter anchor. Only actual input
 // collisions require displacement; bottom scrolling still uses the live edge.
 return value;
}
try{
 for(const width of [1280,420])for(const theme of ['light','dark'])for(const language of ['zh-CN','en-US']){
  const short=language==='en-US',id=`${width}-${theme}-${language}`,errors=[];
  if(process.env.WINONE_BOUNDARY_CASE&&!id.includes(process.env.WINONE_BOUNDARY_CASE))continue;
  page=await browser.newPage({viewport:{width,height:1000},reducedMotion:theme==='dark'?'reduce':'no-preference'});page.on('pageerror',error=>errors.push(error.message));
  if(process.env.WINONE_TRACE_SCROLL==='1')await page.addInitScript(()=>{
   const descriptor=Object.getOwnPropertyDescriptor(Element.prototype,'scrollTop');window.fixtureScrollWrites=[];
   Object.defineProperty(Element.prototype,'scrollTop',{...descriptor,set(value){
    const traced=this.classList.contains('chat-transcript-viewport');const before=descriptor.get.call(this);
    descriptor.set.call(this,value);
    if(traced){const record={before,requested:value,after:descriptor.get.call(this),height:this.scrollHeight,spacer:this.querySelector('.chat-transcript-end-space')?.outerHTML,spacerHeight:this.querySelector('.chat-transcript-end-space')?.getBoundingClientRect().height,spacerTransition:this.querySelector('.chat-transcript-end-space')&&getComputedStyle(this.querySelector('.chat-transcript-end-space')).transition,stack:new Error().stack};window.fixtureScrollWrites.push(record);requestAnimationFrame(()=>{record.later=descriptor.get.call(this);});}
   }});
  });
  if(process.env.WINONE_TRACE_RESIZE==='1')await page.addInitScript(()=>{
   const Native=window.ResizeObserver;window.fixtureResizes=[];window.fixtureErrors=[];
   window.ResizeObserver=class extends Native{constructor(callback){const source=new Error().stack;super((entries,observer)=>{window.fixtureResizes.push({source,targets:entries.map(entry=>({tag:entry.target.tagName,class:entry.target.className,width:entry.contentRect.width,height:entry.contentRect.height}))});window.fixtureResizes=window.fixtureResizes.slice(-12);callback.call(observer,entries,observer);});}};
   window.addEventListener('error',event=>window.fixtureErrors.push({message:event.message,resizes:window.fixtureResizes}));
  });
  await page.goto(`${base}/tests/chat-experience-preview.html?editWidths=${short?'short':'long'}&theme=${theme}&language=${language}`);
  await page.locator('.chat-transcript-viewport').waitFor();await page.locator('.desktop-conversation-message--user').last().waitFor();await settle();
  const labels=await page.evaluate(async language=>(await import('/src/locales/chat.ts')).chatMessages[language==='zh-CN'?'zh':'en'],language);
  const hideSidebar=page.getByRole('button',{name:labels.hideSidebar,exact:true});if(await hideSidebar.count())await hideSidebar.click();await settle();
  const message=page.locator('.desktop-conversation-message--user').last(),bubble=message.locator('.chat-message-bubble'),edit=message.getByRole('button',{name:labels.editMessage,exact:true});
  // Bring the actual Edit button into the unobscured reading area; do not use
  // force-clicks through the overlay or simulate layout with injected CSS.
  await edit.evaluate(el=>{const v=el.closest('.chat-transcript-viewport'),footer=document.querySelector('.chat-input-area');v.scrollTop+=el.getBoundingClientRect().bottom-footer.getBoundingClientRect().top+80;});await settle();
  const original=await bubble.evaluate(el=>el.getBoundingClientRect().width);await edit.click();
  const input=page.getByRole('textbox',{name:labels.editMessage,exact:true});await input.waitFor();await settle();
  const entered=await bubble.evaluate(el=>el.getBoundingClientRect().width);assert(Math.abs(entered-original)<2,`Edit changed the original width: ${original} -> ${entered}`);
  await input.fill('短');await settle();const small=await bubble.evaluate(el=>el.getBoundingClientRect().width);
  await input.fill('这是中等长度的编辑内容');await settle();const medium=await bubble.evaluate(el=>el.getBoundingClientRect().width);
  assert(medium>small+20,`Input did not grow with its text: ${small} -> ${medium}`);
  for(const content of ['输入内容达到原有宽度上限后应自动换行。'.repeat(24),'UnbrokenToken'.repeat(100)]){
   await input.fill(content);await settle();
   const size=await bubble.evaluate(el=>{const parent=el.closest('article').parentElement,style=getComputedStyle(parent),input=el.querySelector('textarea');return {width:el.getBoundingClientRect().width,cap:Math.min(488,parent.clientWidth-parseFloat(style.paddingLeft)-parseFloat(style.paddingRight)),inputWidth:input.clientWidth,inputScroll:input.scrollWidth,inputHeight:input.clientHeight,lineHeight:parseFloat(getComputedStyle(input).lineHeight)};});
   assert(Math.abs(size.width-size.cap)<2,JSON.stringify(size));assert(size.inputScroll<=size.inputWidth+1,'Long input has horizontal overflow');assert(size.inputHeight>size.lineHeight*2,'Long input did not wrap: '+JSON.stringify(size));
  }
  await page.mouse.move(0,0);await page.screenshot({path:join(output,id+'-editing.png')});
  await input.press('Escape');await settle();assert(Math.abs(await bubble.evaluate(el=>el.getBoundingClientRect().width)-original)<2,'Cancel lost the original width');
  const bottomLabel=language==='zh-CN'?'返回底部':'Go to bottom';
  const positions=[await jumpToBottom(bottomLabel)],handle=page.locator('.chat-composer-resize');
  for(const keys of [Array(6).fill('ArrowUp'),Array(20).fill('ArrowUp'),Array(22).fill('ArrowDown')]){
   const before=await geometry();await handle.focus();for(const key of keys)await handle.press(key);await settle();const resized=await geometry();
   assert.equal(resized.viewportHeight,before.viewportHeight,'Resize changed the transcript viewport');assert(Math.abs(resized.scrollTop-before.scrollTop)<1,'Resize moved the transcript');assert.equal(resized.scrollHeight,before.scrollHeight,'Resize changed the physical scroll extent');
   positions.push(await jumpToBottom(bottomLabel));
  }
  if(width===1280){
   const before=await geometry(),box=await handle.boundingBox();await page.mouse.move(box.x+box.width/2,box.y+box.height/2);await page.mouse.down();await page.mouse.move(box.x+box.width/2,box.y+box.height/2-100,{steps:8});await page.mouse.up();await settle();
   const resized=await geometry();assert(Math.abs(resized.scrollTop-before.scrollTop)<1,'Pointer resize moved the transcript');positions.push(await jumpToBottom(bottomLabel));
  }
  await page.mouse.move(0,0);await page.screenshot({path:join(output,id+'-bottom.png')});
  const topLabel=language==='zh-CN'?'返回顶部':'Back to top';await page.getByRole('button',{name:topLabel,exact:true}).click();await page.waitForFunction(()=>document.querySelector('.chat-transcript-viewport').scrollTop<1);
  assert.deepEqual(errors,[]);report.push({id,original,entered,small,medium,positions,topPassed:true});console.log('PASS',id);await page.close();page=null;
 }
 await writeFile(join(output,'result.json'),JSON.stringify({passed:true,report},null,2));console.log(`PASS: ${report.length} edit-width and input-boundary scenarios`);
}catch(error){
 if(page){await page.screenshot({path:join(output,'failure.png')}).catch(()=>{});await writeFile(join(output,'failure-dom.html'),await page.content()).catch(()=>{});await writeFile(join(output,'failure-resizes.json'),JSON.stringify({geometry:await geometry(),writes:await page.evaluate(()=>window.fixtureScrollWrites??[]),errors:await page.evaluate(()=>window.fixtureErrors??[])},null,2));}
 await writeFile(join(output,'failure.json'),JSON.stringify({error:String(error),stack:error.stack,report},null,2));throw error;
}finally{await browser.close();await vite.close();}
