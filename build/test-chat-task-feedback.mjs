// Production Chat rendering with a local Host fixture. No real session, account
// or filesystem mutations. Run from the repository via build/use-local-resources.ps1.
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {pathToFileURL} from 'node:url';
import {execFileSync} from 'node:child_process';
const output=resolve(process.argv[2]||'_temp/chat-cindy-fixes/browser');
execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);await mkdir(output,{recursive:true});
const frontend=resolve('apps/desktop/frontend');
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const {createServer}=await import(pathToFileURL(join(frontend,'node_modules/vite/dist/node/index.js')).href);
const vite=await createServer({root:frontend,cacheDir:join(output,'vite-cache'),server:{host:'127.0.0.1',port:0},logLevel:'warn'});await vite.listen();
const browser=await chromium.launch({headless:true,...(process.env.WINONE_BROWSER_EXE?{executablePath:process.env.WINONE_BROWSER_EXE}:{channel:'msedge'})});
let page;const results=[];
const settle=()=>page.evaluate(async()=>{await document.fonts.ready;await new Promise(done=>requestAnimationFrame(()=>requestAnimationFrame(done)));});
const bubbleWidths=()=>page.locator('.desktop-conversation-message--user .chat-message-bubble').evaluateAll(nodes=>nodes.map(node=>node.getBoundingClientRect().width));
try{
 for(const theme of ['light','dark'])for(const width of [1280,420]){
  const language=width===1280?'zh-CN':'en-US';
  page=await browser.newPage({viewport:{width,height:960},reducedMotion:theme==='dark'?'reduce':'no-preference'});
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto(`http://127.0.0.1:${vite.httpServer.address().port}/tests/chat-experience-preview.html?alignment&statusRows&theme=${theme}&language=${language}`);
  await page.locator('.chat-turn-changes').waitFor();await settle();
  // Supply deterministic events through the same polling boundary as Pi.
  await page.evaluate(()=>{
   const original=window.go.main.App.Chat,events=[];let sequence=0;
   const state={messages:[{id:'short-0',turnId:'short-0',role:'user',content:'？'},{id:'answer-0',turnId:'short-0',role:'assistant',content:'收到。'}],operations:[],hasOlder:false,streaming:false};
   const publish=()=>events.push({sequence:++sequence,data:{type:state.streaming?'resumed':'settled',state:{...state,messages:[...state.messages],operations:[...state.operations]}}});
   window.chatFeedbackFixture={state,publish};publish();
   window.go.main.App.Chat=async(method,raw)=>{
    const params=JSON.parse(raw);
    if(method==='prompt'){
     state.messages.push({id:params.requestId,requestId:params.requestId,turnId:params.requestId,role:'user',content:params.text},{id:'answer-'+sequence,turnId:params.requestId,role:'assistant',content:'收到。'});publish();return '{}';
    }
    if(method==='poll')return JSON.stringify({events:events.filter(event=>event.sequence>params.cursor),cursor:sequence,approvals:[],running:state.streaming,status:state.streaming?'running':'completed'});
    if(method==='stop'){state.streaming=false;publish();return '{}';}
    return original(method,raw);
   };
  });
  await page.locator('[data-chat-turn=short-0]').waitFor();await settle();
  const first=await bubbleWidths();assert(first[0]<75,`A single question mark is too wide: ${first}`);
  const input=page.locator('.chat-composer textarea');
  for(let count=2;count<=5;count++){
   await input.fill('？');await input.press('Enter');
   await page.waitForFunction(count=>document.querySelectorAll('.desktop-conversation-message--user').length===count&&document.querySelector('[data-session-id=experience]')?.getAttribute('data-activity')==='time',count);await settle();
   const widths=await bubbleWidths();assert(widths.every(value=>Math.abs(value-first[0])<1),`Sending changed older bubble widths: ${widths}`);
  }
  if(width===420){const hide=page.getByRole('button',{name:'Hide sidebar',exact:true});if(await hide.count())await hide.click();await settle();}
  const before=await bubbleWidths();await page.setViewportSize({width:width+80,height:960});await settle();assert.deepEqual(await bubbleWidths(),before);await page.setViewportSize({width,height:960});
  // Two real tool turns still expose exactly one changes card and both records.
  await page.evaluate(()=>{const f=window.chatFeedbackFixture;for(let i=1;i<=2;i++)f.state.messages.push({id:'edit-'+i,turnId:'edit-'+i,role:'user',content:'Edit '+i},{id:'write-'+i,turnId:'edit-'+i,role:'assistant',timestamp:100+i,content:[{type:'toolCall',id:'tool-'+i,name:'write',arguments:{path:'README.md'}}]},{role:'toolResult',turnId:'edit-'+i,toolCallId:'tool-'+i,content:'Saved'},{role:'assistant',turnId:'edit-'+i,content:'Done'});f.publish();});
  await page.locator('.chat-change-turn-select select').waitFor();await page.locator('.chat-turn-changes').waitFor();
  assert.equal(await page.locator('.chat-turn-changes').count(),1);assert.equal(await page.locator('.chat-change-turn-select option').count(),2);
  await page.locator('.chat-change-turn-select select').selectOption('edit-1');await settle();assert.equal(await page.locator('.chat-turn-changes').count(),1);
  // Repeated failed samples must preserve both the live preview DOM and a
  // manually expanded work group, while the retry notice sits by the composer.
  await page.evaluate(()=>{const f=window.chatFeedbackFixture;f.state.messages=[{id:'retry-user',turnId:'retry-user',role:'user',content:'Inspect files'},...Array.from({length:8},(_,i)=>({role:'assistant',turnId:'retry-user',timestamp:1000+i,content:[{type:'thinking',thinking:'Inspect file '+i}]}))];f.state.streaming=true;f.state.operations=[{id:'turn',turnId:'retry-user',kind:'turn',status:'running',startedAt:1000}];f.publish();});
  await page.locator('[data-live-work-preview]').waitFor();await settle();
  await page.evaluate(()=>{window.workHeader=document.querySelector('[data-slot=cindy-work]');window.workPreview=document.querySelector('[data-live-work-preview]');});
  for(const expanded of [false,true]){
   if(expanded){const header=page.locator('[data-slot=cindy-work]');await header.evaluate(el=>{el.closest('.chat-transcript-viewport').scrollTop=0;});await header.click();}
   for(let attempt=1;attempt<=3;attempt++){
    await page.evaluate(({attempt,expanded})=>{const f=window.chatFeedbackFixture;f.state.messages.push({role:'assistant',turnId:'retry-user',timestamp:2000+attempt+(expanded?10:0),content:'',stopReason:'error',errorMessage:'HTTP 503'});f.state.operations=[f.state.operations[0],{id:'retry-'+attempt,turnId:'retry-user',kind:'retry',status:'running',startedAt:Date.now(),delayMs:3000,attempt,maxAttempts:3}];f.publish();},{attempt,expanded});
    await page.waitForFunction(attempt=>document.querySelector('.chat-retry-status')?.textContent.includes(`· ${attempt} / 3`),attempt);await settle();
    const state=await page.evaluate(()=>({sameHeader:window.workHeader===document.querySelector('[data-slot=cindy-work]'),samePreview:window.workPreview===document.querySelector('[data-live-work-preview]'),preview:!!document.querySelector('[data-live-work-preview]'),count:document.querySelectorAll('[data-slot=cindy-work]').length,outside:!document.querySelector('.chat-retry-status').closest('.chat-work-process'),nearComposer:!!document.querySelector('.chat-retry-status').closest('.chat-input-content')}));
    assert(state.sameHeader&&state.count===1&&state.outside&&state.nearComposer,JSON.stringify(state));assert.equal(state.preview,!expanded);if(!expanded)assert(state.samePreview);
   }
  }
  if(width===1280){
   // Titles keep their original inset. Status icons protrude into that left
   // gutter instead of moving the icon/title pair (or only the title) right.
   await page.locator('[data-session-id=experience][data-activity=running]').waitFor();await settle();
   const geometry=await page.locator('.chat-sidebar-row').evaluateAll(rows=>rows.map(row=>{const text=row.querySelector('.chat-sidebar-title').getBoundingClientRect(),icon=row.querySelector('.chat-sidebar-leading-status');return {name:row.getAttribute('aria-label'),titleInset:text.left-row.getBoundingClientRect().left,iconInset:icon?icon.getBoundingClientRect().left-row.getBoundingClientRect().left:null,gap:icon?text.left-icon.getBoundingClientRect().right:null,color:icon?getComputedStyle(icon).color:null};}));
   assert(geometry.every(row=>row.titleInset===28),JSON.stringify(geometry));assert(geometry.filter(row=>row.iconInset!==null).every(row=>row.iconInset===4&&row.gap===8),JSON.stringify(geometry));assert.equal(new Set(geometry.filter(row=>['experience','awaiting','failed'].includes(row.name)).map(row=>row.color)).size,3);
   await page.screenshot({path:join(output,`sidebar-${theme}.png`)});
   results.push({theme,width,bubbleWidth:first[0],geometry});
  }else results.push({theme,width,bubbleWidth:first[0]});
  const overflow=await page.locator('.chat-main').evaluate(root=>[root,...root.querySelectorAll('*')].filter(el=>el instanceof HTMLElement&&el.clientWidth&&['auto','scroll'].includes(getComputedStyle(el).overflowX)&&el.scrollWidth>el.clientWidth+1).map(el=>el.className));assert.deepEqual(overflow,[]);
  await page.mouse.move(0,0);await page.screenshot({path:join(output,`chat-${theme}-${width}.png`)});assert.deepEqual(errors,[]);await page.close();page=null;console.log('PASS',theme,width);
 }
 await writeFile(join(output,'result.json'),JSON.stringify({passed:true,results},null,2));
}catch(error){if(page)await page.screenshot({path:join(output,'failure.png')}).catch(()=>{});await writeFile(join(output,'failure.json'),JSON.stringify({error:String(error),stack:error.stack,results},null,2));throw error;}
finally{await browser.close();await vite.close();}
