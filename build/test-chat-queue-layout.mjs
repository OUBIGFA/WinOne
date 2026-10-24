// Real ChatPage layout and controls; only the Host is a local fixture.
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {pathToFileURL} from 'node:url';
import {execFileSync} from 'node:child_process';
const output=resolve(process.argv[2]);execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);await mkdir(output,{recursive:true});
const frontend=resolve('apps/desktop/frontend'),{chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const {createServer}=await import(pathToFileURL(join(frontend,'node_modules/vite/dist/node/index.js')).href);
const vite=await createServer({root:frontend,cacheDir:join(output,'vite-cache'),server:{host:'127.0.0.1',port:0},logLevel:'warn'});await vite.listen();
const browser=await chromium.launch({headless:true,channel:'msedge'}),results=[];let page;
const settle=()=>page.evaluate(async()=>{await document.fonts.ready;await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));});
const geometry=()=>page.evaluate(()=>{const box=selector=>{const r=document.querySelector(selector).getBoundingClientRect();return {top:r.top,bottom:r.bottom,left:r.left,right:r.right,height:r.height};};return {nav:box('.chat-scroll-controls'),input:box('.chat-input-content'),viewport:box('.chat-transcript-viewport'),scroll:document.querySelector('.chat-transcript-viewport').scrollTop};});
try{
 for(const [width,theme,language] of [[1600,'light','zh-CN'],[1120,'dark','en-US'],[420,'light','en-US'],[420,'dark','zh-CN']]){
  page=await browser.newPage({viewport:{width,height:1000},reducedMotion:'reduce'});const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto('http://127.0.0.1:'+vite.httpServer.address().port+'/tests/chat-experience-preview.html?alignment&queueFixture&running=true&theme='+theme+'&language='+language);
  await page.locator('.chat-pending-queue').waitFor();await settle();
  const labels=await page.evaluate(async()=>{const {chatMessages}=await import('/src/locales/chat.ts');return chatMessages[document.documentElement.lang.startsWith('zh')?'zh':'en'];});
  const input=page.getByRole('textbox',{name:labels.message}),original=await input.inputValue();
  const rows=page.locator('.chat-pending-queue [role=listitem]');assert.equal(await rows.count(),3);
  await page.locator('.chat-pending-queue button[aria-expanded]').click();assert.equal(await rows.count(),5);
  await rows.first().hover();await rows.first().getByRole('button',{name:labels.queue_editAria.replace('{{index}}','1')}).click();await page.waitForFunction(()=>document.querySelector('.chat-composer textarea').value==='Queued input 0');
  await input.fill('Edited queue text');await page.getByRole('button',{name:labels.queue_save}).click();await page.waitForFunction(value=>document.querySelector('.chat-composer textarea').value===value,original);
  assert.equal(await rows.first().getByText('Edited queue text',{exact:true}).count(),1);
  await rows.first().hover();await rows.first().getByRole('button',{name:labels.queue_editAria.replace('{{index}}','1')}).click();await input.fill('Discard this edit');await page.getByRole('button',{name:labels.queue_cancelEdit}).click();assert.equal(await input.inputValue(),original);
  await rows.first().hover();await rows.first().getByRole('button',{name:labels.queue_steerAria.replace('{{index}}','1')}).click();await page.waitForFunction(()=>!document.querySelector('.chat-pending-queue')?.textContent.includes('Edited queue text'));
  while(await rows.count()){await rows.first().hover();await rows.first().getByRole('button',{name:labels.queue_removeAria.replace('{{index}}','1')}).click();await settle();}
  const viewport=page.locator('.chat-transcript-viewport');
  // Keep the selected process visible while the input is resized. Its own
  // body and scroll viewport, rather than the overlay, own the action bounds.
  await viewport.evaluate(node=>node.scrollTop=0);await page.locator('[data-slot=cindy-work]').first().click();await page.locator('[data-live-work-activity=thinking]').first().click();await page.locator('.process-collapse-action').waitFor();await settle();
  const before=await geometry(),actionBefore=await page.locator('.process-collapse-action').boundingBox();
  const grip=page.locator('.chat-composer-resize');await grip.focus();await grip.press('ArrowUp');await grip.press('ArrowUp');await settle();
  const after=await geometry(),actionAfter=await page.locator('.process-collapse-action').boundingBox();
  assert.equal(after.viewport.height,before.viewport.height);assert(Math.abs(after.scroll-before.scroll)<1);
  assert(Math.abs(actionAfter.y-actionBefore.y)<1,'Process collapse followed the composer');
  if(before.nav.bottom<after.input.top-32||before.nav.left>=after.input.right)assert(Math.abs(before.nav.top-after.nav.top)<1,'Navigation moved without a collision');
  for(let i=0;i<28;i++)await grip.press('ArrowUp');await settle();
  const tall=await geometry();assert(tall.nav.right<=tall.input.left||tall.nav.left>=tall.input.right||tall.nav.bottom<=tall.input.top,'Navigation covers the editor');
  assert(tall.nav.left>=tall.viewport.left&&tall.nav.right<=tall.viewport.right,'Header navigation is clipped in a narrow pane: '+JSON.stringify(tall));
  for(let i=0;i<30;i++)await grip.press('ArrowDown');await settle();
  const selector=page.getByRole('button',{name:labels.changeRecords,exact:true});
  await page.getByRole('button',{name:language==='zh-CN'?'返回底部':'Go to bottom',exact:true}).click();await settle();await selector.click();await page.getByRole('menu').waitFor();
  const menu=await page.getByRole('menu').evaluate(node=>{const r=node.getBoundingClientRect();return {left:r.left,right:r.right,bottom:r.bottom,width:node.clientWidth,scroll:node.scrollWidth,height:node.clientHeight,scrollHeight:node.scrollHeight};});
  assert(menu.left>=0&&menu.right<=width&&menu.bottom<=1000,JSON.stringify(menu));assert(menu.scroll<=menu.width+1,'File history menu overflows horizontally');assert(menu.scrollHeight>menu.height,'Long history remains vertically scrollable');
  await page.screenshot({path:join(output,width+'-'+theme+'-history.png')});await page.keyboard.press('Escape');await settle();
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);assert.deepEqual(errors,[]);
  results.push({width,theme,language,queueEdit:true,originalDraftRestored:true,cancel:true,steer:true,processStationary:true,before,after,tall,menu});await page.close();page=null;
 }
 await writeFile(join(output,'result.json'),JSON.stringify({passed:true,results},null,2));console.log('PASS: queue controls and layout in '+results.length+' window/theme/language combinations');
}catch(error){if(page){await page.screenshot({path:join(output,'failure.png')});await writeFile(join(output,'failure.html'),await page.content());}throw error;}
finally{await browser.close();await vite.close();}
