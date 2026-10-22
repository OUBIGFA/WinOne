// Real Rust UI -> named pipe Host -> Magpie -> Pi, with only the remote model
// replaced by a loopback fixture. A response gate proves directory operations
// finish DURING generation, not after an accidentally completed short reply.
import {createRequire} from 'node:module';
import {mkdir,readFile,writeFile,stat} from 'node:fs/promises';
import {resolve,join,sep} from 'node:path';
import http from 'node:http';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,folder]=process.argv.slice(2),output=resolve(folder),workspace=join(output,'workspace'),target=join(output,'links');
if(new URL(endpoint).hostname!=='127.0.0.1'||!process.env.WINONE_COMPONENTS_PROFILE?.startsWith(output+sep))throw Error('Use the owned private component runner');
await mkdir(workspace);await mkdir(target);
await writeFile(join(workspace,'proof.txt'),'NATIVE_CONCURRENT_READ_PROOF');
await writeFile(join(workspace,'AGENTS.md'),'# Isolated concurrency check\nRead only this test workspace.\n');
const errors=[],serverErrors=[],requests=[],timings=[],gates=new Map();
let browser,page,conversation;
const gate=name=>new Promise(done=>gates.set(name,done));
const release=name=>{assert.ok(gates.has(name),'Response gate is missing: '+name);gates.get(name)();gates.delete(name);};
const until=async(check,label)=>{const end=Date.now()+20000;while(!await check()){if(Date.now()>end)throw Error('Timed out: '+label);await new Promise(done=>setTimeout(done,20));}};
const vendor=http.createServer(async(req,res)=>{
 try{
  let raw='';for await(const part of req)raw+=part;
  if(req.method==='GET'&&req.url==='/v1/models'){res.setHeader('Content-Type','application/json');res.end(JSON.stringify({data:[{id:'concurrent-model'}]}));return;}
  assert.equal(req.url,'/v1/chat/completions');assert.equal(req.headers.authorization,'Bearer private-concurrency-key');
  const body=JSON.parse(raw),number=requests.push(body);
  res.writeHead(200,{'Content-Type':'text/event-stream','Cache-Control':'no-cache'});
  const emit=(delta,finish_reason=null)=>res.write('data: '+JSON.stringify({id:'concurrency-'+number,object:'chat.completion.chunk',model:'concurrent-model',choices:[{index:0,delta,finish_reason}]})+'\n\n');
  emit({role:'assistant'});
  if(number===1){
   emit({content:'NATIVE_CONCURRENT_PARTIAL'});await gate('first-stream');
   emit({tool_calls:[{index:0,id:'concurrent-read',type:'function',function:{name:'read',arguments:JSON.stringify({path:'proof.txt'})}}]});emit({},'tool_calls');
  }else if(number===2){
   assert.ok(body.messages.some(message=>message.role==='tool'&&JSON.stringify(message.content).includes('NATIVE_CONCURRENT_READ_PROOF')),'Pi did not execute the real file tool');
   emit({content:'NATIVE_AFTER_TOOL_PARTIAL'});await gate('after-tool');emit({content:'\n\nNATIVE_CONCURRENT_DONE'});emit({},'stop');
  }else throw Error('Unexpected extra model request: '+number);
  res.end('data: [DONE]\n\n');
 }catch(error){serverErrors.push(String(error));if(!res.headersSent)res.writeHead(500,{'Content-Type':'application/json'});res.end(JSON.stringify({error:{message:String(error)}}));}
});
await new Promise(done=>vendor.listen(0,'127.0.0.1',done));
const call=(module,method,params={})=>page.evaluate(async({module,method,params})=>JSON.parse(await window.go.main.App[module](method,JSON.stringify(params))),{module,method,params});
async function timed(name,action){const start=performance.now(),result=await action(),ms=Math.round(performance.now()-start);timings.push({name,ms,modelRequests:requests.length,gates:[...gates.keys()]});assert.ok(ms<5000,name+' exceeded the normal short RPC budget: '+ms+' ms');return result;}
async function capture(name){await page.evaluate(async()=>{document.activeElement?.blur();await document.fonts.ready;await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);});await page.mouse.move(0,0);await page.screenshot({path:join(output,name+'.png')});}
try{
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(done=>setTimeout(done,100));}}
 if(!browser)throw Error('Owned WinOne did not expose its test connection');
 while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(page=>page.url().includes('winone.localhost'));if(page)break;await new Promise(done=>setTimeout(done,100));}
 if(!page)throw Error('Owned production document missing');page.setDefaultTimeout(20000);page.on('pageerror',error=>errors.push(error.stack||String(error)));
 await page.locator('.app-navigation').waitFor();
 conversation=await page.evaluate(async({workspace,baseUrl})=>{
  const api=window.go.main.App,snapshot=await api.Snapshot();
  await api.SaveSettings({...snapshot.settings,theme:'light',language:'zh-CN',startup:{...snapshot.settings.startup,keepUIRunning:false}});
  const configuration=JSON.parse(await api.Capture('ai.configuration.get','null'));
  configuration.providers=[{id:'concurrency-fixture',name:'并行链路验收',authMode:'api-key',protocol:'chat',baseUrl,models:['concurrent-model'],hasKey:false}];
  configuration.tasks={default:{provider:'concurrency-fixture',model:'concurrent-model'},chat:{provider:'concurrency-fixture',model:'concurrent-model'}};
  await api.Capture('ai.configuration.save',JSON.stringify({configuration,keys:{'concurrency-fixture':'private-concurrency-key'}}));
  const preferences=JSON.parse(await api.Chat('settings.get','null'));preferences.autoRetry=false;preferences.backgroundCompaction=false;await api.Chat('settings.save',JSON.stringify({settings:preferences}));
  return JSON.parse(await api.Chat('conversation.new',JSON.stringify({directory:workspace,title:'后台并行验收',model:{provider:'concurrency-fixture',model:'concurrent-model'}})));
 },{workspace,baseUrl:'http://127.0.0.1:'+vendor.address().port+'/v1'});
 await page.reload({waitUntil:'domcontentloaded'});await page.locator('[data-page-id="chat"]').click();await page.getByRole('button',{name:'后台并行验收',exact:true}).click();
 const input=page.getByRole('textbox',{name:'消息',exact:true});await input.fill('Read proof.txt and finish after the tool result.');await input.press('Enter');
 await until(()=>gates.has('first-stream'),'first stream');await page.getByText('NATIVE_CONCURRENT_PARTIAL',{exact:true}).waitFor();
 await page.locator('[data-page-id="convenience"]').click();await page.getByRole('tab',{name:'文件与目录链接',exact:true}).click();
 const sourcePane=page.locator('[data-directory-links-drop="source"]'),targetPane=page.locator('[data-directory-links-drop="target"]');
 await timed('visible source folder while streaming',async()=>{
  await sourcePane.getByRole('textbox').fill(workspace);await sourcePane.getByRole('textbox').press('Enter');await sourcePane.getByText('proof.txt',{exact:true}).waitFor();
 });
 await timed('visible destination folder while streaming',async()=>{
  await targetPane.getByRole('textbox').fill(target);await targetPane.getByRole('textbox').press('Enter');await targetPane.locator('.directory-links-file-list[aria-busy="false"]').waitFor();
 });
 // A refused settings transaction must not create a pending global writer or
 // leave Busy=true. Test the next ordinary mutation before releasing the AI.
 const saveFailure=await timed('settings refuse immediately without claiming maintenance',()=>page.evaluate(async()=>{
  const api=window.go.main.App,before=(await api.Snapshot()).settings;
  try{await api.SaveSettings({...before,theme:'dark'});return {rejected:false};}
  catch(error){return {rejected:true,message:String(error),before:before.theme,after:(await api.Snapshot()).settings.theme};}
 }));
 assert.equal(saveFailure.rejected,true,'A settings transaction overtook the active AI request');assert.match(saveFailure.message,/操作正在进行|配置正在保存/);assert.equal(saveFailure.after,saveFailure.before);
 await timed('Chat draft mutation after refused save',()=>call('Chat','draft.save',{id:conversation.id,draft:{text:'NATIVE_DRAFT_SURVIVES_CONCURRENT_FILES'}}));
 const plan=await timed('real link preview while streaming',()=>call('DirectoryLinks','preview',{source:join(workspace,'proof.txt'),directory:target,linkName:'linked.txt',kind:'hardlink'}));
 const created=await timed('real hardlink creation while streaming',()=>call('DirectoryLinks','create',{token:plan.id}));
 assert.equal(await readFile(join(target,'linked.txt'),'utf8'),'NATIVE_CONCURRENT_READ_PROOF');assert.equal(requests.length,1);assert.ok(gates.has('first-stream'));
 await capture('directory-links-during-stream');release('first-stream');
 await until(()=>gates.has('after-tool'),'real Pi read tool completed');
 assert.equal(requests.length,2);assert.equal((await call('Chat','state',{id:conversation.id})).streaming,true);
 await targetPane.getByRole('button',{name:'刷新目标文件夹',exact:true}).click();
 await targetPane.getByRole('button',{name:'重命名：linked.txt',exact:true}).click();
 await page.locator('#links-rename-name').fill('renamed.txt');
 await timed('visible rename while the tool follow-up streams',async()=>{
  await page.getByRole('dialog').getByRole('button',{name:'重命名',exact:true}).click();await page.getByRole('dialog').waitFor({state:'hidden'});await targetPane.getByRole('button',{name:'重命名：renamed.txt',exact:true}).waitFor();
 });
 await assert.rejects(stat(join(target,'linked.txt')),{code:'ENOENT'});assert.equal(await readFile(join(target,'renamed.txt'),'utf8'),'NATIVE_CONCURRENT_READ_PROOF');
 // Use the actual bridge concurrently: it must not serialize independent
 // calls behind file operations, even when their UI poll tickets overlap.
 const parallel=await Promise.all(Array.from({length:8},(_,index)=>timed('parallel browse '+index,()=>call('DirectoryLinks','browse',{directory:index%2?workspace:target}))));
 assert.ok(parallel.every(value=>value.entries.length>0));
 const activity=await timed('catalog-wide Chat activity',()=>call('Chat','activity'));assert.equal(activity[conversation.id].phase,'running');
 const draft=await timed('read preserved Chat draft',()=>call('Chat','draft.get',{id:conversation.id}));assert.equal(draft.text,'NATIVE_DRAFT_SURVIVES_CONCURRENT_FILES');
 await assert.rejects(timed('invalid folder fails without blocking other modules',()=>call('DirectoryLinks','browse',{directory:join(output,'missing')})),/不存在|cannot find|找不到|system cannot/i);
 await timed('browse recovers after folder failure',()=>call('DirectoryLinks','browse',{directory:workspace}));
 await timed('Chat stays writable after folder failure',()=>call('Chat','draft.save',{id:conversation.id,draft:{text:'NATIVE_DRAFT_SURVIVES_CONCURRENT_FILES'}}));
 assert.ok(gates.has('after-tool'));assert.equal((await call('Chat','state',{id:conversation.id})).streaming,true);
 assert.equal(await page.getByRole('alert').count(),0);await capture('directory-links-after-tool');release('after-tool');
 await until(async()=>!(await call('Chat','state',{id:conversation.id})).streaming,'stream completion');
 await page.locator('[data-page-id="chat"]').click();await page.getByRole('button',{name:'后台并行验收',exact:true}).click();await page.getByText('NATIVE_CONCURRENT_DONE',{exact:true}).waitFor();assert.equal(await input.inputValue(),'NATIVE_DRAFT_SURVIVES_CONCURRENT_FILES');
 await input.fill('');await until(async()=>(await call('Chat','draft.get',{id:conversation.id})).text==='','fixture draft cleared');
 await timed('settings save succeeds after generation',()=>page.evaluate(async()=>{const api=window.go.main.App,s=await api.Snapshot();await api.SaveSettings({...s.settings,theme:'dark'});}));
 const saved=JSON.parse(await readFile(join(process.env.WINONE_COMPONENTS_PROFILE,'configuration.json'),'utf8')).committed.values.host;assert.equal(saved.theme,'dark');
 assert.equal(await readFile(join(workspace,'proof.txt'),'utf8'),'NATIVE_CONCURRENT_READ_PROOF');assert.deepEqual(errors,[]);assert.deepEqual(serverErrors,[]);assert.equal(requests.length,2);
 await capture('chat-completed-with-files-intact');
 await writeFile(join(output,'result.json'),JSON.stringify({productionAssets:true,rustHostMagpiePi:true,modelRequests:requests.length,toolReadExecuted:true,linkId:created.id,sourcePreserved:true,draftPreserved:true,settingsExclusive:true,saveFailure,timings,errors,serverErrors},null,2));
 await page.evaluate(()=>{void window.go.main.App.WindowCommand('close');});
}catch(error){
 await writeFile(join(output,'node-error.txt'),String(error.stack||error)+'\n'+errors.join('\n')+'\n'+serverErrors.join('\n'));
 await writeFile(join(output,'partial-evidence.json'),JSON.stringify({timings,requests:requests.length,gates:[...gates.keys()],errors,serverErrors},null,2));
 if(page){await page.screenshot({path:join(output,'failure.png')}).catch(()=>{});await writeFile(join(output,'failure.txt'),await page.locator('body').innerText().catch(()=>''));}
 process.exitCode=1;
}finally{for(const done of gates.values())done();await browser?.close().catch(()=>{});vendor.closeAllConnections();await new Promise(done=>vendor.close(done));}
