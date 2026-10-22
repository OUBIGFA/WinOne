// Real Rust WinOne window, with deterministic Chat transport fixtures only.
// This verifies native rendering, not external model/login authentication.
import {createRequire} from 'node:module';
import {writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {execFileSync} from 'node:child_process';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,folder]=process.argv.slice(2),output=resolve(folder);
if(new URL(endpoint).hostname!=='127.0.0.1'||!process.env.WINONE_COMPONENTS_PROFILE?.startsWith(output))throw Error('Use the isolated desktop test runner');
let browser,page;const errors=[],evidence=[];
const settle=()=>page.evaluate(async()=>{await document.fonts.ready;await new Promise(done=>requestAnimationFrame(()=>requestAnimationFrame(done)));});
try{
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(done=>setTimeout(done,100));}}
 assert(browser,'Native fixture connection missing');
 while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(page=>page.url().includes('winone.localhost'));if(page)break;await new Promise(done=>setTimeout(done,100));}
 assert(page,'Native document missing');page.setDefaultTimeout(20000);page.on('pageerror',error=>errors.push(error.message));await page.locator('.app-navigation').waitFor();
 // Navigation exists before the initial settings snapshot has applied its
 // default page. Wait for that native read, not an arbitrary startup sleep.
 await page.evaluate(async()=>{await window.go.main.App.Snapshot();await window.go.main.App.SettingsNavigation();});await settle();
 await page.waitForFunction(()=>document.querySelector('button[aria-label="刷新状态"]')?.disabled===false);
 await page.evaluate(()=>{
  const app=window.go.main.App,originalCapture=app.Capture,now=Date.now(),events=[];let sequence=0;
  const state={messages:[{id:'u0',turnId:'u0',role:'user',content:'？'},{role:'assistant',turnId:'u0',content:'已收到。'}],operations:[],streaming:false,hasOlder:false};
  const publish=()=>events.push({sequence:++sequence,data:{type:state.streaming?'resumed':'settled',state:structuredClone(state)}});
  window.nativeTaskFixture={state,publish};
  const model={provider:'fixture',model:'model'},conversation={id:'fixture',title:'隔离 Chat 检查',projectId:'project',directory:'C:/fixture',sessionDirectory:'C:/fixture/history',model,permission:'ask',createdAt:'2026-10-21',updatedAt:'2026-10-21'};
  const capture=async(method,raw)=>method==='ai.configuration.get'?JSON.stringify({providers:[{id:'fixture',name:'Local UI fixture',models:['model']}],tasks:{chat:model}}):originalCapture(method,raw);
  const chat=async(method,raw)=>{
   const p=JSON.parse(raw||'{}');
   if(method==='catalog')return JSON.stringify({projects:[{id:'project',name:'Fixture',path:'C:/fixture'}],conversations:[conversation,...['awaiting','error','idle'].map(id=>({...conversation,id,title:id,projectId:undefined}))]});
   if(method==='activity')return JSON.stringify({fixture:{phase:state.streaming?'running':'completed',attention:false},awaiting:{phase:'needs-interaction',attention:false},error:{phase:'error',attention:true},idle:{phase:'idle',attention:false}});
   if(method==='open'||method==='state')return JSON.stringify(state);
   if(method==='poll')return JSON.stringify({events:events.filter(event=>event.sequence>p.cursor),cursor:sequence,running:state.streaming,status:state.streaming?'running':'completed',approvals:[]});
   if(method==='draft.get')return JSON.stringify({text:''});
   if(method==='settings.get')return JSON.stringify({workspaceDirectory:'C:/fixture',skillDirectories:[],pluginDirectories:[],mcpServers:[],titleModel:model});
   if(method==='project.git')return JSON.stringify({repository:false});
   if(method==='prompt'){state.messages.push({id:p.requestId,turnId:p.requestId,requestId:p.requestId,role:'user',content:p.text},{role:'assistant',turnId:p.requestId,content:'已收到。'});publish();return '{}';}
   if(method==='file.stat')return JSON.stringify({files:p.paths.map(path=>({requested:path,file:{path,name:'report.md',directory:false,size:5,modified:now}}))});
   if(method==='review.turn')return JSON.stringify({snapshot:{root:'C:/fixture',revision:p.turnId,files:[{path:'report.md',status:'M'}]},patches:{}});
   if(method==='stop'){state.streaming=false;publish();return '{}';}
   if(['draft.save','release','title.generate'].includes(method))return '{}';
   throw Error('Unexpected isolated Chat method: '+method);
  };
  // Native App is itself a dynamic Proxy, not writable method properties.
  window.go.main.App=new Proxy(app,{get:(target,key)=>key==='Chat'?chat:key==='Capture'?capture:target[key]});
 });
 await page.locator('.app-navigation [data-page-id=chat]').click();await page.getByRole('button',{name:'隔离 Chat 检查',exact:true}).click();await page.locator('[data-chat-turn=u0]').waitFor();await settle();
 const widths=()=>page.locator('.desktop-conversation-message--user .chat-message-bubble').evaluateAll(nodes=>nodes.map(node=>node.getBoundingClientRect().width));
 const original=(await widths())[0];assert(original<75);
 for(let count=2;count<=4;count++){await page.locator('.chat-composer textarea').fill('？');await page.locator('.chat-composer textarea').press('Enter');await page.waitForFunction(count=>document.querySelectorAll('.desktop-conversation-message--user').length===count&&!document.querySelector('.chat-composer-toolbar button[aria-label=停止]'),count);await settle();assert((await widths()).every(width=>Math.abs(width-original)<1));}
 evidence.push({shortBubbleWidths:await widths()});
 // The row/action container must not determine the editor's initial width.
 const message=page.locator('.desktop-conversation-message--user').last();await message.getByRole('button',{name:'编辑消息',exact:true}).click();await settle();assert(Math.abs((await widths()).at(-1)-original)<1);await page.getByRole('textbox',{name:'编辑消息',exact:true}).press('Escape');
 await page.evaluate(()=>{const f=window.nativeTaskFixture;for(let i=1;i<=2;i++)f.state.messages.push({id:'edit-'+i,turnId:'edit-'+i,role:'user',content:'修改 '+i},{role:'assistant',turnId:'edit-'+i,content:[{type:'toolCall',id:'write-'+i,name:'write',arguments:{path:'report.md'}}]},{role:'toolResult',turnId:'edit-'+i,toolCallId:'write-'+i,content:'Saved'},{role:'assistant',turnId:'edit-'+i,content:'Done'});f.publish();});
 await page.locator('.chat-change-turn-select').waitFor();await page.locator('.chat-turn-changes').waitFor();assert.equal(await page.locator('.chat-turn-changes').count(),1);assert.equal(await page.locator('.chat-change-turn-select option').count(),2);await page.locator('.chat-change-turn-select select').selectOption('edit-1');
 await page.evaluate(()=>{const f=window.nativeTaskFixture,now=Date.now()-5000;f.state.messages=[{id:'run',turnId:'run',role:'user',content:'检查文件'},...Array.from({length:7},(_,i)=>({role:'assistant',turnId:'run',timestamp:now+i,content:[{type:'thinking',thinking:'核对文件 '+i}]}))];f.state.streaming=true;f.state.operations=[{id:'turn',turnId:'run',kind:'turn',status:'running',startedAt:now}];f.publish();});
 await page.locator('[data-live-work-preview]').waitFor();await page.evaluate(()=>{window.nativeHeader=document.querySelector('[data-slot=cindy-work]');window.nativePreview=document.querySelector('[data-live-work-preview]');});
 for(let attempt=1;attempt<=3;attempt++){
  await page.evaluate(attempt=>{const f=window.nativeTaskFixture;f.state.messages.push({role:'assistant',turnId:'run',timestamp:Date.now(),content:'',errorMessage:'HTTP 503',stopReason:'error'});f.state.operations=[f.state.operations[0],{id:'retry-'+attempt,turnId:'run',kind:'retry',status:'running',startedAt:Date.now(),delayMs:3000,attempt,maxAttempts:3}];f.publish();},attempt);
  await page.waitForFunction(attempt=>document.querySelector('.chat-retry-status')?.textContent.includes(`· ${attempt} / 3`),attempt);
  assert(await page.evaluate(()=>window.nativeHeader===document.querySelector('[data-slot=cindy-work]')&&window.nativePreview===document.querySelector('[data-live-work-preview]')&&!!document.querySelector('.chat-retry-status').closest('.chat-input-content')));
 }
 const geometry=await page.locator('.chat-sidebar-row').evaluateAll(rows=>rows.map(row=>{const text=row.querySelector('.chat-sidebar-title').getBoundingClientRect(),icon=row.querySelector('.chat-sidebar-leading-status');return {name:row.getAttribute('aria-label'),titleInset:text.left-row.getBoundingClientRect().left,iconInset:icon?icon.getBoundingClientRect().left-row.getBoundingClientRect().left:null,gap:icon?text.left-icon.getBoundingClientRect().right:null,color:icon?getComputedStyle(icon).color:null};}));
 assert(geometry.every(row=>Math.abs(row.titleInset-28)<1));assert(geometry.filter(row=>row.iconInset!==null).every(row=>Math.abs(row.iconInset-4)<1&&Math.abs(row.gap-8)<1));evidence.push({geometry,retryPreviewPreserved:true,singleChangesCard:true});
 for(const theme of ['light','dark']){
  // Visual theme only; never writes the user's or even the fixture's settings.
  await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);await page.mouse.move(0,0);await settle();
  if(process.env.WINONE_WINDOW_SHOT){execFileSync(process.env.WINONE_WINDOW_SHOT,[process.env.WINONE_COMPONENTS_PID,join(output,`native-${theme}.png`)],{windowsHide:true,timeout:15000});if(process.env.WINONE_WINDOW_EDGE_CHECK)execFileSync(process.env.WINONE_WINDOW_EDGE_CHECK,[join(output,`native-${theme}.png`)],{windowsHide:true,timeout:10000});}
  else await page.screenshot({path:join(output,`native-${theme}.png`)});
 }
 assert.deepEqual(errors,[]);await writeFile(join(output,'result.json'),JSON.stringify({nativeWindow:true,chatTransportFixture:true,evidence,errors},null,2));
 execFileSync('powershell.exe',['-NoProfile','-File',resolve('apps/desktop/frontend/tests/native-window-state.ps1'),'-OwnedProcessId',process.env.WINONE_COMPONENTS_PID,'-Executable',join(output,'WinOne.exe'),'-Action','exit'],{windowsHide:true,timeout:10000});
}catch(error){if(page)await page.screenshot({path:join(output,'failure.png')}).catch(()=>{});await writeFile(join(output,'node-error.txt'),String(error.stack||error));await writeFile(join(output,'failure.json'),JSON.stringify({errors,evidence},null,2));process.exitCode=1;}
finally{await browser?.close();}
