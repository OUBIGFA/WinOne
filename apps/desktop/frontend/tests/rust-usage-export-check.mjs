// Actual system save dialog and Host export on the runner's private desktop.
import {createRequire} from 'node:module';
import {mkdir,readFile,writeFile,stat} from 'node:fs/promises';
import {resolve,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import http from 'node:http';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2),run=promisify(execFile),proof=resolve(output,'用量验证.csv');
if(new URL(endpoint).hostname!=='127.0.0.1'||!process.env.WINONE_COMPONENTS_PROFILE?.startsWith(output))throw Error('Use an isolated component runner');
const choose=mode=>run('powershell.exe',['-NoProfile','-File',resolve(dirname(fileURLToPath(import.meta.url)),'native-profile-picker.ps1'),'-OwnedProcessId',process.env.WINONE_COMPONENTS_PID,'-Executable',resolve(output,'WinOne.exe'),'-Path',proof,'-Mode',mode,'-Extension','.csv'],{windowsHide:true,timeout:20000});
const server=http.createServer(async(req,res)=>{for await(const _ of req){}res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({id:'export-fixture',model:'gpt-4o',choices:[{message:{role:'assistant',content:'Export fixture'},finish_reason:'stop'}],usage:{prompt_tokens:20,completion_tokens:5}}));});
await new Promise(done=>server.listen(0,'127.0.0.1',done));
let browser,page;
try{
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(done=>setTimeout(done,100));}}
 if(!browser)throw Error('Owned WebView did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(c=>c.pages()).find(p=>p.url().includes('winone.localhost'));if(page)break;await new Promise(done=>setTimeout(done,100));}
 if(!page)throw Error('Owned UI document missing');
 const errors=[];page.on('pageerror',error=>errors.push(String(error)));
 await page.getByRole('navigation').waitFor();
 const cache=resolve(process.env.WINONE_COMPONENTS_PROFILE,'ai-runtime/cache/magpie');await mkdir(cache,{recursive:true});
 await writeFile(resolve(cache,'models.json'),JSON.stringify({openai:{id:'openai',models:{'gpt-4o':{id:'gpt-4o',cost:{input:2,output:8}}}}}));
 const spare=http.createServer();await new Promise(done=>spare.listen(0,'127.0.0.1',done));const gatewayPort=spare.address().port;await new Promise(done=>spare.close(done));
 const gateway=await page.evaluate(async ({port,gatewayPort})=>{
  const app=window.go.main.App,configuration=JSON.parse(await app.Capture('ai.configuration.get','null'));
  configuration.providers=[{id:'csv-fixture',name:'CSV 验证服务',authMode:'api-key',protocol:'chat',baseUrl:'http://127.0.0.1:'+port+'/v1',models:['gpt-4o'],hasKey:false}];
  await app.Capture('ai.configuration.save',JSON.stringify({configuration,keys:{'csv-fixture':'isolated-csv-key'}}));
  const state=JSON.parse(await app.Capture('ai.gateway.get','null'));
  await app.Capture('ai.gateway.save',JSON.stringify({settings:{revision:state.revision,enabled:true,port:gatewayPort,models:null},resetKey:false}));
  const saved=JSON.parse(await app.Capture('ai.gateway.get','null')),key=JSON.parse(await app.Capture('ai.gateway.key','null')).key;
  return {base:saved.baseUrl,key};
 },{port:server.address().port,gatewayPort});
 for(let index=0;index<2;index++){const response=await fetch(gateway.base+'/chat/completions',{method:'POST',headers:{Authorization:'Bearer '+gateway.key,'Content-Type':'application/json'},body:JSON.stringify({model:'csv-fixture/gpt-4o',messages:[{role:'user',content:'isolated CSV verification'}]})});if(!response.ok)throw Error('Fixture gateway request failed: '+response.status);await response.json();}
 await page.getByRole('button',{name:'AI 服务',exact:true}).click();
 await page.getByRole('tab',{name:'用量统计',exact:true}).click();await page.getByRole('tab',{name:'请求明细',exact:true}).click();
 const exportButton=page.getByRole('button',{name:'导出 CSV',exact:true});await exportButton.waitFor();
 await Promise.all([choose('cancel'),exportButton.click()]);
 await exportButton.waitFor();if(await stat(proof).then(()=>true,()=>false))throw Error('Cancelled export created a file');
 await Promise.all([choose('save'),exportButton.click()]);
 await page.getByText(proof,{exact:false}).waitFor();
 const bytes=await readFile(proof),csv=bytes.toString('utf8');
 if(bytes[0]!==0xef||bytes[1]!==0xbb||bytes[2]!==0xbf||!csv.includes('csv-fixture')||csv.trim().split(/\r?\n/).length!==3)throw Error('Saved CSV lost BOM, selected requests or full rows');
 await page.screenshot({path:resolve(output,'csv-saved.png')});
 const state=JSON.parse(await page.evaluate(()=>window.go.main.App.Capture('ai.gateway.get','null')));
 await page.evaluate(async state=>{await window.go.main.App.Capture('ai.gateway.save',JSON.stringify({settings:{revision:state.revision,enabled:false,port:state.port,models:null},resetKey:false}));},state);
 if(errors.length)throw Error(errors.join('\n'));
 await writeFile(resolve(output,'result.json'),JSON.stringify({nativeSaveDialog:true,cancelCreatedNoFile:true,hostAtomicSave:true,utf8BOM:true,rows:2,path:proof,errors},null,2));
 await run('powershell.exe',['-NoProfile','-File',resolve(dirname(fileURLToPath(import.meta.url)),'native-window-state.ps1'),'-OwnedProcessId',process.env.WINONE_COMPONENTS_PID,'-Executable',resolve(output,'WinOne.exe'),'-Action','exit'],{windowsHide:true,timeout:10000});
}catch(error){await writeFile(resolve(output,'node-error.txt'),String(error.stack||error));if(page){await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});await writeFile(resolve(output,'failure.txt'),await page.locator('body').innerText().catch(()=>''));}process.exitCode=1;}finally{await browser?.close().catch(()=>{});server.closeAllConnections();await new Promise(done=>server.close(done));}

