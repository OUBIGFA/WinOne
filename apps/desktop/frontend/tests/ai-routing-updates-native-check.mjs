// Production document -> authenticated routing iframe -> Host -> owned Worker.
// All configuration and upstream requests belong to the runner's private profile.
import {createRequire} from 'node:module';
import {writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {execFileSync} from 'node:child_process';
import http from 'node:http';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,outputArg]=process.argv.slice(2),output=resolve(outputArg),errors=[],requests=[];
if(new URL(endpoint).hostname!=='127.0.0.1'||!process.env.WINONE_COMPONENTS_PROFILE?.startsWith(output))throw Error('Use the private component runner');
let browser,page;
async function capture(name){
 await page.evaluate(async()=>{document.activeElement?.blur();await document.fonts.ready;await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);});await page.mouse.move(0,0);
 await page.screenshot({path:join(output,name+'.png')});
 if(process.env.WINONE_WINDOW_SHOT)execFileSync(process.env.WINONE_WINDOW_SHOT,[process.env.WINONE_COMPONENTS_PID,join(output,name+'-window.png')],{windowsHide:true,encoding:'utf8',timeout:15000});
}
const vendor=http.createServer(async(req,res)=>{
 let raw='';for await(const chunk of req)raw+=chunk;
 requests.push(JSON.parse(raw));res.writeHead(200,{'Content-Type':'application/json'});
 res.end(JSON.stringify({choices:[{message:{role:'assistant',content:'NATIVE_ROUTING_PROOF'},finish_reason:'stop'}]}));
});
await new Promise(done=>vendor.listen(0,'127.0.0.1',done));
try{
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(done=>setTimeout(done,100));}}
 if(!browser)throw Error('Owned WebView did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(c=>c.pages()).find(p=>p.url().includes('winone.localhost'));if(page)break;await new Promise(done=>setTimeout(done,100));}
 if(!page)throw Error('Owned document missing');page.setDefaultTimeout(20000);page.on('pageerror',error=>errors.push(error.stack||String(error)));
 // Native iframe coordinates can change when the parent scrolls a long page.
 // Record delivery without changing handlers, so a missed click is diagnosable.
 await page.addInitScript(()=>{
  window.routingClickTrace=[];let pressed;
  for(const type of ['pointerdown','mousedown','pointerup','mouseup','click'])addEventListener(type,event=>{
   if(type==='pointerdown')pressed=event.target;
   const button=document.querySelector('.rt-useeffort');
   window.routingClickTrace.push({type,x:event.clientX,y:event.clientY,target:event.target.outerHTML?.slice(0,220),sameTarget:event.target===pressed,pressedConnected:pressed?.isConnected,button:button?.getBoundingClientRect().toJSON(),height:innerHeight,scrollY});
   if(window.routingClickTrace.length>80)window.routingClickTrace.shift();
  },true);
 });
 await page.locator('.app-navigation').waitFor();
 await page.evaluate(async base=>{
  const app=window.go.main.App,snapshot=await app.Snapshot();await app.SaveSettings({...snapshot.settings,theme:'dark',language:'zh-CN',startup:{...snapshot.settings.startup,keepUIRunning:false}});
  const configuration=JSON.parse(await app.Capture('ai.configuration.get','null'));
  configuration.providers=[{id:'native-routing',name:'路由验证服务',authMode:'api-key',protocol:'chat',baseUrl:base,models:['model'],availableModels:['model'],hasKey:false,modelMetadata:{model:{efforts:['low','medium','high'],reasoningEffort:'medium'}}}];
  configuration.tasks={};configuration.magpieSettings={builtinPreset:'essentials',noStats:true,noUsageStats:true};
  await app.Capture('ai.configuration.save',JSON.stringify({configuration}));
 },'http://127.0.0.1:'+vendor.address().port+'/v1');
 await page.reload({waitUntil:'domcontentloaded'});await page.locator('.app-navigation').waitFor();
 await page.locator('.app-navigation [data-page-id="ai-services"]').click();
 await page.getByRole('button',{name:'重命名服务',exact:true}).click();await page.getByRole('textbox',{name:'服务名称',exact:true}).fill('未保存名称');
 await page.getByRole('tab',{name:'路由',exact:true}).click();
 const routing=()=>page.frameLocator('iframe[title="路由"]');
 await routing().locator('.rt-goffon').waitFor();assert.equal(await routing().locator('#rtNewGroup').isVisible(),false);
 await capture('routing-groups-off');
 await routing().locator('.rt-goffon').click();
 await routing().locator('#rtNewGroup').waitFor();
 const saved=await page.evaluate(async()=>JSON.parse(await window.go.main.App.Capture('ai.configuration.get','null')));
 assert.equal(saved.magpieSettings.builtinPlugins.groups.enabled,true);assert.equal(saved.providers[0].name,'路由验证服务');
 await page.getByRole('tab',{name:'自定义供应商',exact:true}).click();
 await page.getByRole('button',{name:'重命名服务',exact:true}).click();assert.equal(await page.getByRole('textbox',{name:'服务名称',exact:true}).inputValue(),'未保存名称');
 await page.getByRole('tab',{name:'路由',exact:true}).click();await routing().locator('#rtNewGroup').waitFor();
 const child=()=>page.frames().find(f=>f.url().startsWith('http://127.0.0.1:'));
 const post=async(path,body)=>child().evaluate(async({path,body})=>{const r=await fetch(path,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});if(!r.ok)throw Error(path+' '+r.status);return r.json();},{path,body});
 await post('/api/groups/save',{id:'native-rule',name:'Native rule',members:['native-routing/model'],rules:[{use:'native-routing/model',tokens:1000,useEffort:'high'}]});
 await page.getByRole('tab',{name:'用量统计',exact:true}).click();await page.getByRole('tab',{name:'路由',exact:true}).click();
 await routing().locator('.rt-group[data-id="native-rule"]').getByRole('button',{name:'编辑',exact:true}).click();
 // Opening the editor expands this cross-origin iframe. CDP can hit the
 // parent's iframe node before Chromium publishes the resized child surface.
 // Scroll first, then let both documents paint; never force or replay a click.
 const effortButton=routing().locator('.rt-useeffort');
 await effortButton.scrollIntoViewIfNeeded();
 await child().evaluate(async()=>{await document.fonts.ready;await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);});
 await page.evaluate(async()=>{await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);});
 // The request below proves the saved selection, not merely a visible list.
 await effortButton.click();
 await routing().locator('#pop:not([hidden])').waitFor();
 await routing().locator('#pop:not([hidden]) #list li[data-i]').filter({hasText:/\blow\b/i}).first().click();
 await routing().locator('.rt-gedit').getByRole('button',{name:'保存',exact:true}).click();
 await routing().locator('.rt-group[data-id="native-rule"]').waitFor();
 const answer=await post('/v1/chat/completions',{model:'group/native-rule',reasoning_effort:'high',messages:[{role:'user',content:'word '.repeat(2000)}]});
 assert.equal(answer.choices[0].message.content,'NATIVE_ROUTING_PROOF');assert.equal(requests.at(-1).reasoning_effort,'low');
 assert.equal(await routing().locator('html').getAttribute('data-theme'),'dark');
 await capture('routing-rule-dark');
 // Commit the preserved provider draft before navigating to global settings.
 // SaveSettings alone writes disk but does not update React's current theme.
 await page.getByRole('button',{name:'保存 AI 设置',exact:true}).click();await page.getByRole('status').filter({hasText:'已保存'}).waitFor();
 await page.getByRole('tab',{name:'插件',exact:true}).click();const plugins=page.frameLocator('iframe[title="插件"]');await plugins.getByRole('button',{name:'发现',exact:true}).waitFor();
 assert.equal(await plugins.locator('html').getAttribute('data-theme'),'dark');
 await capture('plugins-dark');
 await page.locator('.app-navigation [data-page-id="settings"]').click();
 await page.getByRole('radio',{name:'浅色',exact:true}).click();await page.locator('html[data-theme="light"]').waitFor();
 await page.locator('.app-navigation [data-page-id="ai-services"]').click();
 await page.getByRole('tab',{name:'插件',exact:true}).click();await plugins.getByRole('button',{name:'发现',exact:true}).waitFor();await plugins.locator('html[data-theme="light"]').waitFor();
 await capture('plugins-light');
 await page.getByRole('tab',{name:'路由',exact:true}).click();await routing().locator('html[data-theme="light"]').waitFor();
 const groups=await child().evaluate(async()=>{const r=await fetch('/api/groups');return r.json();});assert.equal(groups.groups.find(g=>g.id==='native-rule').rules[0].useEffort,'low');
 await capture('routing-rule-light');assert.deepEqual(errors,[]);
 await writeFile(join(output,'result.json'),JSON.stringify({native:true,productionAssets:true,groupsOffExplained:true,enabledAndReopened:true,unsavedProviderPreserved:true,ruleEffort:'low',vendorRequests:requests.length,darkAndLight:true,pluginTheme:true,errors},null,2),'utf8');
 await page.evaluate(()=>{void window.go.main.App.WindowCommand('close');});
}catch(error){
 if(page){await page.screenshot({path:join(output,'failure.png')}).catch(()=>{});await writeFile(join(output,'failure-frames.json'),JSON.stringify(await Promise.all(page.frames().map(async f=>({url:f.url(),text:await f.locator('body').innerText().catch(()=>''),pointer:await f.evaluate(()=>window.routingClickTrace??[]).catch(()=>[])}))),null,2),'utf8');}
 await writeFile(join(output,'node-error.txt'),String(error.stack||error)+'\n'+errors.join('\n'),'utf8');process.exitCode=1;
}finally{await browser?.close().catch(()=>{});await new Promise(done=>vendor.close(done));}
