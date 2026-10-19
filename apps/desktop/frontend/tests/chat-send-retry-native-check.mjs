// Production Rust document -> Host -> Magpie -> Pi Durable. Only the upstream
// model is controlled; every input, retry, file and saved receipt uses the real
// private-profile chain. Gates keep assertions inside streaming, not after it.
import {createRequire} from 'node:module';
import {mkdir,writeFile,readFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {execFileSync} from 'node:child_process';
import http from 'node:http';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,folder]=process.argv.slice(2),output=resolve(folder),workspace=join(output,'workspace');
if(new URL(endpoint).hostname!=='127.0.0.1'||!process.env.WINONE_COMPONENTS_PROFILE?.startsWith(output))throw Error('Use the owned private component runner');
await mkdir(workspace,{recursive:true});await writeFile(join(workspace,'proof.txt'),'NATIVE_READ_PROOF');await writeFile(join(workspace,'attachment.txt'),'NATIVE_ATTACHMENT_PROOF');await writeFile(join(workspace,'AGENTS.md'),'# Chat UI fixture\n\nUse only the test workspace.\n');
await mkdir(join(workspace,'.pi','skills','native-skill'),{recursive:true});await writeFile(join(workspace,'.pi','skills','native-skill','SKILL.md'),'---\nname: native-skill\ndescription: NATIVE_SKILL_DISCOVERY\n---\n# Native skill instructions\n\nKeep all operations in the isolated workspace.\n');
const requests=[],serverErrors=[],errors=[],releases=new Map(),evidence=[];
let stoppedAtVendor=false;
const gate=name=>new Promise(done=>releases.set(name,done));
const release=name=>{assert.ok(releases.has(name),'Missing response gate '+name);releases.get(name)();releases.delete(name);};
const vendor=http.createServer(async(req,res)=>{
 try{
  let raw='';for await(const part of req)raw+=part;
  if(req.method==='GET'&&req.url==='/v1/models'){res.setHeader('Content-Type','application/json');res.end(JSON.stringify({data:[{id:'fixture-model'}]}));return;}
  assert.equal(req.url,'/v1/chat/completions');assert.equal(req.headers.authorization,'Bearer fixture-ui-key');assert.equal(req.headers['x-winone-fixture'],'chat-send-audit');
  const body=JSON.parse(raw),number=requests.push(body);res.writeHead(200,{'Content-Type':'text/event-stream','Cache-Control':'no-cache'});
  const emit=(delta,finish_reason=null)=>res.write('data: '+JSON.stringify({id:'audit-'+number,object:'chat.completion.chunk',model:'fixture-model',choices:[{index:0,delta,finish_reason}]})+'\n\n');
  emit({role:'assistant'});
  if(number===1){await gate('before-content');emit({content:'NATIVE_INITIAL_PARTIAL'});await gate('original');emit({tool_calls:[{index:0,id:'read-proof',type:'function',function:{name:'read',arguments:JSON.stringify({path:'proof.txt'})}}]});emit({},'tool_calls');}
  else if(number===2){emit({content:'NATIVE_INSERTED_PARTIAL'});await gate('insert');emit({content:'\n\nNATIVE_INSERTED_DONE'});emit({},'stop');}
  else if(number===3){emit({content:'NATIVE_QUEUED_DONE'});emit({},'stop');}
  else if(number===4){emit({content:'NATIVE_RETRY_PARTIAL'});await gate('retry');emit({content:'\n\nNATIVE_RETRY_DONE'});emit({},'stop');}
  else if(number===5){emit({content:'NATIVE_EDITED_PARTIAL'});await gate('edit');emit({content:'\n\nNATIVE_EDITED_DONE'});emit({},'stop');}
  else if(number===6){emit({content:'NATIVE_ATTACHMENT_DONE'});emit({},'stop');}
  else if(number===7){
   res.once('close',()=>{stoppedAtVendor=!res.writableEnded;});
   emit({content:'NATIVE_STOP_PARTIAL'});await gate('stop');
   if(res.destroyed)return;
   emit({content:'NATIVE_STOP_SHOULD_NOT_FINISH'});emit({},'stop');
  }
  else if(number===8){emit({reasoning_content:'Recover the interrupted stream.'});await new Promise(done=>setTimeout(done,150));res.destroy();return;}
  else if(number===9){emit({content:'NATIVE_RECOVERED_WITHOUT_CONTINUE'});emit({},'stop');}
  else if(number===10){emit({reasoning_content:Array.from({length:150},(_,index)=>`实际思考 ${index+1}：保留过程中的完整信息。`).join('\n')});await gate('thinking');emit({content:'NATIVE_THINKING_DONE'});emit({},'stop');}
  else throw Error('An input was submitted more than once: request '+number);
  res.end('data: [DONE]\n\n');
 }catch(error){serverErrors.push(String(error));res.statusCode=500;res.end(JSON.stringify({error:{message:String(error)}}));}
});await new Promise(done=>vendor.listen(0,'127.0.0.1',done));
let browser,page,conversation;
const until=async(check,description)=>{const deadline=Date.now()+20000;while(!await check()){if(Date.now()>deadline)throw Error('Timed out: '+description);await new Promise(done=>setTimeout(done,20));}};
const call=(method,params={})=>page.evaluate(async({method,params})=>JSON.parse(await window.go.main.App.Chat(method,JSON.stringify(params))),{method,params});
const reply=marker=>page.locator('.desktop-conversation-message--assistant .chat-markdown p').filter({hasText:marker});
const settle=()=>page.evaluate(async()=>{await document.fonts.ready;await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);});
async function capture(name){await page.evaluate(()=>document.activeElement?.blur());await page.mouse.move(0,0);await settle();await page.screenshot({path:join(output,name+'.png')});
 if(process.env.WINONE_WINDOW_SHOT){execFileSync(process.env.WINONE_WINDOW_SHOT,[process.env.WINONE_COMPONENTS_PID,join(output,name+'-window.png')],{windowsHide:true,encoding:'utf8',timeout:15000});}
}
async function liveOrder(marker,owner,retired){
 const result=await page.evaluate(({marker,owner,retired})=>{
  const users=[...document.querySelectorAll('.desktop-conversation-message--user')].filter(node=>node.textContent.includes(owner));
  const answer=[...document.querySelectorAll('.desktop-conversation-message--assistant')].find(node=>node.textContent.includes(marker));
  const viewport=document.querySelector('.chat-transcript-viewport').getBoundingClientRect(),footer=document.querySelector('.chat-input-area').getBoundingClientRect();
  const body=users[0]?.querySelector('.chat-user-text'),bounds=body?.getBoundingClientRect();
  const replyTexts=[...document.querySelectorAll('.desktop-conversation-message--assistant .chat-markdown p')].map(node=>node.textContent);
  const duplicateReplies=[...new Set(replyTexts)].filter(text=>text.startsWith('NATIVE_')&&replyTexts.filter(value=>value===text).length>1);
  return {users:users.length,ownerBefore:!!answer&&users.length===1&&!!(users[0].compareDocumentPosition(answer)&Node.DOCUMENT_POSITION_FOLLOWING),retired:!!retired&&[...document.querySelectorAll('.desktop-conversation-message--assistant')].some(node=>node.textContent.includes(retired)),duplicateReplies,ownerVisible:!!body?.checkVisibility({checkOpacity:true,checkVisibilityCSS:true})&&bounds.top>=viewport.top-1&&bounds.bottom<=Math.min(viewport.bottom,footer.top)+1,userLayout:users.map(node=>{const rect=node.getBoundingClientRect();return {x:rect.x,y:rect.y,width:rect.width,height:rect.height,ancestors:[node,node.firstElementChild,node.querySelector('.chat-user-text'),node.querySelector('.chat-markdown')].filter(Boolean).map(el=>{const style=getComputedStyle(el);return {class:el.className,color:style.color,visibility:style.visibility,opacity:style.opacity,display:style.display,contentVisibility:style.contentVisibility};})};})};
 },{marker,owner,retired});evidence.push({scenario:'live order and visible owner',marker,...result});assert.equal(result.users,1);assert.equal(result.ownerBefore,true);assert.equal(result.retired,false);assert.deepEqual(result.duplicateReplies,[],'Historical replies reappeared as live output');assert.equal(result.ownerVisible,true,'Short user input was pushed out of the reading area');
}
try{
 const deadline=Date.now()+30000;while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(done=>setTimeout(done,100));}}
 if(!browser)throw Error('Owned WebView did not start');while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(page=>page.url().includes('winone.localhost'));if(page)break;await new Promise(done=>setTimeout(done,100));}
 if(!page)throw Error('Owned production document not found');page.setDefaultTimeout(20000);page.on('pageerror',error=>errors.push(String(error)));
 await page.addInitScript(()=>{window.chatAuditCSP=[];document.addEventListener('securitypolicyviolation',event=>window.chatAuditCSP.push(event.violatedDirective+':'+event.blockedURI));});
 await page.locator('[data-page-id="chat"]').waitFor();
 evidence.push({scenario:'production navigation ready',navigation:await page.locator('[data-page-id="chat"]').evaluate(node=>({className:node.className,ancestors:[node.parentElement,node.parentElement?.parentElement,node.closest('[data-slot=sidebar]')].filter(Boolean).map(item=>({className:item.className,slot:item.getAttribute('data-slot'),width:item.getBoundingClientRect().width,height:item.getBoundingClientRect().height}))}))});
 conversation=await page.evaluate(async({workspace,baseUrl})=>{
  const api=window.go.main.App,snapshot=await api.Snapshot();await api.SaveSettings({...snapshot.settings,theme:'light',language:'zh-CN',startup:{...snapshot.settings.startup,keepUIRunning:false}});
  const configuration=JSON.parse(await api.Capture('ai.configuration.get','null'));
  configuration.providers=[{id:'fixture',name:'Chat 验收服务',authMode:'api-key',protocol:'chat',baseUrl,models:['fixture-model'],modelNames:{'fixture-model':'Fixture Model'},headers:{'X-WinOne-Fixture':'chat-send-audit'},hasKey:false}];
  configuration.tasks={default:{provider:'fixture',model:'fixture-model'},chat:{provider:'fixture',model:'fixture-model'}};
  await api.Capture('ai.configuration.save',JSON.stringify({configuration,keys:{fixture:'fixture-ui-key'}}));
  const preferences=JSON.parse(await api.Chat('settings.get','null'));preferences.autoRetry=true;preferences.backgroundCompaction=false;await api.Chat('settings.save',JSON.stringify({settings:preferences}));
  return JSON.parse(await api.Chat('conversation.new',JSON.stringify({directory:workspace,title:'Chat 发送与重试验收',model:{provider:'fixture',model:'fixture-model'}})));
 },{workspace,baseUrl:`http://127.0.0.1:${vendor.address().port}/v1`});
 await page.reload({waitUntil:'domcontentloaded'});await page.locator('[data-page-id="chat"]').click();await page.getByRole('button',{name:'Chat 发送与重试验收',exact:true}).click();
 const input=page.getByRole('textbox',{name:'消息',exact:true});await input.waitFor();
 await page.evaluate(()=>{window.chatUserHeights=[];const measure=()=>{const row=[...document.querySelectorAll('.desktop-conversation-message--user')].find(node=>node.textContent.includes('NATIVE_ORIGINAL'));if(row)window.chatUserHeights.push(row.getBoundingClientRect().height);};const observer=new MutationObserver(measure);observer.observe(document.querySelector('.chat-transcript-viewport'),{subtree:true,childList:true,characterData:true});window.chatUserObserver=observer;});
 await input.fill('NATIVE_ORIGINAL');await input.press('Enter');await until(()=>releases.has('before-content'),'model request waiting before content');await settle();
 assert.equal(await page.locator('[data-slot=cindy-thinking]').count(),0,'The old empty thinking surface is still mounted');assert.equal(await page.locator('.chat-work-process').count(),0,'There is no actual work content yet');await capture('native-before-thinking');release('before-content');
 await reply('NATIVE_INITIAL_PARTIAL').waitFor();await liveOrder('NATIVE_INITIAL_PARTIAL','NATIVE_ORIGINAL');
 const userHeights=await page.evaluate(()=>{window.chatUserObserver.disconnect();return window.chatUserHeights;});assert.ok(userHeights.length>1);assert.ok(Math.max(...userHeights)-Math.min(...userHeights)<1,'Receipt metadata changed the user bubble height');assert.equal(await page.locator('.desktop-conversation-message--user [role=status]').count(),0);evidence.push({scenario:'no legacy empty work / user-only bubble',samples:userHeights.length,heights:[...new Set(userHeights)]});
 await input.fill('NATIVE_QUEUED');await input.press('Enter');await page.getByRole('button',{name:'排队发送',exact:true}).click();await until(async()=>await page.getByRole('button',{name:'新对话',exact:true}).isEnabled(),'queued dispatch settled');assert.equal((await call('draft.get',{id:conversation.id})).text,'','accepted queue must not remain an unsent draft');
 await input.fill('NATIVE_INSERTED');await input.press('Enter');assert.equal(await page.getByRole('button',{name:'继续编辑',exact:true}).isVisible(),true);await page.getByRole('button',{name:'直接插入会话',exact:true}).click();await until(async()=>await page.getByRole('button',{name:'新对话',exact:true}).isEnabled(),'steer dispatch settled');assert.equal(await input.inputValue(),'');
 assert.equal(requests.length,1,'same-turn insertion must not abort the in-flight request');
 const queuedState=await call('state',{id:conversation.id});assert.deepEqual(queuedState.messages.filter(message=>message.pending).map(message=>message.deliveryMode),['queue','steer']);
 release('original');await reply('NATIVE_INSERTED_PARTIAL').waitFor();assert.equal(requests.length,2);
 const secondUsers=requests[1].messages.filter(message=>message.role==='user');assert.ok(JSON.stringify(secondUsers).includes('NATIVE_INSERTED'));assert.ok(!JSON.stringify(secondUsers).includes('NATIVE_QUEUED'));
 await liveOrder('NATIVE_INSERTED_PARTIAL','NATIVE_INSERTED');await capture('native-insert-streaming');release('insert');
 await reply('NATIVE_QUEUED_DONE').waitFor();await page.getByRole('button',{name:'停止',exact:true}).waitFor({state:'hidden'});
 const settled=await call('state',{id:conversation.id});assert.deepEqual(settled.messages.filter(message=>message.role==='user').map(message=>message.content),['NATIVE_ORIGINAL','NATIVE_INSERTED','NATIVE_QUEUED']);assert.equal(requests.length,3);
 // Observe the whole retry interval. A final-only screenshot cannot catch the
 // reported bug where input stays below a partial until the run has settled.
 await page.evaluate(()=>{
  window.chatAuditOrder=[];window.chatAuditExpected={partial:'NATIVE_RETRY_PARTIAL',owner:'NATIVE_QUEUED'};
  const inspect=()=>{const expected=window.chatAuditExpected;if(!expected)return;const {partial,owner}=expected;const rows=[...document.querySelectorAll('.desktop-conversation-message')];const answer=rows.findIndex(row=>row.classList.contains('desktop-conversation-message--assistant')&&row.textContent.includes(partial));if(answer<0)return;const users=rows.map((row,index)=>({row,index})).filter(({row})=>row.classList.contains('desktop-conversation-message--user')&&row.textContent.includes(owner));window.chatAuditOrder.push({partial,valid:users.length===1&&users[0].index<answer,rows:rows.map(row=>({role:row.classList.contains('desktop-conversation-message--user')?'user':'assistant',text:row.textContent,editing:!!row.querySelector('textarea')}))});};
  const observer=new MutationObserver(inspect);observer.observe(document.querySelector('.chat-transcript-viewport'),{childList:true,subtree:true,characterData:true});window.chatAuditObserver=observer;
 });
 const oldReply=page.locator('.desktop-conversation-message--assistant').filter({hasText:'NATIVE_QUEUED_DONE'}).last();await oldReply.getByRole('button',{name:'重试',exact:true}).click({clickCount:2});
 await reply('NATIVE_RETRY_PARTIAL').waitFor();assert.equal(requests.length,4);await liveOrder('NATIVE_RETRY_PARTIAL','NATIVE_QUEUED','NATIVE_QUEUED_DONE');await capture('native-retry-streaming');release('retry');
 await reply('NATIVE_RETRY_DONE').waitFor();await page.getByRole('button',{name:'停止',exact:true}).waitFor({state:'hidden'});
 await page.evaluate(()=>{window.chatAuditExpected=null;});
 const userRow=page.locator('.desktop-conversation-message--user').filter({hasText:'NATIVE_QUEUED'});await userRow.getByRole('button',{name:'编辑消息',exact:true}).click();
 const edit=userRow.getByRole('textbox',{name:'编辑消息',exact:true});await edit.fill('NATIVE_EDITED');await page.evaluate(()=>{window.chatAuditExpected={partial:'NATIVE_EDITED_PARTIAL',owner:'NATIVE_EDITED'};});await page.getByRole('button',{name:'保存并重新发送',exact:true}).click();
 await reply('NATIVE_EDITED_PARTIAL').waitFor();await liveOrder('NATIVE_EDITED_PARTIAL','NATIVE_EDITED','NATIVE_RETRY_DONE');assert.equal(requests.length,5);await capture('native-edit-streaming');release('edit');
 await reply('NATIVE_EDITED_DONE').waitFor();await page.getByRole('button',{name:'停止',exact:true}).waitFor({state:'hidden'});await page.evaluate(()=>{window.chatAuditExpected=null;});
 await page.evaluate(path=>window.dispatchEvent(new CustomEvent('winone:chat-files-drop',{detail:{paths:[path]}})),join(workspace,'attachment.txt'));
 await page.locator('.chat-attachments').getByText('attachment.txt',{exact:true}).waitFor();assert.equal(await input.inputValue(),'');await input.press('Control+Enter');
 await reply('NATIVE_ATTACHMENT_DONE').waitFor();await page.getByRole('button',{name:'停止',exact:true}).waitFor({state:'hidden'});assert.equal(requests.length,6);assert.ok(JSON.stringify(requests[5].messages).includes('NATIVE_ATTACHMENT_PROOF'));
 const order=await page.evaluate(()=>{window.chatAuditObserver.disconnect();return window.chatAuditOrder;});await writeFile(join(output,'streaming-order.json'),JSON.stringify(order,null,2));assert.ok(order.some(item=>item.partial==='NATIVE_RETRY_PARTIAL'));assert.ok(order.some(item=>item.partial==='NATIVE_EDITED_PARTIAL'));assert.ok(order.every(item=>item.valid));
 evidence.push({scenario:'retry/edit streaming observer',samples:order.length,allValid:true,modelRequests:requests.length,fileOnly:true});
 // A typed next draft leaves Send in its main slot, but Stop must still abort
 // the real upstream stream without clearing that draft or sending it.
 await input.fill('NATIVE_STOP');await input.press('Enter');await reply('NATIVE_STOP_PARTIAL').waitFor();await liveOrder('NATIVE_STOP_PARTIAL','NATIVE_STOP');
 await input.fill('NATIVE_AFTER_STOP_DRAFT');assert.equal(await page.getByRole('button',{name:'发送',exact:true}).isVisible(),true);
 await page.getByRole('button',{name:'停止',exact:true}).click();await page.getByRole('button',{name:'停止',exact:true}).waitFor({state:'hidden'});
 await until(()=>stoppedAtVendor,'upstream stream aborted');release('stop');
 assert.equal(await input.inputValue(),'NATIVE_AFTER_STOP_DRAFT');assert.equal(requests.length,7);
 assert.equal((await call('state',{id:conversation.id})).streaming,false);
 await until(async()=>(await call('draft.get',{id:conversation.id})).text==='NATIVE_AFTER_STOP_DRAFT','stopped draft retained');
 evidence.push({scenario:'stop with next draft',stoppedAtVendor,modelRequests:requests.length,draftPreserved:true});await capture('native-stop-keeps-draft');
 await input.fill('NATIVE_RECOVERY');await input.press('Enter');await reply('NATIVE_RECOVERED_WITHOUT_CONTINUE').waitFor();await page.getByRole('button',{name:'停止',exact:true}).waitFor({state:'hidden'});assert.equal(requests.length,9);assert.equal(await page.getByText(/socket connection was closed unexpectedly|Stream ended without finish_reason/).count(),0);evidence.push({scenario:'real Host / Magpie / Pi stream drop recovers without a user continuation',requests:requests.length});await capture('native-recovered');
 await input.fill('NATIVE_LONG_WORK');await input.press('Enter');await until(()=>releases.has('thinking'),'real thinking stream');
 const liveThought=page.locator('[data-live-work-activity=thinking]').last();await liveThought.waitFor();await liveThought.click();await page.locator('.process-body[data-process-long=true]').waitFor();await page.locator('.chat-transcript-viewport').evaluate(view=>{const body=view.querySelector('.process-body[data-process-long=true]');view.scrollTop+=body.getBoundingClientRect().top-view.getBoundingClientRect().top+240;});
 const collapse=page.locator('.process-collapse-action');await collapse.waitFor();await capture('native-live-thinking-collapse');await collapse.click();await collapse.waitFor({state:'hidden'});assert.equal(await liveThought.getAttribute('aria-expanded'),'false');release('thinking');await reply('NATIVE_THINKING_DONE').waitFor();await page.getByRole('button',{name:'停止',exact:true}).waitFor({state:'hidden'});evidence.push({scenario:'actual long thinking uses current work chrome and a local floating collapse'});
 await page.getByRole('button',{name:'显示会话详情',exact:true}).click();const panel=page.locator('.chat-side-panel');await panel.getByRole('tab',{name:'文件与资源',exact:true}).click();await panel.getByRole('tab',{name:'资源',exact:true}).click();await panel.getByRole('button',{name:/^native-skill/}).waitFor();await panel.getByRole('button',{name:/^native-skill/}).click();await panel.locator('.cm-editor:visible').waitFor();await panel.getByRole('tab',{name:'文件与资源',exact:true}).click();await panel.getByRole('button',{name:/^shared-native-skill/}).click();await panel.locator('.cm-editor:visible').waitFor();await panel.getByRole('tab',{name:'文件与资源',exact:true}).click();evidence.push({scenario:'native default .pi and shared .agents Skill discovery and file preview'});await panel.getByRole('button',{name:/AGENTS.md/}).last().waitFor();
 assert.equal(await panel.getByRole('button',{name:/添加标签|Add tab/}).isVisible(),false);
 await panel.getByRole('button',{name:/AGENTS.md/}).last().click();await panel.locator('.cm-editor:visible').waitFor();assert.equal(await panel.getByRole('tab',{name:'标签页',exact:true}).getAttribute('aria-selected'),'true');
 await panel.getByRole('tab',{name:'文件与资源',exact:true}).click();assert.equal(await panel.getByRole('tab',{name:'资源',exact:true}).getAttribute('aria-selected'),'true');
 for(const theme of ['light','dark'])for(const [width,height] of [[1280,940],[800,600]]){
  // Save through the real theme control. Calling SaveSettings alone changes
  // disk, not React's current snapshot, and used to label light screenshots dark.
  if(await page.locator('html').getAttribute('data-theme')!==theme){
   await page.locator('[data-page-id="settings"]').click();
   const radio=page.getByRole('radio',{name:theme==='dark'?'深色':'浅色',exact:true});await radio.click();await page.locator(`html[data-theme="${theme}"]`).waitFor();
   await page.locator('[data-page-id="chat"]').click();await page.getByRole('button',{name:'Chat 发送与重试验收',exact:true}).click();await input.waitFor();
   const show=page.getByRole('button',{name:'显示会话详情',exact:true});if(await show.isVisible())await show.click();
   await panel.getByRole('tab',{name:'文件与资源',exact:true}).click();await panel.getByRole('tab',{name:'资源',exact:true}).click();
  }
  assert.equal(await page.locator('.chat-desktop-surface').getAttribute('data-theme'),theme);
  execFileSync('powershell.exe',['-NoProfile','-File',resolve('apps/desktop/frontend/tests/native-window-state.ps1'),'-OwnedProcessId',process.env.WINONE_COMPONENTS_PID,'-Executable',join(output,'WinOne.exe'),'-Action','resize','-Width',String(width),'-Height',String(height)],{windowsHide:true,encoding:'utf8',timeout:15000});
  // Verify the real viewport before input or a screenshot can incidentally
  // wake a deferred native resize. A successful HWND resize alone is not proof.
  await page.waitForFunction(({width,height})=>innerWidth===width&&innerHeight===height,{width,height},{timeout:2000});await settle();
  await input.fill('NATIVE_UNSENT_DRAFT');const grip=page.getByRole('separator',{name:'调整输入框高度'});await grip.focus();for(let i=0;i<8;i++)await grip.press('ArrowUp');await settle();
  const geometry=await page.evaluate(()=>{
   const box=selector=>{const r=document.querySelector(selector).getBoundingClientRect();return {x:r.x,y:r.y,right:r.right,bottom:r.bottom,width:r.width,height:r.height};};const input=document.querySelector('.chat-composer'),stack=document.querySelector('.chat-composer-stack');
   const toolbar=document.querySelector('.chat-composer-toolbar'),b=toolbar.getBoundingClientRect(),clipped=[...toolbar.querySelectorAll('button')].filter(button=>{const r=button.getBoundingClientRect();return r.width&&r.height&&(r.left<b.left-1||r.right>b.right+1||r.top<b.top-1||r.bottom>b.bottom+1);}).map(button=>button.getAttribute('aria-label')||button.textContent);
   const nav=document.querySelector('.chat-scroll-controls'),r=nav.getBoundingClientRect(),v=document.querySelector('.chat-transcript-viewport').getBoundingClientRect();const overlap=box=>box.width>0&&box.height>0&&box.left<r.right-1&&box.right>r.left+1&&box.top<r.bottom-1&&box.bottom>r.top+1;
   const collisions=[...document.querySelectorAll('.client-thread :is(p,pre,button,a),.chat-input-area,.chat-header-title,.chat-header>button')].filter(node=>{const b=node.getBoundingClientRect();return !nav.contains(node)&&overlap(node.closest('.client-thread')?{...b.toJSON(),top:Math.max(b.top,v.top),bottom:Math.min(b.bottom,v.bottom),height:Math.max(0,Math.min(b.bottom,v.bottom)-Math.max(b.top,v.top))}:b);}).map(node=>node.className);
   const tabWidths=[...document.querySelectorAll('.chat-panel-major-tabs [role=tab]')].map(node=>node.getBoundingClientRect().width);
   return {viewport:{width:innerWidth,height:innerHeight},rail:box('.chat-scroll-controls'),reading:box('.chat-reading-pane'),body:box('.chat-conversation-body'),footer:box('.chat-input-area'),major:box('.chat-panel-major-tabs'),tabWidths,collisions,clipped,radius:parseFloat(getComputedStyle(input).borderBottomRightRadius),outerRadius:parseFloat(getComputedStyle(stack,'::before').borderBottomRightRadius),horizontal:document.documentElement.scrollWidth-innerWidth};
  });assert.ok(Math.abs(geometry.reading.width-geometry.body.width)<=1);assert.deepEqual(geometry.collisions,[]);assert.deepEqual(geometry.clipped,[]);assert.equal(geometry.outerRadius,geometry.radius+3);assert.ok(geometry.major.height>=34&&geometry.major.height<=36);assert.ok(Math.abs(geometry.tabWidths[0]-geometry.tabWidths[1])<=1);assert.ok(geometry.horizontal<=1);evidence.push({scenario:'native full-width library / resize / collision-free navigation',theme,width,height,...geometry});await capture(`native-library-${theme}-${width}-${height}`);await until(async()=>(await call('draft.get',{id:conversation.id})).text==='NATIVE_UNSENT_DRAFT','saved layout draft');
 }
 await until(async()=>(await call('draft.get',{id:conversation.id})).text==='NATIVE_UNSENT_DRAFT','saved draft');
 await page.locator('[data-page-id="overview"]').click();await page.locator('[data-page-id="chat"]').click();await page.getByRole('button',{name:'Chat 发送与重试验收',exact:true}).click();await reply('NATIVE_ATTACHMENT_DONE').waitFor();assert.equal(await page.getByRole('textbox',{name:'消息',exact:true}).inputValue(),'NATIVE_UNSENT_DRAFT');
 const snapshot=await call('state',{id:conversation.id});assert.deepEqual(snapshot.messages.filter(message=>message.role==='user').map(message=>message.content),['NATIVE_ORIGINAL','NATIVE_INSERTED','NATIVE_EDITED','','NATIVE_STOP','NATIVE_RECOVERY','NATIVE_LONG_WORK']);assert.equal(snapshot.messages.find(message=>message.role==='user'&&message.content==='').attachments?.length,1);
 assert.ok(!JSON.stringify(snapshot.messages).includes('NATIVE_STOP_SHOULD_NOT_FINISH'));assert.equal(requests.length,10);
 const saved=await call('catalog');const entry=saved.conversations.find(item=>item.id===conversation.id);assert.ok((await readFile(entry.sessionFile,'utf8')).includes('winone.input'));
 await input.fill('');await until(async()=>(await call('draft.get',{id:conversation.id})).text==='','cleared test draft');
 const csp=await page.evaluate(()=>window.chatAuditCSP??[]);assert.deepEqual(csp,[]);assert.deepEqual(errors,[]);assert.deepEqual(serverErrors,[]);
 await writeFile(join(output,'result.json'),JSON.stringify({productionAssets:true,rustHostMagpiePi:true,conversationId:conversation.id,modelRequests:requests.length,evidence,reopenedDraft:true,savedReceipts:true,csp,errors,serverErrors},null,2));
 await writeFile(join(output,'model-requests.json'),JSON.stringify(requests,null,2));await page.evaluate(()=>{void window.go.main.App.WindowCommand('close');});
}catch(error){await writeFile(join(output,'partial-evidence.json'),JSON.stringify({evidence,modelRequests:requests.length,errors,serverErrors},null,2));await writeFile(join(output,'node-error.txt'),String(error.stack||error)+'\n'+errors.join('\n')+'\n'+serverErrors.join('\n'));if(page){await writeFile(join(output,'failure-dom.html'),await page.content().catch(()=>''));await page.screenshot({path:join(output,'failure.png')}).catch(()=>{});await writeFile(join(output,'failure.txt'),await page.locator('body').innerText().catch(()=>''));}process.exitCode=1;}
finally{for(const done of releases.values())done();await browser?.close().catch(()=>{});vendor.closeAllConnections();await new Promise(done=>vendor.close(done));}
