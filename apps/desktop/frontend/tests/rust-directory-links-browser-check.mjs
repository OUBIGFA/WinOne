// Runs only against build/test-desktop-components.ps1's isolated desktop and
// profile. Folder reads and link creation use the real Rust bridge and Host;
// injected drop events are reported separately from Windows OLE delivery.
import {createRequire} from 'node:module';
import {mkdir,readFile,readdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2);
const wait=ms=>new Promise(done=>setTimeout(done,ms));
const check=(condition,message)=>{if(!condition)throw Error(message);};
let browser,page;const results={layouts:[],nativeOleDrop:false};
try{
 const source=resolve(output,'sources'),target=resolve(output,'destination'),folder=join(source,'项目资料');
 await mkdir(folder,{recursive:true});await mkdir(target,{recursive:true});
 await writeFile(join(source,'说明.txt'),'Source contents remain in place.','utf8');
 await writeFile(join(source,'next.md'),'Next source.','utf8');
 await writeFile(join(folder,'inside.txt'),'Folder target contents.','utf8');
 await writeFile(join(target,'existing.txt'),'Existing destination content.','utf8');
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await wait(100);}}
 check(browser,'Isolated WebView2 did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(value=>value.url().includes('winone.localhost'));if(page)break;await wait(100);}
 check(page,'Isolated settings page did not load');
 const errors=[];page.on('pageerror',error=>errors.push(String(error)));
 const call=(method,params={})=>page.evaluate(async({method,params})=>JSON.parse(await window.go.main.App.DirectoryLinks(method,JSON.stringify(params))),{method,params});
 async function openPage(english=false){
  await page.getByRole('navigation').waitFor();
  await page.waitForFunction(()=>performance.getEntriesByName('winone-ui-ready').length>0);
  await page.evaluate(()=>window.go.main.App.Snapshot());
  await page.getByRole('button',{name:english?'Convenience':'便捷功能',exact:true}).click();
  await page.getByRole('tab',{name:english?/File.*links/i:/文件.*链接/}).click();
  await page.locator('[data-directory-links-drop="source"]').waitFor();
 }
 async function browseBoth(english=false){
  const left=page.locator('[data-directory-links-drop="source"]'),right=page.locator('[data-directory-links-drop="target"]');
  await left.getByRole('textbox').fill(source);await left.getByRole('textbox').press('Enter');
  await left.getByRole('checkbox',{name:english?'Select source: 说明.txt':'选择来源：说明.txt',exact:true}).waitFor();
  await right.getByRole('textbox').fill(target);await right.getByRole('textbox').press('Enter');
  await right.getByText('existing.txt',{exact:true}).waitFor();
  return {left,right};
 }
 await openPage();let {left,right}=await browseBoth();
 await left.getByRole('checkbox',{name:'选择来源：说明.txt',exact:true}).check();
 await left.getByRole('checkbox',{name:'选择来源：项目资料',exact:true}).check();
 check(await right.getByText('待检查',{exact:true}).count()===2,'Selected sources did not appear immediately on the right');
 check((await readdir(target)).length===1,'Selection created filesystem entries before confirmation');
 await page.getByRole('button',{name:'预览创建计划',exact:true}).click();
 await page.getByRole('button',{name:'创建全部链接',exact:true}).waitFor();
 check(await right.getByText('待创建',{exact:true}).count()===2,'Host-reviewed rows did not become ready');
 await page.screenshot({path:resolve(output,'before-create.png')});
 await page.getByRole('button',{name:'创建全部链接',exact:true}).click();
 await page.getByText('已核查生效 2 / 2 项。未完成项的原因保留在历史中。',{exact:true}).waitFor();
 check((await readFile(join(target,'说明.txt'),'utf8'))==='Source contents remain in place.','Created file link did not resolve');
 check((await readFile(join(target,'项目资料','inside.txt'),'utf8'))==='Folder target contents.','Created junction did not resolve');
 const records=await call('list');
 check(records.length===2&&records.every(record=>record.status==='active')&&records.some(record=>record.kind==='hardlink')&&records.some(record=>record.kind==='junction'),'Created links were not verified as the requested real kinds');
 check(await right.getByText('已存在',{exact:true}).count()===3,'Right pane did not refresh the new entries');
 results.realCreation={hardlink:true,junction:true,sourceRetained:true,verifiedRecords:records.map(({name,kind,status})=>({name,kind,status}))};
 await page.screenshot({path:resolve(output,'after-create.png')});
 await left.getByRole('button',{name:'打开文件夹：项目资料',exact:true}).click();
 await left.getByText('inside.txt',{exact:true}).waitFor();
 await left.getByRole('button',{name:'源文件夹上一级',exact:true}).click();
 await left.getByRole('checkbox',{name:'选择来源：说明.txt',exact:true}).waitFor();
 results.folderNavigation=true;

 // Exercise the native event contract with real absolute paths, including its
 // right-pane position. This is not evidence of OS-level OLE event delivery.
 const rightBounds=await right.boundingBox();
 await page.evaluate(({paths,position})=>window.dispatchEvent(new CustomEvent('winone:directory-links-drop',{detail:{paths,position}})),{paths:[join(source,'说明.txt')],position:{x:rightBounds.x+rightBounds.width/2,y:rightBounds.y+rightBounds.height/2}});
 await page.getByText(/右侧只接收文件夹/).waitFor();
 check(await right.getByRole('textbox').inputValue()===target,'Invalid target drop discarded the valid destination');
 await page.evaluate(paths=>window.dispatchEvent(new CustomEvent('winone:directory-links-drop',{detail:{paths}})),[join(source,'说明.txt'),folder]);
 await right.getByText('同名冲突',{exact:true}).first().waitFor();
 check(await right.getByText('同名冲突',{exact:true}).count()===2,'Dropped collisions were hidden');
 check((await readdir(target)).length===3,'Dropping performed creation');
 results.injectedNativeDropContract={sourceFilesAndFolders:true,rightFileRejected:true,destinationPreserved:true,collisions:true,noImplicitCreation:true};
 await page.getByRole('button',{name:'清空源列表',exact:true}).click();

 await page.getByRole('tab',{name:'查找已有链接',exact:true}).click();
 await page.getByLabel('查找文件夹',{exact:true}).fill(target);
 await page.getByRole('checkbox',{name:'包含子文件夹',exact:true}).uncheck();
 await page.getByRole('button',{name:'查找并登记',exact:true}).click();
 await page.getByText(/检查了 .*项目/).waitFor();
 const inspected=await call('list');check(inspected.length===2&&inspected.every(record=>record.status==='active'),'Scanning lost or duplicated existing records');
 results.scan=true;

 // Real recycle action is restricted to an entry this driver just created.
 const history=page.locator('.directory-links-history');
 const fileRecord=history.locator('article').filter({has:page.locator('.directory-links-record-name',{hasText:'说明.txt'})});
 await fileRecord.getByRole('button',{name:'移除链接入口',exact:true}).click();
 await page.getByRole('dialog').getByRole('button',{name:'移到回收站',exact:true}).click();
 await fileRecord.getByText('已回收',{exact:true}).waitFor();
 check(!(await readdir(target)).includes('说明.txt'),'Recycled link is still present');
 check((await readFile(join(source,'说明.txt'),'utf8'))==='Source contents remain in place.','Removing the link changed its source');
 results.recycle={ownedEntryOnly:true,sourceRetained:true};

 for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark'])for(const width of [1180,820]){
  await page.evaluate(async({language,theme})=>{const api=window.go.main.App;const snapshot=await api.Snapshot();await api.SaveSettings({...snapshot.settings,language,theme});},{language,theme});
  await page.setViewportSize({width,height:920});await page.reload();await openPage(language==='en-US');
  ({left,right}=await browseBoth(language==='en-US'));
  await left.getByRole('checkbox',{name:language==='en-US'?'Select source: next.md':'选择来源：next.md',exact:true}).check();
  await page.getByRole('button',{name:language==='en-US'?'Preview creation':'预览创建计划',exact:true}).click();
  await page.getByRole('button',{name:language==='en-US'?'Create all links':'创建全部链接',exact:true}).waitFor();
  await page.evaluate(()=>document.fonts.ready);await page.locator('.directory-links-panes').scrollIntoViewIfNeeded();
  const layout=await page.evaluate(()=>{
   const rect=element=>{const {x,y,width,height}=element.getBoundingClientRect();return {x,y,width,height};};
   const left=document.querySelector('[data-directory-links-drop="source"]'),right=document.querySelector('[data-directory-links-drop="target"]');
   const pane=document.querySelector('.directory-links-panes');
   return {left:rect(left),right:rect(right),scrollWidth:pane.scrollWidth,clientWidth:pane.clientWidth,viewport:innerWidth,rootWidth:document.documentElement.scrollWidth};
  });
  check(layout.left.x+layout.left.width<=layout.right.x+1&&Math.abs(layout.left.y-layout.right.y)<1,'The two folder panes are not side by side');
  check(layout.scrollWidth<=layout.clientWidth+1&&layout.rootWidth<=layout.viewport+1,'The double-pane view overflows the window horizontally');
  results.layouts.push({language,theme,width,...layout});
  await page.screenshot({path:resolve(output,`panes-${language}-${theme}-${width}.png`)});
 }
 check(errors.length===0,errors.join('\n'));
 await writeFile(resolve(output,'result.json'),JSON.stringify(results,null,2),'utf8');
 await page.getByRole('button',{name:'Clear sources',exact:true}).click();
 await page.evaluate(async()=>{const api=window.go.main.App;const snapshot=await api.Snapshot();await api.SaveSettings({...snapshot.settings,startup:{...snapshot.settings.startup,keepUIRunning:false}});void api.WindowCommand('close');});
}catch(error){
 await writeFile(resolve(output,'node-error.txt'),String(error.stack||error),'utf8');
 await writeFile(resolve(output,'result-partial.json'),JSON.stringify(results,null,2),'utf8');
 if(page)await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});
 process.exitCode=1;
}finally{if(browser)await browser.close().catch(()=>{});}
