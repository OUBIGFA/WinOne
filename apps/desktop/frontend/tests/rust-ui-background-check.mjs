// Real component UI and Host on the runner's private desktop. Never send
// global keyboard input or inspect unrelated browser/process state.
import {createRequire} from 'node:module';
import {writeFile} from 'node:fs/promises';
import {resolve,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
import {execFileSync} from 'node:child_process';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2);
const executable=resolve(output,'WinOne.exe');
const mode=process.env.WINONE_BACKGROUND_EXIT_MODE??'release';
const probe=action=>JSON.parse(execFileSync('powershell.exe',['-NoProfile','-File',resolve(dirname(fileURLToPath(import.meta.url)),'native-window-state.ps1'),'-OwnedProcessId',process.env.WINONE_COMPONENTS_PID,'-Executable',executable,'-Action',action],{encoding:'utf8',windowsHide:true,timeout:10000}));
const reopen=()=>execFileSync(executable,['--pipe',process.env.WINONE_COMPONENTS_PIPE,'--data-dir',process.env.WINONE_COMPONENTS_PROFILE,'--standard-session'],{windowsHide:true,timeout:10000});
const wait=ms=>new Promise(resolve=>setTimeout(resolve,ms));
let browser,page;
try{
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await wait(100);}}
 if(!browser)throw Error('Isolated WebView2 did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(page=>page.url().includes('winone.localhost'));if(page)break;await wait(100);}
 if(!page)throw Error('Component UI document did not load');
 await page.getByRole('navigation').waitFor();
 const snapshot=await page.evaluate(()=>window.go.main.App.Snapshot());
 await writeFile(resolve(output,'initial-snapshot.json'),JSON.stringify(snapshot,null,2));
 const errors=[];page.on('pageerror',error=>errors.push(String(error)));
 await page.getByRole('button',{name:'设置',exact:true}).click();
 const toggle=page.getByRole('switch',{name:'保持界面后台运行',exact:true});
 if(await toggle.getAttribute('aria-checked')!=='true')throw Error('Background retention is not on by default');
 const marker='retention-'+Date.now();await page.evaluate(value=>window.__retentionMarker=value,marker);
 const initial=probe('state');
 await page.evaluate(()=>window.go.main.App.WindowCommand('close'));
 let hidden=probe('state');if(!hidden.alive||hidden.visible||hidden.window!==initial.window)throw Error('Close did not retain the hidden native window');
 const started=Date.now();reopen();
 const visible=probe('state');if(!visible.alive||!visible.visible||visible.window!==initial.window)throw Error('Reopen replaced or failed to reveal the existing window');
 if(await page.evaluate(()=>window.__retentionMarker)!==marker)throw Error('Ordinary close reloaded the WebView document');
 const reopenMilliseconds=Date.now()-started;
 const input=page.getByRole('textbox',{name:'打开 WinOne',exact:true});
 const original=await input.inputValue();
 await input.click();await page.getByText('请按下组合键，Esc 取消，Delete 清除。',{exact:true}).waitFor();
 await page.keyboard.press('Control+Alt+F9');
 await page.waitForFunction(()=>[...document.querySelectorAll('button')].some(button=>button.textContent==='保存快捷键'&&!button.disabled));
 probe('exit');await page.getByRole('dialog').waitFor();
 await page.getByRole('button',{name:'继续编辑',exact:true}).click();
 if(await input.inputValue()!=='Ctrl+Alt+F9')throw Error('Cancelling application exit lost the draft');
 probe('close');await page.getByRole('dialog').waitFor();
 await page.getByRole('button',{name:'放弃并关闭',exact:true}).click();
 await wait(200);
 hidden=probe('state');if(!hidden.alive||hidden.visible)throw Error('Cancelling full exit did not restore normal close-to-background');
 reopen();
 await page.getByRole('navigation').waitFor();
 await page.getByRole('button',{name:'设置',exact:true}).click();
 if(await page.getByRole('textbox',{name:'打开 WinOne',exact:true}).inputValue()!==original)throw Error('Discard and close retained the abandoned draft');
 if(await page.getByRole('dialog').count())throw Error('Discard left an unsaved-change prompt behind');
 await page.screenshot({path:resolve(output,'background-settings.png')});
 if(mode==='release'){
  await page.getByRole('switch',{name:'保持界面后台运行',exact:true}).click();
  await page.waitForFunction(()=>[...document.querySelectorAll('[role=switch]')].some(element=>element.getAttribute('aria-labelledby')==='winone-keep-ui-label'&&element.getAttribute('aria-checked')==='false'));
  const snapshot=await page.evaluate(()=>window.go.main.App.Snapshot());
  if(snapshot.settings.startup.keepUIRunning!==false)throw Error('Disabling retention was not persisted');
  await page.evaluate(()=>{void window.go.main.App.WindowCommand('close');}).catch(()=>{});
 }else if(mode==='application'){probe('exit');}else{throw Error('Unknown exit scenario');}
 await writeFile(resolve(output,'result.json'),JSON.stringify({mode,initial,hidden,visible,reopenMilliseconds,defaultEnabled:true,sameProcessAndWebView:true,unsavedCancelPreserved:true,discardRemovedDraft:true,errors},null,2));
 if(errors.length)throw Error(errors.join('\n'));
}catch(error){
 await writeFile(resolve(output,'node-error.txt'),String(error.stack||error));
 if(page){await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});await writeFile(resolve(output,'failure.txt'),await page.locator('body').innerText().catch(()=>''));}
 process.exitCode=1;
}finally{if(browser)await browser.close().catch(()=>{});}
