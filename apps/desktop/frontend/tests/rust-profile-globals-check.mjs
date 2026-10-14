// Real archive export, preview and atomic import through the packaged UI/Host.
// The native file dialog selects only this runner-owned archive location.
import {createRequire} from 'node:module';
import {writeFile} from 'node:fs/promises';
import {resolve,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2);
const run=promisify(execFile);
const choose=mode=>run('powershell.exe',['-NoProfile','-File',resolve(dirname(fileURLToPath(import.meta.url)),'native-profile-picker.ps1'),'-OwnedProcessId',process.env.WINONE_COMPONENTS_PID,'-Executable',resolve(output,'WinOne.exe'),'-Path',resolve(output,'global-preferences.winone'),'-Mode',mode],{windowsHide:true,timeout:20000});
const wait=ms=>new Promise(resolve=>setTimeout(resolve,ms));
let browser,page;
try{
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await wait(100);}}
 if(!browser)throw Error('Isolated WebView2 did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(page=>page.url().includes('winone.localhost'));if(page)break;await wait(100);}
 await page.getByRole('navigation').waitFor();
 const path=resolve(output,'global-preferences.winone');
 await page.evaluate(async()=>{
  const api=window.go.main.App;let snapshot=await api.Snapshot();
  await api.SaveSettings({...snapshot.settings,defaultPage:'chat',openShortcut:'Ctrl+Alt+F9',startup:{...snapshot.settings.startup,keepUIRunning:false}});
 });
 const [,exported]=await Promise.all([choose('save'),page.evaluate(()=>window.go.main.App.ExportProfile())]);
 if(exported!==path)throw Error('Native export selected a different destination');
 await page.evaluate(async()=>{
  const api=window.go.main.App;
  const snapshot=await api.Snapshot();await api.SaveSettings({...snapshot.settings,defaultPage:'overview',openShortcut:'Alt+A',startup:{...snapshot.settings.startup,keepUIRunning:true}});
 });
 await page.getByRole('button',{name:'设置',exact:true}).click();
 await Promise.all([choose('open'),page.getByRole('button',{name:'导入配置',exact:true}).click()]);
 await page.getByText('默认页面：Chat',{exact:true}).waitFor();
 await page.getByText('保持界面后台运行：关闭',{exact:true}).waitFor();
 await page.getByText('打开 WinOne：Ctrl+Alt+F9',{exact:true}).waitFor();
 for(const name of ['截图助手','置顶功能','程序管理','划词助手','快捷助手','鼠标手势']){
  const checkbox=page.getByRole('checkbox',{name,exact:true});if(await checkbox.getAttribute('aria-checked')==='true')await checkbox.click();
 }
 const review=page.locator('.profile-import__review');await review.scrollIntoViewIfNeeded();
 await page.screenshot({path:resolve(output,'global-preferences-preview.png')});
 await page.getByRole('button',{name:'确认导入',exact:true}).click();
 await page.getByText('配置已导入',{exact:true}).waitFor();
 const result=await page.evaluate(()=>window.go.main.App.Snapshot());
 if(result.settings.defaultPage!=='chat'||result.settings.openShortcut!=='Ctrl+Alt+F9'||result.settings.startup.keepUIRunning!==false||result.administrator!==false)throw Error('Archive import did not restore the reviewed global preferences');
 await writeFile(resolve(output,'result.json'),JSON.stringify({archive:path,exportPreviewImport:true,onlySelectedGlobalPreferencesApplied:true,settings:result.settings,administrator:result.administrator},null,2));
 await page.evaluate(()=>{void window.go.main.App.WindowCommand('close');}).catch(error=>{if(!String(error).includes('closed'))throw error;});
}catch(error){await writeFile(resolve(output,'node-error.txt'),String(error.stack||error));if(page){await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});await writeFile(resolve(output,'failure.txt'),await page.locator('body').innerText().catch(()=>''));}process.exitCode=1;}finally{if(browser)await browser.close().catch(()=>{});}
