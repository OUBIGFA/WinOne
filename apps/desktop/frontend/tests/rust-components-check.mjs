// Attach only to this invocation's isolated WebView2, never a user browser.
import {createRequire} from 'node:module';
import {writeFile,readFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import {execFileSync} from 'node:child_process';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2);
let browser;
const deadline=Date.now()+30000;
while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(resolve=>setTimeout(resolve,100));}}
if(!browser)throw Error('Isolated WebView2 did not start');
let page;
try{
 while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(page=>page.url().includes('winone.localhost'));if(page)break;await new Promise(resolve=>setTimeout(resolve,100));}
 if(!page)throw Error('Current frontend document did not load');
 const errors=[];page.on('pageerror',error=>errors.push(String(error)));
 await page.getByRole('navigation').waitFor({timeout:20000});
 const snapshot=await page.evaluate(()=>window.go.main.App.Snapshot());
 if(!snapshot.settings)throw Error('Rust bridge did not return Host settings');
 await page.waitForFunction(()=>document.querySelector('nav [aria-current="page"]')?.textContent==='首页');
 await page.waitForFunction(()=>performance.getEntriesByName('winone-ui-ready').length>0);
 // Home must expose every capture action before the startup marker fires.
 await page.getByText('截图识别并复制',{exact:true}).waitFor();
 await page.getByText('截图翻译',{exact:true}).waitFor();
 await page.screenshot({path:resolve(output,'home-shortcuts.png')});
 await page.getByRole('button',{name:'截图',exact:true}).click();
 await page.getByRole('tab',{name:'OCR 与翻译',exact:true}).click();
 await page.getByText('内置 Google 翻译',{exact:true}).waitFor();
 await page.getByRole('combobox',{name:'翻译引擎'}).click();
 await page.getByRole('option',{name:'自定义 LLM',exact:true}).click();
 await page.getByLabel('API 基础地址',{exact:true}).fill('https://example.com/v1');
 await page.getByLabel('模型名称',{exact:true}).fill('integration-model');
 await page.getByLabel('API 密钥',{exact:true}).fill('integration-test-key');
 await page.getByRole('tab',{name:'诊断日志',exact:true}).click();
 await page.getByRole('tab',{name:'OCR 与翻译',exact:true}).click();
 if(await page.getByLabel('模型名称',{exact:true}).inputValue()!=='integration-model')throw Error('Changing capture tabs discarded AI preferences');
 await page.getByRole('button',{name:'保存翻译设置',exact:true}).click();
 await page.getByText('已保存',{exact:true}).waitFor();
 if(await page.getByLabel('API 密钥',{exact:true}).inputValue()!=='')throw Error('The saved key remained in the settings form');
 await page.screenshot({path:resolve(output,'ai-settings.png')});
 await page.getByRole('combobox',{name:'登录方式'}).click();
 await page.getByRole('option',{name:'智能体账号',exact:true}).click();
 await page.getByText('尚未登录',{exact:true}).waitFor();
 await page.getByRole('button',{name:'登录',exact:true}).waitFor();
 await page.waitForFunction(()=>Array.from(document.querySelectorAll('button')).some(button=>button.textContent==='登录'&&!button.disabled));
 await page.screenshot({path:resolve(output,'agent-accounts.png')});
 await page.getByRole('button',{name:'撤销修改',exact:true}).click();
 const script=resolve(output,'launch-probe.cmd');
 await writeFile(script,'@echo off\r\necho launched>"%~dp0launch-result.txt"\r\n');
 // This process has a fresh, isolated profile. The fixture entries never run
 // or collect real windows; their only purpose is exercising the original UI.
 await page.evaluate(async script=>{
   const api=window.go.main.App;
   const current=JSON.parse(await api.WinTray('settings.get','null'));
   current.settings.managedApps=Array.from({length:80},(_,i)=>({id:`fixture-${i}`,name:`程序 ${i+1}`,exePath:`C:\\Missing-UI-Fixture\\app${i}.exe`,args:'',runOnStartup:false,collectTrayIcon:false,launchViaLogonTask:false,launchHiddenInBackground:false,trayBehavior:{autoMinimizeAndHideOnLaunch:false,closeDelaySeconds:0},schedule:{enabled:false,startDelayMinutes:0,autoExitMinutes:0,frequencyEnabled:false,frequencyDays:1,frequencyRuns:1}}));
   current.settings.managedApps[0].exePath=script;current.settings.managedApps[0].launchHiddenInBackground=true;
   let job=JSON.parse(await api.WinTray('settings.save',JSON.stringify({revision:current.revision,settings:current.settings})));
   const deadline=Date.now()+10000;
   while(job.state==='running'&&Date.now()<deadline){await new Promise(resolve=>setTimeout(resolve,100));job=JSON.parse(await api.WinTray('jobs.get',JSON.stringify({id:job.id})));}
   if(job.state!=='completed')throw Error('Fixture settings save failed: '+JSON.stringify(job));
 },script);
 await page.getByRole('button',{name:'程序管理',exact:true}).click();
 await page.getByRole('tab',{name:'程序列表',exact:true}).waitFor();
 await page.getByRole('button',{name:'添加程序',exact:true}).waitFor();
 await page.locator('.wintray-program-row').last().waitFor({state:'attached'});
 await page.screenshot({path:resolve(output,'programs.png')});
 const layout=await page.evaluate(()=>({document:document.documentElement.scrollHeight,viewport:innerHeight,components:!!document.querySelector('[data-slot="tabs-list"]'),icons:document.querySelectorAll('svg.lucide').length}));
 if(layout.document>layout.viewport+1||!layout.components||layout.icons===0)throw Error('Original components or fixed page layout are missing');
 const list=page.locator('.wintray-program-list');
 const row=page.locator('.wintray-program-row').first();
 if(await row.getByRole('combobox').count()!==1)throw Error('The current row is missing its sort-position dropdown');
 await row.getByRole('combobox').click();await page.getByRole('option',{name:'3',exact:true}).click();
 await page.getByRole('tab',{name:'更多设置',exact:true}).click();
 await page.getByRole('textbox',{name:'名称',exact:true}).fill('迁移验证程序');
 await page.getByRole('button',{name:'设置',exact:true}).click();
 await page.getByRole('dialog').waitFor();await page.getByRole('button',{name:'继续编辑',exact:true}).click();
 if(await page.getByRole('textbox',{name:'名称',exact:true}).inputValue()!=='迁移验证程序')throw Error('Navigation discarded unsaved program edits');
 await page.getByRole('button',{name:'保存修改',exact:true}).click();
 await page.getByText('已保存',{exact:true}).waitFor();
 const programs=await page.evaluate(async()=>JSON.parse(await window.go.main.App.WinTray('settings.get','null')).settings.managedApps);
 if(programs[2].id!=='fixture-0'||programs[2].name!=='迁移验证程序')throw Error('Program name or dropdown order was not saved');
 await page.getByRole('button',{name:'立即启动',exact:true}).click();
 const launchDeadline=Date.now()+10000;let launched=false;
 while(Date.now()<launchDeadline){try{launched=(await readFile(resolve(output,'launch-result.txt'),'utf8')).includes('launched');if(launched)break;}catch{}await new Promise(resolve=>setTimeout(resolve,100));}
 if(!launched)throw Error('The UI did not launch its own fixture script');
 const detailBefore=await page.locator('.wintray-program-detail').boundingBox();
 await list.evaluate(element=>{element.scrollTop=element.scrollHeight;});
 const detailAfter=await page.locator('.wintray-program-detail').boundingBox();
 if(await list.evaluate(element=>element.scrollTop)<=0||Math.abs(detailBefore.y-detailAfter.y)>1)throw Error('List scrolling moved the detail pane');
 await page.getByRole('button',{name:'设置',exact:true}).click();
 await page.screenshot({path:resolve(output,'settings.png')});
 execFileSync(resolve(output,'WinOne.exe'),['--pipe',process.env.WINONE_COMPONENTS_PIPE,'--data-dir',process.env.WINONE_COMPONENTS_PROFILE,'--open-settings','--standard-session'],{timeout:10000,windowsHide:true});
 await page.waitForFunction(()=>document.querySelector('nav [aria-current="page"]')?.textContent==='首页');
 await writeFile(resolve(output,'result.json'),JSON.stringify({layout,errors,settings:snapshot.settings,programSave:true,orderDropdown:true,unsavedProtection:true,ownedScriptLaunched:launched,defaultHome:true,trayReopenHome:true},null,2));
 if(errors.length)throw Error(errors.join('\n'));
 await page.evaluate(()=>{void window.go.main.App.WindowCommand('close');}).catch(error=>{if(!String(error).includes('closed'))throw error;});
 console.log('PASS: original UI components, icons, navigation, real Host snapshot and normal close');
}catch(error){
 await writeFile(resolve(output,'node-error.txt'),String(error.stack||error));
 if(page){await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});await writeFile(resolve(output,'failure.txt'),await page.locator('body').innerText().catch(()=>String(error)));}
 throw error;
}finally{await browser.close();}
