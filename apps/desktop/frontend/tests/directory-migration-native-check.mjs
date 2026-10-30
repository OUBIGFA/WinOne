// Real isolated WinOne + Host. Native pickers are driven through the owned
// dialog's accessibility controls; all data belongs to this invocation.
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {mkdir,writeFile,readFile,lstat,readdir,access} from 'node:fs/promises';
import {resolve,join,basename,dirname} from 'node:path';
import {spawn,execFileSync} from 'node:child_process';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2),root=process.cwd(),testRoot=dirname(output);
const data=join(testRoot,basename(output)+'-data'),source=join(data,'source'),destination=join(data,'destination'),backups=join(data,'backups');
let browser,page;const errors=[],results=[];
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
async function nativePick(click,path,kind='folder'){
 const child=spawn('powershell.exe',['-NoProfile','-ExecutionPolicy','Bypass','-File',resolve('build/directory-dialog-probe.ps1'),'-OwnerPID',process.env.WINONE_COMPONENTS_PID,'-ExpectedExe',join(output,'WinOne.exe'),'-TestRoot',testRoot,'-Path',path,'-Kind',kind],{windowsHide:true});
 let log='';child.stdout.on('data',chunk=>log+=chunk);child.stderr.on('data',chunk=>log+=chunk);
 const done=new Promise((res,rej)=>{child.once('error',rej);child.once('exit',code=>code===0?res():rej(Error('Native picker: '+log)));});
 // Attach rejection handling immediately while the click opens the dialog.
 const checked=done.catch(error=>{throw error;});checked.catch(()=>{});
 await click();await checked;
}
const call=(method,args=null)=>page.evaluate(async({method,args})=>JSON.parse(await window.go.main.App.DirectoryLinks(method,JSON.stringify(args))),{method,args});
async function openLinks(language='zh-CN'){
 await page.locator('.app-navigation [data-page-id="convenience"]').click();
 await page.getByRole('tab',{name:language==='zh-CN'?'文件与目录链接':'File and folder links',exact:true}).click();
}
try{
 await mkdir(data);for(const path of [source,destination,backups])await mkdir(path);
 await mkdir(join(source,'nested'));
 await writeFile(join(source,'nested','文档.txt'),'fixture-data-迁移还原');
 await writeFile(join(source,'nested','文档.txt:metadata'),'fixture-stream');
 const deadline=Date.now()+45000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await sleep(100);}}
 if(!browser)throw Error('Isolated WinOne did not expose CDP');
 while(Date.now()<deadline){page=browser.contexts().flatMap(c=>c.pages()).find(p=>p.url().includes('winone.localhost'));if(page)break;await sleep(100);}
 if(!page)throw Error('Isolated native UI document missing');
 page.setDefaultTimeout(25000);page.on('pageerror',error=>errors.push(String(error)));
 await page.locator('.app-navigation').waitFor();await page.waitForFunction(()=>performance.getEntriesByName('winone-ui-ready').length>0);
 await openLinks();await page.getByRole('tab',{name:'搬迁与还原',exact:true}).click();
 const sourceInput=page.getByLabel('要搬迁的目录',{exact:true}),destinationInput=page.getByLabel('新的存放文件夹',{exact:true});
 await nativePick(()=>sourceInput.locator('..').getByRole('button').click(),source);
 assert.equal((await sourceInput.inputValue()).toLowerCase(),source.toLowerCase());
 await nativePick(()=>destinationInput.locator('..').getByRole('button').click(),destination);
 assert.equal((await destinationInput.inputValue()).toLowerCase(),destination.toLowerCase());
 await page.getByLabel('新目录名称',{exact:true}).fill('moved');
 await page.getByRole('button',{name:'检查并预览',exact:true}).click();await page.getByText('预览已准备好，尚未搬迁',{exact:true}).waitFor();
 assert.equal((await lstat(source)).isSymbolicLink(),false);
 await page.getByRole('button',{name:'开始搬迁',exact:true}).click();await page.getByRole('dialog').waitFor();await page.screenshot({path:join(output,'move-confirm.png')});
 await page.getByRole('dialog').getByRole('button',{name:'确认执行',exact:true}).click();await page.getByText('任务已完成',{exact:true}).waitFor();
 let rows=await call('list'),moved=rows.find(r=>r.migration?.operation==='migrate');
 assert.ok(moved&&moved.status==='active');assert.equal(moved.migration.phase,'complete');assert.equal(moved.migration.retained?.length??0,0);
 assert.equal((await lstat(source)).isSymbolicLink(),true);
 assert.equal(await readFile(join(destination,'moved','nested','文档.txt:metadata'),'utf8'),'fixture-stream');
 results.push({step:'native directory pickers / preview / move / real junction / stream verification / native recycle',record:moved});
 // A separate ordinary hardlink exercises backup import without discarding
 // the authoritative migration journal needed for the later restore.
 const plainSource=join(data,'archive-source.txt');await writeFile(plainSource,'archive-data');
 const p=await call('preview',{source:plainSource,directory:data,linkName:'archive-link.txt',kind:'hardlink',name:'Archive fixture'});
 const plain=await call('create',{token:p.id});
 await page.locator('.app-navigation [data-page-id="overview"]').click();await openLinks();await page.getByRole('tab',{name:'查找已有链接',exact:true}).click();
 let article=page.locator('.directory-links-record').filter({has:page.locator('.directory-links-record-name',{hasText:'Archive fixture'})});
 await article.locator('xpath=ancestor::details[contains(@class,"directory-links-history-group")]').locator(':scope > summary').click();
 await article.locator('[data-slot="checkbox"]').first().click();
 await nativePick(()=>page.getByRole('button',{name:'导出选中记录',exact:true}).click(),backups);
 await page.getByText(/历史备份已保存/).waitFor();
 const names=await readdir(backups);assert.equal(names.length,1);const archive=join(backups,names[0]);
 const raw=JSON.parse(await readFile(archive,'utf8'));assert.equal(raw.entries.length,1);assert.equal(raw.entries[0].linkPath,plain.linkPath);assert.deepEqual(Object.keys(raw.entries[0]).sort(),['kind','linkPath','name','targetPath']);
 await article.getByRole('button',{name:'移除记录',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'仅移除记录',exact:true}).click();await article.waitFor({state:'detached'});
 await nativePick(()=>page.getByRole('button',{name:'导入历史备份',exact:true}).click(),archive,'file');
 await page.getByRole('dialog').getByText('可导入 1 条，跳过 0 条。',{exact:true}).waitFor();
 await page.getByRole('dialog').getByRole('button',{name:'确认导入记录',exact:true}).click();await page.getByText('已导入 1 条记录，真实文件和链接未改变。',{exact:true}).waitFor();
 rows=await call('list');const imported=rows.find(r=>r.name==='Archive fixture');assert.ok(imported&&imported.id!==plain.id&&imported.status==='active');assert.equal(await readFile(plain.linkPath,'utf8'),'archive-data');
 results.push({step:'native folder/file dialogs / selected export / history-only forget / verified import',imported});
 // Restore through the actual history action and confirmation, not a direct RPC.
 article=page.locator('.directory-links-record').filter({has:page.locator('.directory-links-record-name',{hasText:/^source$/})});
 const group=article.locator('xpath=ancestor::details[contains(@class,"directory-links-history-group")]');if(!await group.getAttribute('open').then(v=>v!==null))await group.locator(':scope > summary').click();
 await article.getByRole('button',{name:'还原到原位置',exact:true}).click();await page.getByRole('button',{name:'检查并预览',exact:true}).click();await page.getByText('预览已准备好，尚未搬迁',{exact:true}).waitFor();
 await page.getByRole('button',{name:'还原到原位置',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'确认执行',exact:true}).click();await page.getByText('任务已完成',{exact:true}).waitFor();
 rows=await call('list');moved=rows.find(r=>r.id===moved.id);assert.equal(moved.status,'restored');assert.equal(moved.migration.retained?.length??0,0);
 assert.equal((await lstat(source)).isSymbolicLink(),false);assert.equal(await readFile(join(source,'nested','文档.txt'),'utf8'),'fixture-data-迁移还原');await assert.rejects(access(join(destination,'moved')),error=>error.code==='ENOENT');
 const disk=JSON.parse(await readFile(join(process.env.WINONE_COMPONENTS_PROFILE,'tools','directory-links','history.json'),'utf8'));assert.equal(disk.records.find(r=>r.id===moved.id).status,'restored');
 results.push({step:'history restore / ordinary folder / original bytes and streams / durable history / recycled former location',record:moved});
 const resize=join(output,'resize-migration-window.ps1');
 await writeFile(resize,`param([int]$OwnerPID,[string]$ExpectedExe,[int]$Width)\n$ErrorActionPreference='Stop'\n$p=Get-Process -Id $OwnerPID\nif($p.Path -ne $ExpectedExe){throw 'Wrong window owner'}\nAdd-Type 'using System;using System.Runtime.InteropServices;public static class MigrationWindowSize{[DllImport("user32.dll")]public static extern bool SetProcessDPIAware();[DllImport("user32.dll")]public static extern bool SetWindowPos(IntPtr h,IntPtr a,int x,int y,int w,int height,uint flags);}'\n[void][MigrationWindowSize]::SetProcessDPIAware()\nif(-not [MigrationWindowSize]::SetWindowPos($p.MainWindowHandle,[IntPtr]::Zero,80,60,$Width,930,0x14)){throw 'Resize failed'}\n`);
 for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark'])for(const width of [520,1100]){
  await page.evaluate(async({language,theme})=>{const api=window.go.main.App;const snapshot=await api.Snapshot();await api.SaveSettings({...snapshot.settings,language,theme,startup:{...snapshot.settings.startup,runAtLogon:false,keepUIRunning:false}});},{language,theme});
  await page.reload();await page.locator('.app-navigation').waitFor();await openLinks(language);await page.getByRole('tab',{name:language==='zh-CN'?'搬迁与还原':'Move and restore',exact:true}).click();
  execFileSync('powershell.exe',['-NoProfile','-File',resize,'-OwnerPID',process.env.WINONE_COMPONENTS_PID,'-ExpectedExe',join(output,'WinOne.exe'),'-Width',String(width)],{windowsHide:true});
  await page.evaluate(async()=>{await document.fonts.ready;await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);});
  const metrics=await page.evaluate(()=>({page:document.documentElement.scrollWidth,viewport:innerWidth,panes:[...document.querySelectorAll('[data-slot="page-scroll"]')].filter(e=>e.getClientRects().length).map(e=>({width:e.clientWidth,scroll:e.scrollWidth}))}));
  assert.ok(metrics.page<=metrics.viewport+1&&metrics.panes.every(p=>p.scroll<=p.width+1),JSON.stringify(metrics));
  await page.screenshot({path:join(output,`${language}-${theme}-${width}.png`)});results.push({step:'native layout',language,theme,width,metrics});
 }
 assert.deepEqual(errors,[]);await writeFile(join(output,'result.json'),JSON.stringify({results,errors,scope:'Owned native WinOne, all fixture modules disabled, same NTFS volume; no real account/configuration or other programs changed.'},null,2));
 await page.evaluate(()=>{void window.go.main.App.WindowCommand('close');}).catch(error=>{if(!String(error).includes('closed'))throw error;});
}catch(error){await writeFile(join(output,'node-error.txt'),String(error.stack||error));if(page){await page.screenshot({path:join(output,'failure.png')}).catch(()=>{});await writeFile(join(output,'failure.txt'),await page.locator('body').innerText().catch(()=>''));}throw error;}
finally{await browser?.close().catch(()=>{});}
