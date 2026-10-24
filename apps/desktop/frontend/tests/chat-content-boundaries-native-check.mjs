// Real isolated WinOne/Rust/Host/Pi. Only the model response is a local fixture.
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {createServer} from 'node:http';
import {once} from 'node:events';
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,folder]=process.argv.slice(2),output=resolve(folder),workspace=join(output,'workspace');
if(new URL(endpoint).hostname!=='127.0.0.1'||!process.env.WINONE_COMPONENTS_PROFILE?.startsWith(output))throw Error('Use the isolated desktop test runner');
await mkdir(workspace,{recursive:true});
let requests=0,stream;const errors=[],serverErrors=[],evidence=[];
const filePath=join(workspace,'设计手册.md');await writeFile(filePath,'# 本地文件预览\n\n隔离验收内容。');
const response=Array.from({length:24},(_,i)=>`### 检查段落 ${i+1}\n\n这是隔离窗口中的正文，用于验证输入框高度变化后的底部位置。`).join('\n\n')+'\n\n最后一段内容应完整显示在输入区域上方。\n\n文件： `'+filePath+'`';
const server=createServer(async(req,res)=>{
 try{
  for await(const _ of req){} // Drain the request without retaining prompt data.
  if(req.url==='/v1/models'){res.setHeader('Content-Type','application/json');res.end(JSON.stringify({data:[{id:'fixture-model',object:'model'}]}));return;}
  assert.equal(req.url,'/v1/chat/completions');assert.equal(req.method,'POST');requests++;
  res.setHeader('Content-Type','text/event-stream');
  const chunk=(delta,finish=null)=>res.write('data: '+JSON.stringify({id:'boundary-fixture',object:'chat.completion.chunk',created:1,model:'fixture-model',choices:[{index:0,delta,finish_reason:finish}]})+'\n\n');
  // Keep the real Host/Pi request streaming while the UI is resized. The
  // driver, not a timer, controls the next token and completion boundary.
  stream={append:content=>chunk({content}),finish:()=>{chunk({},'stop');res.end('data: [DONE]\n\n');}};
  chunk({role:'assistant',content:response});
 }catch(error){serverErrors.push(String(error));res.statusCode=500;res.end(String(error));}
});server.listen(0,'127.0.0.1');await once(server,'listening');
let browser,page;
const settle=()=>page.evaluate(async()=>{await document.fonts.ready;await new Promise(done=>requestAnimationFrame(()=>requestAnimationFrame(done)));});
const geometry=()=>page.evaluate(()=>{const v=document.querySelector('.chat-transcript-viewport'),f=document.querySelector('.chat-input-area'),n=document.querySelector('.chat-scroll-controls');return {top:v.scrollTop,height:v.clientHeight,extent:v.scrollHeight,footer:f.getBoundingClientRect().top,navClear:(()=>{const box=n.getBoundingClientRect(),surface=f.querySelector('.chat-input-content')?.getBoundingClientRect();return !surface||box.right<=surface.left+1||surface.right<=box.left+1||box.bottom<=surface.top+1||surface.bottom<=box.top+1;})(),headerNav:n.classList.contains('chat-scroll-controls--header'),end:v.querySelector('.chat-transcript-end-space').getBoundingClientRect().top,navBottom:n.getBoundingClientRect().bottom};});
async function bottom(){const before=await geometry();if(Math.abs(before.end-before.footer+32)>0.5)await page.getByRole('button',{name:'返回底部',exact:true}).click();await page.waitForFunction(()=>Math.abs(document.querySelector('.chat-transcript-end-space').getBoundingClientRect().top-document.querySelector('.chat-input-area').getBoundingClientRect().top+32)<0.5);const after=await geometry();assert(after.headerNav||after.navClear,'Navigation covers the input content');return after;}
try{
 const deadline=Date.now()+30000;while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(done=>setTimeout(done,100));}}
 assert(browser,'Native test connection missing');
 while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(page=>page.url().includes('winone.localhost'));if(page)break;await new Promise(done=>setTimeout(done,100));}
 assert(page,'Native document missing');page.setDefaultTimeout(20000);page.on('pageerror',error=>errors.push(error.message));
 await page.locator('.app-navigation').waitFor();
 await page.evaluate(async({workspace,baseUrl})=>{
  const app=window.go.main.App;
  // Startup module actions can legitimately hold the configuration lease;
  // this layout check does not need a global settings transaction on launch.
  const configuration=JSON.parse(await app.Capture('ai.configuration.get','null'));
  configuration.providers=[{id:'fixture',name:'Local boundary fixture',authMode:'api-key',protocol:'chat',baseUrl,models:['fixture-model'],hasKey:false}];
  configuration.tasks={default:{provider:'fixture',model:'fixture-model'},chat:{provider:'fixture',model:'fixture-model'}};
  await app.Capture('ai.configuration.save',JSON.stringify({configuration,keys:{fixture:'fixture-only-key'}}));
  await app.Chat('conversation.new',JSON.stringify({directory:workspace,title:'Content boundary fixture',model:{provider:'fixture',model:'fixture-model'}}));
 },{workspace,baseUrl:`http://127.0.0.1:${server.address().port}/v1`});
 await page.reload({waitUntil:'domcontentloaded'});await page.locator('.app-navigation [data-page-id=chat]').click();await page.getByRole('button',{name:'Content boundary fixture',exact:true}).click();
 // Wait for the configured provider to reach the composer; pressing Enter
 // before that point leaves the draft unsent and produces a false layout pass.
 await page.locator('.chat-composer textarea').waitFor();await page.waitForFunction(()=>{const trigger=document.querySelector('.chat-model-trigger');return trigger&&!trigger.hasAttribute('disabled')&&!trigger.querySelector('.chat-model-name')?.textContent?.includes('选择模型')&&trigger.querySelector('.chat-model-name')?.textContent?.trim();});await page.locator('.chat-model-trigger').filter({hasText:'Local boundary fixture'}).waitFor();
 const composer=page.locator('.chat-composer textarea');
 await composer.fill('核对这条较长的原始消息在编辑前后的宽度，保留原始内容与输入草稿。'.repeat(14));await composer.press('Enter');
 await page.getByText('最后一段内容应完整显示在输入区域上方。',{exact:true}).waitFor({timeout:60000});await settle();
 const handle=page.locator('.chat-composer-resize');
 for(const mode of ['keyboard','pointer']){
  await page.locator('.chat-composer-toolbar button[aria-label="停止"]').waitFor();
  const before=await bottom();
  if(mode==='keyboard'){await handle.focus();for(let i=0;i<6;i++)await handle.press('ArrowUp');}
  else{const box=await handle.boundingBox();await page.mouse.move(box.x+box.width/2,box.y+box.height/2);await page.mouse.down();await page.mouse.move(box.x+box.width/2,box.y+box.height/2-70,{steps:6});await page.mouse.up();}
  await settle();const resized=await geometry();assert.equal(resized.height,before.height);assert.equal(resized.extent,before.extent);assert(Math.abs(resized.top-before.top)<1,'Active resize moved the transcript');
  const marker=`新增流式段落 ${mode}`;stream.append('\n\n'+marker+'\n\n继续输出内容。'.repeat(16));await page.getByText(marker,{exact:true}).waitFor();await settle();
  const appended=await geometry();assert(Math.abs(appended.top-before.top)<1,'A streaming token re-enabled following after resize');
  evidence.push({phase:'active-resize',mode,before,resized,appended,returned:await bottom()});
 }
 stream.finish();await page.waitForFunction(()=>!document.querySelector('.chat-composer-toolbar button[aria-label="停止"]'));await settle();
 // Actual local path, actual sidebar preview, and the shared Cindy menu.
 const chip=page.getByRole('button',{name:filePath,exact:true});await chip.evaluate(el=>{const v=el.closest('.chat-transcript-viewport'),f=document.querySelector('.chat-input-area');v.scrollTop+=el.getBoundingClientRect().bottom-f.getBoundingClientRect().top+80;});await settle();await chip.click();
 await page.locator('.chat-side-panel').getByText('隔离验收内容。',{exact:true}).waitFor();assert.equal(await page.getByRole('menu').count(),0);
 await page.locator('.chat-side-panel').getByRole('button',{name:'关闭侧栏',exact:true}).click();
 await chip.click({button:'right'});await page.getByRole('menuitem',{name:'复制文件',exact:true}).waitFor();await page.getByRole('menuitem',{name:'复制路径',exact:true}).waitFor();await page.keyboard.press('Escape');
 evidence.push({phase:'local-file',path:filePath,chip:true,leftPreview:true,rightMenu:true});
 for(const theme of ['initial','dark']){
  if(theme==='dark'){
   // Leave Chat and release its worker before a global theme transaction.
   await page.locator('.app-navigation [data-page-id=overview]').click();
   await page.evaluate(async()=>{const app=window.go.main.App,catalog=JSON.parse(await app.Chat('catalog','null')),id=catalog.conversations.find(item=>item.title==='Content boundary fixture').id;await app.Chat('release',JSON.stringify({id}));const value=await app.Snapshot();await app.SaveSettings({...value.settings,theme:'dark'});});
   await page.reload({waitUntil:'domcontentloaded'});await page.locator('.app-navigation [data-page-id=chat]').click();await page.getByRole('button',{name:'Content boundary fixture',exact:true}).click();await page.getByText('最后一段内容应完整显示在输入区域上方。',{exact:true}).waitFor();
  }
  const message=page.locator('.desktop-conversation-message--user').last(),bubble=message.locator('.chat-message-bubble'),edit=message.getByRole('button',{name:'编辑消息',exact:true});
  await edit.evaluate(el=>{const v=el.closest('.chat-transcript-viewport'),f=document.querySelector('.chat-input-area');v.scrollTop+=el.getBoundingClientRect().bottom-f.getBoundingClientRect().top+80;});await settle();
  const original=await bubble.evaluate(el=>el.getBoundingClientRect().width);await edit.click();await settle();const entered=await bubble.evaluate(el=>el.getBoundingClientRect().width);assert(Math.abs(original-entered)<2);
  const editor=page.getByRole('textbox',{name:'编辑消息',exact:true});await editor.fill('短');await settle();const small=await bubble.evaluate(el=>el.getBoundingClientRect().width);
  await editor.fill('长内容必须在正常气泡上限换行。'.repeat(40));await settle();const large=await bubble.evaluate(el=>el.getBoundingClientRect().width);assert(large>small+30);assert(await editor.evaluate(el=>el.scrollWidth<=el.clientWidth+1));await editor.press('Escape');
  const positions=[await bottom()],handle=page.locator('.chat-composer-resize');
  for(const keys of [Array(8).fill('ArrowUp'),Array(30).fill('ArrowUp'),Array(26).fill('ArrowDown')]){
   const before=await geometry();await handle.focus();for(const key of keys)await handle.press(key);await settle();const resized=await geometry();evidence.push({phase:'resize',theme,keys:keys.length,direction:keys[0],before,resized});assert.equal(resized.height,before.height);assert.equal(resized.extent,before.extent);assert(Math.abs(resized.top-before.top)<1);positions.push(await bottom());
  }
  await page.mouse.move(0,0);await page.screenshot({path:join(output,`native-${theme}-bottom.png`)});evidence.push({theme,original,entered,small,large,positions});
 }
 assert.equal(requests,1,'Layout actions resent the task');assert.deepEqual(serverErrors,[]);assert.deepEqual(errors,[]);
 await writeFile(join(output,'result.json'),JSON.stringify({native:true,modelFixture:true,requests,evidence,errors},null,2));
 execFileSync('powershell.exe',['-NoProfile','-File',resolve('apps/desktop/frontend/tests/native-window-state.ps1'),'-OwnedProcessId',process.env.WINONE_COMPONENTS_PID,'-Executable',join(output,'WinOne.exe'),'-Action','exit'],{windowsHide:true,encoding:'utf8',timeout:10000});
}catch(error){if(page)await page.screenshot({path:join(output,'failure.png')}).catch(()=>{});await writeFile(join(output,'node-error.txt'),String(error.stack||error)+'\n'+serverErrors.join('\n'));await writeFile(join(output,'failure.json'),JSON.stringify({evidence,errors,requests,geometry:page?await geometry().catch(()=>null):null},null,2));process.exitCode=1;}
finally{await browser?.close();server.closeAllConnections();await new Promise(done=>server.close(done));}
