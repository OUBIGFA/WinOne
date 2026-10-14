import {fileURLToPath} from 'node:url';
import {createRequire} from 'node:module';
import {readFile,writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import {execFileSync} from 'node:child_process';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const probe=fileURLToPath(new URL('../../../../build/desktop-resize-probe.ps1',import.meta.url));
const [endpoint,output]=process.argv.slice(2);let browser;
const deadline=Date.now()+20000;
while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:500});break}catch{await new Promise(r=>setTimeout(r,50))}}
if(!browser)throw Error('No isolated UI');
try{
 let page;
 while(Date.now()<deadline){page=browser.contexts().flatMap(c=>c.pages()).find(p=>p.url().includes('winone.localhost'));if(page)break;await new Promise(r=>setTimeout(r,50))}
 await page.getByRole('navigation').waitFor();
 if(process.env.WINONE_NATIVE_SHOT_PID){
  execFileSync('powershell.exe',['-NoProfile','-File',probe,'-TargetPid',process.env.WINONE_NATIVE_SHOT_PID,'-OutputPath',resolve(output,'cursor-initial.json'),'-ResizeWindow'],{timeout:15000,windowsHide:true});
  await page.waitForFunction(()=>innerWidth===800);
 }
 const results=[],nativeResults=[],scrollResults=[];
 for(const label of ['首页','截图','置顶功能','程序管理','设置']){
  await page.getByRole('button',{name:label,exact:true}).click();
  if(label==='截图'){
   await page.getByRole('tab',{name:'截图后',exact:true}).waitFor();
  }
  const {w,h}=await page.evaluate(()=>({w:innerWidth,h:innerHeight}));
  for(const [edge,x,y] of [['nw',1,1],['n',w/2,1],['ne',w-2,1],['w',1,h/2],['e',w-2,h/2],['sw',1,h-2],['s',w/2,h-2],['se',w-2,h-2]]){
   await page.mouse.move(w/2,h/2);await page.mouse.move(x,y);
   const state=await page.evaluate(({x,y})=>({edge:window.wails?.flags.resizeEdge,cursor:getComputedStyle(document.elementFromPoint(x,y)).cursor,root:document.documentElement.dataset.windowResizing,tag:document.elementFromPoint(x,y).tagName}),{x,y});
   results.push({page:label,expectedEdge:edge,x,y,...state});
   if(state.edge!==`${edge}-resize`||state.cursor!==`${edge}-resize`)throw Error(`${label}: wrong DOM cursor at ${edge}`);
  }
  if(process.env.WINONE_NATIVE_SHOT_PID){
   const path=resolve(output,`cursor-${label}.json`);
   execFileSync('powershell.exe',['-NoProfile','-File',probe,'-TargetPid',process.env.WINONE_NATIVE_SHOT_PID,'-OutputPath',path,'-Drag'],{timeout:15000,windowsHide:true});
   const samples=JSON.parse(await readFile(path,'utf8'));nativeResults.push({page:label,samples});
   if(samples.length!==8||samples.some(sample=>!sample.Pass||!sample.Dragged))throw Error(`${label}: native cursor or actual resize failed`);
  }
  const scroll=page.locator('[data-slot="page-scroll"]');
  const scrollbar=await scroll.evaluate(element=>{
   element.scrollTop=0;const rect=element.getBoundingClientRect();
   const width=element.offsetWidth-element.clientWidth;
   return {active:element.scrollHeight>element.clientHeight+1&&width>6,x:rect.right-width/2,y:rect.top+Math.min(45,element.clientHeight*element.clientHeight/element.scrollHeight/2)};
  });
  if(scrollbar.active){
   await page.mouse.move(scrollbar.x,scrollbar.y);await page.mouse.down();await page.mouse.move(scrollbar.x,scrollbar.y+90,{steps:6});await page.mouse.up();
   const scrolled=await scroll.evaluate(element=>element.scrollTop>0);scrollResults.push({page:label,scrolled});
   if(!scrolled)throw Error(`${label}: scrollbar drag was blocked`);
  }
 }
 await page.evaluate(()=>window.go.main.App.WindowCommand('toggle-maximize'));
 await page.waitForFunction(()=>document.querySelectorAll('[data-window-resize-handle]').length===0);
 await page.evaluate(()=>window.go.main.App.WindowCommand('toggle-maximize'));
 await page.waitForFunction(()=>document.querySelectorAll('[data-window-resize-handle]').length===8);
 await page.screenshot({path:resolve(output,'restored-settings.png')});
 await writeFile(resolve(output,'resize.json'),JSON.stringify({results,nativeResults,scrollResults,maximizeRestore:true},null,2));
 await page.evaluate(()=>{void window.go.main.App.WindowCommand('close')}).catch(e=>{if(!String(e).includes('closed'))throw e});
}catch(error){await writeFile(resolve(output,'node-error.txt'),String(error.stack||error));throw error}finally{await browser.close()}
