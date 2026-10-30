// Actual shipped components, owned test desktop/profile, and a loopback vendor.
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {execFileSync} from 'node:child_process';
import {resolve} from 'node:path';
import {createServer} from 'node:http';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2);let browser,page;
const server=createServer(async(req,res)=>{for await(const _ of req){}res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({id:'fixture-reply',model:'gpt-4o',choices:[{message:{role:'assistant',content:'Test response'},finish_reason:'stop'}],usage:{prompt_tokens:20,completion_tokens:5}}));});
await new Promise(done=>server.listen(0,'127.0.0.1',done));
const vendorPort=server.address().port,report=[];
try{
 // Deterministic published-price fixture; cold refresh and offline behavior
 // are exercised separately by the real adapter's HTTP tests.
 const priceDirectory=resolve(process.env.WINONE_COMPONENTS_PROFILE,'ai-runtime/cache/magpie');
 await mkdir(priceDirectory,{recursive:true});
 await writeFile(resolve(priceDirectory,'models.json'),JSON.stringify({openai:{id:'openai',models:{'gpt-4o':{id:'gpt-4o',cost:{input:2,output:8}}}}}));
 const deadline=Date.now()+25000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(r=>setTimeout(r,100));}}
 if(!browser)throw Error('Owned UI did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(c=>c.pages()).find(p=>p.url().includes('winone.localhost'));if(page)break;await new Promise(r=>setTimeout(r,100));}
 if(!page)throw Error('Owned UI missing');
 const errors=[];page.on('pageerror',error=>errors.push(String(error)));
 const nav=()=>page.locator('.app-navigation [data-slot="sidebar-menu-button"]');
 const content=()=>page.locator('[data-slot="page-content"]');
 const tabs=()=>content().getByRole('tablist').first();
 const settle=()=>page.evaluate(async()=>{await document.fonts.ready;await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));});
 const top=()=>page.locator('[data-slot="page-scroll"]').evaluate(e=>{e.scrollTop=0;});
 const nativeShots=[];
 const nativeShot=name=>{if(process.env.WINONE_WINDOW_SHOT){const path=resolve(output,name+'.png');nativeShots.push(execFileSync(process.env.WINONE_WINDOW_SHOT,[process.env.WINONE_COMPONENTS_PID,path,'-wait','200'],{windowsHide:true,encoding:'utf8',timeout:15000}));if(process.env.WINONE_CHECK_EDGES)nativeShots.push(execFileSync(process.env.WINONE_CHECK_EDGES,[path],{windowsHide:true,encoding:'utf8',timeout:10000}));}};
 await nav().first().waitFor();
 const titles=await nav().evaluateAll(elements=>elements.map(e=>e.getAttribute('aria-label')));
 if(titles[4]!=='程序管理'||titles[5]!=='便捷功能'||titles.includes('置顶功能'))throw Error('Convenience navigation order: '+JSON.stringify(titles));
 await page.evaluate(async port=>{
  const app=window.go.main.App,config=JSON.parse(await app.Capture('ai.configuration.get','null'));
  config.providers=[{id:'usage-fixture',name:'统计测试服务',authMode:'api-key',protocol:'chat',baseUrl:'http://127.0.0.1:'+port+'/v1',models:['gpt-4o',...Array.from({length:20},(_,i)=>'test-model-'+i)],hasKey:false,keys:[{id:'primary',enabled:true,hasKey:false}]}];
  await app.Capture('ai.configuration.save',JSON.stringify({configuration:config,keyUpdates:{'usage-fixture':{primary:'isolated-fixture-key'}}}));
 },vendorPort);
 await page.setViewportSize({width:1200,height:1200});await nav().nth(6).click();await tabs().waitFor();
 const names=await tabs().getByRole('tab').allTextContents();
 if(JSON.stringify(names)!==JSON.stringify(['自定义供应商','账号供应商','默认 AI 服务','本地网关','用量统计']))throw Error('AI tab order: '+JSON.stringify(names));
 await page.getByRole('tab',{name:'本地网关',exact:true}).click();const list=page.locator('.gateway-model-list');await list.waitFor();
 await list.evaluate(e=>e.scrollIntoView({block:'center'}));await settle();let box=await list.boundingBox();const originalHeight=box.height;
 if(Math.abs(originalHeight-400)>2)throw Error('Gateway default height: '+originalHeight);
 await page.mouse.move(box.x+box.width-3,box.y+box.height-3);await page.mouse.down();await page.mouse.move(box.x+box.width-3,box.y+box.height+97,{steps:12});await page.mouse.up();await settle();
 const resized=(await list.boundingBox()).height;if(resized<originalHeight+70)throw Error('Native resize grip did not grow list: '+resized);
 await nav().first().click();await nav().nth(6).click();await page.getByRole('tab',{name:'本地网关',exact:true}).click();await list.waitFor();
 if(Math.abs((await list.boundingBox()).height-resized)>2)throw Error('Gateway height did not persist');
 // A spare loopback port for the owned gateway, independent of the vendor.
 const spare=createServer();await new Promise(done=>spare.listen(0,'127.0.0.1',done));const gatewayPort=spare.address().port;await new Promise(done=>spare.close(done));
 const key=await page.evaluate(async port=>{const app=window.go.main.App;const state=JSON.parse(await app.Capture('ai.gateway.get','null'));await app.Capture('ai.gateway.save',JSON.stringify({settings:{revision:state.revision,enabled:true,port,models:null},resetKey:false}));return JSON.parse(await app.Capture('ai.gateway.key','null')).key;},gatewayPort);
 for(let i=0;i<2;i++){const response=await fetch('http://127.0.0.1:'+gatewayPort+'/v1/chat/completions',{method:'POST',headers:{Authorization:'Bearer '+key,'Content-Type':'application/json'},body:JSON.stringify({model:'usage-fixture/gpt-4o',messages:[{role:'user',content:'isolated test'}]})});if(response.status!==200)throw Error('Gateway request failed: '+response.status);await response.json();}
 await page.getByRole('tab',{name:'用量统计',exact:true}).click();await page.getByRole('region',{name:'请求明细',exact:true}).waitFor();
 const usage=await page.evaluate(async()=>JSON.parse(await window.go.main.App.Capture('ai.usage.get',JSON.stringify({period:'all',provider:'',source:'',failed:false,offset:0}))));
 if(usage.total!==2||usage.sum.input!==40||usage.sum.output!==10)throw Error('End-to-end usage totals: '+JSON.stringify(usage.sum));
 if(usage.sum.cost!==0.00016||usage.sum.unpriced!==0||usage.priceWarning)throw Error('End-to-end price estimate: '+JSON.stringify(usage));
 await page.getByRole('combobox',{name:'来源',exact:true}).click();await page.getByRole('option',{name:'WinOne 功能',exact:true}).click();await page.getByText('此范围内还没有请求记录。',{exact:true}).waitFor();
 await page.getByRole('combobox',{name:'来源',exact:true}).click();await page.getByRole('option',{name:'本地网关',exact:true}).click();await page.getByRole('region',{name:'请求明细',exact:true}).locator('li').first().waitFor();
 if(await page.getByRole('region',{name:'请求明细',exact:true}).locator('li').count()!==2)throw Error('Source filter lost gateway requests');
 await page.getByRole('checkbox',{name:'仅显示失败请求',exact:true}).click();await page.getByText('此范围内还没有请求记录。',{exact:true}).waitFor();await page.getByRole('checkbox',{name:'仅显示失败请求',exact:true}).click();
 await page.evaluate(async()=>{const app=window.go.main.App,state=JSON.parse(await app.Capture('ai.gateway.get','null'));await app.Capture('ai.gateway.save',JSON.stringify({settings:{revision:state.revision,enabled:false,port:state.port,models:null},resetKey:false}));});
 await nav().nth(1).click();await page.getByRole('tab',{name:'截图后',exact:true}).waitFor();
 if(await page.getByRole('button',{name:'窗口截图',exact:true}).count()||await page.getByText('最近截图',{exact:true}).count())throw Error('Removed capture section is visible');
 if(!await content().locator('.lucide-send-horizontal').count())throw Error('Shortcut icon differs');
 await nav().nth(2).click();await page.locator('.selection-action-trigger').first().waitFor();if(!await content().locator('.lucide-code-xml').count())throw Error('Code icon differs');
 await nav().nth(5).click();await page.getByRole('tab',{name:'置顶功能',exact:true}).waitFor();
 const convenienceTabs=await tabs().getByRole('tab').allTextContents();
 if(JSON.stringify(convenienceTabs)!==JSON.stringify(['置顶功能','鼠标手势','文件与目录链接','定时关机']))throw Error('Convenience tab order: '+JSON.stringify(convenienceTabs));
 const enabled=content().getByRole('switch',{name:'置顶功能',exact:true});await enabled.waitFor();
 const initial=await enabled.isChecked();await enabled.click();await page.waitForFunction(expected=>document.querySelector('[aria-labelledby="module-always-on-top-name"]')?.getAttribute('aria-checked')===String(expected),!initial);
 await enabled.click();await page.waitForFunction(expected=>document.querySelector('[aria-labelledby="module-always-on-top-name"]')?.getAttribute('aria-checked')===String(expected),initial);
 const sound=content().getByRole('switch',{name:'播放提示音',exact:true});const originalSound=await sound.isChecked();await sound.click();
 await page.waitForFunction(()=>document.querySelector('[aria-labelledby="module-always-on-top-name"]')?.matches(':disabled,[aria-disabled="true"],[data-disabled]'));
 await nav().first().click();await page.getByRole('dialog').waitFor();await page.locator('[data-slot="dialog-overlay"]').click({position:{x:8,y:8}});await page.getByRole('dialog').waitFor({state:'hidden'});
 if(await sound.isChecked()===originalSound)throw Error('Outside click discarded the convenience draft');
 await page.locator('[data-slot="settings-actions"]').getByRole('button',{name:'撤销修改',exact:true}).click();await page.waitForFunction(()=>!document.querySelector('[aria-labelledby="module-always-on-top-name"]')?.matches(':disabled,[aria-disabled="true"],[data-disabled]'));
 for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark']){
  await nav().nth(7).click();await page.getByRole('radio',{name:language==='zh-CN'?'简体中文':'English',exact:true}).click();await page.waitForFunction(value=>document.documentElement.lang===value,language);
  await page.getByRole('radio',{name:language==='zh-CN'?(theme==='light'?'浅色':'深色'):(theme==='light'?'Light':'Dark'),exact:true}).click();await page.waitForFunction(value=>document.documentElement.dataset.theme===value,theme);
  for(const width of [540,1040]){
   await page.setViewportSize({width,height:860});
   for(let index=0;index<8;index++){
    await nav().nth(index).click();await settle();await top();
    const tablist=tabs(),hasTabs=await tablist.count();
    if(hasTabs){await tablist.waitFor();const choices=tablist.getByRole('tab'),count=await choices.count();await choices.first().click();await settle();await top();const before=await tablist.boundingBox();
     for(let tab=0;tab<count;tab++){await choices.nth(tab).click();await settle();await top();const after=await tablist.boundingBox();if(['x','y','width','height'].some(key=>Math.abs(before[key]-after[key])>0.5))throw Error('Tab group moved: '+JSON.stringify({language,theme,width,index,tab,before,after}));
      if(index===6&&tab===4)await page.getByRole('region',{name:language==='zh-CN'?'请求明细':'Request history',exact:true}).waitFor();
      if(width===1040&&index===6&&tab>=3){await page.mouse.move(0,0);await page.screenshot({path:resolve(output,language+'-'+theme+'-'+index+'-'+tab+'.png')});}
      if(width===1040&&language==='zh-CN'&&theme==='light'&&index===6&&tab===4)nativeShot('native-usage');
     }
     await choices.first().click();await settle();await top();
    }
    const overflow=await page.evaluate(()=>[...document.querySelectorAll('html,body,[data-slot="page-scroll"],[data-slot="page-content"]')].filter(e=>e.clientWidth>0&&e.scrollWidth>e.clientWidth+1).map(e=>e.getAttribute('data-slot')||e.tagName));
    if(overflow.length)throw Error('Horizontal overflow: '+JSON.stringify({language,theme,width,index,overflow}));
    report.push({language,theme,width,index});
    if(width===1040){await page.mouse.move(0,0);await page.screenshot({path:resolve(output,language+'-'+theme+'-'+index+'.png')});}
    if(width===1040&&language==='zh-CN'&&theme==='light'&&[1,5].includes(index))nativeShot('native-page-'+index);
   }
  }
 }
 if(errors.length)throw Error(errors.join('; '));
 await writeFile(resolve(output,'result.json'),JSON.stringify({report,originalHeight,resized,usage:usage.sum,usageFilters:true,convenience:true,unsavedProtection:true,outsideClickKeepsDraft:true,icons:true,captureSettings:true,nativeShots,errors},null,2));
 await page.evaluate(()=>{void window.go.main.App.WindowCommand('close')});
}catch(error){await writeFile(resolve(output,'node-error.txt'),String(error.stack||error));if(page)await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});throw error;}finally{server.close();if(browser)await browser.close();}
