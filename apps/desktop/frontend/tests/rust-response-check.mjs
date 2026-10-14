// Explicit isolated product check. Uses the bundled Cline discovery without
// credentials; synthetic delays exercise navigation independently of disk speed.
import {createRequire} from 'node:module';
import {writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2);let browser,page;const results={switches:[]};
try{
 const deadline=Date.now()+25000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(resolve=>setTimeout(resolve,100));}}
 if(!browser)throw Error('Isolated WebView2 did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(p=>p.url().includes('winone.localhost'));if(page)break;await new Promise(resolve=>setTimeout(resolve,100));}
 if(!page)throw Error('Settings document did not load');
 const errors=[];page.on('pageerror',error=>errors.push(String(error)));
 await page.getByRole('button',{name:'AI 服务',exact:true}).waitFor();
 await page.evaluate(async()=>{
  const app=window.go.main.App;
  const configuration=JSON.parse(await app.Capture('ai.configuration.get','null'));
  configuration.providers=[{id:'response-cline',name:'响应测试账号',authMode:'account',agent:'cline',agentBackend:'plugin',models:[],availableModels:[],hasKey:false}];configuration.tasks={};
  await app.Capture('ai.configuration.save',JSON.stringify({configuration}));
  window.__accountReads=[];
  const capture=async(method,payload)=>{
   const start=performance.now();const read=method==='ai.accounts.list'?{start,elapsed:null}:null;
   if(read)window.__accountReads.push(read);
   try{return await app.Capture(method,payload);}finally{if(read)read.elapsed=performance.now()-start;}
  };
  window.go.main.App=new Proxy({}, {get:(_target,name)=>name==='Capture'?capture:app[name]});
 });
 await page.getByRole('button',{name:'AI 服务',exact:true}).click();
 const tabs=page.getByRole('tablist');const position=await tabs.boundingBox();
 const switchTab=async name=>{
  const start=performance.now();await page.getByRole('tab',{name,exact:true}).click({timeout:1000});
  if(await page.getByRole('tab',{name,exact:true}).getAttribute('aria-selected')!=='true')throw Error(`Tab ${name} did not respond`);
  const elapsed=performance.now()-start;results.switches.push({name,ms:Math.round(elapsed)});
  if(elapsed>1000)throw Error(`Tab ${name} took ${elapsed} ms`);
 };
 await switchTab('账号供应商');
 await page.waitForFunction(()=>window.__accountReads.length===1);
 await switchTab('自定义供应商');await switchTab('账号供应商');
 await page.waitForFunction(()=>Array.from(document.querySelectorAll('button')).some(button=>button.textContent==='登录'&&!button.disabled),{},{timeout:20000});
 for(let i=0;i<3;i++){await switchTab('默认 AI 服务');await switchTab('账号供应商');}
 const reads=await page.evaluate(()=>window.__accountReads);
 if(reads.length!==1)throw Error(`Tab switches started ${reads.length} account reads`);
 results.accountReads=reads;results.reusedRead=true;
 const after=await tabs.boundingBox();
 if(Math.abs(after.x-position.x)>1||Math.abs(after.y-position.y)>1)throw Error('Loading moved the navigation tabs');
 await page.getByRole('region',{name:'账号登录',exact:true}).getByRole('button',{name:'刷新状态',exact:true}).click();
 await page.waitForFunction(()=>window.__accountReads.length===2);
 await switchTab('自定义供应商');await switchTab('账号供应商');
 await page.waitForFunction(()=>window.__accountReads[1].elapsed!==null);
 results.accountReads=await page.evaluate(()=>window.__accountReads);
 results.manualRefresh=true;
 await page.screenshot({path:resolve(output,'accounts.png')});
 await page.getByRole('button',{name:'设置',exact:true}).click();
 await page.getByRole('button',{name:'AI 服务',exact:true}).click();
 await switchTab('账号供应商');
 await page.waitForFunction(()=>Array.from(document.querySelectorAll('button')).some(button=>button.textContent==='登录'&&!button.disabled));
 if(await page.evaluate(()=>window.__accountReads.length)!==2)throw Error('Reopening AI services restarted account discovery');
 results.reopenedPageRead=true;
 // A global snapshot is a read too. Slow refresh must not block another page.
 await page.evaluate(()=>{
  const app=window.go.main.App;const gate=new Promise(resolve=>{window.__releaseRead=resolve;});
  window.go.main.App=new Proxy({}, {get:(_target,name)=>name==='Snapshot'?async()=>{await gate;return app.Snapshot();}:app[name]});
 });
 await page.locator('header.page-actions').getByRole('button').click();
 await page.getByRole('button',{name:'设置',exact:true}).click();
 if(await page.getByRole('dialog').count())throw Error('Global refresh blocks page navigation');
 await page.getByRole('tab',{name:'诊断日志',exact:true}).waitFor({timeout:1000});
 await page.evaluate(()=>window.__releaseRead());
 results.globalRefreshNavigation=true;
 // Account reads can also be abandoned by closing the actual native window.
 if(errors.length)throw Error(errors.join('\n'));
 results.errors=errors;
 await writeFile(resolve(output,'result.json'),JSON.stringify(results,null,2));
 await page.evaluate(()=>{
  void window.go.main.App.Capture('ai.accounts.list',JSON.stringify({agent:'cline',provider:'response-cline'})).catch(()=>{});
  void window.go.main.App.WindowCommand('close');
 }).catch(error=>{if(!String(error).includes('closed'))throw error;});
}catch(error){
 await writeFile(resolve(output,'node-error.txt'),String(error.stack||error));
 await writeFile(resolve(output,'partial-result.json'),JSON.stringify(results,null,2));
 if(page){await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});await page.evaluate(()=>window.__releaseRead?.()).catch(()=>{});}
 process.exitCode=1;
}finally{await browser?.close();}
