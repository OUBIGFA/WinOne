// Real desktop -> Host -> Magpie settings and connectivity, with a loopback
// provider and the component runner's private profile. No user account is read.
import {createRequire} from 'node:module';
import {readFile,writeFile} from 'node:fs/promises';
import {resolve,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import http from 'node:http';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,outputArg]=process.argv.slice(2),output=resolve(outputArg),run=promisify(execFile),testDirectory=dirname(fileURLToPath(import.meta.url));
if(new URL(endpoint).hostname!=='127.0.0.1'||!process.env.WINONE_COMPONENTS_PROFILE?.startsWith(output))throw Error('Use an isolated component runner');
const calls=[],serverErrors=[];let reject=false;
const server=http.createServer(async(req,res)=>{
 try{
  res.setHeader('Content-Type','application/json');
  if(req.method==='GET'){res.end(JSON.stringify({data:[{id:'actual-model'}]}));return;}
  const chunks=[];for await(const chunk of req)chunks.push(chunk);const body=JSON.parse(Buffer.concat(chunks).toString('utf8'));
  calls.push(body);
  if(req.headers.authorization!=='Bearer native-settings-fixture'||body.model!=='actual-model')throw Error('Draft key or model mapping did not reach the vendor');
  if(reject){res.writeHead(403);res.end(JSON.stringify({error:{message:'NATIVE_MODEL_DENIED'}}));return;}
  res.end(JSON.stringify({choices:[{message:{role:'assistant',content:'OK'},finish_reason:'stop'}]}));
 }catch(error){serverErrors.push(String(error));res.writeHead(500);res.end(JSON.stringify({error:{message:String(error)}}));}
});
await new Promise(done=>server.listen(0,'127.0.0.1',done));
let browser,page;const results={};
try{
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(done=>setTimeout(done,100));}}
 if(!browser)throw Error('Owned WebView did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(page=>page.url().includes('winone.localhost'));if(page)break;await new Promise(done=>setTimeout(done,100));}
 if(!page)throw Error('Owned UI document missing');
 const errors=[];page.on('pageerror',error=>errors.push(String(error)));await page.getByRole('navigation').waitFor();
 await page.evaluate(async()=>{
  const api=window.go.main.App,configuration=JSON.parse(await api.Capture('ai.configuration.get','null'));
  configuration.providers=[{id:'native-settings',name:'设置验证服务',authMode:'api-key',protocol:'chat',baseUrl:'http://127.0.0.1:1/v1',models:['alias-model'],hasKey:false},{id:'second-settings',name:'第二验证服务',authMode:'api-key',protocol:'chat',baseUrl:'http://127.0.0.1:1/v1',models:['second-model'],hasKey:false}];configuration.tasks={};
  await api.Capture('ai.configuration.save',JSON.stringify({configuration}));
 });
 await page.getByRole('button',{name:'AI 服务',exact:true}).click();
 await page.getByLabel('API 基础地址',{exact:true}).fill(`http://127.0.0.1:${server.address().port}/v1`);
 await page.getByLabel('API 密钥',{exact:true}).fill('native-settings-fixture');
 await page.getByRole('button',{name:'模型设置：alias-model',exact:true}).click();
 const dialog=page.getByRole('dialog',{name:'模型设置',exact:true});
 await dialog.getByLabel('请求模型名称',{exact:true}).fill('actual-model');
 await dialog.getByLabel('显示名称',{exact:true}).fill('原生映射验证');
 await dialog.getByRole('button',{name:'关闭',exact:true}).click();
 await page.getByRole('button',{name:'测试已选模型',exact:true}).click();
 await page.getByText('全部 1 个模型测试通过',{exact:true}).waitFor({timeout:95000});
 const before=await page.evaluate(async()=>JSON.parse(await window.go.main.App.Capture('ai.configuration.get','null')));
 if(before.providers[0].baseUrl!=='http://127.0.0.1:1/v1'||before.providers[0].modelMetadata?.['alias-model']?.upstreamName)throw Error('Testing saved the draft');
 results.unsavedConnectionAndMapping=true;
 // Vendor error bodies may contain secrets: assert the failure/status while
 // ensuring the raw response text is not disclosed by the desktop.
 reject=true;await page.getByRole('button',{name:'测试已选模型',exact:true}).click();
 await page.getByText('1 个模型中有 1 个测试失败',{exact:true}).waitFor({timeout:95000});
 await page.getByText(/403 Model connectivity test failed/).waitFor();
 if((await page.locator('body').innerText()).includes('NATIVE_MODEL_DENIED'))throw Error('Vendor error body leaked into the UI');
 reject=false;results.realFailureVisible=true;results.vendorErrorBodyRedacted=true;
 await page.getByRole('button',{name:'保存 AI 设置',exact:true}).click();await page.getByText('已保存',{exact:true}).waitFor();
 await page.getByRole('tab',{name:'额度设置',exact:true}).click();await page.getByRole('button',{name:'剩余额度',exact:true}).click();
 await page.getByRole('tab',{name:'隐私设置',exact:true}).click();await page.getByRole('switch',{name:'脱敏个人信息',exact:true}).click();
 await page.getByLabel('自定义脱敏词',{exact:true}).fill('NATIVE_PRIVATE_WORD');
 await page.getByRole('tab',{name:'模型设置',exact:true}).click();
 await page.getByLabel('搜索 API 密钥',{exact:true}).fill('native-search-fixture');await page.getByRole('button',{name:'添加搜索 API',exact:true}).click();
 await page.getByRole('button',{name:'保存 AI 设置',exact:true}).click();await page.getByText('已保存',{exact:true}).waitFor();
 const saved=await page.evaluate(async()=>JSON.parse(await window.go.main.App.Capture('ai.configuration.get','null')));
 if(!saved.magpieSettings.quotaLeft||!saved.magpieSettings.redactPersonal||!saved.magpieSettings.redactWords.includes('NATIVE_PRIVATE_WORD')||!saved.magpieSettings.searchAPIs[0].hasKey||saved.magpieSettings.searchAPIs[0].key)throw Error('Global settings did not persist safely');
 if((await readFile(resolve(process.env.WINONE_COMPONENTS_PROFILE,'ai-settings.json'),'utf8')).includes('native-search-fixture'))throw Error('Search key was stored as plaintext');
 results.globalSettingsAndEncryption=true;
 await page.getByRole('button',{name:'设置',exact:true}).click();await page.getByRole('button',{name:'AI 服务',exact:true}).click();
 await page.getByRole('tab',{name:'隐私设置',exact:true}).click();
 if(await page.getByRole('switch',{name:'脱敏个人信息',exact:true}).getAttribute('aria-checked')!=='true')throw Error('Reopened privacy state differs');
 const summary=page.getByRole('region',{name:'已启用供应商',exact:true});
 await summary.getByRole('button',{name:'第二验证服务（1）',exact:true}).click();
 await page.getByRole('region',{name:'模型设置',exact:true}).getByRole('checkbox',{name:'second-model',exact:true}).waitFor();
 await summary.getByRole('button',{name:'设置验证服务（1）',exact:true}).click();
 await page.getByRole('region',{name:'模型设置',exact:true}).getByRole('checkbox',{name:'原生映射验证',exact:true}).waitFor();
 results.reopenAndOverviewNavigation=true;
 if(calls.length!==2||errors.length||serverErrors.length)throw Error(JSON.stringify({calls:calls.length,errors,serverErrors}));
 results.requests=calls.length;results.errors=[];
 await page.screenshot({path:resolve(output,'ai-settings.png')});await writeFile(resolve(output,'result.json'),JSON.stringify(results,null,2),'utf8');
 await run('powershell.exe',['-NoProfile','-File',resolve(testDirectory,'native-window-state.ps1'),'-OwnedProcessId',process.env.WINONE_COMPONENTS_PID,'-Executable',resolve(output,'WinOne.exe'),'-Action','exit'],{windowsHide:true,timeout:10000});
}catch(error){await writeFile(resolve(output,'node-error.txt'),String(error.stack||error),'utf8');if(page){await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});await writeFile(resolve(output,'failure.txt'),await page.locator('body').innerText().catch(()=>''),'utf8');}process.exitCode=1;}
finally{await browser?.close().catch(()=>{});server.closeAllConnections();await new Promise(done=>server.close(done));}
