// Real Rust settings UI, on the runner's isolated Windows desktop and profile.
import {createRequire} from 'node:module';
import {writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2);let browser,page;
try{
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(r=>setTimeout(r,100));}}
 if(!browser)throw Error('Isolated desktop did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(c=>c.pages()).find(p=>p.url().includes('winone.localhost'));if(page)break;await new Promise(r=>setTimeout(r,100));}
 if(!page)throw Error('Settings document not found');
 await page.getByRole('navigation').waitFor();
 const original=await page.evaluate(async()=>JSON.parse(await window.go.main.App.Selection('settings.get','null')));
 const nav=await page.locator('nav button').allTextContents();
 if(nav.indexOf('快捷助手')!==nav.indexOf('划词助手')+1)throw Error('Quick assistant navigation position changed');
 await page.getByRole('button',{name:'快捷助手',exact:true}).click();
 await page.getByRole('button',{name:'添加自定义功能',exact:true}).waitFor();
 if(await page.getByRole('button',{name:'编辑 复制',exact:true}).count())throw Error('Copy remains in quick action settings');
 const snapshot=await page.evaluate(()=>window.go.main.App.Snapshot());
 if(snapshot.settings.quickAssistantShortcut!=='Ctrl+Shift+Q'||snapshot.settings.translateShortcut!=='Alt+Shift+A'||snapshot.settings.ocrShortcut!=='Shift+C')throw Error('Wrong defaults');
 await page.getByRole('combobox',{name:'位置',exact:true}).first().click();await page.getByRole('option',{name:'2',exact:true}).click();
 await page.getByRole('button',{name:'添加自定义功能',exact:true}).click();
 await page.getByLabel('功能名称',{exact:true}).fill('邮件回复');await page.getByLabel('AI 指令',{exact:true}).fill('写一封简短的邮件回复。');
 await page.getByRole('button',{name:'划词助手',exact:true}).click();await page.getByRole('dialog').waitFor();await page.getByRole('button',{name:'继续编辑',exact:true}).click();
 if(await page.getByLabel('功能名称',{exact:true}).inputValue()!=='邮件回复')throw Error('Unsaved action lost');
 await page.getByRole('button',{name:'保存设置',exact:true}).click();await page.getByText('已保存',{exact:true}).waitFor();
 const saved=await page.evaluate(async()=>({quick:JSON.parse(await window.go.main.App.QuickAssistant('settings.get','null')),selection:JSON.parse(await window.go.main.App.Selection('settings.get','null'))}));
 if(saved.quick.actions[0].id!=='explain'||saved.quick.actions.at(-1).name!=='邮件回复'||JSON.stringify(saved.selection.actions)!==JSON.stringify(original.actions))throw Error('Independent actions not preserved');
 const checks=[];
 for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark'])for(const width of [620,1040]){
  await page.evaluate(async({theme,language})=>{const api=window.go.main.App;const snapshot=await api.Snapshot();await api.SaveSettings({...snapshot.settings,theme,language});},{theme,language});
  await page.reload();await page.getByRole('navigation').waitFor();
  await page.setViewportSize({width,height:780});
  const zh=language==='zh-CN';await page.getByRole('button',{name:zh?'快捷助手':'Quick assistant',exact:true}).click();
  await page.getByRole('button',{name:zh?'添加自定义功能':'Add custom action',exact:true}).waitFor();
  const overflow=await page.evaluate(()=>[...document.querySelectorAll('html,body,[data-slot="page-scroll"],.module-section,.selection-action-row')].filter(e=>e.clientWidth&&e.scrollWidth>e.clientWidth+1).map(e=>e.className||e.tagName));
  if(overflow.length)throw Error('Horizontal overflow: '+JSON.stringify(overflow));
  await page.screenshot({path:resolve(output,language+'-'+theme+'-'+width+'.png')});checks.push({language,theme,width,overflow});
 }
 await writeFile(resolve(output,'result.json'),JSON.stringify({navigation:true,defaults:true,independentActions:true,unsavedProtection:true,checks},null,2));
 await page.evaluate(()=>{void window.go.main.App.WindowCommand('close');}).catch(()=>{});
}catch(error){await writeFile(resolve(output,'node-error.txt'),String(error.stack||error));if(page)await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});process.exitCode=1;}
finally{if(browser)await browser.close();}

