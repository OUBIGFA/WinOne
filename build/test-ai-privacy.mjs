// A real, isolated AI worker serves the extracted page and masks synthetic
// requests against a loopback echo provider. No real account or model download.
import {createRequire} from 'node:module';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import http from 'node:http';
import {randomBytes} from 'node:crypto';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const root=resolve(process.env.WINONE_LAYOUT_OUTPUT||'_temp/ai-privacy-layout');
await mkdir(root,{recursive:true});
const workers=[],errors=[],results=[],vendorTexts=[];
const listen=server=>new Promise(done=>server.listen(0,'127.0.0.1',done));
const vendor=http.createServer(async(request,response)=>{
 let raw='';for await(const chunk of request)raw+=chunk;
 const body=JSON.parse(raw),text=body.messages.find(m=>m.role==='user')?.content;
 vendorTexts.push(text);
 response.setHeader('Content-Type','application/json');
 response.end(JSON.stringify({id:'privacy-fixture',object:'chat.completion',model:body.model,choices:[{index:0,message:{role:'assistant',content:text},finish_reason:'stop'}],usage:{prompt_tokens:3,completion_tokens:1}}));
});
async function worker(settings={}){
 const profile=join(root,'profile-'+randomBytes(6).toString('hex'));await mkdir(profile,{recursive:true});
 const child=spawn(process.env.WINONE_ROUTING_TEST_EXE,[],{windowsHide:true,env:{...process.env,USERPROFILE:profile,APPDATA:profile,LOCALAPPDATA:profile,XDG_CONFIG_HOME:profile,XDG_CACHE_HOME:profile,CODEX_HOME:join(profile,'.codex')},stdio:['pipe','pipe','pipe']});
 workers.push(child);child.stderr.resume();
 const token=randomBytes(32).toString('hex');
 child.stdin.write(JSON.stringify({operation:'routing.serve',key:token,magpieSettings:{lang:'zh-CN',theme:'light',noStats:true,noUsageStats:true,...settings},auxiliaryProviders:[{id:'fixture',name:'Fixture AI',chat:`http://127.0.0.1:${vendor.address().port}/v1`,key:'fixture-key',models:['model']}]})+'\n');
 const address=await new Promise((resolve,reject)=>{
  const timer=setTimeout(()=>reject(Error('AI worker startup timed out')),15000);let raw='';
  child.once('error',error=>{clearTimeout(timer);reject(error);});
  child.stdout.on('data',chunk=>{raw+=chunk;const end=raw.indexOf('\n');if(end>=0){clearTimeout(timer);try{const value=JSON.parse(raw.slice(0,end));if(value.url)resolve(value.url);else reject(Error(JSON.stringify(value)));}catch(error){reject(error);}}});
  child.once('exit',code=>{clearTimeout(timer);reject(Error('Router exited '+code));});
 });
 return {origin:address,page:new URL('/session/'+token+'/masking.html',address).href,async api(path,body,expectedStatus=200){
  const response=await fetch(new URL(path,address),{method:body===undefined?'GET':'POST',headers:{Authorization:'Bearer '+token,'Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body),signal:AbortSignal.timeout(15000)});
  assert.equal(response.status,expectedStatus,`API ${path} returned ${response.status}`);return response.json();
 }};
}
let current,browser;
const parentServer=http.createServer((request,response)=>{
 const query=new URL(request.url,'http://localhost').searchParams,language=query.get('language')||'zh-CN',theme=query.get('theme')||'light';
 const source=new URL(current.page);if(query.has('record'))source.searchParams.set('record',query.get('record'));
 response.setHeader('Content-Type','text/html');
 response.end(`<style>body{margin:0;padding:20px;box-sizing:border-box}iframe{display:block;width:100%;height:320px;border:0}</style><iframe src="${source.href}" onload="this.contentWindow.postMessage({type:'winone-plugin-theme',language:'${language}',theme:'${theme}'},'${current.origin}')"></iframe><div id="settings" tabindex="-1">WinOne Privacy</div><script>window.received=[];addEventListener('message',event=>{const frame=document.querySelector('iframe');if(event.source!==frame.contentWindow||event.origin!=='${current.origin}')return;window.received.push(event.data?.type);if(event.data?.type==='winone-masking-actions')window.maskKinds=event.data.kinds;if(event.data?.type==='winone-plugin-height')frame.style.height=event.data.height+'px';});</script>`);
});
try{
 await listen(vendor);current=await worker();await listen(parentServer);
 browser=await chromium.launch({headless:true,channel:'msedge'});
 const tab=await browser.newPage({viewport:{width:1280,height:820},reducedMotion:'reduce'});
 tab.setDefaultTimeout(10000);tab.on('pageerror',error=>errors.push(error.message));
 tab.on('console',message=>{if(message.type()==='error'&&message.text().startsWith('[magpie]'))errors.push(message.text());});
 const parentURL=`http://localhost:${parentServer.address().port}/`;
 const embedded=tab.frameLocator('iframe');
 const content=()=>tab.frames().find(item=>item.url().startsWith(current.origin));
 async function layout(language,theme,width,populated=false,record){
  await tab.setViewportSize({width,height:820});
  await tab.goto(parentURL+'?'+new URLSearchParams({language,theme,...(record?{record}:{})}));
  await embedded.locator('#view-masking:not([hidden])').waitFor();
  await embedded.locator('#maskTryList textarea').waitFor();
  await content().evaluate(async()=>{await document.fonts.ready;await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);});
  const view=await content().evaluate(()=>({pages:window.magpie?.pages?.(),scripts:[...document.scripts].map(s=>s.getAttribute('src')),shell:document.querySelectorAll('#side,header.top,#view-agents,#view-providers,#view-settings,#view-routing').length,overflow:document.documentElement.scrollWidth-document.documentElement.clientWidth,lang:document.documentElement.lang}));
  assert.deepEqual(view.pages,['masking']);assert.equal(view.shell,0);
  assert.ok(!view.scripts.some(src=>/^(app|start|plugins)\.js$/.test(src)));
  assert.ok(view.overflow<=1,'Privacy page scrolls sideways: '+view.overflow);assert.equal(view.lang,language);
  if(populated){
   await embedded.locator('#maskOne:not([hidden]) .msk-content').first().waitFor();
   await embedded.locator('#maskOne .msk-who').getByText('Fixture AI',{exact:true}).waitFor();
  }else{
   await embedded.locator('.msk-card.off').waitFor();
   const before=await tab.evaluate(()=>window.received.filter(type=>type==='winone-masking-settings').length);
   await embedded.locator('.msk-card.off button.primary').click();
   await tab.waitForFunction(count=>window.received.filter(type=>type==='winone-masking-settings').length>count,before);
   await embedded.locator('#maskTryList textarea').fill('key sk-proj-'+'a'.repeat(40));
   await embedded.locator('#maskTryList').getByText(/\{\{API_KEY_/).first().waitFor();
  }
  await tab.waitForFunction(()=>parseFloat(document.querySelector('iframe').style.height)>320);
  const height=await tab.evaluate(()=>parseFloat(document.querySelector('iframe').style.height));assert.ok(height>320);
  await tab.screenshot({path:join(root,`privacy-${populated?'record':'empty'}-${language}-${theme}-${width}.png`),fullPage:true});
  results.push({language,theme,width,populated,...view,height});
 }
 for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark'])for(const width of [520,1280])await layout(language,theme,width);
 current=await worker({redact:true});
 const secret='sk-proj-abcdEFGH1234ijklMNOP5678qrst';
 const reply=await current.api('/v1/chat/completions',{model:'fixture/model',messages:[{role:'user',content:'key '+secret}]});
 assert.equal(reply.choices[0].message.content,'key '+secret,'Response must restore the synthetic secret');
 assert.ok(vendorTexts.at(-1).includes('{{API_KEY_'));assert.ok(!vendorTexts.at(-1).includes(secret),'Original value reached vendor');
 const {records}=await current.api('/api/masking/records?limit=10');assert.equal(records.length,1);
 const record=records[0].key;assert.equal(typeof record,'string','Nanosecond IDs must not become rounded JS numbers');
 assert.ok(BigInt(record)>BigInt(Number.MAX_SAFE_INTEGER));
 for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark'])for(const width of [520,1280])await layout(language,theme,width,true,record);
 // A failed read must be visible and retry against the real endpoint, not
 // remain as a skeleton or silently look like an empty record history.
 await tab.route('**/api/masking/summary?*',route=>route.fulfill({status:503,contentType:'application/json',body:JSON.stringify({error:'fixture summary unavailable'})}));
 await tab.goto(parentURL+'?language=en-US');await embedded.locator('#status').getByText('fixture summary unavailable',{exact:false}).waitFor();
 await tab.unroute('**/api/masking/summary?*');await embedded.locator('#status').getByRole('button',{name:'Try again'}).click();
 await embedded.locator('#maskList .msk-row').first().waitFor();
 assert.ok(!(await embedded.locator('#status').innerText()).includes('fixture summary unavailable'));
 // Keep-key-form reuses the actual upstream masker and response restoration.
 current=await worker({redact:true,redactShape:true});
 const shaped=await current.api('/v1/chat/completions',{model:'fixture/model',messages:[{role:'user',content:'key '+secret}]});
 assert.equal(shaped.choices[0].message.content,'key '+secret);
 assert.ok(vendorTexts.at(-1).startsWith('key sk-proj-'));assert.notEqual(vendorTexts.at(-1),'key '+secret);
 // The blocked request stays off the vendor. Its action asks WinOne to edit
 // the draft; it must neither bypass the host nor report an automatic save.
 current=await worker({redact:true,redactActions:{API_KEY:'block'}});
 const beforeBlocked=vendorTexts.length;
 await current.api('/v1/chat/completions',{model:'fixture/model',messages:[{role:'user',content:'key '+secret}]},400);
 assert.equal(vendorTexts.length,beforeBlocked);
 const blocked=await current.api('/api/masking/records?limit=10');assert.equal(blocked.records[0].blocked,true);
 await tab.goto(parentURL+'?language=en-US&record='+blocked.records[0].key);
 await embedded.locator('.msk-blocked').getByRole('button',{name:'Mask it instead',exact:true}).click();
 await tab.waitForFunction(()=>window.received.includes('winone-masking-actions'));
 assert.deepEqual(await tab.evaluate(()=>window.maskKinds),['API_KEY']);
 assert.equal((await current.api('/api/settings')).redactActions.API_KEY,'block');
 assert.deepEqual(errors,[],'Privacy page errors');
 await writeFile(join(root,'report.json'),JSON.stringify({results,errors,transport:{masked:true,restored:true,recordDeepLink:true,keyShape:true,blocked:true},draftAction:true,summaryRetry:true},null,2));
 console.log(`Privacy page: ${results.length} layouts; real masking/restoration, exact record deep-link, key shape, blocked-request draft handoff and summary retry passed`);
}catch(error){
 const shown=browser?await Promise.all(browser.contexts().flatMap(context=>context.pages()).map(async page=>{await page.screenshot({path:join(root,'failure.png'),fullPage:true});return Promise.all(page.frames().filter(frame=>frame.parentFrame()).map(frame=>frame.locator('body').innerText()));})):[];
 await writeFile(join(root,'failure.json'),JSON.stringify({message:String(error),errors,results,shown},null,2));throw error;
}finally{
 await browser?.close();parentServer.close();vendor.close();
 for(const child of workers){if(child.exitCode!==null)continue;const exited=once(child,'exit');child.stdin.end();const timer=setTimeout(()=>child.kill(),3000);await exited;clearTimeout(timer);}
}
