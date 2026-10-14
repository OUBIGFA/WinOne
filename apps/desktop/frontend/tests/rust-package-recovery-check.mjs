// Only runs on the test runner's isolated desktop/profile. The signed fixture
// is deliberately damaged before launch and repaired from an owned copy here.
import {createRequire} from 'node:module';
import {readFile,writeFile,copyFile} from 'node:fs/promises';
import {resolve} from 'node:path';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2);let browser,page;
try {
 const deadline=Date.now()+25000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(r=>setTimeout(r,100));}}
 if(!browser)throw Error('Isolated WebView2 did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(c=>c.pages()).find(p=>p.url().includes('winone.localhost'));if(page)break;await new Promise(r=>setTimeout(r,100));}
 if(!page)throw Error('Settings document missing');
 const errors=[];page.on('pageerror',error=>errors.push(String(error)));
 const layouts=[];
 for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark'])for(const width of [620,1280]){
  await page.getByRole('navigation').waitFor();
  await page.evaluate(async({language,theme})=>{const api=window.go.main.App;const current=await api.Snapshot();await api.SaveSettings({...current.settings,language,theme});},{language,theme});
  await page.setViewportSize({width,height:900});await page.reload();
  const button=page.getByRole('button',{name:language==='zh-CN'?'重试加载':'Retry loading',exact:true});await button.waitFor();
  const card=page.getByRole('listitem').filter({has:page.locator('#module-example\\.tool-name')});
  await card.scrollIntoViewIfNeeded();
  const overflow=await page.locator('[data-slot="page-scroll"]').evaluate(node=>node.scrollWidth>node.clientWidth+1);
  if(overflow)throw Error('Fault card overflowed '+language+' '+width);
  await page.screenshot({path:resolve(output,`fault-${language}-${theme}-${width}.png`)});layouts.push({language,theme,width,overflow});
 }
 await page.evaluate(async()=>{const api=window.go.main.App;const current=await api.Snapshot();await api.SaveSettings({...current.settings,language:'zh-CN',theme:'dark'});});await page.setViewportSize({width:1280,height:900});await page.reload();
 const retry=page.getByRole('button',{name:'重试加载',exact:true});await retry.waitFor();await retry.scrollIntoViewIfNeeded();
 const toggle=page.getByRole('switch',{name:'example.tool',exact:true});if(await toggle.isEnabled())throw Error('Faulted module switch remained editable');
 await page.getByText('查看原因',{exact:true}).click();await retry.scrollIntoViewIfNeeded();await page.screenshot({path:resolve(output,'package-fault.png')});
 await retry.click();await page.getByRole('alert').filter({hasText:/hash|digest|checksum|changed|modified|mismatch|摘要|校验|清单/i}).first().waitFor();
 await page.waitForFunction(()=>[...document.querySelectorAll('button')].some(button=>button.textContent==='重试加载'&&!button.disabled));
 await page.getByRole('button',{name:'设置',exact:true}).click();await page.getByRole('tab',{name:'诊断日志',exact:true}).click();await page.getByRole('heading',{name:'WinOne 日志',exact:true}).waitFor();
 await page.getByRole('heading',{name:'WinOne 日志',exact:true}).scrollIntoViewIfNeeded();await page.screenshot({path:resolve(output,'logs-available.png')});
 const profile=process.env.WINONE_COMPONENTS_PROFILE;const registry=JSON.parse(await readFile(resolve(profile,'packages','.winone-module-registry.json'),'utf8'));
 const target=resolve(profile,'packages',registry.records['example.tool'].stageId,'worker.exe');
 if(!target.startsWith(resolve(profile,'packages')+'\\'))throw Error('Fixture path escaped profile');
 await copyFile(process.env.WINONE_PACKAGE_REPAIR_WORKER,target);
 await page.getByRole('button',{name:'首页',exact:true}).click();await retry.waitFor();await retry.click();
 await page.getByText('example.tool已重新加载',{exact:true}).waitFor();
 if(await page.getByRole('button',{name:'重试加载',exact:true}).count())throw Error('Recovered package still marked unavailable');
 const restored=page.getByRole('switch',{name:'测试工具',exact:true});await restored.waitFor();if(!await restored.isEnabled())throw Error('Recovered switch still blocked');
 await restored.scrollIntoViewIfNeeded();await page.screenshot({path:resolve(output,'package-restored.png')});
 if(errors.length)throw Error(errors.join('\n'));
 await writeFile(resolve(output,'result.json'),JSON.stringify({faultVisible:true,blockedSwitch:true,failedRetryRetained:true,globalLogsAccessible:true,repairedThroughUI:true,layouts,errors},null,2));
 await page.evaluate(()=>{void window.go.main.App.WindowCommand('close');}).catch(error=>{if(!String(error).includes('closed'))throw error;});
}catch(error){await writeFile(resolve(output,'node-error.txt'),String(error.stack||error));if(page){await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});await writeFile(resolve(output,'failure.txt'),await page.locator('body').innerText().catch(()=>''));}throw error;}
finally{if(browser)await browser.close();}
