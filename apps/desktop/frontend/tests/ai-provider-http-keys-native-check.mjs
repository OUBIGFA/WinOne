// Explicit real-window check: only the component runner's private profile and
// a second loopback IP are used. Fixture keys must never become public settings.
import {createRequire} from 'node:module';
import {readFile,writeFile} from 'node:fs/promises';
import {resolve,dirname,relative,isAbsolute} from 'node:path';
import {fileURLToPath} from 'node:url';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import http from 'node:http';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,outputArg]=process.argv.slice(2),output=resolve(outputArg),run=promisify(execFile),testDirectory=dirname(fileURLToPath(import.meta.url));
const profile=process.env.WINONE_COMPONENTS_PROFILE??'',inside=relative(output,resolve(profile));
if(new URL(endpoint).hostname!=='127.0.0.1'||!inside||inside.startsWith('..')||isAbsolute(inside))throw Error('Use an isolated component runner');
const keys={first:'fixture-http-first',second:'fixture-http-second',replacement:'fixture-http-replacement'};
const calls=[],serverErrors=[],results={layouts:[]};
const server=http.createServer(async(req,res)=>{
 try{
  if(![keys.first,keys.replacement].some(key=>req.headers.authorization===`Bearer ${key}`))throw Error('Expected fixture credential was not forwarded');
  res.setHeader('Content-Type','application/json');
  if(req.method==='GET'&&req.url==='/v1/models'){calls.push('models');res.end(JSON.stringify({data:[{id:'fixture-http-model'}]}));return;}
  const chunks=[];for await(const chunk of req)chunks.push(chunk);const body=JSON.parse(Buffer.concat(chunks).toString('utf8'));
  if(req.url!=='/v1/chat/completions'||body.model!=='fixture-http-model')throw Error('Wrong endpoint or model');
  calls.push('generation');res.end(JSON.stringify({choices:[{message:{role:'assistant',content:'HTTP_OK'},finish_reason:'stop'}]}));
 }catch(error){serverErrors.push(String(error));res.writeHead(500);res.end(JSON.stringify({error:{message:'Fixture request rejected'}}));}
});
// The previous implementation admitted only 127.0.0.1, localhost and ::1.
// 127.0.0.2 reproduces that restriction without exposing a LAN listener.
await new Promise((done,reject)=>{server.once('error',reject);server.listen(0,'127.0.0.2',done);});
const baseUrl=`http://127.0.0.2:${server.address().port}/v1`;
let browser,page;
const check=(condition,message)=>{if(!condition)throw Error(message);};
try{
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(done=>setTimeout(done,100));}}
 check(browser,'Owned WebView did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(page=>page.url().includes('winone.localhost'));if(page)break;await new Promise(done=>setTimeout(done,100));}
 check(page,'Owned UI document missing');await page.getByRole('navigation').waitFor();await page.emulateMedia({reducedMotion:'reduce'});
 const errors=[];page.on('pageerror',error=>errors.push(String(error)));
 const call=(method,params=null)=>page.evaluate(async({method,params})=>JSON.parse(await window.go.main.App.Capture(method,JSON.stringify(params))),{method,params});
 const configuration=await call('ai.configuration.get');
 configuration.providers=[
  {id:'http-one',name:'HTTP 验证服务',authMode:'api-key',protocol:'chat',baseUrl:'http://relay.example:3000/v1',models:['fixture-http-model'],keys:[{id:'first',enabled:true},{id:'second',enabled:false}]},
  {id:'http-two',name:'第二 HTTP 服务',authMode:'api-key',protocol:'chat',baseUrl,models:['fixture-http-model'],keys:[{id:'primary',enabled:true}]},
 ];configuration.tasks={};configuration.magpieSettings={...configuration.magpieSettings,noStats:true,noUsageStats:true};
 await call('ai.configuration.save',{configuration,keyUpdates:{'http-one':{first:keys.first,second:keys.second},'http-two':{primary:keys.second}}});
 await page.getByRole('button',{name:'AI 服务',exact:true}).click();
 const first=()=>page.getByLabel('API 密钥 1',{exact:true}),save=()=>page.getByRole('button',{name:'保存 AI 设置',exact:true});
 await first().waitFor();check(await page.getByLabel('API 基础地址',{exact:true}).inputValue()==='http://relay.example:3000/v1','HTTP hostname did not round-trip through the Host');
 await page.getByText('HTTP 会明文传输密钥和请求内容，请仅用于可信网络。',{exact:true}).waitFor();
 const original=await readFile(resolve(profile,'ai-settings.json'),'utf8');
 for(let cycle=0;cycle<3;cycle++){
  await page.getByRole('button',{name:'显示 API 密钥 1',exact:true}).click();
  await page.waitForFunction(expected=>document.querySelector('#ai-keys-http-one-first')?.value===expected,keys.first);
  check(await first().getAttribute('type')==='text','Saved key was not visible');
  await page.getByRole('button',{name:'隐藏 API 密钥 1',exact:true}).click();
  check(await first().inputValue()===''&&await first().getAttribute('type')==='password','Hidden saved key remained in the input');
 }
 await page.getByRole('button',{name:'显示 API 密钥 2',exact:true}).click();
 await page.waitForFunction(expected=>document.querySelector('#ai-keys-http-one-second')?.value===expected,keys.second);
 await page.getByRole('button',{name:'隐藏 API 密钥 2',exact:true}).click();
 check(await save().isDisabled(),'Viewing a saved key dirtied the form');
 check(await readFile(resolve(profile,'ai-settings.json'),'utf8')===original,'Viewing keys rewrote the settings file');results.repeatableSavedAndDisabledKeys=true;
 await page.getByRole('button',{name:'添加密钥',exact:true}).click();await page.getByLabel('API 密钥 3',{exact:true}).fill('fixture-draft-key');
 for(let cycle=0;cycle<2;cycle++){
  await page.getByRole('button',{name:'显示 API 密钥 3',exact:true}).click();check(await page.getByLabel('API 密钥 3',{exact:true}).inputValue()==='fixture-draft-key','Draft key disappeared');
  await page.getByRole('button',{name:'隐藏 API 密钥 3',exact:true}).click();check(await page.getByLabel('API 密钥 3',{exact:true}).getAttribute('type')==='password','Draft key was not masked');
 }
 await page.getByRole('button',{name:'删除密钥 3',exact:true}).click();results.repeatableDraftKey=true;
 await page.getByLabel('API 基础地址',{exact:true}).fill(baseUrl);
 await page.getByRole('button',{name:'获取模型列表',exact:true}).click();await page.getByRole('checkbox',{name:'fixture-http-model',exact:true}).waitFor();
 await page.getByRole('button',{name:'测试已选模型',exact:true}).click();await page.getByText('全部 1 个模型测试通过',{exact:true}).waitFor({timeout:95000});
 check(calls.includes('models')&&calls.includes('generation'),'Real HTTP discovery or generation was not exercised');
 check((await call('ai.configuration.get')).providers[0].baseUrl==='http://relay.example:3000/v1','Draft model calls saved the connection');results.realHTTPDiscoveryAndModelTest=true;
 await save().click();await page.getByText('已保存',{exact:true}).waitFor();check(await first().inputValue()==='','Saving retained a revealed key');
 await page.getByRole('button',{name:'显示 API 密钥 1',exact:true}).click();await page.waitForFunction(expected=>document.querySelector('#ai-keys-http-one-first')?.value===expected,keys.first);
 await first().fill(keys.replacement);await save().click();await page.getByText('已保存',{exact:true}).waitFor();check(await first().inputValue()==='','Replacement stayed exposed after save');
 await page.getByRole('button',{name:'显示 API 密钥 1',exact:true}).click();await page.waitForFunction(expected=>document.querySelector('#ai-keys-http-one-first')?.value===expected,keys.replacement);
 await page.getByRole('button',{name:'隐藏 API 密钥 1',exact:true}).click();results.savedReplacementCanBeRevealed=true;
 const snapshot=JSON.stringify(await call('ai.configuration.get')),disk=await readFile(resolve(profile,'ai-settings.json'),'utf8');
 check(!Object.values(keys).some(key=>snapshot.includes(key)||disk.includes(key)),'Plaintext key escaped explicit reveal');results.settingsAndDiskRedacted=true;
 const summary=page.getByRole('region',{name:'已启用供应商',exact:true});
 await summary.getByRole('button',{name:'第二 HTTP 服务（1）',exact:true}).click();
 await page.getByRole('button',{name:'显示 API 密钥',exact:true}).click();await page.waitForFunction(expected=>document.querySelector('#ai-keys-http-two-primary')?.value===expected,keys.second);
 await summary.getByRole('button',{name:'HTTP 验证服务（1）',exact:true}).click();check(await first().inputValue()==='','Switching providers leaked the previous key');
 await page.getByRole('button',{name:'设置',exact:true}).click();await page.getByRole('button',{name:'AI 服务',exact:true}).click();
 await page.getByRole('button',{name:'显示 API 密钥 1',exact:true}).click();await page.waitForFunction(expected=>document.querySelector('#ai-keys-http-one-first')?.value===expected,keys.replacement);
 await page.getByRole('button',{name:'隐藏 API 密钥 1',exact:true}).click();results.providerSwitchAndPageReopen=true;
 const windowState=(action,width=1280,height=940)=>run('powershell.exe',['-NoProfile','-ExecutionPolicy','Bypass','-File',resolve(testDirectory,'native-window-state.ps1'),'-OwnedProcessId',process.env.WINONE_COMPONENTS_PID,'-Executable',resolve(output,'WinOne.exe'),'-Action',action,'-Width',String(width),'-Height',String(height)],{windowsHide:true,timeout:15000});
 for(const [theme,width,height] of [['light',1280,940],['dark',800,680]]){
  await windowState('resize',width,height);await page.waitForFunction(width=>innerWidth===width,width);await page.evaluate(async theme=>{document.documentElement.dataset.theme=theme;await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);},theme);
  // Resize is asynchronous in the native host. Center the real key row after
  // layout settles, rather than screenshotting an earlier scroll position.
  await first().evaluate(node=>node.scrollIntoView({block:'center',behavior:'instant'}));
  await page.waitForFunction(()=>{const row=document.querySelector('#ai-keys-http-one-first').getBoundingClientRect(),footer=document.querySelector('.settings-save-bar:not([hidden])').getBoundingClientRect();return row.top>=40&&row.bottom<=footer.top;});
  await page.evaluate(()=>{document.activeElement?.blur();});await page.mouse.move(0,0);await page.waitForTimeout(300);
  const geometry=await page.evaluate(()=>({overflow:[...document.querySelectorAll('html,body,main,[data-slot="page-scroll"]')].filter(element=>element.clientWidth&&element.scrollWidth>element.clientWidth+1).map(element=>element.tagName),controls:[...document.querySelectorAll('input[id^="ai-keys-"][autocomplete="new-password"]')].map(element=>{const r=element.getBoundingClientRect();return {width:r.width,left:r.left,right:r.right,top:r.top,bottom:r.bottom,viewport:innerWidth,footer:document.querySelector('.settings-save-bar:not([hidden])').getBoundingClientRect().top};})}));
  results.layouts.push({theme,width,height,...geometry});
  check(!geometry.overflow.length&&geometry.controls.length===2&&geometry.controls.every(r=>r.width>=100&&r.left>=0&&r.right<=r.viewport+1&&r.top>=40&&r.bottom<=r.footer),'Key controls overflow or became unusably narrow');
  await page.screenshot({path:resolve(output,`keys-${theme}-${width}.png`)});
  if(process.env.WINONE_WINDOW_SHOT){const shot=await run(process.env.WINONE_WINDOW_SHOT,[process.env.WINONE_COMPONENTS_PID,resolve(output,`keys-${theme}-${width}-window.png`),'-wait','300'],{windowsHide:true,timeout:15000});await writeFile(resolve(output,`keys-${theme}-${width}-window.log`),shot.stdout+shot.stderr);}
 }
 check(!errors.length&&!serverErrors.length,'Runtime or vendor fixture errors occurred');results.calls=calls;results.errors=[];
 await writeFile(resolve(output,'result.json'),JSON.stringify(results,null,2));await windowState('exit');
}catch(error){await writeFile(resolve(output,'partial-result.json'),JSON.stringify({...results,calls,serverErrors},null,2));await writeFile(resolve(output,'node-error.txt'),String(error.stack||error));if(page){await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});await writeFile(resolve(output,'failure.txt'),await page.locator('body').innerText().catch(()=>''));}process.exitCode=1;}
finally{await browser?.close().catch(()=>{});server.closeAllConnections();await new Promise(done=>server.close(done));}
