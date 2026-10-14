// Explicit isolated desktop check: real bundled plugin prompts, then synthetic
// device authorization progress. No vendor login or credential submission.
import {createRequire} from 'node:module';
import {writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2);let browser,page;
try {
 const deadline=Date.now()+25000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(resolve=>setTimeout(resolve,100));}}
 if(!browser)throw Error('Isolated WebView2 did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(p=>p.url().includes('winone.localhost'));if(page)break;await new Promise(resolve=>setTimeout(resolve,100));}
 if(!page)throw Error('Settings document did not load');
 const errors=[];page.on('pageerror',e=>errors.push(String(e)));
 await page.getByRole('button',{name:'AI服务',exact:true}).click();
 await page.getByRole('tab',{name:'账号供应商',exact:true}).click();
 await page.getByRole('button',{name:'添加账号',exact:true}).click();
 await page.getByRole('dialog').getByRole('button',{name:'Cline',exact:true}).click();
 await page.getByRole('combobox',{name:'登录方式',exact:true}).click();
 await page.getByRole('option',{name:/Cline API key/}).click();
 await page.getByRole('button',{name:'登录',exact:true}).click();
 const key=page.locator('#ai-signin-input');await key.waitFor();
 if(await key.getAttribute('type')!=='password')throw Error('Bundled plugin did not expose its protected key prompt');
 await key.fill('isolated-unsent-fixture');
 await key.scrollIntoViewIfNeeded();await page.screenshot({path:resolve(output,'cline-key-prompt.png')});
 await page.getByRole('button',{name:'取消登录',exact:true}).click();
 await page.getByText('已取消登录。',{exact:true}).waitFor();
 if(await key.count())throw Error('Canceled secret input remained visible');
 await page.getByRole('combobox',{name:'登录方式',exact:true}).click();
 await page.getByRole('option',{name:'Sign in to Cline (browser)',exact:true}).click();
 // Exercise device UI in the built frontend without contacting WorkOS/Cline.
 await page.evaluate(()=>{
  const app=window.go.main.App,original=app.Capture.bind(app);
  const capture=async(method,payload)=>{
   if(method.startsWith('ai.accounts.signin.'))return JSON.stringify({id:'isolated-device',status:{agent:'cline',state:method.endsWith('.cancel')?'canceled':'waiting',url:'https://example.invalid/device',code:'HFPM-MCLY',instructions:'Approve the sign-in in the browser — the code is HFPM-MCLY.'}});
   return original(method,payload);
  };
  // The Rust bridge returns methods from a get-only Proxy; wrap the bridge
  // rather than assigning a property that its getter would ignore.
  window.go.main.App=new Proxy({}, {get:(_target,name)=>name==='Capture'?capture:app[name]});
 });
 await page.getByRole('button',{name:'登录',exact:true}).click();
 await page.getByLabel('设备授权码',{exact:true}).waitFor();
 if(await page.getByLabel('设备授权码',{exact:true}).inputValue()!=='HFPM-MCLY')throw Error('Device code not visible');
 await page.getByRole('button',{name:'复制设备码',exact:true}).waitFor();
 await page.getByRole('button',{name:'复制登录链接',exact:true}).waitFor();
 if(await page.getByRole('button',{name:'提交',exact:true}).count())throw Error('Device flow incorrectly accepts a code back into WinOne');
 await page.getByLabel('设备授权码',{exact:true}).scrollIntoViewIfNeeded();await page.screenshot({path:resolve(output,'cline-device-code.png')});
 const overflow=await page.locator('[data-slot="page-scroll"]').evaluate(e=>e.scrollWidth>e.clientWidth+1);
 await page.getByRole('button',{name:'取消登录',exact:true}).click();
 await page.getByText('已取消登录。',{exact:true}).waitFor();
 await page.getByRole('button',{name:'保存 AI 设置',exact:true}).click();
 await page.getByText('已保存',{exact:true}).waitFor();
 if(errors.length||overflow)throw Error(JSON.stringify({errors,overflow}));
 await writeFile(resolve(output,'result.json'),JSON.stringify({realPluginKeyPrompt:true,cancelClearsSecret:true,deviceCodeFixture:true,copyControls:true,noDeviceCodeSubmission:true,providerSavedWithoutCredentials:true,overflow,errors},null,2));
 await page.evaluate(()=>{void window.go.main.App.WindowCommand('close');}).catch(e=>{if(!String(e).includes('closed'))throw e;});
}catch(error){await writeFile(resolve(output,'node-error.txt'),String(error.stack||error));if(page){await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});await writeFile(resolve(output,'failure.txt'),await page.locator('body').innerText().catch(()=>''));}throw error;}
finally{if(browser)await browser.close();}
