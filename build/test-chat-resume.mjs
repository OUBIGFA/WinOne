// Real browser geometry for the current Chat repair requirements.
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import path from 'node:path';
import {execFileSync} from 'node:child_process';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [base,folder]=process.argv.slice(2),output=path.resolve(folder);
assert.equal(new URL(base).hostname,'127.0.0.1');execFileSync('git',['check-ignore','--quiet','--no-index',path.join(output,'probe')]);await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,channel:'msedge'}),report=[];
const settle=page=>page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
const geometry=page=>page.evaluate(()=>{const box=selector=>{const r=document.querySelector(selector).getBoundingClientRect();return {top:r.top,bottom:r.bottom,left:r.left,right:r.right,width:r.width,height:r.height};};return {stage:box('.chat-welcome-stage'),logo:box('.chat-welcome-logo'),area:box('.chat-welcome-composer'),composer:box('.chat-composer'),toolbar:box('.chat-composer-toolbar'),context:box('.chat-project-context')};});
try{
 for(const width of [640,1120,1600])for(const height of [640,900]){
  const page=await browser.newPage({viewport:{width,height},reducedMotion:'reduce'}),errors=[];page.on('pageerror',e=>errors.push(e.message));
  await page.goto(`${base}/tests/chat-experience-preview.html?welcome`);await page.locator('.chat-composer').waitFor();await settle(page);
  const before=await geometry(page);
  assert.ok(Math.abs(before.context.left-before.composer.left-12)<1&&Math.abs(before.composer.right-before.context.right-12)<1,'project bar lost its fixed inset: '+JSON.stringify(before));
  assert.ok(Math.abs(before.context.bottom-before.composer.top-16)<1,'project bar detached from composer');
  assert.ok(Math.abs((before.logo.top+before.area.bottom)-(before.stage.top+before.stage.bottom))<4,'initial group must be centered: '+JSON.stringify(before));
  assert.ok(before.area.top-before.logo.bottom>=127,'logo gap');
  const input=page.getByRole('textbox',{name:'消息'});await input.fill('短内容');await settle(page);const short=await geometry(page);
  assert.ok(Math.abs(short.composer.height-before.composer.height)<1,'short input changed height');
  await input.fill(Array.from({length:150},(_,i)=>`第 ${i+1} 行，中文长内容用于检查自动增高。`).join('\n'));await settle(page);
  const long=await geometry(page);assert.ok(Math.abs(long.context.bottom-long.composer.top-16)<1,'project bar drifted during growth');assert.ok(long.composer.height>before.composer.height,'long input did not grow');
  assert.ok(long.area.top>=long.stage.top+8&&long.area.bottom<=long.stage.bottom-8,'editor outside viewport');
  assert.ok(long.area.height>=long.stage.height-28,'long editor did not fill viewport: '+JSON.stringify(long));
  assert.ok(long.toolbar.bottom<=long.composer.bottom&&long.toolbar.top>=long.composer.top,'toolbar escaped');
  await input.fill('');await settle(page);const restored=await geometry(page);assert.ok(Math.abs(restored.composer.height-before.composer.height)<1,'empty input did not return to default: '+JSON.stringify({before,restored,input:await input.evaluate(n=>({value:n.value,height:n.scrollHeight,css:getComputedStyle(n).cssText,min:getComputedStyle(n).minHeight,flex:getComputedStyle(n).flex,inline:n.style.cssText}))}));
  // Hidden WebViews report zero bounds during minimize; simulate this layout
  // transition without touching the user's installed desktop process.
  await page.locator('.chat-main').evaluate(node=>node.style.display='none');await settle(page);await page.locator('.chat-main').evaluate(node=>node.style.display='');await settle(page);
  const shown=await geometry(page);assert.ok(Math.abs(shown.composer.top-before.composer.top)<2,'restore moved composer');
  await page.getByRole('button',{name:'附件与工具',exact:true}).click();await page.getByRole('menuitem',{name:'添加附件'}).click();await page.locator('.cindy-attachment-card').last().waitFor();await settle(page);
  const attached=await geometry(page);assert.ok(attached.toolbar.bottom<=attached.composer.bottom&&attached.toolbar.top>=attached.composer.top,'attachments pushed out the toolbar');
  assert.equal(await page.locator('.cindy-attachment-card').count(),18);
  await page.screenshot({path:path.join(output,`welcome-${width}-${height}.png`)});assert.deepEqual(errors,[]);report.push({width,height,before,long,shown});await page.close();
 }
 const page=await browser.newPage({viewport:{width:1120,height:900}});
 await page.goto(`${base}/tests/chat-experience-preview.html?regression`);await page.locator('.chat-markdown h2').waitFor();
 assert.equal(await page.locator('[data-slot="cindy-work"]').count(),2,'completed work must remain split');
 assert.equal(await page.getByText('已读完配置。接下来整理需要修改的文件。',{exact:true}).count(),1,'delivered prose hidden');
 await page.getByRole('button',{name:'加载更早消息'}).click();await page.waitForFunction(()=>document.querySelector('.desktop-conversation-message--user')?.textContent.includes('Earlier message 0'));await settle(page);
 assert.equal(await page.getByRole('button',{name:'加载更早消息'}).count(),0);assert.equal(await page.getByText('更早消息已加载，向上滚动查看').count(),0);
 assert.ok(await page.locator('.chat-transcript-viewport').evaluate(node=>node.scrollTop<2),'history not scrolled to newly loaded messages');
 await page.screenshot({path:path.join(output,'history.png')});
 await writeFile(path.join(output,'report.json'),JSON.stringify({passed:true,report},null,2));console.log('Chat repair browser checks passed: 6 sizes and history/work segmentation.');
}finally{await browser.close();}
