// Runs only against the runner's own Rust window and isolated profile.
import {createRequire} from 'node:module';
import {writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import {createServer} from 'node:net';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2);let browser,page;
try {
 const deadline=Date.now()+25000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(r=>setTimeout(r,100));}}
 if(!browser)throw Error('Owned UI did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(c=>c.pages()).find(p=>p.url().includes('winone.localhost'));if(page)break;await new Promise(r=>setTimeout(r,100));}
 if(!page)throw Error('Owned UI document missing');
 const errors=[];page.on('pageerror',e=>errors.push(String(e)));
 const nav=()=>page.locator('.app-navigation [data-slot="sidebar-menu-button"]');
 await page.evaluate(async()=>{const app=window.go.main.App;const config=JSON.parse(await app.Capture('ai.configuration.get','null'));config.providers=[{id:'custom-fixture',name:'自定义服务',authMode:'api-key',protocol:'chat',baseUrl:'https://example.invalid/v1',models:['gpt-5','gemini-2.5-pro'],hasKey:false},{id:'account-fixture',name:'独立账号',authMode:'account',agent:'codex',models:['gpt-5'],hasKey:false}];await app.Capture('ai.configuration.save',JSON.stringify({configuration:config}));});
 await nav().nth(5).click();await page.getByRole('tab',{name:'本地网关',exact:true}).click();await page.getByLabel('端口',{exact:true}).waitFor();
 if(JSON.stringify(await page.getByRole('tab').allTextContents())!==JSON.stringify(['自定义供应商','账号供应商','默认 AI 服务','本地网关']))throw Error('AI tab order differs');
 const port=await new Promise(resolvePort=>{const s=createServer();s.listen(0,'127.0.0.1',()=>{const p=s.address().port;s.close(()=>resolvePort(p));});});
 await page.getByLabel('端口',{exact:true}).fill(String(port));
 await page.getByRole('switch',{name:'启用本地网关'}).click();
 await nav().nth(0).click();await page.getByRole('dialog').waitFor();await page.getByRole('button',{name:'继续编辑',exact:true}).click();
 if(await page.getByLabel('端口',{exact:true}).inputValue()!==String(port))throw Error('Gateway draft lost');
 await page.getByRole('button',{name:'保存网关设置',exact:true}).click();await page.getByText('运行状态: 正在运行',{exact:true}).waitFor();
 await page.getByRole('button',{name:'显示访问密钥',exact:true}).click();
 await page.waitForFunction(()=>document.querySelector('#gateway-key')?.value.startsWith('winone-'));
 const key=await page.getByLabel('访问密钥',{exact:true}).inputValue();
 const response=await fetch('http://127.0.0.1:'+port+'/v1/models',{headers:{Authorization:'Bearer '+key}});
 if(response.status!==200)throw Error('Gateway client HTTP status '+response.status);
 if((await response.json()).data.length!==3)throw Error('Gateway did not expose both configured provider types');
 // Clear the revealed key before retaining any screenshots.
 await page.getByRole('button',{name:'刷新状态',exact:true}).last().click();
 await page.getByRole('switch',{name:'启用本地网关'}).click();await page.getByRole('button',{name:'保存网关设置',exact:true}).click();await page.getByText('运行状态: 已停止',{exact:true}).waitFor();
 const report=[];
 for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark']){
  await nav().nth(6).click();await page.getByRole('radio',{name:language==='zh-CN'?'简体中文':'English',exact:true}).click();
  await page.waitForFunction(l=>document.documentElement.lang===l,language);
  const themeLabel=language==='zh-CN'?(theme==='light'?'浅色':'深色'):(theme==='light'?'Light':'Dark');
  await page.getByRole('radio',{name:themeLabel,exact:true}).click();await page.waitForFunction(t=>document.documentElement.dataset.theme===t,theme);
  for(const width of [540,1040]){
   await page.setViewportSize({width,height:860});
   for(let i=0;i<7;i++){
    await nav().nth(i).click();
    if(i===2){await page.locator('.selection-action-trigger').first().waitFor();const tabs=await page.getByRole('tab').allTextContents();if(tabs[0]!== (language==='zh-CN'?'工具栏':'Toolbar'))throw Error('Selection order differs');}
    if(i===5){await page.getByRole('tab',{name:language==='zh-CN'?'本地网关':'Local gateway',exact:true}).click();await page.locator('#gateway-port').waitFor();}
    if(await page.locator('[data-slot="page-content"] h1').count())throw Error('Page heading remains');
    await page.evaluate(()=>document.fonts.ready);
    const overflow=await page.evaluate(()=>[...document.querySelectorAll('html,body,[data-slot="page-scroll"],[data-slot="page-content"]')].filter(e=>e.clientWidth>0&&e.scrollWidth>e.clientWidth+1).map(e=>e.getAttribute('data-slot')||e.tagName));
    report.push({language,theme,width,page:i,overflow});if(overflow.length)throw Error(JSON.stringify(report.at(-1)));
    await page.mouse.move(0,0);
    if(width===1040 || i===5)await page.screenshot({path:resolve(output,language+'-'+theme+'-'+width+'-page-'+i+'.png')});
   }
  }
 }
 if(errors.length)throw Error(errors.join('; '));
 await writeFile(resolve(output,'result.json'),JSON.stringify({gatewayStartStop:true,thirdPartyModels:true,unsavedProtection:true,report,errors},null,2));
 await page.evaluate(()=>{void window.go.main.App.WindowCommand('close')});
} catch(e){await writeFile(resolve(output,'node-error.txt'),String(e.stack||e));if(page)await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});throw e;}finally{if(browser)await browser.close();}
