// Opt-in end-to-end: real settings → plugin manager → Host → Magpie/Bun →
// author's dedicated browser. Only the website is local; no personal login.
import {createRequire} from 'node:module';
import {mkdir,writeFile,readFile,readdir} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {pathToFileURL} from 'node:url';
import {execFileSync} from 'node:child_process';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,folder]=process.argv.slice(2),output=resolve(folder),checkout=resolve(process.env.WINONE_CHATGPT_WEB_CHECKOUT);
if(!process.env.WINONE_COMPONENTS_PROFILE?.startsWith(output))throw Error('Use the isolated desktop runner');
const {fixture}=await import(pathToFileURL(join(checkout,'test/helpers.mjs'))),site=await fixture();
site.state.autoLogin=false;
const profiles=join(output,'browser-profiles'),plugin=join(output,'browser-plugin');
await mkdir(plugin);
await writeFile(join(plugin,'package.json'),JSON.stringify({name:'winone-browser-login-fixture',type:'module',main:'index.mjs',version:'1.0.0'}));
await writeFile(join(plugin,'index.mjs'),`import {createPluginHooks} from ${JSON.stringify(pathToFileURL(join(checkout,'dist/plugin.js')).href)};import {BrowserBridge} from ${JSON.stringify(pathToFileURL(join(checkout,'dist/bridge.js')).href)};import {parseOptions} from ${JSON.stringify(pathToFileURL(join(checkout,'dist/options.js')).href)};export default {server:async(_input,options)=>createPluginHooks(options,new BrowserBridge(parseOptions(options),${JSON.stringify(site.url)}))};`);
let browser,page,loginBrowser;const errors=[],calls=[];
try{
 const until=Date.now()+30000;
 while(Date.now()<until){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(r=>setTimeout(r,100));}}
 assert(browser);page=browser.contexts().flatMap(c=>c.pages()).find(p=>p.url().includes('winone.localhost'));assert(page);page.setDefaultTimeout(20000);
 page.on('pageerror',error=>errors.push(error.message));
 await page.locator('.dashboard-tool').first().waitFor();
 await page.evaluate(async()=>{await window.go.main.App.Snapshot();await window.go.main.App.SettingsNavigation();});
 await page.locator('.app-navigation [data-page-id="ai-services"]').click();
 await page.getByRole('tab',{name:/^(插件|Plugins)$/}).click();
 const frame=page.frameLocator('iframe[title="插件"],iframe[title="Plugins"]');
 await frame.getByRole('textbox',{name:/^(插件|Plugin)$/}).fill(plugin);
 await frame.getByRole('button',{name:/^(安装|Install)$/}).last().click();
 await frame.getByRole('button',{name:/^(已安装|Installed)/}).click();
 await frame.getByRole('button',{name:/^(登录|Sign in)$/}).waitFor({timeout:30000});
 const f=await (await page.locator('iframe[title="插件"],iframe[title="Plugins"]').elementHandle()).contentFrame();
 // Configuration is the actual upstream plugin option API, not a mocked login.
 await f.evaluate(async({plugin,profiles})=>{await api('plugins/options',{spec:plugin,options:{profilesDir:profiles,channel:'msedge',headless:false,pollMs:100,settleMs:150}});},{plugin,profiles});
 await page.evaluate(()=>{const original=window.go.main.App;window.loginCalls=[];window.go.main.App=new Proxy(original,{get:(target,key)=>key==='Capture'?async(method,args)=>{if(method.startsWith('ai.accounts'))window.loginCalls.push({method,args});return target.Capture(method,args);}:target[key]});});
 await frame.getByRole('button',{name:/^(登录|Sign in)$/}).click();
 await page.locator('#ai-account-connection').waitFor();
 await page.getByRole('button',{name:/^(取消登录|Cancel sign-in)$/}).waitFor({timeout:30000});
 const end=Date.now()+30000;let ws;
 while(Date.now()<end&&!ws){for(const dir of await readdir(profiles).catch(()=>[])){try{const [port,p]=(await readFile(join(profiles,dir,'DevToolsActivePort'),'utf8')).trim().split(/\r?\n/);ws=`ws://127.0.0.1:${port}${p}`;}catch{}}if(!ws)await new Promise(r=>setTimeout(r,100));}
 assert(ws,'Login click never opened the dedicated browser');
 loginBrowser=await chromium.connectOverCDP(ws);const login=loginBrowser.contexts()[0].pages()[0];
 await login.waitForURL(url=>url.origin===site.url);
 assert.equal(new URL(login.url()).origin,site.url);
 const visible=execFileSync('powershell.exe',['-NoProfile','-Command',`Add-Type -TypeDefinition 'using System;using System.Runtime.InteropServices;public static class LoginVisibility{[DllImport("user32.dll")]public static extern bool IsWindowVisible(IntPtr h);}'; Get-CimInstance Win32_Process -Filter "Name = 'msedge.exe'" | Where-Object { $_.CommandLine.Contains('${profiles.replaceAll("'","''")}') } | ForEach-Object { $p=Get-Process -Id $_.ProcessId; if([LoginVisibility]::IsWindowVisible($p.MainWindowHandle)){ 'visible' } }`],{encoding:'utf8',windowsHide:true});
 assert(visible.includes('visible'),'Dedicated browser exists but its native window is hidden');
 await page.screenshot({path:join(output,'login-waiting.png')});await login.screenshot({path:join(output,'dedicated-browser.png')});
 site.state.autoLogin=true;await login.reload();
 await page.getByRole('status').filter({hasText:/登录成功|Signed in/}).waitFor({timeout:20000});
 await new Promise(resolve=>setTimeout(resolve,1200));
 assert(loginBrowser.isConnected(),'Worker exit closed the plugin keep-open browser');
 assert.equal(await login.evaluate(()=>document.readyState),'complete');
 await writeFile(join(output,'result.json'),JSON.stringify({errors,calls:await page.evaluate(()=>window.loginCalls),browser:login.url(),dedicatedBrowserSurvived:loginBrowser.isConnected()},null,2));
 assert.equal(errors.length,0);
 await page.getByRole('button',{name:/^(保存 AI 设置|Save AI settings)$/}).click();
 await page.getByRole('status').filter({hasText:/已保存|Saved/}).waitFor();
 await page.evaluate(()=>{void window.go.main.App.WindowCommand('close');}).catch(()=>{});
}catch(error){
 if(page){await page.screenshot({path:join(output,'failure.png')}).catch(()=>{});await writeFile(join(output,'state.json'),JSON.stringify(await page.evaluate(()=>({text:document.body.innerText,calls:window.loginCalls})),null,2));}
 await writeFile(join(output,'node-error.txt'),String(error)+'\n'+error.stack);process.exitCode=1;
}finally{
 if(loginBrowser){try{const session=await loginBrowser.newBrowserCDPSession();await session.send('Browser.close');}catch{}await loginBrowser.close().catch(()=>{});}
 await browser?.close();await site.close();
}
