// Real desktop -> Host -> embedded Magpie routing page. The runner supplies a
// private profile, so no user account, installation or configuration is read.
import {createRequire} from 'node:module';
import {writeFile} from 'node:fs/promises';
import {resolve,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,outputArg]=process.argv.slice(2),output=resolve(outputArg),run=promisify(execFile),testDirectory=dirname(fileURLToPath(import.meta.url));
if(new URL(endpoint).hostname!=='127.0.0.1'||!process.env.WINONE_COMPONENTS_PROFILE?.startsWith(output))throw Error('Use an isolated component runner');
let browser,page;
const results={};
try{
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(done=>setTimeout(done,100));}}
 if(!browser)throw Error('Owned WebView did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(candidate=>candidate.url().includes('winone.localhost'));if(page)break;await new Promise(done=>setTimeout(done,100));}
 if(!page)throw Error('Owned UI document missing');
 const errors=[];page.on('pageerror',error=>errors.push(String(error)));await page.getByRole('navigation').waitFor();
 // A local provider makes the saved group reference an enabled model. No
 // request reaches it; only the routing page's authenticated save is tested.
 await page.evaluate(async()=>{const api=window.go.main.App,configuration=JSON.parse(await api.Capture('ai.configuration.get','null'));configuration.providers=[{id:'native-routing',name:'路由验证服务',authMode:'api-key',protocol:'chat',baseUrl:'http://127.0.0.1:1/v1',models:['model'],hasKey:false}];configuration.tasks={};await api.Capture('ai.configuration.save',JSON.stringify({configuration}));});
 // Let the initial Host snapshot apply its default page before navigating;
 // otherwise the late snapshot can override a very early click.
 await page.getByText('选择需要的工具，关闭后保留设置。',{exact:true}).waitFor({timeout:30000});await page.waitForTimeout(800);
 // The component fixture keeps module workers out; navigation labels alone
 // still exercise the real page, CSP and Host RPC path.
 await page.getByRole('button',{name:'AI 服务',exact:true}).click();
 await page.waitForTimeout(200);
 await writeFile(resolve(output,'after-click.txt'),await page.locator('body').innerText(),'utf8');
 await page.getByRole('tab',{name:'自定义供应商',exact:true}).waitFor({timeout:20000});
 await page.waitForTimeout(1500);
 await writeFile(resolve(output,'after-nav.txt'),await page.locator('body').innerText(),'utf8');
 await page.screenshot({path:resolve(output,'after-nav.png')});
 await page.getByRole('tab',{name:'路由',exact:true}).click();
 const frame=page.frameLocator('iframe[title="路由"]');await frame.locator('#rtNewGroup').waitFor({timeout:20000});
 // The theme/language message is posted after load; allow that handshake.
 await frame.getByText('新建组',{exact:true}).first().waitFor({timeout:15000});
 // Save through the page itself, then reopen the tab and verify persistence.
 const child=page.frames().find(candidate=>candidate.url().startsWith('http://127.0.0.1:'));
 const status=await child.evaluate(async()=>{const response=await fetch('/api/groups/save',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:'native-check',name:'Native check',members:['native-routing/model'],routing:'order'})});return response.status;});
 if(status!==200)throw Error('Embedded routing save failed with '+status);
 // Reopen the tab after a save: the Host restarts the routing process with the
 // new configuration, and the embedded page must obtain a fresh capability.
 await page.getByRole('tab',{name:'用量统计',exact:true}).click();await page.getByRole('tab',{name:'路由',exact:true}).click();
 const reopened=page.frameLocator('iframe[title="路由"]');await reopened.locator('#rtNewGroup').waitFor({timeout:30000});
 await reopened.getByText('Native check',{exact:true}).first().waitFor({timeout:30000});
 if(errors.length)throw Error(errors.join('\n'));
 results.embeddedRoutingEditor=true;results.language=true;results.savedGroupReopened=true;results.errors=[];
 await page.screenshot({path:resolve(output,'ai-routing.png')});await writeFile(resolve(output,'result.json'),JSON.stringify(results,null,2),'utf8');
 await run('powershell.exe',['-NoProfile','-File',resolve(testDirectory,'native-window-state.ps1'),'-OwnedProcessId',process.env.WINONE_COMPONENTS_PID,'-Executable',resolve(output,'WinOne.exe'),'-Action','exit'],{windowsHide:true,timeout:10000});
}catch(error){await writeFile(resolve(output,'node-error.txt'),String(error.stack||error),'utf8');if(page){await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});await writeFile(resolve(output,'failure.txt'),await page.locator('body').innerText().catch(()=>''),'utf8');}process.exitCode=1;}
finally{await browser?.close().catch(()=>{});}
