// Exercise the production Chat page inside the real app shell, using isolated
// provider/file fixtures. This never calls a provider or changes user files.
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
 for(const width of [800,1440])for(const dark of [false,true])for(const running of [false,true]){
  const page=await browser.newPage({viewport:{width,height:1000},reducedMotion:'reduce'}),errors=[];
  page.on('pageerror',error=>errors.push(error.message));
  await page.goto(`${base}/tests/chat-experience-preview.html?alignment&shell&regression&language=en-US&theme=${dark?'dark':'light'}&running=${running}${running?'&titleInherit':''}`,{waitUntil:'domcontentloaded'});
  await page.getByText('I will check the project first.',{exact:true}).waitFor();
  const card=page.locator('.chat-turn-changes');await card.waitFor();
  await page.evaluate(async()=>{await document.fonts.ready;await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));});
  await page.getByText('I will check the project first.',{exact:true}).scrollIntoViewIfNeeded();
  await page.waitForFunction(()=>[...document.querySelectorAll('.desktop-conversation-message--assistant .chat-markdown p')].every(p=>getComputedStyle(p).fontSize==='15px'));
  const metrics=await page.evaluate(()=>{
   const messages=[...document.querySelectorAll('.desktop-conversation-message--assistant')];
   const replies=messages.filter(el=>el.querySelector('.chat-markdown p')).map(el=>{const p=el.querySelector('.chat-markdown p'),s=getComputedStyle(p);return {text:p.textContent,font:s.fontSize,line:s.lineHeight,color:s.color,inProcess:!!p.closest('.chat-work-process')};});
   const shell=document.querySelector('[data-slot=page-scroll]'),workspace=document.querySelector('.chat-workspace');
   return {replies,gutter:getComputedStyle(shell).scrollbarGutter,gap:shell.getBoundingClientRect().right-workspace.getBoundingClientRect().right,cardHeight:document.querySelector('.chat-turn-changes').getBoundingClientRect().height};
  });
  for(const reply of metrics.replies){assert.equal(reply.inProcess,false);assert.equal(reply.font,'15px',JSON.stringify({width,dark,running,reply}));assert.equal(reply.line,'24px');assert.equal(reply.color,metrics.replies[0].color);}
  assert.equal(metrics.gutter,'auto');assert(Math.abs(metrics.gap)<1);assert(metrics.cardHeight<190);
  const groups=page.locator('[data-slot=cindy-work]');assert(await groups.count()>0);
  if(running){for(const group of await groups.all())assert.equal(await group.getAttribute('aria-expanded'),'true');}
  else{
   await groups.first().click();await page.getByText('Inspect the existing files.').waitFor();await groups.first().click();
   assert(await page.getByText('I will check the project first.').isVisible());
   await card.evaluate(el=>el.scrollIntoView({block:'center'}));
   await page.locator('.chat-transcript-viewport').evaluate(el=>{el.scrollTop=el.scrollHeight;});
   const visibleCard=await card.boundingBox(),footer=await page.locator('.chat-input-area').boundingBox();assert(visibleCard.y+visibleCard.height<=footer.y);
   await card.locator('button[aria-expanded=false]:not([aria-haspopup])').click();assert.equal(await card.locator('button[title]').count(),5);
   await page.locator('.chat-transcript-viewport').evaluate(el=>{el.scrollTop=el.scrollHeight;});
   await card.locator('button[aria-expanded=true]:not([aria-haspopup])').click();
   await card.getByRole('button',{name:/Undo/}).click();await card.getByRole('button',{name:'Reapply'}).click();
   await page.getByRole('textbox',{name:'Message',exact:true}).fill('Check the next step.');
   await page.getByRole('button',{name:'Send',exact:true}).click();await page.getByText('The follow-up is complete.').waitFor();
   assert(await page.evaluate(()=>!!(document.querySelector('.chat-transcript-changes').compareDocumentPosition([...document.querySelectorAll('.chat-markdown p')].find(p=>p.textContent==='The follow-up is complete.'))&Node.DOCUMENT_POSITION_PRECEDING)));
  }
  await page.getByRole('button',{name:'Show chat details',exact:true}).click();await page.locator('.chat-side-panel').waitFor();
  await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
  const panel=await page.locator('.chat-side-panel').boundingBox(),shell=await page.locator('[data-slot=page-scroll]').boundingBox();assert(Math.abs(panel.x+panel.width-shell.x-shell.width)<1,JSON.stringify({width,dark,running,panel,shell}));
  const handle=await page.getByRole('separator').last().boundingBox();await page.mouse.move(handle.x+handle.width/2,handle.y+handle.height/2);await page.mouse.down();await page.mouse.move(shell.x+shell.width-2,handle.y+handle.height/2,{steps:14});await page.mouse.up();
  await page.locator('.chat-side-panel').waitFor({state:'hidden'});
  await page.getByRole('button',{name:'Show chat details',exact:true}).click();await page.locator('.chat-side-panel').waitFor();
  await page.getByRole('button',{name:'Hide chat details',exact:true}).click();
  await page.screenshot({path:join(output,`chat-${width}-${dark}-${running}.png`)});
  await page.getByRole('button',{name:'Chat settings',exact:true}).click();
  if(running){await page.getByRole('combobox',{name:'Title generation model · Provider',exact:true}).click();await page.getByRole('option',{name:'Local AI',exact:true}).click();}
  await page.getByRole('combobox',{name:'Title generation model · Model name',exact:true}).click();
  const options=page.getByRole('option');await options.first().waitFor();assert.equal(await options.count(),90);
  await options.last().scrollIntoViewIfNeeded();assert(await options.last().isVisible());
  const last=await options.last().boundingBox(),popup=await page.locator('[data-slot=select-content][data-open]').boundingBox();assert(last.y>=popup.y&&last.y+last.height<=popup.y+popup.height+1);
  await options.first().scrollIntoViewIfNeeded();assert(await options.first().isVisible());
  await page.screenshot({path:join(output,`models-${width}-${dark}-${running}.png`)});
  assert.deepEqual(errors,[]);results.push({width,dark,running,...metrics});await page.close();
 }
 await writeFile(join(output,'report.json'),JSON.stringify(results,null,2));
 console.log(`Chat Cindy alignment: ${results.length} shell, reply, activity, file-change, resize and model-list cases passed.`);
}finally{await browser.close();}
