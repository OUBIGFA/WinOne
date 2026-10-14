// Drive the shipped Rust UI and its real Host/Magpie/Pi processes. Only the
// upstream model is a local HTTP fixture; tool writes stay in this test folder.
import {createRequire} from 'node:module';
import {mkdir,readFile,writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import http from 'node:http';
import {once} from 'node:events';
import {execFileSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import {dirname} from 'node:path';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,outputArg]=process.argv.slice(2),output=resolve(outputArg);
if(new URL(endpoint).hostname!=='127.0.0.1'||!process.env.WINONE_COMPONENTS_PROFILE?.startsWith(output))throw Error('Use the isolated desktop test launcher');
const project=resolve(output,'project'),archive=resolve(output,'conversation-data');await mkdir(project,{recursive:true});await mkdir(archive,{recursive:true});
const calls=[],serverErrors=[];
const server=http.createServer(async(req,res)=>{
 try{
  const parts=[];for await(const part of req)parts.push(part);const body=JSON.parse(Buffer.concat(parts).toString('utf8')||'{}');
  if(req.url==='/v1/models'){res.setHeader('Content-Type','application/json');res.end(JSON.stringify({data:[{id:'fixture-model',object:'model'}]}));return;}
  if(req.method!=='POST'||req.url!=='/v1/chat/completions')throw Error('Unexpected model route '+req.method+' '+req.url);
  if(req.headers.authorization!=='Bearer fixture-ui-key'||req.headers['x-winone-fixture']!=='desktop-chain')throw Error('Custom headers or saved credentials did not reach Magpie');
  calls.push(body);res.setHeader('Content-Type','text/event-stream');res.setHeader('Cache-Control','no-cache');
  const emit=(delta,finish=null)=>res.write('data: '+JSON.stringify({id:'fixture-'+calls.length,object:'chat.completion.chunk',created:1,model:'fixture-model',choices:[{index:0,delta,finish_reason:finish}]})+'\n\n');
  emit({role:'assistant',reasoning_content:'核对项目目录和操作权限。'});
  if(calls.length<=2){
   const tool=calls.length===1?{name:'write',arguments:JSON.stringify({path:'ui-proof.txt',content:'Rust -> Host -> Magpie -> Pi'})}:{name:'powershell',arguments:JSON.stringify({command:"Get-Content -LiteralPath 'ui-proof.txt'"})};
   emit({content:'正在执行项目操作。',tool_calls:[{index:0,id:'ui-call-'+calls.length,type:'function',function:tool}]});emit({},'tool_calls');
  }else{emit({content:'链路验证完成。'});emit({},'stop');}
  res.end('data: [DONE]\n\n');
 }catch(error){serverErrors.push(String(error));res.statusCode=500;res.end(JSON.stringify({error:{message:String(error)}}));}
});
server.listen(0,'127.0.0.1');await once(server,'listening');
let browser,page;
try{
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(done=>setTimeout(done,100));}}
 if(!browser)throw Error('Isolated WebView2 did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(page=>page.url().includes('winone.localhost'));if(page)break;await new Promise(done=>setTimeout(done,100));}
 if(!page)throw Error('WinOne document missing');
 const errors=[];page.on('pageerror',error=>errors.push(String(error)));
 await page.getByRole('navigation').waitFor({timeout:20000});
 const picture=await readFile(resolve('internal/branding/chat-welcome.png'));
 let upload='',icon='';
 for(let offset=0;offset<picture.length;offset+=256<<10){
  const response=await page.evaluate(async value=>JSON.parse(await window.go.main.App.Capture('ai.icons.store',JSON.stringify(value))),{data:picture.subarray(offset,offset+(256<<10)).toString('base64'),size:picture.length,offset,upload});upload=response.upload??'';icon=response.icon??icon;
 }
 if(!icon.startsWith('file:'))throw Error('Real avatar storage did not return a picture');
 const conversation=await page.evaluate(async({project,archive,baseUrl,icon})=>{
  const api=window.go.main.App;
  const configuration=JSON.parse(await api.Capture('ai.configuration.get','null'));
  configuration.providers=[{id:'fixture',name:'桌面链路服务',authMode:'api-key',protocol:'chat',baseUrl,models:['fixture-model'],modelNames:{'fixture-model':'Fixture Model'},headers:{'X-WinOne-Fixture':'desktop-chain'},avatar:icon,hasKey:false}];
  configuration.tasks={default:{provider:'fixture',model:'fixture-model'},chat:{provider:'fixture',model:'fixture-model'}};
  await api.Capture('ai.configuration.save',JSON.stringify({configuration,keys:{fixture:'fixture-ui-key'}}));
  const selected=JSON.parse(await api.Chat('project.add',JSON.stringify({path:project,name:'UI fixture'})));
  return JSON.parse(await api.Chat('conversation.new',JSON.stringify({projectId:selected.id,title:'Desktop E2E',model:{provider:'fixture',model:'fixture-model'},saveDirectory:archive})));
 },{project,archive,baseUrl:`http://127.0.0.1:${server.address().port}/v1`,icon});
 await page.getByRole('button',{name:'Chat',exact:true}).click();
 await page.locator('.chat-welcome-logo').waitFor();
 // Verify the current welcome requirements and preserve the complete image.
 const welcome=await page.locator('.chat-welcome-logo').evaluate(image=>({width:image.getBoundingClientRect().width,height:image.getBoundingClientRect().height,fit:getComputedStyle(image).objectFit,naturalWidth:image.naturalWidth,naturalHeight:image.naturalHeight}));
 if(Math.abs(welcome.width-132)>.6||welcome.fit!=='contain'||!welcome.naturalWidth||!welcome.naturalHeight)throw Error('Welcome image is missing or does not preserve its current size and fit');
 await page.getByRole('heading',{name:'WinOne Set Your Creativity Free',exact:true}).waitFor();
 await page.getByRole('button',{name:'Desktop E2E',exact:true}).click();
 await page.locator('.chat-model-trigger').filter({hasText:'桌面链路服务'}).waitFor();
 await page.waitForFunction(()=>document.querySelector('.chat-model-trigger [data-icon^="file:"] img')?.naturalWidth>0);
 const input=page.locator('.chat-composer textarea');await input.fill('在项目中创建证明文件，然后用 PowerShell 读取它。');await input.press('Enter');
 for(let step=0;step<2;step++){
  const approval=page.locator('.chat-approval');await approval.waitFor({timeout:45000});
  const tool=step===0?'write':'powershell';await approval.locator('strong').filter({hasText:tool}).waitFor();
  await approval.getByRole('button',{name:/^允许一次/}).click();
 }
 await page.getByText('链路验证完成。',{exact:true}).waitFor({timeout:45000});
 await page.waitForFunction(()=>document.querySelector('.chat-work-process [data-slot="cindy-work"] svg.lucide-layers'));
 if(await readFile(resolve(project,'ui-proof.txt'),'utf8')!=='Rust -> Host -> Magpie -> Pi')throw Error('Pi did not write the project file');
 if(calls.length!==3||!JSON.stringify(calls[2].messages).includes('Rust -> Host -> Magpie -> Pi'))throw Error('PowerShell tool output did not return through the conversation');
 const disclosure=page.locator('[data-slot="cindy-work"]');if(await disclosure.getAttribute('aria-expanded')!=='false')throw Error('Completed work process was not initially collapsed');
 await disclosure.click();await page.getByText('正在执行项目操作。',{exact:true}).first().waitFor();
 await page.screenshot({path:resolve(output,'chat-process.png')});
 const state=await page.evaluate(async id=>JSON.parse(await window.go.main.App.Chat('state',JSON.stringify({id}))),conversation.id);
 if(!state.operations?.some(operation=>operation.kind==='turn'&&operation.status==='completed'&&operation.durationMs>=0))throw Error('UI state lost real process timing');
 const catalog=await page.evaluate(async()=>JSON.parse(await window.go.main.App.Chat('catalog','null')));
 const saved=catalog.conversations.find(value=>value.id===conversation.id);
 if(!saved?.sessionDirectory.startsWith(archive))throw Error('Conversation data escaped the selected location');
 const history=await readFile(saved.sessionFile,'utf8');if(!history.includes('winone.operation')||!history.includes('ui-proof.txt'))throw Error('Conversation process/history was not saved');
 await page.getByRole('button',{name:'首页',exact:true}).click();await page.getByRole('button',{name:'Chat',exact:true}).click();await page.getByRole('button',{name:'Desktop E2E',exact:true}).click();
 await page.getByText('链路验证完成。',{exact:true}).waitFor();
 if(errors.length||serverErrors.length)throw Error([...errors,...serverErrors].join('\n'));
 await writeFile(resolve(output,'result.json'),JSON.stringify({rustHostMagpiePi:true,savedHeaders:true,avatar:true,write:true,powershell:true,approvals:2,processTiming:true,customSaveDirectory:true,reopened:true,modelRequests:calls.length,errors},null,2));
 // Ordinary close now hides the retained UI. The test must request the full
 // application-close path against its owned window, not leave it resident.
 execFileSync('powershell.exe',['-NoProfile','-File',resolve(dirname(fileURLToPath(import.meta.url)),'native-window-state.ps1'),'-OwnedProcessId',process.env.WINONE_COMPONENTS_PID,'-Executable',resolve(output,'WinOne.exe'),'-Action','exit'],{windowsHide:true,encoding:'utf8',timeout:10000});
}catch(error){
 await writeFile(resolve(output,'node-error.txt'),String(error.stack||error));
 if(page){await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});await writeFile(resolve(output,'failure.txt'),await page.locator('body').innerText().catch(()=>''));}
 throw error;
}finally{await browser?.close();server.closeAllConnections();await new Promise(done=>server.close(done));}
