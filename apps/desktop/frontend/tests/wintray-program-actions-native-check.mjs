// Native WinOne/Host/WinTray integration. Vite supplies only current UI assets;
// the fixture program and profile live under the runner's ignored directory.
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {pathToFileURL} from 'node:url';
import {execFileSync} from 'node:child_process';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,folder]=process.argv.slice(2),output=resolve(folder),script=join(output,'managed fixture.cmd');
if(!process.env.WINONE_COMPONENTS_PROFILE?.startsWith(output))throw Error('Use the isolated native runner');
await writeFile(script,'@echo off\r\necho Native program window fixture\r\n:wait\r\nif exist "%~dp0stop.fixture" exit /b\r\nping -n 2 127.0.0.1 >nul\r\ngoto wait\r\n');
let browser,page,vite;const errors=[],evidence=[];
try{
 const until=Date.now()+30000;while(Date.now()<until){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(done=>setTimeout(done,100));}}
 assert(browser);page=browser.contexts().flatMap(context=>context.pages()).find(page=>page.url().includes('winone.localhost'));assert(page);page.setDefaultTimeout(20000);page.on('pageerror',error=>errors.push(error.message));
 await page.locator('.app-navigation').waitFor();await page.evaluate(async()=>{await document.fonts.ready;});
 const frontend=resolve('apps/desktop/frontend'),{createServer}=await import(pathToFileURL(join(frontend,'node_modules/vite/dist/node/index.js')).href);
 vite=await createServer({root:frontend,cacheDir:join(output,'vite-cache'),server:{host:'127.0.0.1',port:0,hmr:false},logLevel:'warn'});await vite.listen();
 await page.route('http://winone.localhost/**',async route=>{const url=new URL(route.request().url()),path=['/','/index.html'].includes(url.pathname)?'/tests/settings-native-preview.html':url.pathname;const response=await fetch(`http://127.0.0.1:${vite.httpServer.address().port}${path}${url.search}`);await route.fulfill({status:response.status,contentType:response.headers.get('content-type')||'application/octet-stream',body:Buffer.from(await response.arrayBuffer())});});
 await page.reload({waitUntil:'domcontentloaded'});await page.locator('.app-navigation').waitFor();
 const call=(method,params)=>page.evaluate(async({method,params})=>JSON.parse(await window.go.main.App.WinTray(method,JSON.stringify(params))),{method,params});
 let saved=await call('settings.get',null);saved.settings.runAtLogon=false;
 saved.settings.managedApps=[{id:'fixture',name:'Window fixture',exePath:script,args:'',runOnStartup:false,collectTrayIcon:false,launchViaLogonTask:false,launchHiddenInBackground:true,trayBehavior:{autoMinimizeAndHideOnLaunch:false,closeDelaySeconds:0},schedule:{enabled:false,startDelayMinutes:0,autoExitMinutes:0,frequencyEnabled:false,frequencyDays:1,frequencyRuns:1}}];
 let job=await call('settings.save',{revision:saved.revision,settings:saved.settings});while(job.state==='running'){await new Promise(done=>setTimeout(done,100));job=await call('jobs.get',{id:job.id});}assert.equal(job.state,'completed');
 await page.locator('.app-navigation [data-page-id=wintray]').click();await page.getByRole('button',{name:'立即启动',exact:true}).click();await page.getByRole('button',{name:'关闭程序',exact:true}).waitFor();
 const initial=await call('program.state',{id:'fixture'});assert(initial.running&&initial.hasWindow&&!initial.visible);evidence.push({initial});
 // Repeated actions must target the same native program, not restart it.
 for(const theme of ['light','dark']){
  // Change only appearance for the screenshot; no global configuration save
  // is needed to test the actions and no saved settings are replaced.
  execFileSync('powershell.exe',['-NoProfile','-File',resolve('apps/desktop/frontend/tests/native-window-state.ps1'),'-OwnedProcessId',process.env.WINONE_COMPONENTS_PID,'-Executable',join(output,'WinOne.exe'),'-Action','resize','-Width',theme==='dark'?'620':'1280','-Height','940'],{windowsHide:true,timeout:10000});
  await page.evaluate(theme=>{document.documentElement.dataset.theme=theme;},theme);
  await page.getByRole('button',{name:'显示窗口',exact:true}).click();await page.getByRole('button',{name:'隐藏窗口',exact:true}).waitFor();assert((await call('program.state',{id:'fixture'})).visible);
  await page.getByRole('button',{name:'隐藏窗口',exact:true}).click();await page.getByRole('button',{name:'显示窗口',exact:true}).waitFor();assert.equal((await call('program.state',{id:'fixture'})).visible,false);
  await page.getByRole('button',{name:'关闭程序',exact:true}).click();await page.getByRole('dialog').waitFor();
  await page.getByRole('dialog').evaluate(async root=>{await Promise.all(root.getAnimations({subtree:true}).map(animation=>animation.finished.catch(()=>{})));});
  assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'Page has horizontal overflow');
  assert(await page.getByRole('dialog').evaluate(root=>root.scrollWidth<=root.clientWidth+1),'Confirmation has horizontal overflow');
  await page.mouse.move(0,0);await page.screenshot({path:join(output,`program-actions-${theme}.png`)});
  await page.getByRole('button',{name:'取消',exact:true}).click();assert((await call('program.state',{id:'fixture'})).running);evidence.push({theme,showHide:true,cancelKeptRunning:true});
 }
 await page.getByRole('button',{name:'关闭程序',exact:true}).click();await page.getByRole('button',{name:'确认关闭',exact:true}).click();await page.getByRole('button',{name:'立即启动',exact:true}).waitFor();assert.equal((await call('program.state',{id:'fixture'})).running,false);
 assert.deepEqual(errors,[]);await writeFile(join(output,'result.json'),JSON.stringify({native:true,liveUI:true,evidence,closed:true,errors},null,2));
 execFileSync('powershell.exe',['-NoProfile','-File',resolve('apps/desktop/frontend/tests/native-window-state.ps1'),'-OwnedProcessId',process.env.WINONE_COMPONENTS_PID,'-Executable',join(output,'WinOne.exe'),'-Action','exit'],{windowsHide:true,timeout:10000});
}catch(error){await page?.screenshot({path:join(output,'failure.png')}).catch(()=>{});await writeFile(join(output,'node-error.txt'),String(error.stack||error));await writeFile(join(output,'failure.json'),JSON.stringify({evidence,errors},null,2));process.exitCode=1;}
finally{
 await writeFile(join(output,'stop.fixture'),'stop');
 // Even a failed assertion may leave the fixture console alive. Only the
 // explicitly registered fixture is eligible; no user programs are touched.
 await page?.evaluate(async()=>{try{let job=JSON.parse(await window.go.main.App.WinTray('program.stop',JSON.stringify({id:'fixture',confirmed:true})));while(job.state==='running'){await new Promise(done=>setTimeout(done,100));job=JSON.parse(await window.go.main.App.WinTray('jobs.get',JSON.stringify({id:job.id})));}}catch{}}).catch(()=>{});
 await browser?.close();await vite?.close();
}
