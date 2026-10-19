// Explicit isolated integration: upstream management UI, local plugin code,
// real gateway calls and the upstream scripts-only page sandbox. No accounts.
import {createRequire} from 'node:module';
import {mkdir,writeFile,readFile} from 'node:fs/promises';
import {resolve,join,relative,dirname} from 'node:path';
import {spawn} from 'node:child_process';
import {createInterface} from 'node:readline';
import {randomBytes} from 'node:crypto';
import {createServer} from 'node:http';
const require=createRequire(import.meta.url),{chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE||'playwright');
const [executableArg,outputArg]=process.argv.slice(2),executable=resolve(executableArg),output=resolve(outputArg);
if(!relative(resolve('_temp'),output)||relative(resolve('_temp'),output).startsWith('..'))throw Error('Use a new repository _temp task directory');
await mkdir(output,{recursive:false});
const environment={...process.env};
for(const name of Object.keys(environment))if(/^(XDG_|MAGPIE_|WINONE_AI_|CODEX_|CLAUDE_|ANTHROPIC_|OPENAI_|GEMINI_|COPILOT_|CURSOR_|GROK_|KIRO_|DEVIN_|WORKBUDDY_|QODER_|ZED_|FACTORY_|DROID_|MIMO_|ZCODE_|AWS_|AZURE_)/i.test(name)||['GH_TOKEN','GITHUB_TOKEN','GOOGLE_APPLICATION_CREDENTIALS'].includes(name))delete environment[name];
Object.assign(environment,{XDG_CONFIG_HOME:join(output,'config'),XDG_CACHE_HOME:join(output,'cache'),USERPROFILE:join(output,'home'),APPDATA:join(output,'home/roaming'),LOCALAPPDATA:join(output,'home/local'),WINONE_AI_PLUGINS_HOME:join(output,'third-party'),MAGPIE_NO_STATS:'1',MAGPIE_OTEL_ENABLED:'false'});
for(const path of [environment.USERPROFILE,environment.XDG_CONFIG_HOME,environment.XDG_CACHE_HOME,environment.APPDATA,environment.LOCALAPPDATA])await mkdir(path,{recursive:true});
let observedBody, browser, page, child;
const upstream=createServer(async(req,res)=>{let body='';for await(const bytes of req)body+=bytes;observedBody=JSON.parse(body);res.setHeader('Content-Type','application/json');res.end(JSON.stringify({choices:[{message:{role:'assistant',content:'Fixture answer'},finish_reason:'stop'}]}));});
await new Promise(done=>upstream.listen(0,'127.0.0.1',done));
const key=randomBytes(32).toString('hex'),errors=[];
try{
 child=spawn(executable,[],{cwd:dirname(executable),env:environment,windowsHide:true,stdio:['pipe','pipe','pipe']});
 let stderr='';child.stderr.on('data',bytes=>{stderr+=bytes;});
 const lines=createInterface({input:child.stdout});
 const address=Promise.race([new Promise((done,reject)=>{lines.once('line',line=>{try{done(JSON.parse(line));}catch(error){reject(error);}});child.once('exit',code=>reject(Error('AI process exited '+code+': '+stderr)));}),new Promise((_,reject)=>setTimeout(()=>reject(Error('AI startup timed out: '+stderr)),30000).unref())]);
 child.stdin.write(JSON.stringify({operation:'routing.serve',key,magpieSettings:{noStats:true,noUsageStats:true},auxiliaryProviders:[{id:'fixture',name:'Fixture',chat:'http://127.0.0.1:'+upstream.address().port+'/v1',key:'fixture-key',models:['model']}]})+'\n');
 const startup=await address;
 await writeFile(join(output,'startup.json'),JSON.stringify(startup),'utf8');
 if(!startup.url)throw Error('AI startup failed: '+JSON.stringify(startup)+' '+stderr);
 const {url}=startup;
 const api=async(path,body)=>{const response=await fetch(url+'/api/'+path,{method:body===undefined?'GET':'POST',headers:{Authorization:'Bearer '+key,'Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body)});const text=await response.text();if(!response.ok)throw Error(path+' '+response.status+': '+text);return JSON.parse(text);};
 const fixture=join(output,'local-plugin');await mkdir(fixture);
 await writeFile(join(fixture,'package.json'),JSON.stringify({name:'winone-local-fixture',version:'1.0.0',description:'Generated integration fixture',magpie:{hooks:'request.js',middleware:'provider.js',page:{title:'Fixture page',titles:{zh:'夹具页面'},file:'page.html',replaces:'usage'},panels:[...['agents','usage','settings','provider'].map(slot=>({slot,title:'Fixture '+slot,titles:{zh:'夹具 '+slot},file:'page.html'})),{slot:'provider',title:'Neighbour panel',file:'neighbour.html'}],actions:[{id:'inspect-provider',menu:'provider',title:'Inspect provider',titles:{zh:'检查供应商'}},{id:'inspect-agent',menu:'agent',title:'Inspect agent',titles:{zh:'检查智能体'}}],permissions:['usage:summary']}}),'utf8');
 await writeFile(join(fixture,'request.js'),'export async function onRequest(body){return {...body,temperature:0.125}}','utf8');
 await writeFile(join(fixture,'provider.js'),'export function onProvider(body,ctx){if(ctx.provider.id==="fixture")return {...body,top_p:0.42};return body}','utf8');
 await writeFile(join(fixture,'page.html'),'<!doctype html><html><head><script src="_magpie/bridge.js"></script><script src="fixture.js" defer></script></head><body><p id="ok">Sandbox fixture</p><pre id="action"></pre></body></html>','utf8');
 await writeFile(join(fixture,'fixture.js'),'magpie.ready.then(async()=>{const o=await magpie.options.get();if(o.breakPanel&&magpie.context.provider)parent.postMessage({magpie:1,method:"error",params:{message:"Expected fixture panel failure"}},"*");setTimeout(()=>magpie.onAction(value=>{document.getElementById("action").textContent=JSON.stringify(value);}),400);});','utf8');
 await writeFile(join(fixture,'neighbour.html'),'<!doctype html><html><head><script src="_magpie/bridge.js"></script></head><body><p>Neighbour remains usable</p></body></html>','utf8');
 browser=await chromium.launchPersistentContext(join(output,'browser'),{headless:true,executablePath:process.env.WINONE_CHROMIUM,viewport:{width:1000,height:850},downloadsPath:join(output,'downloads')});
 page=await browser.newPage();page.on('pageerror',error=>errors.push(error.stack||String(error)));
 await page.goto(url+'/session/'+key+'/plugins.html');
 await page.getByRole('button',{name:'Discover',exact:true}).waitFor({timeout:30000});
 if(await page.getByRole('button',{name:'Built-in',exact:true}).count())throw Error('Integrated features remain on third-party page');
 await page.getByRole('textbox',{name:'Plugin',exact:true}).fill(fixture);
 await page.getByRole('button',{name:'Install',exact:true}).last().click();
 await page.getByRole('button',{name:/^Installed/}).click();
 await page.getByText('local-plugin',{exact:true}).first().waitFor({timeout:30000});
 const installed=await api('plugins');if(installed.plugins.length!==1)throw Error('Expected only the explicitly installed fixture');
 await api('plugins/options',{spec:fixture,options:{fixtureValue:17}});
 if((await api('plugins')).plugins[0].options.fixtureValue!==17)throw Error('Plugin options did not persist');
 const call=async()=>{const response=await fetch(url+'/v1/chat/completions',{method:'POST',headers:{Authorization:'Bearer '+key,'Content-Type':'application/json'},body:JSON.stringify({model:'fixture/model',messages:[{role:'user',content:'Exercise local hook'}]})});if(!response.ok)throw Error(await response.text());await response.text();};
 await call();if(observedBody.temperature!==0.125||observedBody.top_p!==0.42)throw Error('Installed request/provider hook did not reach real gateway request');
 const pages=await api('plugin-pages');if(pages.length!==1)throw Error('Installed page was not exposed');
 await page.getByRole('button',{name:'Fixture page',exact:true}).click();
 const frame=page.locator('iframe.plugin-frame');await frame.waitFor();
 if(await frame.getAttribute('sandbox')!=='allow-scripts')throw Error('Plugin page sandbox was widened');
 await page.frameLocator('iframe.plugin-frame').getByText('Sandbox fixture',{exact:true}).waitFor();
 const pluginFrame=await (await frame.elementHandle()).contentFrame();
 if(!pluginFrame)throw Error('Sandbox document missing');
 const denied=await pluginFrame.evaluate(()=>new Promise(resolve=>{
  const id='denied-fixture';
  const receive=event=>{if(event.source===parent&&event.data?.magpie===1&&event.data?.id===id){removeEventListener('message',receive);resolve(event.data.error);}};
  addEventListener('message',receive);parent.postMessage({magpie:1,id,method:'read',params:{name:'providers:list'}},'*');
 }));
 if(!denied)throw Error('Sandbox read exceeded declared permissions');
 let serverDenied=false;try{await api('plugin-pages/read',{spec:fixture,read:'providers:list'});}catch(error){serverDenied=/403/.test(String(error));}
 if(!serverDenied)throw Error('Server did not enforce page read permissions');
 // P5/P6 use the same extracted document at each WinOne-owned insertion point.
 // No full Magpie application is present, including with a replaces-only page.
 const surfaces=await api('plugin-panels');
 if(surfaces.length!==5||pages[0].replaces!=='usage'||pages[0].actions.length!==2)throw Error('P5/P6 manifests were lost');
 const surface=await browser.newPage();surface.on('pageerror',error=>errors.push(error.stack||String(error)));
 const base=url+'/session/'+key+'/plugins.html';
 for(const slot of ['agents','usage','settings','provider']){
  await surface.goto(base+'?surface=1&slot='+slot+'&item-id=fixture&item-name=Public%20provider');
  await surface.frameLocator('iframe.plugin-panel-frame').first().getByText('Sandbox fixture',{exact:true}).waitFor();
  const frame=surface.locator('iframe.plugin-panel-frame').first();
  if(await frame.getAttribute('sandbox')!=='allow-scripts')throw Error('Panel sandbox was widened');
  const context=await (await (await frame.elementHandle()).contentFrame()).evaluate(async()=>{await magpie.ready;return magpie.context;});
  const expected=slot==='provider'?{provider:{id:'fixture',name:'Public provider'}}:{};
  if(JSON.stringify(context)!==JSON.stringify(expected))throw Error('Panel context leaked or changed: '+JSON.stringify(context));
  await surface.evaluate(()=>window.winOnePluginTheme({language:'zh-CN',theme:'dark'}));
  await surface.getByText('夹具 '+slot,{exact:true}).waitFor();
  await surface.screenshot({path:join(output,'surface-'+slot+'.png'),fullPage:true});
  if(await surface.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error('Panel horizontal overflow');
 }
 await api('plugins/options',{spec:fixture,options:{fixtureValue:17,breakPanel:true}});
 await surface.reload();await surface.getByText('Expected fixture panel failure',{exact:true}).waitFor();
 await surface.frameLocator('iframe.plugin-panel-frame').getByText('Neighbour remains usable',{exact:true}).waitFor();
 await api('plugins/options',{spec:fixture,options:{fixtureValue:17}});
 await surface.getByRole('button',{name:/Try again|重试/}).click();
 await surface.frameLocator('iframe.plugin-panel-frame').first().getByText('Sandbox fixture',{exact:true}).waitFor();
 for(const menu of ['provider','agent']){
  await surface.goto(base+'?page='+encodeURIComponent(pages[0].key)+'&action=inspect-'+menu+'&menu='+menu+'&item-id=fixture&item-name=Public%20name');
  const result=surface.frameLocator('iframe.plugin-frame').locator('#action');
  await result.filter({hasText:'inspect-'+menu}).waitFor();
  const value=JSON.parse(await result.textContent());
  if(JSON.stringify(value)!==JSON.stringify({id:'inspect-'+menu,[menu]:{id:'fixture',name:'Public name'}}))throw Error('Late menu action changed public context: '+JSON.stringify(value));
 }
 await surface.close();
 await page.getByRole('button',{name:'Plugins',exact:true}).click();
 for(const language of ['en-US','zh-CN'])for(const theme of ['light','dark'])for(const width of [520,1000]){
  await page.setViewportSize({width,height:850});await page.evaluate(value=>window.winOnePluginTheme(value),{language,theme});
  await page.getByRole('button',{name:language==='zh-CN'?'发现':'Discover',exact:true}).waitFor();
  await page.getByRole('button',{name:language==='zh-CN'?/^已安装/:/^Installed/}).waitFor();
  await page.screenshot({path:join(output,language+'-'+theme+'-'+width+'.png'),fullPage:true});
  if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error('Plugin page overflows at '+width+' '+language);
 }
 await page.reload();await page.getByText('local-plugin',{exact:true}).first().waitFor();
 if((await api('plugins')).plugins[0].options.fixtureValue!==17)throw Error('Options lost after reopening the manager');
 await api('plugins/off',{spec:fixture,off:true});await call();if(observedBody.temperature===0.125)throw Error('Disabled hook still modified gateway request');
 await api('plugins/off',{spec:fixture,off:false});await call();if(observedBody.temperature!==0.125)throw Error('Re-enabled hook did not run');
 await api('plugins/remove',{spec:fixture});if((await api('plugins')).plugins.length)throw Error('Removed plugin remains installed');
 if(errors.length)throw Error(errors.join('\n'));
 await writeFile(join(output,'result.json'),JSON.stringify({upstreamManager:true,localInstall:true,optionsPersistedAfterReopen:true,realGatewayHook:true,disableEnable:true,removed:true,sandboxScriptsOnly:true,pageAndServerPermissionDenied:true,dynamicChinese:true,layoutCases:8,panelSlots:4,panelFailureIsolated:true,panelRetry:true,pageReplacement:true,lateRegisteredActions:2,providerMiddleware:true,errors},null,2),'utf8');
 console.log('PASS: upstream plugin manager, local install/options/hooks/disable/enable/remove and sandbox');
}catch(error){await writeFile(join(output,'errors.json'),JSON.stringify(errors),'utf8');if(page){await page.screenshot({path:join(output,'failure.png'),fullPage:true}).catch(()=>{});await writeFile(join(output,'failure.txt'),await page.locator('body').innerText().catch(()=>''),'utf8');}throw error;}
finally{await browser?.close();child?.stdin.end();await new Promise(done=>upstream.close(done));}
