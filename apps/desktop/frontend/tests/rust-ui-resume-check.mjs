// Real HWND pixel regression for close/minimize -> re-open. Run only through
// build/test-desktop-components.ps1, which owns the executable/profile/process.
import {createRequire} from 'node:module';
import {writeFile} from 'node:fs/promises';
import {resolve,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
import {execFileSync} from 'node:child_process';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2);
let browser,page;
try{
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(r=>setTimeout(r,100));}}
 assert.ok(browser,'Isolated browser connection missing');
 while(Date.now()<deadline){page=browser.contexts().flatMap(c=>c.pages()).find(p=>p.url().includes('winone.localhost'));if(page)break;await new Promise(r=>setTimeout(r,100));}
 assert.ok(page,'Native settings document missing');
 const errors=[];page.on('pageerror',error=>errors.push(String(error)));
 await page.locator('.app-navigation').waitFor({timeout:30000});
 const token=String(Date.now());
 await page.evaluate(async token=>{
  const api=window.go.main.App,snapshot=await api.Snapshot();
  if(!snapshot.settings.startup?.keepUIRunning)await api.SaveSettings({...snapshot.settings,startup:{...snapshot.settings.startup,keepUIRunning:true}});
  await document.fonts.ready;
  await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);
  window.__resumeIdentity=token;
 },token);
 const native=resolve(dirname(fileURLToPath(import.meta.url)),'native-window-resume.ps1');
 const samples=JSON.parse(execFileSync('powershell.exe',['-NoProfile','-File',native,'-OwnedProcessId',process.env.WINONE_COMPONENTS_PID,'-Executable',resolve(output,'WinOne.exe'),'-Pipe',process.env.WINONE_COMPONENTS_PIPE,'-Profile',process.env.WINONE_COMPONENTS_PROFILE,'-OutputDirectory',resolve(output,'native-frames')],{encoding:'utf8',windowsHide:true,timeout:45000}));
 assert.equal(samples.length,6);
 assert.equal(await page.evaluate(()=>window.__resumeIdentity),token,'The UI document was reloaded during re-open');
 const viewport=await page.evaluate(()=>({width:innerWidth,height:innerHeight,visibility:document.visibilityState}));
 assert.ok(viewport.width>=800&&viewport.height>=560,'The restored viewport collapsed');
 assert.equal(viewport.visibility,'visible');
 assert.deepEqual(errors,[]);
 await writeFile(resolve(output,'result.json'),JSON.stringify({nativePixelsVerified:true,sameProcessAndDocument:true,viewport,samples,errors},null,2));
 await page.evaluate(async()=>{const api=window.go.main.App,s=await api.Snapshot();await api.SaveSettings({...s.settings,startup:{...s.settings.startup,keepUIRunning:false}});void api.WindowCommand('close');}).catch(error=>{if(!String(error).includes('closed'))throw error;});
}catch(error){await writeFile(resolve(output,'node-error.txt'),String(error.stack||error));process.exitCode=1;}
finally{if(browser)await browser.close().catch(()=>{});}
