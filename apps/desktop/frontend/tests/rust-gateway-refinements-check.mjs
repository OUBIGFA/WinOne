// Actual WinOne components on the runner's private desktop and fresh profile.
import {createRequire} from 'node:module';
import {writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import {createServer} from 'node:net';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2);
let browser,page;const transitions=[];
try {
 const deadline=Date.now()+25000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(r=>setTimeout(r,100));}}
 if(!browser)throw Error('Owned UI did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(c=>c.pages()).find(p=>p.url().includes('winone.localhost'));if(page)break;await new Promise(r=>setTimeout(r,100));}
 if(!page)throw Error('Owned UI document missing');
 const errors=[];page.on('pageerror',e=>errors.push(String(e)));
 const nav=()=>page.locator('.app-navigation [data-slot="sidebar-menu-button"]');
 const tabs=()=>page.locator('[data-slot="page-content"]').getByRole('tablist').first();
 const settle=()=>page.evaluate(async()=>{await document.fonts.ready;await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));});
 const compare=(before,after,label)=>{const record={label,before,after};transitions.push(record);if(!before||!after||['x','y','width','height'].some(k=>Math.abs(before[k]-after[k])>0.5))throw Error('Tab navigation moved: '+JSON.stringify(record));};
 const top=()=>page.locator('[data-slot="page-scroll"]').evaluate(e=>{e.scrollTop=0;});
 await page.evaluate(async()=>{
  const app=window.go.main.App,config=JSON.parse(await app.Capture('ai.configuration.get','null'));
  config.providers=[{id:'vendor-work-opaque',name:'工作服务',authMode:'api-key',protocol:'chat',baseUrl:'https://openrouter.ai/api/v1',models:['deployment-01','deployment-02'],modelNames:{'deployment-01':'GPT 5','deployment-02':'Claude Sonnet 4'},hasKey:false},{id:'vendor-personal-opaque',name:'私人账号',authMode:'account',agent:'codex',models:['deployment-01'],modelNames:{'deployment-01':'GPT 5'},hasKey:false}];
  await app.Capture('ai.configuration.save',JSON.stringify({configuration:config}));
 });
 await nav().nth(5).click();await tabs().waitFor();await settle();await top();
 const initial=await tabs().boundingBox();await page.getByRole('tab',{name:'本地网关',exact:true}).click();await page.locator('#gateway-port').waitFor();await settle();
 compare(initial,await tabs().boundingBox(),'custom to gateway');
 const list=page.locator('.gateway-model-list');await list.waitFor();
 const visible=await list.innerText();if(/vendor-work-opaque|vendor-personal-opaque|deployment-0/.test(visible))throw Error('Internal route IDs are visible');
 if(!await list.locator('[data-icon="openrouter-color"], [data-icon="openrouter"]').count()||!await list.locator('[data-icon="openai"]').count()||!await list.locator('[data-icon="claude-color"]').count())throw Error('Provider or model brand icon missing');
 const selected=page.getByRole('checkbox',{name:'工作服务 GPT 5',exact:true});
 await selected.click();await nav().nth(0).click();await page.getByRole('dialog').waitFor();await page.getByRole('button',{name:'继续编辑',exact:true}).click();
 if(await selected.isChecked())throw Error('Unsaved model choice was lost');
 const port=await new Promise(done=>{const server=createServer();server.listen(0,'127.0.0.1',()=>{const p=server.address().port;server.close(()=>done(p));});});
 await page.locator('#gateway-port').fill(String(port));await page.getByRole('switch',{name:'启用本地网关',exact:true}).click();await page.getByRole('button',{name:'保存网关设置',exact:true}).click();await page.getByText('已保存',{exact:true}).waitFor();
 await page.getByRole('button',{name:'显示访问密钥',exact:true}).click();await page.waitForFunction(()=>document.querySelector('#gateway-key')?.value.startsWith('winone-'));
 const key=await page.locator('#gateway-key').inputValue(),headers={Authorization:'Bearer '+key,'Content-Type':'application/json'},base='http://127.0.0.1:'+port+'/v1';
 const models=async()=>{const response=await fetch(base+'/models',{headers});if(response.status!==200)throw Error('Gateway model list: '+response.status);return (await response.json()).data;};
 let published=await models();if(published.length!==2||published.some(m=>m.id==='vendor-work-opaque/deployment-01'))throw Error('Gateway ignored the saved selection');
 const denied=await fetch(base+'/chat/completions',{method:'POST',headers,body:JSON.stringify({model:'vendor-work-opaque/deployment-01',messages:[{role:'user',content:'must not be sent'}]})});if(denied.status!==403)throw Error('Disabled route reached inference: '+denied.status);
 await page.getByRole('button',{name:'全部停用',exact:true}).click();await page.getByRole('button',{name:'保存网关设置',exact:true}).click();await page.getByText('已保存',{exact:true}).waitFor();if((await models()).length)throw Error('Empty selection exposed models');
 await nav().nth(0).click();await nav().nth(5).click();await page.getByRole('tab',{name:'本地网关',exact:true}).click();await page.locator('.gateway-model-list').waitFor();if(await selected.isChecked())throw Error('Reopening reset the model policy');
 await page.getByRole('button',{name:'全部启用',exact:true}).click();await page.getByRole('button',{name:'保存网关设置',exact:true}).click();await page.getByText('已保存',{exact:true}).waitFor();if((await models()).length!==3)throw Error('Enable all failed');
 await page.getByRole('textbox',{name:'搜索供应商或模型',exact:true}).fill('工作 GPT');if(await list.getByRole('checkbox').count()!==2)throw Error('Provider/model search does not compose');await page.getByRole('textbox',{name:'搜索供应商或模型',exact:true}).fill('');
 await page.getByRole('switch',{name:'启用本地网关',exact:true}).click();await page.getByRole('button',{name:'保存网关设置',exact:true}).click();await page.getByText('已保存',{exact:true}).waitFor();

 // Hold a real bridge read to compare loading, failure and retry without a
 // timeout-based fake result. The native bridge is restored before retry.
 await nav().nth(0).click();
 await page.evaluate(()=>{const app=window.go.main.App;window.__appBefore=app;window.go.main.App=new Proxy(app,{get:(target,property)=>property==='Capture'?((method,params)=>method==='ai.configuration.get'?new Promise((resolve,reject)=>{window.__rejectAI=reject;}):target.Capture(method,params)):Reflect.get(target,property)});});
 await nav().nth(5).click();await page.waitForFunction(()=>typeof window.__rejectAI==='function');await top();await settle();const loading=await tabs().boundingBox();
 await page.evaluate(()=>window.__rejectAI(Error('fixture configuration read failed')));await page.getByRole('alert').waitFor();await settle();compare(loading,await tabs().boundingBox(),'AI loading to failure');
 await page.evaluate(()=>{window.go.main.App=window.__appBefore;delete window.__appBefore;delete window.__rejectAI;});await page.getByRole('button',{name:'重试',exact:true}).click();await page.getByRole('button',{name:'添加自定义供应商',exact:true}).waitFor();await settle();compare(loading,await tabs().boundingBox(),'AI failure to loaded');

 for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark']){
  await nav().nth(6).click();await page.getByRole('radio',{name:language==='zh-CN'?'简体中文':'English',exact:true}).click();await page.waitForFunction(value=>document.documentElement.lang===value,language);
  await page.getByRole('radio',{name:language==='zh-CN'?(theme==='light'?'浅色':'深色'):(theme==='light'?'Light':'Dark'),exact:true}).click();await page.waitForFunction(value=>document.documentElement.dataset.theme===value,theme);
  for(const width of [540,800,1280]){
   await page.setViewportSize({width,height:860});
   for(const index of [1,2,3,4,5]){
    await nav().nth(index).click();await tabs().waitFor();await top();await settle();
    const choices=tabs().getByRole('tab'),count=await choices.count();await choices.first().click();await settle();const before=await tabs().boundingBox();
    for(let tab=1;tab<count;tab++){await choices.nth(tab).click();await settle();compare(before,await tabs().boundingBox(),language+'/'+theme+'/'+width+'/page-'+index+'/tab-'+tab);}
    await choices.first().click();await settle();compare(before,await tabs().boundingBox(),language+'/'+theme+'/'+width+'/page-'+index+'/return');
    const overflow=await page.evaluate(()=>[...document.querySelectorAll('html,body,[data-slot="page-scroll"],[data-slot="page-content"],.gateway-model-list')].filter(e=>e.clientWidth>0&&e.scrollWidth>e.clientWidth+1).map(e=>e.getAttribute('data-slot')||e.className||e.tagName));if(overflow.length)throw Error('Horizontal overflow: '+JSON.stringify(overflow));
    if(index===5){await choices.last().click();await list.waitFor();await list.scrollIntoViewIfNeeded();await page.mouse.move(0,0);await page.screenshot({path:resolve(output,language+'-'+theme+'-'+width+'-models.png')});await top();await page.screenshot({path:resolve(output,language+'-'+theme+'-'+width+'-gateway.png')});}
   }
  }
 }
 if(errors.length)throw Error(errors.join('; '));
 await writeFile(resolve(output,'result.json'),JSON.stringify({transitions,modelSelection:true,modelIcons:true,noRouteIDs:true,modelPolicyEnforced:true,unsavedProtection:true,emptySelection:true,errors},null,2));
 await page.evaluate(()=>{void window.go.main.App.WindowCommand('close')});
} catch(error){await writeFile(resolve(output,'node-error.txt'),String(error.stack||error));await writeFile(resolve(output,'transitions.json'),JSON.stringify(transitions,null,2));if(page){await page.locator('#gateway-key').evaluate(e=>{e.value='';}).catch(()=>{});await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});}throw error;}finally{if(browser)await browser.close();}
