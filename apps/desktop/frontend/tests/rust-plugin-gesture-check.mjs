// The real settings window and Host, using only the runner's private profile.
import {createRequire} from 'node:module';
import {writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,folder]=process.argv.slice(2),output=resolve(folder);
assert(process.env.WINONE_COMPONENTS_PROFILE?.startsWith(output));
let browser,page;const errors=[],screens=[];
try {
 const until=Date.now()+30000;
 while(Date.now()<until){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(done=>setTimeout(done,100));}}
 assert(browser,'Owned desktop did not start');
 while(Date.now()<until){page=browser.contexts().flatMap(c=>c.pages()).find(p=>p.url().includes('winone.localhost'));if(page)break;await new Promise(done=>setTimeout(done,100));}
 assert(page);page.setDefaultTimeout(20000);page.on('pageerror',e=>errors.push(e.message));
 await page.waitForFunction(()=>performance.getEntriesByName('winone-ui-ready').length>0);

 const screenshot=async name=>{await page.evaluate(()=>Promise.all([document.fonts.ready,...document.getAnimations().map(a=>a.finished)]));await page.screenshot({path:resolve(output,name+'.png')});screens.push(name);assert.equal(await page.locator('[data-slot="page-scroll"]').evaluateAll(es=>es.some(e=>e.scrollWidth>e.clientWidth+1)),false);};
 await page.getByRole('button',{name:'便捷功能',exact:true}).click();
 await page.getByRole('tab',{name:'鼠标手势',exact:true}).click();
 await page.getByRole('tab',{name:'使用场景',exact:true}).waitFor();
 const enabled=await page.getByRole('switch',{name:'启用鼠标手势'}).getAttribute('aria-checked')==='true';
 assert(enabled,'Stroke was not enabled by the isolated settings fixture');
 await screenshot('gesture-scenes');
 await page.getByRole('tab',{name:'高级',exact:true}).click();
 for(const name of ['动作包','编译','执行线程'])await page.getByRole('tab',{name,exact:true}).waitFor();
 await screenshot('gesture-advanced');
 await page.getByRole('tab',{name:'常规',exact:true}).click();await screenshot('gesture-general');
 await page.getByRole('button',{name:'AI 服务',exact:true}).click();
 await page.getByRole('tab',{name:'高级设置',exact:true}).click();await page.getByRole('tab',{name:'模型设置',exact:true}).click();
 await page.getByLabel('路由判断超时（秒）',{exact:true}).fill('23');
 await page.getByRole('button',{name:'保存 AI 设置',exact:true}).click();await page.getByText('已保存',{exact:true}).waitFor();
 const saved=JSON.parse(await page.evaluate(async()=>window.go.main.App.Capture('ai.configuration.get',JSON.stringify(null))));
 assert.equal(saved.magpieSettings.classifyTimeout,23);await screenshot('classifier-saved');
 await page.getByRole('button',{name:'便捷功能',exact:true}).click();await page.getByRole('tab',{name:'鼠标手势',exact:true}).click();

 assert.deepEqual(errors,[]);
 await writeFile(resolve(output,'result.json'),JSON.stringify({passed:true,enabled,classifyTimeout:23,screens,errors},null,2));
 await page.evaluate(async()=>{const api=window.go.main.App,s=await api.Snapshot();await api.SaveSettings({...s.settings,startup:{...s.settings.startup,keepUIRunning:false}});void api.WindowCommand('close');});
}catch(error){await writeFile(resolve(output,'node-error.txt'),String(error.stack||error));if(page)await page.screenshot({path:resolve(output,'failed.png')}).catch(()=>{});process.exitCode=1;}
finally{await browser?.close().catch(()=>{});}
