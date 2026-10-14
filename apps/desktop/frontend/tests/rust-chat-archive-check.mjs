// Full native UI -> Host -> Magpie -> Pi -> Cindy archive workflow. Only the
// upstream model is a loopback fixture; all writes use the runner's private data.
import {createRequire} from 'node:module';
import {mkdir,readFile,writeFile,stat} from 'node:fs/promises';
import {resolve,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import http from 'node:http';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,outputArg]=process.argv.slice(2),output=resolve(outputArg),run=promisify(execFile),testDirectory=dirname(fileURLToPath(import.meta.url));
if(new URL(endpoint).hostname!=='127.0.0.1'||!process.env.WINONE_COMPONENTS_PROFILE?.startsWith(output))throw Error('Use an isolated component runner');
const first=resolve(output,'first-project'),second=resolve(output,'second-project'),attachment=resolve(output,'attachment.txt'),proof=resolve(output,'完整任务.cshare');
await mkdir(first,{recursive:true});await mkdir(second,{recursive:true});await writeFile(attachment,'NATIVE_ATTACHMENT_PROOF\n'.repeat(2000),'utf8');
const choose=mode=>run('powershell.exe',['-NoProfile','-File',resolve(testDirectory,'native-profile-picker.ps1'),'-OwnedProcessId',process.env.WINONE_COMPONENTS_PID,'-Executable',resolve(output,'WinOne.exe'),'-Path',proof,'-Mode',mode,'-Extension','.cshare'],{windowsHide:true,timeout:20000});
const calls=[],serverErrors=[];
const server=http.createServer(async(req,res)=>{
 try{const chunks=[];for await(const chunk of req)chunks.push(chunk);const body=JSON.parse(Buffer.concat(chunks).toString('utf8')||'{}');
  if(req.url==='/v1/models'){res.setHeader('Content-Type','application/json');res.end(JSON.stringify({data:[{id:'fixture-model',object:'model'}]}));return;}
  if(req.method!=='POST'||req.url!=='/v1/chat/completions'||req.headers.authorization!=='Bearer native-archive-fixture-key')throw Error('Unexpected local fixture request');
  calls.push(body);res.setHeader('Content-Type','text/event-stream');
  const emit=(delta,finish=null)=>res.write('data: '+JSON.stringify({id:'archive-'+calls.length,object:'chat.completion.chunk',created:1,model:'fixture-model',choices:[{index:0,delta,finish_reason:finish}]})+'\n\n');
  emit({role:'assistant'});
  if(calls.length===1){emit({content:'NATIVE_BEFORE_MOVE'});emit({},'stop');}
  else if(calls.length===2){emit({tool_calls:[{index:0,id:'native-moved-write',type:'function',function:{name:'write',arguments:JSON.stringify({path:'moved-proof.txt',content:'SECOND_PROJECT_ONLY'})}}]});emit({},'tool_calls');}
  else{emit({content:'NATIVE_AFTER_MOVE'});emit({},'stop');}
  res.end('data: [DONE]\n\n');
 }catch(error){serverErrors.push(String(error));res.writeHead(500);res.end(JSON.stringify({error:{message:String(error)}}));}
});
await new Promise(done=>server.listen(0,'127.0.0.1',done));
let browser,page;
try{
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(done=>setTimeout(done,100));}}
 if(!browser)throw Error('Owned WebView did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(page=>page.url().includes('winone.localhost'));if(page)break;await new Promise(done=>setTimeout(done,100));}
 if(!page)throw Error('Owned UI document missing');
 const errors=[];page.on('pageerror',error=>errors.push(String(error)));await page.getByRole('navigation').waitFor();
 const created=await page.evaluate(async({first,second,attachment,baseUrl})=>{
  const api=window.go.main.App,configuration=JSON.parse(await api.Capture('ai.configuration.get','null'));
  configuration.providers=[{id:'native-archive',name:'导出验证服务',authMode:'api-key',protocol:'chat',baseUrl,models:['fixture-model'],hasKey:false}];configuration.tasks={default:{provider:'native-archive',model:'fixture-model'},chat:{provider:'native-archive',model:'fixture-model'}};
  await api.Capture('ai.configuration.save',JSON.stringify({configuration,keys:{'native-archive':'native-archive-fixture-key'}}));
  const a=JSON.parse(await api.Chat('project.add',JSON.stringify({path:first,name:'First archive project'}))),b=JSON.parse(await api.Chat('project.add',JSON.stringify({path:second,name:'Second archive project'})));
  const conversation=JSON.parse(await api.Chat('conversation.new',JSON.stringify({projectId:a.id,title:'Native archive proof',model:{provider:'native-archive',model:'fixture-model'}})));
  const added=JSON.parse(await api.Chat('attachments.add',JSON.stringify({id:conversation.id,paths:[attachment]})));
  await api.Chat('draft.save',JSON.stringify({id:conversation.id,draft:{text:'Inspect the attachment.',attachments:added.attachments}}));
  return {conversation,second:b};
 },{first,second,attachment,baseUrl:`http://127.0.0.1:${server.address().port}/v1`});
 await page.getByRole('button',{name:'Chat',exact:true}).click();await page.getByRole('button',{name:'Native archive proof',exact:true}).click();
 const input=page.locator('.chat-composer textarea');await input.waitFor();
 // Opening a chat and restoring its draft are separate native requests. A
 // visible composer can still be empty/disabled until the draft has arrived.
 await page.waitForFunction(()=>{const input=document.querySelector('.chat-composer textarea');return input&&!input.disabled&&input.value==='Inspect the attachment.';});
 await input.press('Enter');await page.getByText('NATIVE_BEFORE_MOVE',{exact:true}).waitFor({timeout:45000});
 await input.fill('UNSENT_NATIVE_DRAFT');
 const waitDraft=()=>page.waitForFunction(async id=>JSON.parse(await window.go.main.App.Chat('draft.get',JSON.stringify({id}))).text==='UNSENT_NATIVE_DRAFT',created.conversation.id);
 await waitDraft();
 await page.getByRole('button',{name:'更多操作 · Native archive proof',exact:true}).click();await page.getByRole('menuitem',{name:'移到项目',exact:true}).click();
 const moving=page.locator('.chat-move-dialog');await moving.getByRole('button',{name:/Second archive project/}).click();await moving.getByRole('button',{name:'移动',exact:true}).click();await moving.waitFor({state:'hidden'});
 await page.waitForFunction(async({id,projectId})=>JSON.parse(await window.go.main.App.Chat('catalog','null')).conversations.find(item=>item.id===id)?.projectId===projectId,{id:created.conversation.id,projectId:created.second.id});
 if(await input.inputValue()!=='UNSENT_NATIVE_DRAFT')throw Error('UI migration discarded the draft');
 await input.fill('Write the proof in the new project.');await input.press('Enter');
 const approval=page.locator('.chat-approval');await approval.waitFor({timeout:45000});await approval.getByRole('button',{name:/^允许一次/}).click();await page.getByText('NATIVE_AFTER_MOVE',{exact:true}).waitFor({timeout:45000});
 if(await readFile(resolve(second,'moved-proof.txt'),'utf8')!=='SECOND_PROJECT_ONLY'||await stat(resolve(first,'moved-proof.txt')).then(()=>true,()=>false))throw Error('Native migration used the former cwd');
 await input.fill('UNSENT_NATIVE_DRAFT');await waitDraft();
 const saved=await page.evaluate(async id=>JSON.parse(await window.go.main.App.Chat('catalog','null')).conversations.find(item=>item.id===id),created.conversation.id);
 if(saved.sessionDirectory!==created.conversation.sessionDirectory)throw Error('Project migration moved the session storage');
 await page.getByRole('button',{name:'更多操作 · Native archive proof',exact:true}).click();await page.getByRole('menuitem',{name:'导出任务',exact:true}).click();
 const exporting=page.locator('.chat-export-dialog');await exporting.waitFor();
 await Promise.all([choose('cancel'),exporting.getByRole('button',{name:'导出',exact:true}).click()]);
 await exporting.getByRole('button',{name:'导出',exact:true}).waitFor();if(await stat(proof).then(()=>true,()=>false))throw Error('Cancelled native save wrote an archive');
 await exporting.getByRole('checkbox').click();await exporting.getByLabel('密码',{exact:true}).fill('ArchiveProof123');await exporting.getByLabel('再次输入密码',{exact:true}).fill('ArchiveProof123');
 await Promise.all([choose('save'),exporting.getByRole('button',{name:'导出',exact:true}).click()]);await exporting.waitFor({state:'hidden'});await page.getByText(proof,{exact:false}).waitFor();
 const verifier=resolve(testDirectory,'../../../../modules/chat-pi/tests/archive-file-check.mjs');
 const verified=await run(resolve(output,'shared/ai/runtime/bun.exe'),[verifier,proof,saved.sessionFile,attachment],{windowsHide:true,timeout:30000,encoding:'utf8',env:{...process.env,WINONE_CHAT_ARCHIVE_TEST_PASSWORD:'ArchiveProof123'}});
 const archive=JSON.parse(verified.stdout.trim());if(!archive.encrypted||archive.fidelity!=='full'||!JSON.stringify(calls[0]).includes('NATIVE_ATTACHMENT_PROOF'))throw Error('Native export did not retain the expected complete task');
 if(errors.length||serverErrors.length)throw Error([...errors,...serverErrors].join('\n'));
 await page.screenshot({path:resolve(output,'chat-archive-saved.png')});
 await writeFile(resolve(output,'result.json'),JSON.stringify({nativeSaveDialog:true,nativeCancelCreatedNoFile:true,uiProjectMove:true,realPiDestinationCwd:true,storageRetained:true,...archive,errors},null,2),'utf8');
 await run('powershell.exe',['-NoProfile','-File',resolve(testDirectory,'native-window-state.ps1'),'-OwnedProcessId',process.env.WINONE_COMPONENTS_PID,'-Executable',resolve(output,'WinOne.exe'),'-Action','exit'],{windowsHide:true,timeout:10000});
}catch(error){await writeFile(resolve(output,'node-error.txt'),String(error.stack||error),'utf8');if(page){await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});await writeFile(resolve(output,'failure.txt'),await page.locator('body').innerText().catch(()=>''),'utf8');}process.exitCode=1;}
finally{await browser?.close().catch(()=>{});server.closeAllConnections();await new Promise(done=>server.close(done));}
