import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {execFileSync} from 'node:child_process';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const base=process.env.WINONE_LAYOUT_BASE||'http://127.0.0.1:5198';
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||'_temp/ui-recheck-20261011/chat-layout');
execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,...(process.env.WINONE_BROWSER_EXE?{executablePath:process.env.WINONE_BROWSER_EXE}:{channel:'msedge'})}),results=[],failures=[];
const check=(ok,message)=>{if(!ok)throw Error(message);};
try{
 for(const width of [760,1360])for(const theme of ['light','dark']){
  const page=await browser.newPage({viewport:{width,height:900},reducedMotion:'reduce'});
  try{
   await page.goto(`${base}/tests/chat-experience-preview.html?welcome&theme=${theme}`);
   await page.locator('.chat-composer textarea').waitFor();
   const welcome=await page.evaluate(()=>{const bounds=s=>document.querySelector(s).getBoundingClientRect().toJSON();return {logo:bounds('.chat-welcome-logo'),composer:bounds('.chat-composer'),main:bounds('.chat-main'),project:bounds('.chat-project-context')};});
   check(welcome.logo.y>180,'Welcome logo is still at the top of the workspace');
   check(welcome.logo.bottom<welcome.project.y&&welcome.composer.bottom<880,'Welcome group overflows or overlaps');
   const welcomeGap=welcome.composer.y-welcome.project.bottom;
   check(welcomeGap>=-16&&welcomeGap<=0,'Welcome project is too far from composer');
   await page.goto(`${base}/tests/chat-experience-preview.html?theme=${theme}`);
   await page.locator('.desktop-conversation-message--assistant .chat-markdown').first().waitFor();
   const gap=await page.evaluate(()=>{const composer=document.querySelector('.chat-composer').getBoundingClientRect(),project=document.querySelector('.chat-input-area .chat-project-context').getBoundingClientRect();return composer.top-project.bottom;});
   check(Math.abs(gap-welcomeGap)<1&&gap<=12,'Project/composer spacing differs between new and existing chats');
   const actions=await page.locator('.desktop-conversation-message--assistant').last().evaluate(root=>{const article=root.firstElementChild.getBoundingClientRect(),buttons=root.querySelector('.desktop-conversation-message-actions').getBoundingClientRect();return {gap:buttons.top-article.bottom,article,buttons};});
   check(actions.gap>=0&&actions.gap<=10,'Message actions are detached from the bottom of the reply');
   const input=page.locator('.chat-composer textarea'),model=page.locator('.chat-model-trigger');
   const before=await model.boundingBox();await input.fill('第一行\n第二行\n第三行\n第四行');const after=await model.boundingBox();
   check(Math.abs(before.x-after.x)<1&&Math.abs(before.y-after.y)<1,'Typing or saving moved the model control');
   await model.click();
   const first=page.getByRole('option',{name:/Example Model/}),second=page.getByRole('option',{name:/Another Model/});
   const optionMetrics=async option=>option.evaluate(node=>{const root=node.getBoundingClientRect();const r=s=>{const box=node.querySelector(s).getBoundingClientRect();return {x:box.left-root.left,width:box.width,right:root.right-box.right};};return {label:r('.chat-model-label'),badge:r('[data-model-badge]'),check:r('.chat-model-check')};});
   const choiceBefore=await optionMetrics(second);await second.click();await model.click();const choiceAfter=await optionMetrics(second);
   check(Math.abs(choiceBefore.label.x-choiceAfter.label.x)<1&&Math.abs(choiceBefore.badge.x-choiceAfter.badge.x)<1,'Model choice moved label or badges: '+JSON.stringify({choiceBefore,choiceAfter}));
   const indent=await first.evaluate(node=>node.querySelector('[data-icon]')?.getBoundingClientRect().left-node.closest('section').querySelector('h4 [data-icon]').getBoundingClientRect().left);
   check(indent>=20,'Model hierarchy indentation was removed');await page.keyboard.press('Escape');
   const handle=page.getByRole('separator',{name:'调整输入框高度'}),frameBefore=await page.locator('.chat-composer').boundingBox();
   await handle.press('ArrowUp');const frameAfter=await page.locator('.chat-composer').boundingBox();
   check(frameAfter.height>frameBefore.height,'Composer cannot be resized');
   const details=page.getByRole('button',{name:'显示会话详情'});if(await details.count())await details.click();
   const inspectorBefore=await page.locator('.chat-panel-content:not([hidden]) .chat-file-browser').boundingBox();
   const work=page.locator('[data-slot="cindy-work"]').first();await work.click();
   await page.locator('.process-body').first().waitFor();
   for(let step=0;step<7;step++)await handle.press('ArrowUp');
   const boundary=await page.evaluate(()=>{const transcript=document.querySelector('.chat-transcript').getBoundingClientRect(),composer=document.querySelector('.chat-composer').getBoundingClientRect(),process=document.querySelector('.process-body').getBoundingClientRect();return {gap:composer.top-transcript.bottom,processHeight:process.height,available:transcript.height};});
   check(boundary.gap<0&&boundary.processHeight>boundary.available,'Expanded process was constrained instead of scrolling with the transcript');
   const inspectorAfter=await page.locator('.chat-panel-content:not([hidden]) .chat-file-browser').boundingBox();
   check(Math.abs(inspectorBefore.height-inspectorAfter.height)<1,'Resizing composer changed the independent details panel');
   const tracks=await page.locator('.lobe-scroll-root').evaluateAll(roots=>roots.map(root=>{const view=root.querySelector('.lobe-scroll-viewport');return {root:root.getBoundingClientRect().width,view:view.getBoundingClientRect().width};}).filter(r=>r.root>30));
   check(tracks.every(r=>r.root-r.view>=16),'Custom scrollbar track still overlaps the content viewport');
   await page.screenshot({path:join(output,`chat-${width}-${theme}.png`)});
   results.push({width,theme,welcome,gap,actions,modelStable:true,choiceStable:true,indent,composerResizable:true,boundary,tracks});
  }catch(error){failures.push({width,theme,error:String(error)});await page.screenshot({path:join(output,`failure-${width}-${theme}.png`)});}
  finally{await page.close();}
 }
 await writeFile(join(output,'report.json'),JSON.stringify({results,failures},null,2));
 check(!failures.length,JSON.stringify(failures));console.log(JSON.stringify({cases:results.length,passed:true}));
}finally{await browser.close();}
