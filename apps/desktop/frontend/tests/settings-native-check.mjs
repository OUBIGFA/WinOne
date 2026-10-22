// Run with build/test-desktop-components.ps1 -FullBundle and a repository-owned
// profile: saving the global theme also applies settings to bundled workers.
// This driver exercises a real WinOne process, never the user's installed app.
import {createRequire} from 'node:module';
import {readFile,writeFile,mkdir} from 'node:fs/promises';
import {resolve,join,extname,sep} from 'node:path';
import {execFileSync} from 'node:child_process';
import {pathToFileURL} from 'node:url';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2),errors=[],views=[],interactions=[];
let browser,page,devServer;
try{
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(r=>setTimeout(r,100));}}
 if(!browser)throw Error('Isolated WinOne did not expose its test connection');
 while(Date.now()<deadline){page=browser.contexts().flatMap(c=>c.pages()).find(p=>p.url().includes('winone.localhost'));if(page)break;await new Promise(r=>setTimeout(r,100));}
 if(!page)throw Error('The native settings document was not found');
 page.setDefaultTimeout(12000);
 page.on('pageerror',error=>errors.push(error.stack||String(error)));
 const assets=process.env.WINONE_SETTINGS_ASSETS&&resolve(process.env.WINONE_SETTINGS_ASSETS);
 const live=process.env.WINONE_SETTINGS_DEV==='1';
 if(assets&&live)throw Error('Choose frozen assets or live development UI, not both');
 if(live){
  // Review current production components in a runner-owned native window
  // without compiling/replacing the application. Host IPC and profile saves
  // remain real; only static UI requests are served by the local Vite server.
  const frontend=resolve('apps/desktop/frontend');
  const {createServer}=await import(pathToFileURL(join(frontend,'node_modules/vite/dist/node/index.js')));
  devServer=await createServer({root:frontend,configFile:join(frontend,'vite.config.ts'),server:{host:'127.0.0.1',port:5207,strictPort:true,hmr:false}});
  await devServer.listen();
  await page.locator('.app-navigation').waitFor({timeout:30000});
  await page.route('http://winone.localhost/**',async route=>{
   const url=new URL(route.request().url());
   const path=url.pathname==='/'||url.pathname==='/index.html'?'/tests/settings-native-preview.html':url.pathname;
   const response=await fetch('http://127.0.0.1:5207'+path+url.search);
   await route.fulfill({status:response.status,contentType:response.headers.get('content-type')||'application/octet-stream',body:Buffer.from(await response.arrayBuffer())});
  });
  await page.reload({waitUntil:'domcontentloaded',timeout:30000});
 }
 if(assets){
  // Finish the bundled document's initial font requests before switching
  // resources. Otherwise a late old-font request is misclassified as a missing
  // file in the reviewed UI even though the new document never references it.
  await page.locator('.app-navigation').waitFor({timeout:30000});
  await page.waitForLoadState('load');
  await page.evaluate(async()=>{await document.fonts.ready;});
  const types={'.html':'text/html','.js':'text/javascript','.css':'text/css','.svg':'image/svg+xml','.png':'image/png','.woff2':'font/woff2','.woff':'font/woff','.json':'application/json','.wasm':'application/wasm'};
  await page.route('http://winone.localhost/**',async route=>{
   const pathname=decodeURIComponent(new URL(route.request().url()).pathname),file=resolve(assets,'.'+pathname);
   if(!file.startsWith(assets+sep))throw Error('UI asset escaped its isolated folder');
   try{await route.fulfill({body:await readFile(file),contentType:types[extname(file)]||'application/octet-stream'});}
   catch(error){errors.push('Missing UI asset: '+pathname+' '+error);await route.fulfill({status:404,body:'Missing isolated UI asset'});}
  });
  await page.reload({waitUntil:'domcontentloaded',timeout:30000});
 }
 await page.locator('.app-navigation').waitFor({timeout:30000});
 const resize=join(output,'resize-window.ps1');
 await writeFile(resize,[
  'param([int]$OwnerPID,[string]$ExpectedExe,[int]$Width,[int]$Height)',
  '$ErrorActionPreference="Stop"',
  '$p=Get-Process -Id $OwnerPID',
  'if($p.Path -ne $ExpectedExe){throw "The window does not belong to this check"}',
  'Add-Type -TypeDefinition @\'',
  'using System;using System.Runtime.InteropServices;public static class NativeSettingsSize{[DllImport("user32.dll")]public static extern bool SetProcessDPIAware();[DllImport("user32.dll",SetLastError=true)]public static extern bool SetWindowPos(IntPtr h,IntPtr a,int x,int y,int w,int height,uint flags);}',
  '\'@',
  '[void][NativeSettingsSize]::SetProcessDPIAware()',
  'if(-not [NativeSettingsSize]::SetWindowPos($p.MainWindowHandle,[IntPtr]::Zero,40,40,$Width,$Height,0x14)){throw "Window resize failed"}',
 ].join('\n'),'utf8');
 const settle=async()=>{
  await page.evaluate(async()=>{await document.fonts.ready;await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);});
  // Catalogs load after a tab mounts. Do not count a loading screen as a
  // reviewed page or miss the nested tabs that arrive with its real data.
  await page.waitForFunction(()=>[...document.querySelectorAll('[data-settings-page] [role="status"]')].every(node=>
   !node.getClientRects().length||! /^(正在读取|正在加载|Loading|Reading)/i.test(node.textContent.trim())));
 };
 const setSize=async width=>{execFileSync('powershell.exe',['-NoProfile','-File',resize,'-OwnerPID',process.env.WINONE_COMPONENTS_PID,'-ExpectedExe',join(output,'WinOne.exe'),'-Width',String(width),'-Height','940'],{windowsHide:true});await settle();};
 const open=async id=>{
  await page.locator('.app-navigation').locator(`[data-page-id="${id}"]`).click();
  await page.locator('[data-settings-page="'+id+'"]').waitFor();await settle();
 };
 const inspect=async(id,path,variant)=>{
  // The real Host can populate a page differently from the browser fixture.
  // Check every scroll position here too, so nested panels cannot hide Save.
  const viewport=page.locator('[data-slot="page-scroll"]');
  const scrollSize=await viewport.evaluate(root=>Math.max(0,root.scrollHeight-root.clientHeight));
  const footers=[];
  for(const top of [...new Set([0,Math.round(scrollSize/2),scrollSize])]){
   await viewport.evaluate((root,top)=>{root.scrollTop=top;},top);await settle();
   const result=await viewport.evaluate(root=>{
    const visible=node=>node.getClientRects().length>0&&!node.closest('[hidden],[aria-hidden="true"]');
    const shell=root.parentElement,clip=shell.getBoundingClientRect();
    const footer=shell.querySelector('[data-slot="settings-footer"]'),footerRect=footer?.getBoundingClientRect();
    const navigation=[...shell.querySelectorAll('[data-slot="page-scroll-controls"] button')].map(node=>node.getBoundingClientRect());
    const bars=[...shell.querySelectorAll('.settings-save-bar')].filter(visible).map(bar=>{
     const rect=bar.getBoundingClientRect();
     return {inFooter:!!bar.closest('[data-slot="settings-footer"]'),visible:rect.top>=clip.top-1&&rect.bottom<=clip.bottom+1,buttons:[...bar.querySelectorAll('button')].filter(visible).map(button=>{
      const bounds=button.getBoundingClientRect(),hit=document.elementFromPoint(bounds.x+bounds.width/2,bounds.y+bounds.height/2);
      return {name:button.textContent.trim(),visible:bounds.top>=clip.top-1&&bounds.bottom<=clip.bottom+1,unobscured:!!hit&&bar.contains(hit)};
     })};
    });
    const unanchored=[...shell.querySelectorAll('button')].filter(visible).filter(button=>/^(保存(设置|快捷键)?|Save( settings| shortcuts)?)$/i.test(button.textContent.trim())&&!button.closest('.settings-save-bar')).map(button=>button.textContent.trim());
    return {scrollTop:root.scrollTop,bars,unanchored,footerAtBottom:!footerRect?.height||Math.abs(footerRect.bottom-clip.bottom)<=1,
     navigationInRow:navigation.length!==2||Math.abs(navigation[0].top-navigation[1].top)<=1&&navigation[0].right<=navigation[1].left};
   });
   footers.push(result);
   assert.ok(result.footerAtBottom&&result.navigationInRow&&result.bars.every(bar=>bar.inFooter), 'Native footer stays at bottom with horizontal navigation');
   assert.equal(result.unanchored.length,0,`Native save outside footer: ${id} ${path}`);
   assert.ok(result.bars.every(bar=>bar.visible&&bar.buttons.every(button=>button.visible&&button.unobscured)),`Native save hidden after scrolling: ${id} ${path} ${top}`);
  }
  await viewport.evaluate(root=>{root.scrollTop=0;});await settle();
  const metrics=await page.locator('[data-settings-page]').evaluate(root=>{
   const visible=e=>e.getBoundingClientRect().width>0&&e.getBoundingClientRect().height>0;
   const containers=[root,root.parentElement,...root.querySelectorAll('.settings-group,.selection-action-row,.wintray-program-list,.wintray-program-detail')];
   return {width:innerWidth,height:innerHeight,overflow:containers.filter(e=>visible(e)&&e.scrollWidth>e.clientWidth+1).map(e=>({name:e.className,client:e.clientWidth,scroll:e.scrollWidth})),text:root.innerText.slice(0,180)};
  });
  const privacyModel=page.locator('[aria-label="本地模型辅助脱敏"],[aria-label="Find with a local model"]');
  if(id==='ai-services'&&await privacyModel.isVisible()){
   await privacyModel.getByRole('switch').waitFor();
   await page.waitForFunction(()=>!document.querySelector('[aria-label="本地模型辅助脱敏"] [role="status"],[aria-label="Find with a local model"] [role="status"]'));
   assert.equal(await privacyModel.getByRole('alert').count(),0,'Native privacy status must reach the real Host');
  }
  const file=String(views.length+1).padStart(3,'0')+'-'+variant+'-'+id+'-'+(path.join('-')||'default')+'.png';
  if(process.env.WINONE_WINDOW_SHOT_EXE)execFileSync(process.env.WINONE_WINDOW_SHOT_EXE,[process.env.WINONE_COMPONENTS_PID,join(output,file),'-wait','250'],{windowsHide:true});
  else await page.screenshot({path:join(output,file)});
  assert.equal(await viewport.getAttribute('data-page'),id,'Screenshot must not change the page under review');
  views.push({id,path,variant,file,footers,...metrics});
  if(metrics.overflow.length)throw Error('Native horizontal overflow: '+JSON.stringify(views.at(-1)));
 };
 async function tabs(id,variant,depth=0,path=[]){
  const list=page.getByRole('tablist').nth(depth);
  if(!await list.count()||depth>2){await inspect(id,path,variant);return;}
  const names=await list.getByRole('tab').allTextContents();
  for(let index=0;index<names.length;index++){
   await page.getByRole('tablist').nth(depth).getByRole('tab',{name:names[index].trim(),exact:true}).click();await settle();
   await tabs(id,variant,depth+1,[...path,index]);
  }
 }
 // The native open request and initial module read complete after navigation
 // first appears. Start user interactions after that initial Home is ready.
 await page.locator('.dashboard-tool').first().waitFor();
 // A fresh profile follows Windows language. Fix the test's baseline through
 // its owned Host so restoring it later does not unexpectedly select English.
 await page.evaluate(async()=>{
  const app=window.go.main.App;
  // Test setup can race the initial module discovery action. Retry only an
  // explicitly refused admission (no configuration job was accepted).
  const deadline=Date.now()+5000;
  for(;;){try{const snapshot=await app.Snapshot();await app.SaveSettings({...snapshot.settings,theme:'light',language:'zh-CN',defaultPage:'overview',startup:{...snapshot.settings.startup,keepUIRunning:false}});break;}
   catch(error){if(!String(error).includes('(configuration_busy)')||Date.now()>=deadline)throw error;await new Promise(done=>setTimeout(done,100));}}
 });
 await page.reload({waitUntil:'domcontentloaded'});await page.locator('.dashboard-tool').first().waitFor();
 await page.evaluate(()=>window.go.main.App.SettingsNavigation());await settle();
 await open('settings');
 await setSize(1280);
 const original=await page.evaluate(async()=>structuredClone((await window.go.main.App.Snapshot()).settings));
 // Use real controls, then re-read the owned Host profile and reload the UI.
 // All module switches, startup tasks and hotkeys are off in this fixture.
 const theme=page.getByRole('radiogroup',{name:'主题',exact:true});
 // A label is clickable even while its nested radio is disabled during the
 // initial Host read. Wait for the actual control before testing empty space.
 await page.locator('#theme-dark:not([data-disabled]):not(:disabled)').waitFor();
 const dark=theme.locator('label.settings-choice').filter({hasText:'深色'}),box=await dark.boundingBox();
 await dark.click({position:{x:box.width-12,y:box.height/2}});
 await page.waitForFunction(()=>document.documentElement.dataset.theme==='dark');
 await theme.getByRole('radio',{name:'深色',exact:true}).focus();
 await page.keyboard.press('ArrowLeft');
 await page.waitForFunction(()=>document.documentElement.dataset.theme==='light');
 await theme.getByRole('radio',{name:'深色',exact:true}).click();
 await page.waitForFunction(()=>document.documentElement.dataset.theme==='dark');
 await page.getByRole('radio',{name:'English',exact:true}).click();
 await page.getByRole('radiogroup',{name:'Language',exact:true}).waitFor();
 await page.getByRole('radio',{name:'Icons and names',exact:true}).click();
 await page.getByRole('combobox',{name:'Start page',exact:true}).click();
 await page.getByRole('option',{name:'Capture assistant',exact:true}).click();
 await page.waitForFunction(async()=>(await window.go.main.App.Snapshot()).settings.defaultPage==='capture');
 // Snapshot may observe the pending in-memory revision. A re-enabled field
 // means the UI has received the completed save, including the durable commit.
 await page.locator('#default-page:not([data-disabled]):not(:disabled)').waitFor();
 assert.equal(await page.getByRole('alert').count(),0);
 const saved=JSON.parse(await readFile(join(process.env.WINONE_COMPONENTS_PROFILE,'configuration.json'),'utf8')).committed.values.host;
 assert.equal(saved.theme,'dark');assert.equal(saved.language,'en-US');assert.equal(saved.defaultPage,'capture');
 await page.reload({waitUntil:'domcontentloaded'});
 await page.locator('[data-settings-page="capture"]').waitFor();
 await open('settings');
 assert.equal(await page.getByRole('radio',{name:'Dark',exact:true}).getAttribute('aria-checked'),'true');
 assert.equal(await page.getByRole('radio',{name:'English',exact:true}).getAttribute('aria-checked'),'true');
 assert.equal(await page.getByRole('radio',{name:'Icons and names',exact:true}).getAttribute('aria-checked'),'true');
 assert.ok((await page.locator('.app-navigation').boundingBox()).width>=180);
 assert.match(await page.getByRole('combobox',{name:'Start page',exact:true}).textContent(),/Capture assistant/);
 interactions.push('whole choice target / theme keyboard / real Host disk save / language / sidebar / start page after reload');
 await page.getByRole('radio',{name:'Icons only',exact:true}).click();
 await page.evaluate(async original=>{const api=window.go.main.App,s=await api.Snapshot();await api.SaveSettings({...s.settings,theme:original.theme,language:original.language,defaultPage:original.defaultPage});},original);
 await page.reload({waitUntil:'domcontentloaded'});await page.locator('.dashboard-tool').first().waitFor();
 // Save a harmless filename preference through the manual footer, then check
 // its durable value after reload. Navigation cancellation must retain draft.
 await open('capture');
 await page.getByRole('tab',{name:'文件命名',exact:true}).click();await settle();
 const template=page.getByRole('textbox',{name:'文件名模板',exact:true});
 const originalTemplate=await template.inputValue(),savedTemplate='WinOne-footer-check-{timestamp}';
 await template.fill(savedTemplate);
 await page.locator('.app-navigation [data-page-id="overview"]').click();
 await page.getByRole('dialog').waitFor();
 await page.getByRole('button',{name:'继续编辑',exact:true}).click();
 assert.equal(await template.inputValue(),savedTemplate,'Navigation cancellation keeps the manual-save draft');
 await page.getByRole('tab',{name:'截图后',exact:true}).click();await settle();
 await page.locator('[data-slot="page-scroll"]').evaluate(root=>{root.scrollTop=0;});await settle();
 await page.getByRole('button',{name:'保存设置',exact:true}).click();
 await page.getByRole('status').filter({hasText:'已保存'}).waitFor();
 const savedCapture=()=>readFile(join(process.env.WINONE_COMPONENTS_PROFILE,'configuration.json'),'utf8').then(text=>JSON.parse(text).committed.values.capture);
 assert.equal((await savedCapture()).fileNameTemplate,savedTemplate);
 await page.reload({waitUntil:'domcontentloaded'});await page.locator('.app-navigation').waitFor();
 await open('capture');await page.getByRole('tab',{name:'文件命名',exact:true}).click();await settle();
 assert.equal(await template.inputValue(),savedTemplate,'Manual footer save survives reload');
 await template.fill(originalTemplate);await page.getByRole('button',{name:'保存设置',exact:true}).click();
 await page.getByRole('status').filter({hasText:'已保存'}).waitFor();
 assert.equal((await savedCapture()).fileNameTemplate,originalTemplate);
 interactions.push('manual footer save from the top of a long page / navigation draft protection / disk commit / reload / original value restored');
 for(const variant of ['wide-light-zh','narrow-dark-en']){
  if(variant==='narrow-dark-en'){
   await page.evaluate(async()=>{const api=window.go.main.App,s=await api.Snapshot();await api.SaveSettings({...s.settings,theme:'dark',language:'en-US'});});
   await page.reload({waitUntil:'domcontentloaded'});await page.locator('.app-navigation').waitFor();
  }
  await setSize(variant==='wide-light-zh'?1280:620);
  for(const id of ['overview','settings','capture','selection','quick-assistant','wintray','convenience','ai-services']){await open(id);await tabs(id,variant);}
 }
 if(errors.length)throw Error(errors.join('\n'));
 await writeFile(join(output,'result.json'),JSON.stringify({native:true,assetMode:live?'live Vite UI (development CSP only)':assets?'frozen UI review':'bundled UI',interactions,views,errors},null,2));
 await page.evaluate(()=>{void window.go.main.App.WindowCommand('close');}).catch(error=>{if(!String(error).includes('closed'))throw error;});
}catch(error){
 if(page){
  await page.screenshot({path:join(output,'failure.png')}).catch(failure=>errors.push('Failure screenshot unavailable: '+failure));
  const state=await page.evaluate(()=>({url:location.href,scripts:[...document.scripts].map(node=>node.src),styles:[...document.querySelectorAll('link[rel="stylesheet"]')].map(node=>node.href),saveButtons:[...document.querySelectorAll('button')].filter(node=>/^(保存|Save)/.test(node.textContent.trim())).map(node=>node.outerHTML),theme:document.documentElement.dataset.theme,focused:document.activeElement?.outerHTML.slice(0,2000),text:document.querySelector('[data-slot="page-content"]')?.textContent})).catch(failure=>({error:String(failure)}));
  await writeFile(join(output,'failure-state.json'),JSON.stringify(state,null,2));
 }
 await writeFile(join(output,'result.json'),JSON.stringify({native:true,interactions,views,errors},null,2));
 await writeFile(join(output,'node-error.txt'),String(error)+'\n'+error.stack+'\n'+errors.join('\n'));
 process.exitCode=1;
}finally{await browser?.close();await devServer?.close();}
