// Production Chat components with isolated Host data; no personal files/models.
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {pathToFileURL} from 'node:url';
import {execFileSync} from 'node:child_process';
const output=resolve(process.argv[2]);execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);await mkdir(output,{recursive:true});
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const frontend=resolve('apps/desktop/frontend'),{createServer}=await import(pathToFileURL(join(frontend,'node_modules/vite/dist/node/index.js')).href);
const vite=await createServer({root:frontend,cacheDir:join(output,'vite-cache'),server:{host:'127.0.0.1',port:0},logLevel:'warn'});await vite.listen();
const base=`http://127.0.0.1:${vite.httpServer.address().port}`;
const browser=await chromium.launch({headless:true,channel:'msedge'}),report=[];let page;
const settle=()=>page.evaluate(async()=>{await document.fonts.ready;await new Promise(done=>requestAnimationFrame(()=>requestAnimationFrame(done)));});
try{
 for(const theme of ['light','dark'])for(const width of [1280,420]){
  const errors=[];page=await browser.newPage({viewport:{width,height:940},reducedMotion:'reduce'});page.on('pageerror',error=>errors.push(error.message));
  await page.goto(`${base}/tests/chat-experience-preview.html?liveThinking&growThinking&statusRows&running=true&theme=${theme}`);
  await page.locator('[data-session-id=experience][data-activity=running]').waitFor();await settle();
  const rows=await page.locator('.chat-sidebar-item').evaluateAll(items=>items.map(item=>{const title=item.querySelector('.chat-sidebar-title').getBoundingClientRect(),row=item.querySelector('.chat-sidebar-row').getBoundingClientRect(),icons=item.querySelectorAll('[data-sidebar-right-status]');return {id:item.dataset.sessionId,count:icons.length,leading:[...icons].every(icon=>icon.classList.contains('chat-sidebar-leading-status')),titleInset:title.left-row.left};}));
  for(const row of rows){assert.equal(row.count,row.id==='empty'?0:1);assert(row.leading);assert(Math.abs(row.titleInset-28)<1,'Status moved title: '+JSON.stringify(row));}
  await page.getByRole('button',{name:'收起会话列表',exact:true}).click();
  const thought=page.locator('[data-live-work-activity=thinking]');await thought.click();await settle();
  assert.equal(await page.locator('.process-collapse-action').count(),0,'Short process has a collapse overlay');
  // Only a model snapshot grows the content. No scroll gesture is sent.
  await page.evaluate(()=>window.chatExperienceGrowThinking());
  await page.locator('.process-body[data-process-long=true]').waitFor();
  const collapse=page.locator('.process-collapse-action');await collapse.waitFor();await settle();
  const visible=await collapse.boundingBox();assert(visible&&visible.height>0);
  // At the natural bottom it must remain available, not disappear until the
  // user first nudges upward (the original regression).
  await page.locator('.process-body[data-process-long=true]').first().evaluate(node=>{const viewport=node.closest('.chat-transcript-viewport'),footer=document.querySelector('.chat-input-area');viewport.scrollTop+=node.getBoundingClientRect().bottom-footer.getBoundingClientRect().top+8;});await settle();
  await collapse.waitFor();const natural=await collapse.boundingBox();assert(natural&&natural.height>0);
  const border=await collapse.evaluate(node=>getComputedStyle(node).borderTopWidth);assert.equal(border,'1px');
  await page.mouse.move(0,0);await page.screenshot({path:join(output,`${theme}-${width}.png`)});
  await collapse.click();await collapse.waitFor({state:'hidden'});assert.equal(await thought.getAttribute('aria-expanded'),'false');
  assert.deepEqual(errors,[]);report.push({theme,width,rows,immediate:visible,natural,border,errors});await page.close();
 }
 await writeFile(join(output,'result.json'),JSON.stringify({passed:true,report},null,2));console.log(`PASS ${report.length} status/stream-growth/collapse scenarios`);
}catch(error){await page?.screenshot({path:join(output,'failure.png')}).catch(()=>{});await writeFile(join(output,'failure.json'),JSON.stringify({error:String(error),stack:error.stack,report},null,2));throw error;}
finally{await browser.close();await vite.close();}
