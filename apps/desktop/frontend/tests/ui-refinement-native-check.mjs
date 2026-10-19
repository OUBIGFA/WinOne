// Real Windows file operations through the production UI and Host. Every
// fixture and process belongs to build/test-desktop-components.ps1's output.
import {createRequire} from 'node:module';
import {mkdir,writeFile,readFile,stat,link,symlink} from 'node:fs/promises';
import {join,resolve} from 'node:path';
import {execFileSync} from 'node:child_process';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,outputArg]=process.argv.slice(2),output=resolve(outputArg),errors=[],views=[],renamed=[];
if(new URL(endpoint).hostname!=='127.0.0.1'||!process.env.WINONE_COMPONENTS_PROFILE?.startsWith(output))throw Error('Use the isolated component runner');
let browser,page;
try{
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(done=>setTimeout(done,100));}}
 if(!browser)throw Error('Owned WinOne test connection missing');
 while(Date.now()<deadline){page=browser.contexts().flatMap(c=>c.pages()).find(p=>p.url().includes('winone.localhost'));if(page)break;await new Promise(done=>setTimeout(done,100));}
 if(!page)throw Error('Owned WinOne document missing');
 page.setDefaultTimeout(15000);page.on('pageerror',error=>errors.push(String(error)));
 await page.locator('.app-navigation').waitFor();
 const resize=join(output,'resize-window.ps1');
 await writeFile(resize,[
  'param([int]$OwnerPID,[string]$ExpectedExe,[int]$Width)',
  '$ErrorActionPreference="Stop"',
  '$p=Get-Process -Id $OwnerPID',
  'if($p.Path -ne $ExpectedExe){throw "Unexpected window owner"}',
  'Add-Type -TypeDefinition \'using System; using System.Runtime.InteropServices; public class OwnedWindowSize { [DllImport("user32.dll",SetLastError=true)] public static extern bool SetWindowPos(IntPtr h,IntPtr after,int x,int y,int w,int hgt,uint flags); }\'',
  'if(-not [OwnedWindowSize]::SetWindowPos($p.MainWindowHandle,[IntPtr]::Zero,0,0,$Width,940,0x16)){throw "Window resize failed"}',
 ].join('\n'),'utf8');
 const settle=()=>page.evaluate(async()=>{await document.fonts.ready;await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);});
 const size=async width=>{execFileSync('powershell.exe',['-NoProfile','-File',resize,'-OwnerPID',process.env.WINONE_COMPONENTS_PID,'-ExpectedExe',join(output,'WinOne.exe'),'-Width',String(width)],{windowsHide:true});await settle();};
 const open=async id=>{await page.locator('.app-navigation [data-page-id="'+id+'"]').click();await page.locator('[data-settings-page="'+id+'"]').waitFor();};
 const snapshot=async name=>{
  await settle();
  for(const frame of page.frames().filter(f=>f!==page.mainFrame())){
   await frame.evaluate(async()=>{await document.fonts.ready;await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);});
   const state=await frame.evaluate(()=>{
    const head=document.querySelector('.pm-head'),before=head&&getComputedStyle(head,'::before');
    const nodes=[document.documentElement,document.body,...document.querySelectorAll('.view,.pm-head,.pm-manual,.pm-body')].filter(n=>n.getClientRects().length);
    return {overflow:nodes.filter(n=>n.scrollWidth>n.clientWidth+1).map(n=>n.className||n.tagName),frosted:!!before&&before.content!=='none'&&before.display!=='none'&&before.backdropFilter!=='none'};
   });
   assert.deepEqual(state.overflow,[],'Embedded content must not create a horizontal scrollbar');
   assert.equal(state.frosted,false,'Plugin header must not paint a frosted overlay over the content');
  }
  await settle();await page.screenshot({path:join(output,name+'.png')});views.push({name,viewport:await page.evaluate(()=>({width:innerWidth,height:innerHeight,overflow:document.documentElement.scrollWidth-innerWidth}))});
 };
 await size(1280);await open('convenience');
 await page.getByRole('tab',{name:'文件与目录链接',exact:true}).click();
 const target=join(output,'rename-target'),source=join(output,'rename-source');
 await mkdir(target);await mkdir(source);await mkdir(join(target,'原文件夹'));
 await writeFile(join(target,'原文件.txt'),'file stays intact','utf8');
 await writeFile(join(target,'原文件夹','keep.txt'),'folder stays intact','utf8');
 await writeFile(join(target,'collision.txt'),'collision stays intact','utf8');
 await writeFile(join(source,'source.txt'),'link source stays intact','utf8');
 await writeFile(join(source,'keep.txt'),'junction source stays intact','utf8');
 await link(join(source,'source.txt'),join(target,'原硬链接.txt'));
 await symlink(source,join(target,'原目录链接'),'junction');
 const pane=page.locator('[data-directory-links-drop="target"]');
 await pane.getByRole('textbox').fill(target);await pane.getByRole('textbox').press('Enter');
 const dialog=page.getByRole('dialog'),name=page.locator('#links-rename-name');
 for(const entry of [{before:'原文件.txt',after:'已改名文件.txt',content:'file stays intact'},{before:'原文件夹',after:'已改名文件夹',content:'folder stays intact',directory:true},{before:'原硬链接.txt',after:'已改名硬链接.txt',content:'link source stays intact'},{before:'原目录链接',after:'已改名目录链接',content:'junction source stays intact',directory:true}]){
  const row=pane.getByRole('listitem').filter({has:page.getByRole('button',{name:'重命名：'+entry.before,exact:true})});
  if(entry.directory){await row.focus();await row.press('F2');}else await row.getByRole('button',{name:'重命名：'+entry.before,exact:true}).click();
  await name.fill(entry.after);await dialog.getByRole('button',{name:'重命名',exact:true}).click();await dialog.waitFor({state:'hidden'});
  await pane.getByRole('button',{name:'重命名：'+entry.after,exact:true}).waitFor();
  await assert.rejects(stat(join(target,entry.before)),{code:'ENOENT'});
  assert.equal(await readFile(join(target,entry.after,...(entry.directory?['keep.txt']:[])),'utf8'),entry.content);
  renamed.push(entry.after);
 }
 assert.equal(await readFile(join(source,'source.txt'),'utf8'),'link source stays intact');
 assert.equal(await readFile(join(source,'keep.txt'),'utf8'),'junction source stays intact');
 await pane.getByRole('button',{name:'重命名：已改名文件.txt',exact:true}).click();
 await name.fill('collision.txt');await dialog.getByRole('button',{name:'重命名',exact:true}).click();
 await dialog.getByText(/已有同名项目/).waitFor();assert.equal(await name.inputValue(),'collision.txt');
 await snapshot('rename-collision');await dialog.getByRole('button',{name:'取消',exact:true}).click();
 assert.equal(await readFile(join(target,'collision.txt'),'utf8'),'collision stays intact');
 await snapshot('renamed-files');
 await page.getByRole('tab',{name:'鼠标手势',exact:true}).click();
 await page.getByRole('tab',{name:'动作包',exact:true}).click();
 await page.locator('.stroke-package-name').first().click();
 const original=await page.locator('#stroke-package-name').inputValue();
 await page.locator('#stroke-package-name').fill('资源管理器文件夹·全局');
 for(const width of [1280,800]){
  await size(width);
  const label=await page.locator('.stroke-package-name').first().evaluate(n=>({title:n.textContent,visible:n.clientWidth,required:n.scrollWidth,whiteSpace:getComputedStyle(n).whiteSpace}));
  assert.ok(label.required<=label.visible+1,JSON.stringify(label));assert.equal(label.whiteSpace,'nowrap');
  await snapshot('package-title-'+width);
 }
 await page.locator('#stroke-package-name').fill(original);
 await open('ai-services');
 const tabs=page.getByRole('tablist',{name:'AI 服务',exact:true});
 for(const width of [800,1280]){
  await size(width);await tabs.getByRole('tab',{name:'路由',exact:true}).click();
  const routing=page.frameLocator('iframe[title="路由"]');
  await routing.locator('#rtNewGroup').waitFor();await routing.locator('.rt-gtpl-card').first().waitFor();
  await snapshot('routing-native-'+width);
  await tabs.getByRole('tab',{name:'插件',exact:true}).click();
  const plugins=page.frameLocator('iframe[title="插件"]');
  await plugins.getByRole('textbox',{name:'插件',exact:true}).waitFor();
  await plugins.getByRole('textbox',{name:'插件',exact:true}).fill('draft-plugin-path');
  await plugins.getByRole('button',{name:/^已安装/}).click();
  assert.equal(await plugins.getByRole('textbox',{name:'插件',exact:true}).inputValue(),'draft-plugin-path');
  await plugins.getByRole('button',{name:'发现',exact:true}).click();
  assert.equal(await plugins.getByRole('textbox',{name:'插件',exact:true}).inputValue(),'draft-plugin-path');
  await snapshot('plugins-native-'+width);
 }
 for(const frame of page.frames().filter(f=>f!==page.mainFrame())){
  const overflow=await frame.evaluate(()=>document.documentElement.scrollWidth-innerWidth);assert.ok(overflow<=1,'Embedded page overflow');
 }
 assert.deepEqual(errors,[]);assert.ok(views.every(v=>v.viewport.overflow<=1));
 await writeFile(join(output,'result.json'),JSON.stringify({native:true,renamed,collisionProtected:true,linkSourcesPreserved:true,pluginInstallDraftPreserved:true,views,errors},null,2),'utf8');
 // Closing the CDP connection only detaches the driver; request normal closure
 // of this runner-owned window so the outer runner can verify process exit.
 await page.evaluate(()=>{void window.go.main.App.WindowCommand('close');}).catch(error=>{if(!String(error).includes('closed'))throw error;});
}catch(error){
 if(page)await page.screenshot({path:join(output,'failure.png')}).catch(()=>{});
 await writeFile(join(output,'node-error.txt'),String(error)+'\n'+error.stack+'\n'+errors.join('\n'),'utf8');throw error;
}finally{if(browser)await browser.close();}
