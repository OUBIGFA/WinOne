// Real native file picker + existing WinOne page; all inputs are signed fixtures
// on the runner's private desktop. No user package/profile is selected.
import {createRequire} from 'node:module';
import {copyFile,writeFile} from 'node:fs/promises';
import {execFileSync} from 'node:child_process';
import {resolve} from 'node:path';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2);let browser,page;
try{
 const deadline=Date.now()+25000;while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(r=>setTimeout(r,100));}}
 if(!browser)throw Error('Isolated UI did not start');while(Date.now()<deadline){page=browser.contexts().flatMap(c=>c.pages()).find(p=>p.url().includes('winone.localhost'));if(page)break;await new Promise(r=>setTimeout(r,100));}
 if(!page)throw Error('UI document missing');const errors=[];page.on('pageerror',e=>errors.push(String(e)));
 const choose=async(label='安装模块',file=process.env.WINONE_MODULE_ARCHIVE)=>{await page.getByRole('button',{name:label,exact:true}).click();execFileSync(process.env.WINONE_FILE_DIALOG_PROBE,['--module-file-dialog',process.env.WINONE_COMPONENTS_PID,file],{windowsHide:true,timeout:20000});};
 await page.getByRole('button',{name:'安装模块',exact:true}).waitFor();await choose('安装模块','-');
 if(await page.getByRole('dialog').count())throw Error('Canceling the file picker created a preview');
 // A failed real archive check must remain retryable after the local file is
 // repaired. This follows the native picker and Host without replacing the API.
 const repairable=resolve(output,'repairable.winone-module');await writeFile(repairable,'invalid archive fixture');await choose('安装模块',repairable);
 await page.getByRole('button',{name:'重新检查',exact:true}).waitFor();
 if(await page.getByRole('button',{name:'确认安装',exact:true}).isEnabled())throw Error('Invalid archive can be installed');
 await copyFile(process.env.WINONE_MODULE_ARCHIVE,repairable);await page.getByRole('button',{name:'重新检查',exact:true}).click();
 await page.getByRole('dialog').getByText('签名已验证',{exact:true}).waitFor();await page.getByRole('dialog').getByRole('button',{name:'取消',exact:true}).click();
 const layouts=[];
 for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark'])for(const width of [620,1280]){
  await page.evaluate(async({language,theme})=>{const api=window.go.main.App;const current=await api.Snapshot();await api.SaveSettings({...current.settings,language,theme});},{language,theme});await page.setViewportSize({width,height:900});await page.reload();
  const en=language==='en-US';await page.getByRole('button',{name:en?'Install module':'安装模块',exact:true}).waitFor();await choose(en?'Install module':'安装模块');
  const dialog=page.getByRole('dialog');await dialog.getByText(en?'Signature verified':'签名已验证',{exact:true}).waitFor();
  await page.waitForFunction(()=>[...document.querySelectorAll('button')].some(b=>['确认安装','Confirm installation'].includes(b.textContent)&&!b.disabled));
  const overflow=await dialog.evaluate(node=>node.scrollWidth>node.clientWidth+1);if(overflow)throw Error('Preview overflow');
  await page.screenshot({path:resolve(output,`preview-${language}-${theme}-${width}.png`)});layouts.push({language,theme,width,overflow});
  await dialog.getByRole('button',{name:en?'Cancel':'取消',exact:true}).click();await dialog.waitFor({state:'hidden'});
 }
 await page.evaluate(async()=>{const api=window.go.main.App;const current=await api.Snapshot();await api.SaveSettings({...current.settings,language:'zh-CN',theme:'light'});});await page.setViewportSize({width:1280,height:900});await page.reload();
 if(await page.getByRole('switch',{name:'测试工具',exact:true}).count())throw Error('Canceled previews installed a module');
 await choose();await page.getByRole('dialog').getByText('签名已验证',{exact:true}).waitFor();await page.getByRole('button',{name:'确认安装',exact:true}).click();
 await page.getByText('模块已安装，尚未开启',{exact:true}).waitFor();const toggle=page.getByRole('switch',{name:'测试工具',exact:true});await toggle.waitFor();if(await toggle.isChecked())throw Error('Install enabled the module');
 await toggle.scrollIntoViewIfNeeded();await page.screenshot({path:resolve(output,'installed-disabled.png')});
 let updated=false,rolledBack=false;
 if(process.env.WINONE_MODULE_UPDATE_ARCHIVE){
  for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark'])for(const width of [620,1280]){
   await page.evaluate(async({language,theme})=>{const api=window.go.main.App;const current=await api.Snapshot();await api.SaveSettings({...current.settings,language,theme});},{language,theme});await page.setViewportSize({width,height:900});await page.reload();
   const en=language==='en-US';await choose(en?'Install module':'安装模块',process.env.WINONE_MODULE_UPDATE_ARCHIVE);
   const dialog=page.getByRole('dialog');await dialog.getByText('1.0.0 → 1.1.0',{exact:true}).waitFor();await dialog.getByRole('button',{name:en?'Confirm update':'确认更新',exact:true}).waitFor();
   if(process.env.WINONE_MODULE_EXPECT_MIGRATION==='1'){
    await dialog.getByText(en?'Settings format changes from version 1 to 2. Original settings are restored if the update fails.':'配置格式将从版本 1 转为 2；失败时恢复原配置。',{exact:true}).waitFor();
    await dialog.getByText(en?'Affected settings fields':'涉及的配置字段',{exact:true}).click();await dialog.getByText('/label',{exact:true}).waitFor();
   }
   const overflow=await dialog.evaluate(node=>node.scrollWidth>node.clientWidth+1);if(overflow)throw Error('Update preview overflow');
   await page.screenshot({path:resolve(output,`update-${language}-${theme}-${width}.png`)});layouts.push({phase:'update',language,theme,width,overflow});
   await dialog.getByRole('button',{name:en?'Cancel':'取消',exact:true}).click();await dialog.waitFor({state:'hidden'});
  }
  await page.evaluate(async()=>{const api=window.go.main.App;const current=await api.Snapshot();await api.SaveSettings({...current.settings,language:'zh-CN',theme:'light'});});await page.setViewportSize({width:1280,height:900});await page.reload();
  await choose('安装模块',process.env.WINONE_MODULE_UPDATE_ARCHIVE);await page.getByRole('button',{name:'确认更新',exact:true}).click();await page.getByText('模块已更新，配置和启用状态已保留',{exact:true}).waitFor();if(await toggle.isChecked())throw Error('Update enabled the module');updated=true;
  if(process.env.WINONE_MODULE_BROKEN_ARCHIVE){
   await choose('安装模块',process.env.WINONE_MODULE_BROKEN_ARCHIVE);await page.getByRole('button',{name:'确认更新',exact:true}).click();await page.getByText(/更新失败，已恢复旧版本/).waitFor();
   await page.screenshot({path:resolve(output,'update-rolled-back.png')});await page.getByRole('dialog').getByRole('button',{name:'取消',exact:true}).click();if(await toggle.isChecked())throw Error('Rollback enabled the module');rolledBack=true;
  }
 }
 await page.getByRole('button',{name:'卸载模块',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'取消',exact:true}).click();await toggle.waitFor();
 await page.getByRole('button',{name:'卸载模块',exact:true}).click();await page.getByRole('button',{name:'确认卸载',exact:true}).click();await page.getByText('模块已卸载，配置已保留',{exact:true}).waitFor();if(await toggle.count())throw Error('Uninstalled module remained listed');
 await choose('安装模块',process.env.WINONE_MODULE_UPDATE_ARCHIVE||process.env.WINONE_MODULE_ARCHIVE);await page.getByText('将恢复保留的配置，安装后保持关闭。',{exact:true}).waitFor();await page.getByRole('button',{name:'确认安装',exact:true}).click();await toggle.waitFor();if(await toggle.isChecked())throw Error('Reinstall enabled the module');
 if(errors.length)throw Error(errors.join('\n'));await writeFile(resolve(output,'result.json'),JSON.stringify({nativePicker:true,nativeCancel:true,failedCheckRetried:true,previewCancel:true,installedDisabled:true,updated,rolledBack,migrationPreview:process.env.WINONE_MODULE_EXPECT_MIGRATION==='1',uninstall:true,reinstall:true,layouts,errors},null,2));
 await page.evaluate(()=>{void window.go.main.App.WindowCommand('close');}).catch(e=>{if(!String(e).includes('closed'))throw e;});
}catch(error){await writeFile(resolve(output,'node-error.txt'),String(error.stack||error));if(page){await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});await writeFile(resolve(output,'failure.txt'),await page.locator('body').innerText().catch(()=>''));}throw error;}finally{if(browser)await browser.close();}
