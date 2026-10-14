// Exercise IFileOpenDialog in the actual shipped desktop process.
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2),run=promisify(execFile),selected=resolve(output,'中文项目');
await mkdir(selected,{recursive:true});
const helper=mode=>run('powershell.exe',['-NoProfile','-File',resolve(dirname(fileURLToPath(import.meta.url)),'native-profile-picker.ps1'),'-OwnedProcessId',process.env.WINONE_COMPONENTS_PID,'-Executable',resolve(output,'WinOne.exe'),'-Path',selected,'-Mode',mode,'-Folder'],{windowsHide:true,timeout:20000});
let browser,page;
try{
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(done=>setTimeout(done,100));}}
 if(!browser)throw Error('Owned WebView did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(c=>c.pages()).find(p=>p.url().includes('winone.localhost'));if(page)break;await new Promise(done=>setTimeout(done,100));}
 await page.getByRole('navigation').waitFor();
 await writeFile(resolve(output,'progress.txt'),'native cancel start');
 const [,cancelled]=await Promise.all([helper('cancel'),page.evaluate(()=>window.go.main.App.PickDirectory())]);
 await writeFile(resolve(output,'progress.txt'),'native cancel completed');
 if(cancelled!=='')throw Error('Cancelled folder picker returned a selected path');
 const [choice,path]=await Promise.all([helper('open'),page.evaluate(()=>window.go.main.App.PickDirectory())]);
 await writeFile(resolve(output,'progress.txt'),'native folder returned: '+path+'\n'+choice.stdout);
 if(path!==selected)throw Error('Folder picker returned a different directory: '+path);
 await page.getByRole('button',{name:'Chat',exact:true}).click();
 await page.getByRole('button',{name:'打开或新建项目',exact:true}).click();
 await page.getByRole('dialog').waitFor();
 await Promise.all([helper('open'),page.getByRole('dialog').getByRole('button',{name:'选择位置',exact:true}).click()]);
 await page.waitForFunction(()=>document.querySelector('#chat-project-name-open')?.value==='中文项目');
 const name=await page.getByRole('dialog').getByRole('textbox',{name:'项目名称',exact:true}).inputValue();
 if(name!=='中文项目')throw Error('Selected folder did not populate its actual basename: '+name);
 await page.screenshot({path:resolve(output,'project-folder.png')});
 await page.getByRole('dialog').getByRole('button',{name:'取消',exact:true}).click();
 await writeFile(resolve(output,'result.json'),JSON.stringify({nativeExplorerFolderPicker:true,cancelledEmpty:true,selectedPath: path,projectNameFromFolder:name},null,2));
 await run('powershell.exe',['-NoProfile','-File',resolve(dirname(fileURLToPath(import.meta.url)),'native-window-state.ps1'),'-OwnedProcessId',process.env.WINONE_COMPONENTS_PID,'-Executable',resolve(output,'WinOne.exe'),'-Action','exit'],{windowsHide:true,timeout:10000});
}catch(error){await writeFile(resolve(output,'node-error.txt'),String(error.stack||error));if(page){await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});await writeFile(resolve(output,'failure.txt'),await page.locator('body').innerText().catch(()=>''));}process.exitCode=1;}finally{await browser?.close().catch(()=>{});}

