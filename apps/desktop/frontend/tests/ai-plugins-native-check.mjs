// Real WinOne Host, isolated profile and local plugin. Uses the existing
// development review entry to check the current UI and real native bridge.
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {pathToFileURL} from 'node:url';
import assert from 'node:assert/strict';
import {checkAISync} from './ai-sync-native-steps.mjs';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2),errors=[],production=process.env.WINONE_NATIVE_PRODUCTION_ASSETS==='1';
let browser,page,server;
try{
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(done=>setTimeout(done,100));}}
 if(!browser)throw Error('Isolated WinOne did not expose its test connection');
 while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(value=>value.url().includes('winone.localhost'));if(page)break;await new Promise(done=>setTimeout(done,100));}
 if(!page)throw Error('Native settings document missing');
 page.setDefaultTimeout(20000);page.on('pageerror',error=>errors.push(String(error)));
 if(!production){const frontend=resolve('apps/desktop/frontend');
 const {createServer}=await import(pathToFileURL(join(frontend,'node_modules/vite/dist/node/index.js')));
 server=await createServer({root:frontend,configFile:join(frontend,'vite.config.ts'),server:{host:'127.0.0.1',port:5208,strictPort:true,hmr:false}});await server.listen();}
 await page.locator('.app-navigation').waitFor();
 await page.evaluate(async()=>{const app=window.go.main.App,snapshot=await app.Snapshot();await app.SaveSettings({...snapshot.settings,language:'zh-CN',theme:'light'});});
 if(!production)await page.route('http://winone.localhost/**',async route=>{
  const url=new URL(route.request().url()),path=url.pathname==='/'||url.pathname==='/index.html'?'/tests/settings-native-preview.html':url.pathname;
  const response=await fetch('http://127.0.0.1:5208'+path+url.search);
  await route.fulfill({status:response.status,contentType:response.headers.get('content-type')||'application/octet-stream',body:Buffer.from(await response.arrayBuffer())});
 });
 // Record the insertion site before CSP rejects a dynamic stylesheet. The
 // production check must use embedded assets, not Vite's permissive dev CSP.
 await page.addInitScript(()=>{
  window.__winoneCSPViolations=[];window.__winoneStyleInsertions=[];
  document.addEventListener('securitypolicyviolation',event=>window.__winoneCSPViolations.push({directive:event.effectiveDirective,blocked:event.blockedURI,source:event.sourceFile,line:event.lineNumber,column:event.columnNumber,sample:event.sample}));
  for(const name of ['appendChild','insertBefore']){const original=Node.prototype[name];Node.prototype[name]=function(node,...args){
   if(node instanceof HTMLStyleElement)window.__winoneStyleInsertions.push({text:node.textContent,attributes:Array.from(node.attributes,attribute=>[attribute.name,attribute.value]),stack:new Error().stack});
   return Reflect.apply(original,this,[node,...args]);
  };}
 });
 await page.reload({waitUntil:'domcontentloaded'});
 await page.locator('.app-navigation [data-page-id="ai-services"]').click();
 const tabs=page.getByRole('tablist',{name:'AI 服务',exact:true});
 await tabs.getByRole('tab',{name:'插件',exact:true}).click();
 const manager=page.frameLocator('iframe[title="插件"]');
 await manager.getByRole('button',{name:'发现',exact:true}).waitFor();
 assert.equal(await page.getByRole('button',{name:'保存 AI 设置',exact:true}).count(),0,'Immediate operations do not add a redundant save');
 const fixture=join(output,'native-provider');await mkdir(fixture);
 await writeFile(join(fixture,'package.json'),JSON.stringify({name:'winone-native-provider-fixture',version:'1.0.0',type:'module',main:'index.js'}),'utf8');
 await writeFile(join(fixture,'index.js'),"export default async()=>({config(c){c.provider??={};c.provider.nativefixture={name:'Native provider fixture',models:{m1:{name:'Model One'}}}},auth:{provider:'nativefixture',methods:[{type:'api',label:'Fixture key'}]}})",'utf8');
 await manager.getByRole('textbox',{name:'插件',exact:true}).fill(fixture);
 await manager.getByRole('button',{name:'安装',exact:true}).last().click();
 await manager.getByText(fixture+' 已安装',{exact:true}).waitFor();
 await manager.getByRole('button',{name:/^已安装/}).click();
 await manager.getByText('native-provider',{exact:true}).first().waitFor();
 await manager.getByRole('button',{name:'登录',exact:true}).click();
 await page.getByRole('combobox',{name:'账号',exact:true}).waitFor();
 assert.match(await page.getByRole('combobox',{name:'账号',exact:true}).innerText(),/Native provider fixture/);
 await page.getByRole('button',{name:'保存 AI 设置',exact:true}).click();
 await page.getByRole('status').filter({hasText:'已保存'}).waitFor();
 const account=page.locator('#ai-account-connection');
 await account.getByRole('button',{name:'登录',exact:true}).click();
 const credential=account.locator('input[type="password"]');await credential.waitFor();
 await account.getByRole('button',{name:'取消登录',exact:true}).click();
 await account.getByRole('status').filter({hasText:'已取消登录'}).waitFor();
 await account.getByRole('button',{name:'登录',exact:true}).click();
 await credential.fill('isolated-native-fixture-key');
 await account.getByRole('button',{name:'继续',exact:true}).click();
 await account.getByRole('status').filter({hasText:'登录成功'}).waitFor();
 await page.getByRole('button',{name:'获取模型列表',exact:true}).click();
 await page.getByRole('checkbox',{name:/Model One/}).check();
 await page.getByRole('button',{name:'保存 AI 设置',exact:true}).click();
 await page.getByRole('status').filter({hasText:'已保存'}).waitFor();
 await tabs.getByRole('tab',{name:'高级设置',exact:true}).click();
 await page.getByRole('tab',{name:'内置功能',exact:true}).click();
 await page.getByRole('tab',{name:'内置扩展',exact:true}).click();
 await page.locator('summary:visible').first().click();
 const json=page.getByRole('textbox',{name:'插件选项（JSON）',exact:true}).first();
 await json.fill('{"unfinished":');
 await tabs.getByRole('tab',{name:'插件',exact:true}).click();
 await manager.getByRole('button',{name:'发现',exact:true}).waitFor();
 await tabs.getByRole('tab',{name:'高级设置',exact:true}).click();
 assert.equal(await json.inputValue(),'{"unfinished":','Invalid draft survives leaving advanced settings');
 assert.equal(await page.getByRole('button',{name:'保存 AI 设置',exact:true}).isDisabled(),true);
 await json.fill('{}');
 await page.getByRole('button',{name:'保存 AI 设置',exact:true}).click();
 await page.getByRole('status').filter({hasText:'已保存'}).waitFor();
 await page.screenshot({path:join(output,'advanced-draft.png')});
 await tabs.getByRole('tab',{name:'插件',exact:true}).click();
 await manager.getByRole('button',{name:'发现',exact:true}).waitFor();
 await manager.getByRole('button',{name:/^已安装/}).click();
 await manager.getByText('native-provider',{exact:true}).first().waitFor();
 await page.screenshot({path:join(output,'installed.png')});
 if(process.env.WINONE_AI_SYNC_NATIVE==='1')await checkAISync(page,output);
 assert.deepEqual(errors,[]);
 const cspViolations=await page.evaluate(()=>window.__winoneCSPViolations);
 await writeFile(join(output,'csp-diagnostics.json'),JSON.stringify(await page.evaluate(()=>({violations:window.__winoneCSPViolations,insertions:window.__winoneStyleInsertions,fonts:Array.from(document.fonts,font=>({family:font.family,status:font.status,style:font.style}))})),null,2),'utf8');
 assert.deepEqual(cspViolations,[]);
 await writeFile(join(output,'result.json'),JSON.stringify({native:true,assetMode:production?'embedded production assets':'live Vite review',upstreamManager:true,installed:true,providerNavigation:true,accountSaved:true,signInCanceled:true,apiKeySignIn:true,invalidDraftPreserved:true,errors,cspViolations},null,2),'utf8');
 await page.evaluate(()=>{void window.go.main.App.WindowCommand('close');}).catch(error=>{if(!String(error).includes('closed'))throw error;});
}catch(error){
 if(page){await page.screenshot({path:join(output,'failure.png')}).catch(()=>{});await writeFile(join(output,'failure.txt'),await page.locator('body').innerText().catch(()=>''),'utf8');await writeFile(join(output,'failure-frames.json'),JSON.stringify(await Promise.all(page.frames().map(async frame=>({url:frame.url(),text:await frame.locator('body').innerText().catch(()=>''),}))),null,2),'utf8');}
 await writeFile(join(output,'node-error.txt'),String(error)+'\n'+error.stack+'\n'+errors.join('\n'),'utf8');process.exitCode=1;
}finally{await browser?.close();await server?.close();}

