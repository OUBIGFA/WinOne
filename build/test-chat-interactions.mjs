// Exercise the real ChatPage with only the native transport replaced by fixtures.
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {execFileSync} from 'node:child_process';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [base,folder]=process.argv.slice(2),output=resolve(folder),results=[];
assert.equal(new URL(base).hostname,'127.0.0.1');execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,...(process.env.WINONE_BROWSER_EXE?{executablePath:process.env.WINONE_BROWSER_EXE}:{channel:'msedge'})});
const settle=page=>page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
async function geometry(page){return page.evaluate(()=>{const rect=selector=>{const box=document.querySelector(selector).getBoundingClientRect();return {top:box.top,bottom:box.bottom,height:box.height,left:box.left,right:box.right};};return {stage:rect('.chat-welcome-stage'),logo:rect('.chat-welcome-logo'),area:rect('.chat-welcome-composer'),composer:rect('.chat-composer')};});}
try{
 for(const width of [640,1120,1600])for(const height of [640,900]){
  const page=await browser.newPage({viewport:{width,height}}),errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.addInitScript(()=>localStorage.setItem('winone.chat.composer-height.v1','620'));
  await page.goto(new URL('/tests/chat-experience-preview.html?welcome&regression',base).href);await page.locator('.chat-welcome-logo').waitFor();await settle(page);
  const before=await geometry(page);assert.ok(Math.abs((before.logo.top+before.area.bottom)/2-(before.stage.top+before.stage.bottom)/2)<2,JSON.stringify(before));
  assert.equal(await page.locator('.chat-composer-resize').count(),1);assert.ok(before.composer.height<=220.5);if(width===1600)assert.ok(before.composer.right-before.composer.left>=958,'Wide chat did not use the requested 960px reading width');
  const grip=await page.locator('.chat-composer-resize').boundingBox();await page.mouse.move(grip.x+grip.width/2,grip.y+grip.height/2);await page.mouse.down();await page.mouse.move(grip.x+grip.width/2,grip.y-35,{steps:8});await settle(page);
  const middle=await geometry(page);assert.ok(middle.composer.top<before.composer.top-20);assert.ok(Math.abs(middle.composer.bottom-before.composer.bottom)<2,'Small growth changed the bottom anchor');
  await page.mouse.move(grip.x+grip.width/2,0,{steps:12});await page.mouse.up();await settle(page);
  const full=await geometry(page);assert.ok(full.area.top>=full.stage.top+8);assert.ok(full.area.bottom<=full.stage.bottom-8);assert.ok(full.area.height>=full.stage.height-28,JSON.stringify(full));
  await page.getByRole('button',{name:'新对话',exact:true}).click();await settle(page);const reopened=await geometry(page);assert.ok(reopened.composer.height<=220.5);assert.ok(Math.abs((reopened.logo.top+reopened.area.bottom)/2-(reopened.stage.top+reopened.stage.bottom)/2)<2);
  const smallGrip=await page.locator('.chat-composer-resize').boundingBox();await page.mouse.move(smallGrip.x+smallGrip.width/2,smallGrip.y+smallGrip.height/2);await page.mouse.down();await page.mouse.move(smallGrip.x+smallGrip.width/2,smallGrip.y+100,{steps:6});await settle(page);const minimum=await geometry(page);await page.mouse.move(smallGrip.x+smallGrip.width/2,smallGrip.y+240,{steps:6});await settle(page);const unchanged=await geometry(page);await page.mouse.up();assert.equal(Math.round(minimum.composer.height),132);assert.ok(Math.abs(unchanged.composer.top-minimum.composer.top)<1,'Minimum-height composer kept drifting down');
  await page.getByRole('button',{name:'新对话',exact:true}).click();await settle(page);
  await page.getByRole('button',{name:'模型',exact:true}).click();await page.getByRole('option',{name:'model-72',exact:true}).waitFor();await settle(page);
  const selected=await page.getByRole('option',{name:'model-72',exact:true}).boundingBox(),viewport=await page.locator('.chat-model-viewport').boundingBox();assert.ok(selected.y>=viewport.y&&selected.y+selected.height<=viewport.y+viewport.height,'Selected model is outside the visible list');
  assert.equal(await page.getByRole('textbox',{name:'搜索模型'}).evaluate(node=>node===document.activeElement),true);
  await page.keyboard.press('Escape');await page.getByRole('listbox').waitFor({state:'hidden'});await page.screenshot({path:join(output,`welcome-${width}-${height}.png`)});assert.deepEqual(errors,[]);results.push({width,height,centered:true,continuousResize:true,fullHeight:true,newChatReset:true,selectedModelVisible:true});await page.close();
 }
 const page=await browser.newPage({viewport:{width:1120,height:900}});
 await page.goto(new URL('/tests/chat-experience-preview.html?regression',base).href);await page.locator('.chat-markdown h2').waitFor();
 await page.getByRole('alert').filter({hasText:'Request aborted'}).getByRole('button',{name:'关闭'}).click();await page.waitForTimeout(1500);assert.equal(await page.getByText('Request aborted',{exact:true}).count(),0);
 await page.getByRole('button',{name:'Chat 设置'}).click();await page.getByRole('textbox',{name:'默认工作目录'}).fill('C:\\fixture\\updated');await page.getByRole('button',{name:'保存设置'}).click();await page.getByRole('dialog').waitFor({state:'hidden'});assert.equal(await page.getByText('设置已保存',{exact:true}).count(),0);
 const transcript=page.locator('.chat-transcript-viewport');await transcript.evaluate(node=>{node.scrollTop=0;});await page.getByRole('button',{name:'加载更早消息'}).focus();
 const anchor=await page.locator('.desktop-conversation-message--user').first().boundingBox();await page.keyboard.press('Enter');await page.getByText('正在加载更早消息',{exact:true}).waitFor();await page.waitForFunction(()=>document.querySelector('.desktop-conversation-message--user')?.textContent.includes('Earlier message 0'));await settle(page);
 const after=await page.getByText('检查项目文件，说明修改结果。',{exact:true}).evaluate(node=>node.closest('.desktop-conversation-message--user').getBoundingClientRect().y);assert.ok(await transcript.evaluate(node=>node.scrollTop<2),'Earlier messages were not brought into view');
 assert.equal(await page.getByRole('textbox',{name:'消息'}).evaluate(node=>node===document.activeElement),false,'Loading history stole editor focus');
 await page.screenshot({path:join(output,'history.png')});results.push({dismissalPersists:true,settingsClose:true,historyAnchor:true,noFocusGrab:true});await page.close();
 await writeFile(join(output,'report.json'),JSON.stringify({cases:results.length,results},null,2));console.log(`Chat interactions: ${results.length} cases passed.`);
}finally{await browser.close();}
