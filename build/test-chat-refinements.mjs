// Browser geometry and shared styling checks against the real Chat component.
// This fixture owns its data; no provider calls or user files are accessed.
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {execFileSync} from 'node:child_process';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [base,folder]=process.argv.slice(2),output=resolve(folder);
assert.equal(new URL(base).hostname,'127.0.0.1');
execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,...(process.env.WINONE_BROWSER_EXE?{executablePath:process.env.WINONE_BROWSER_EXE}:{channel:'msedge'})});
const results=[];
try{
 const page=await browser.newPage({viewport:{width:1280,height:900},reducedMotion:'reduce'});
 const errors=[];page.on('pageerror',e=>errors.push(e.message));
 const settle=()=>page.evaluate(async()=>{await document.fonts.ready;await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));});
 const box=selector=>page.locator(selector).boundingBox();
 const load=async(welcome,theme='light',width=1280,height=900)=>{
  await page.setViewportSize({width,height});await page.goto(`${base}/tests/chat-experience-preview.html?language=en-US&theme=${theme}${welcome?'&welcome':''}`);
  await page.locator('.chat-composer').waitFor();await settle();
 };
 for(const theme of ['light','dark'])for(const [width,height] of [[1280,900],[720,600]]){
  await load(true,theme,width,height);await page.evaluate(()=>localStorage.removeItem('winone.chat.composer-height.v1'));await page.reload();await page.locator('.chat-composer').waitFor();await settle();
  const logo=await box('.chat-welcome-logo'),area=await box('.chat-welcome-composer'),stage=await box('.chat-welcome-stage');
  assert.ok(Math.abs((logo.y+area.y+area.height)/2-(stage.y+stage.height/2))<2,'welcome group is not centered');
  const before=await box('.chat-composer'),top=page.locator('.chat-composer-resize');
  assert.equal(await page.locator('.chat-composer-resize--bottom').count(),0,'welcome retains a second grip');
  const upper=await top.boundingBox();await page.mouse.move(upper.x+upper.width/2,upper.y+upper.height/2);await page.mouse.down();await page.mouse.move(upper.x+upper.width/2,upper.y-32,{steps:8});await page.mouse.up();await settle();
  const grownDown=await box('.chat-composer');assert.ok(grownDown.y<before.y,'top grip did not follow pointer');assert.ok(Math.abs(grownDown.y+grownDown.height-before.y-before.height)<2,'small growth moved the bottom anchor');
  await top.focus();for(let i=0;i<40;i++){await top.press('ArrowUp');await settle();}await settle();const grownUp=await box('.chat-composer');
  const docked=await box('.chat-welcome-composer');assert.ok(Math.abs(docked.y-stage.y-12)<2,'editor does not fill available height: '+JSON.stringify({stage,docked,grownUp}));assert.ok(Math.abs(docked.y+docked.height-stage.y-stage.height+12)<2,'resized welcome did not reach page bottom');
  assert.ok(await page.evaluate(()=>document.documentElement.scrollHeight<=innerHeight&&document.documentElement.scrollWidth<=innerWidth),'welcome created page scrollbars');
  await page.screenshot({path:join(output,`welcome-${theme}-${width}.png`)});results.push({theme,width,height,logo,area,grownDown,grownUp});
 }
 await load(true,'light',720,600);await page.evaluate(()=>localStorage.setItem('winone.chat.composer-height.v1','550'));await page.reload();await page.locator('.chat-composer').waitFor();await settle();
 const savedLogo=await box('.chat-welcome-logo'),savedStage=await box('.chat-welcome-stage');assert.ok(savedLogo.y>=savedStage.y,'saved tall editor hides the initial logo');
 await load(false);await page.locator('.chat-markdown h2').waitFor();
 const viewport=page.locator('.chat-transcript-viewport');
 const process=page.locator('[data-slot=cindy-work]').first();await process.click();await settle();
 await viewport.evaluate(e=>e.scrollTop=e.scrollHeight);await page.locator('.process-collapse-action').click();await settle();
 const heading=await process.boundingBox(),view=await viewport.boundingBox();assert.ok(Math.abs(heading.y-view.y-8)<3,'collapse did not return to process heading');
 assert.equal(await page.locator('.process-body').count(),0);
 assert.equal(await page.locator('.chat-input-area').evaluate(e=>getComputedStyle(e,'::before').backgroundImage),'none');
 assert.equal(await page.locator('.chat-input-area').evaluate(e=>getComputedStyle(e).borderTopWidth),'0px');
 await page.locator('.chat-model-trigger').click();await page.locator('.chat-model-menu').waitFor();
 const modelStyles=await page.locator('.chat-model-menu').evaluate(e=>{const header=getComputedStyle(e.querySelector('.chat-model-group')),row=getComputedStyle(e.querySelector('[role=option]')),group=getComputedStyle(e.querySelector('h4'));return {border:header.borderTopWidth,radius:header.borderRadius,fill:header.backgroundColor,group:group.backgroundColor,rowBorder:row.borderTopWidth};});
 assert.equal(modelStyles.border,'1px');assert.equal(modelStyles.radius,'999px');assert.equal(modelStyles.rowBorder,'0px');assert.equal(modelStyles.group,'rgba(0, 0, 0, 0)');
 await page.keyboard.press('Escape');await page.getByRole('button',{name:'Show chat details',exact:true}).click();await page.locator('.client-tools-grid').waitFor();
 await page.getByRole('button',{name:/^Files.*Ctrl/}).click();await settle();
 const tabs=await page.locator('.client-browser-tab').evaluateAll(items=>items.filter(e=>e.querySelector('[aria-selected=false]')).map(e=>getComputedStyle(e).backgroundColor));assert.ok(tabs.length);assert.ok(tabs.every(color=>color==='rgb(255, 255, 255)'));
 await page.getByRole('button',{name:'Tab actions',exact:true}).click();await page.getByRole('menuitem',{name:'Resources',exact:true}).click();await page.locator('.chat-resource-sections').waitFor();
 assert.equal(await page.locator('.chat-resource-sections h3>svg').count(),4);await page.screenshot({path:join(output,'resources.png')});
 assert.deepEqual(errors,[]);results.push({modelStyles,tabs,errors});
 await writeFile(join(output,'report.json'),JSON.stringify(results,null,2));console.log(JSON.stringify({checks:results.length,passed:true}));
}finally{await browser.close();}
