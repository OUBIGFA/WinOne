// Real product page and native picker on the runner's isolated desktop.
import {createRequire} from 'node:module';
import {readFile,writeFile} from 'node:fs/promises';
import {execFileSync} from 'node:child_process';
import {resolve} from 'node:path';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2);let browser,page;
try{
 const fixtures=JSON.parse(await readFile(process.env.WINONE_SHARED_FIXTURES,'utf8'));
 const deadline=Date.now()+25000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(r=>setTimeout(r,100));}}
 if(!browser)throw Error('Isolated UI did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(c=>c.pages()).find(p=>p.url().includes('winone.localhost'));if(page)break;await new Promise(r=>setTimeout(r,100));}
 if(!page)throw Error('UI document missing');
 const errors=[];page.on('pageerror',error=>errors.push(String(error)));
 const choose=async(file,label='安装模块')=>{
  await page.getByRole('button',{name:label,exact:true}).click();
  execFileSync(process.env.WINONE_FILE_DIALOG_PROBE,['--module-file-dialog',process.env.WINONE_COMPONENTS_PID,fixtures[file]],{windowsHide:true,timeout:20000});
 };
 const install=async(file,message)=>{
  await choose(file);const dialog=page.getByRole('dialog');await dialog.getByText('签名已验证',{exact:true}).waitFor();
  await dialog.getByRole('button',{name:'确认安装',exact:true}).click();await page.getByText(message,{exact:true}).waitFor();
 };
 await page.getByRole('button',{name:'安装模块',exact:true}).waitFor();
 await choose('example.first.winone-module');
 await page.getByRole('button',{name:'重新检查',exact:true}).waitFor();
 if(await page.getByRole('button',{name:'确认安装',exact:true}).isEnabled())throw Error('Missing dependency was installable');
 await page.getByRole('dialog').getByRole('button',{name:'取消',exact:true}).click();
 await install('shared-1.0.0.winone-runtime','共享依赖已安装');
 await install('shared-2.0.0.winone-runtime','共享依赖已安装');
 if(await page.getByRole('switch',{name:'测试工具',exact:true}).count())throw Error('Runtime created a module switch');
 await install('example.first.winone-module','模块已安装，尚未开启');
 const tool=page.getByRole('listitem').filter({has:page.getByRole('switch',{name:'测试工具',exact:true})});
 await tool.getByText('依赖 · 1',{exact:true}).click();await tool.getByText('example.shared · 1.0.0',{exact:true}).waitFor();
 const layouts=[];
 for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark'])for(const width of [620,1280]){
  await page.evaluate(async({language,theme})=>{const api=window.go.main.App;const snapshot=await api.Snapshot();await api.SaveSettings({...snapshot.settings,language,theme});},{language,theme});
  await page.setViewportSize({width,height:900});await page.reload();
  const en=language==='en-US';const section=page.getByRole('region',{name:en?'Shared dependencies':'共享依赖'});await section.waitFor();await section.scrollIntoViewIfNeeded();
  const rows=section.getByRole('listitem');if(await rows.count()!==2)throw Error('Shared runtime versions are missing');
  const used=rows.filter({hasText:'1.0.0'}),unused=rows.filter({hasText:'2.0.0'});
  const remove=en?'Remove dependency':'卸载依赖';
  if(await used.getByRole('button',{name:remove}).isEnabled())throw Error('Referenced version offered removal');
  if(!await unused.getByRole('button',{name:remove}).isEnabled())throw Error('Unused version has no removal entry');
  await used.getByText(en?'Used by Test tool':'由 测试工具 使用',{exact:true}).waitFor();
  const overflow=await section.evaluate(node=>node.scrollWidth>node.clientWidth+1);
  if(overflow)throw Error('Shared dependency section overflows');
  await page.screenshot({path:resolve(output,`shared-${language}-${theme}-${width}.png`)});
  await unused.getByRole('button',{name:remove}).click();
  const dialog=page.getByRole('dialog');await dialog.getByText((en?'Shared dependency':'共享依赖')+' · 2.0.0',{exact:true}).waitFor();
  if(await dialog.evaluate(node=>node.scrollWidth>node.clientWidth+1))throw Error('Dependency removal preview overflows');
  await page.screenshot({path:resolve(output,`remove-${language}-${theme}-${width}.png`)});
  await dialog.getByRole('button',{name:en?'Cancel':'取消',exact:true}).click();
  layouts.push({language,theme,width,overflow,usedProtected:true,unusedRemovalPreview:true});
 }
 if(errors.length)throw Error(errors.join('\n'));
 await writeFile(resolve(output,'result.json'),JSON.stringify({nativePicker:true,missingDependencyRejected:true,versionsCoexist:true,moduleDependencyVisible:true,noRuntimeToggle:true,layouts,errors},null,2));
 await page.evaluate(()=>{void window.go.main.App.WindowCommand('close');}).catch(error=>{if(!String(error).includes('closed'))throw error;});
}catch(error){await writeFile(resolve(output,'node-error.txt'),String(error.stack||error));if(page){await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});await writeFile(resolve(output,'failure.txt'),await page.locator('body').innerText().catch(()=>''));}throw error;}finally{if(browser)await browser.close();}
