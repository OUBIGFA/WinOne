// Genuine WinForms IDataObject/DoDragDrop -> Wry -> Rust -> browser flow.
// Run only through the explicitly selected -CurrentDesktop fixture runner.
import {createRequire} from 'node:module';
import {copyFile,mkdir,readFile,readdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
const run=promisify(execFile),{chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2),wait=ms=>new Promise(done=>setTimeout(done,ms));
const check=(condition,message)=>{if(!condition)throw Error(message);};
let browser,page;const report={nativeOle:false,probes:[]};
try{
 const probe=join(output,'WinOne.OleDropProbe.exe');await copyFile(process.env.WINONE_OLE_DROP_PROBE,probe);
 const source=join(output,'sources'),folder=join(source,'拖入文件夹'),target=join(output,'destination');
 await mkdir(folder,{recursive:true});await mkdir(target,{recursive:true});
 const file=join(source,'拖入文件.txt'),second=join(source,'另一个文件.txt');
 await writeFile(file,'Owned OLE fixture.','utf8');await writeFile(second,'Second OLE fixture.','utf8');await writeFile(join(folder,'kept.txt'),'Kept.','utf8');
 await writeFile(join(target,'拖入文件.txt'),'Existing destination must remain unchanged.','utf8');
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await wait(100);}}
 check(browser,'Isolated WebView2 did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(value=>value.url().includes('winone.localhost'));if(page)break;await wait(100);}
 check(page,'Isolated settings document did not load');
 await page.waitForFunction(()=>performance.getEntriesByName('winone-ui-ready').length>0);
 await page.evaluate(()=>window.go.main.App.Snapshot());
 await page.getByRole('button',{name:'便捷功能',exact:true}).click();await page.getByRole('tab',{name:/文件.*链接/}).click();
 const left=page.locator('[data-directory-links-drop="source"]'),right=page.locator('[data-directory-links-drop="target"]');
 await left.getByRole('textbox').waitFor();
 async function drag(paths,pane,name){
  await pane.scrollIntoViewIfNeeded();
  const box=await pane.boundingBox(),scale=await page.evaluate(()=>devicePixelRatio);
  const input={taskRoot:resolve(output),targetPid:Number(process.env.WINONE_COMPONENTS_PID),targetExe:resolve(output,'WinOne.exe'),paths,x:box.x+box.width/2,y:Math.min(box.y+box.height/2,(await page.evaluate(()=>innerHeight))-30),scale,resultPath:resolve(output,name+'-ole.json')};
  const specification=resolve(output,name+'-input.json');await writeFile(specification,JSON.stringify(input),'utf8');
  await run(probe,[specification],{windowsHide:false,timeout:10000});
  const result=JSON.parse(await readFile(input.resultPath,'utf8'));
  check(result.state==='completed'&&!result.error,'OLE source failed: '+JSON.stringify(result));
  report.probes.push({name,paths,...result});
 }
 await drag([file,folder],left,'mixed-sources');
 await left.getByRole('checkbox',{name:'选择来源：拖入文件.txt',exact:true}).waitFor();
 await left.getByRole('checkbox',{name:'选择来源：拖入文件夹',exact:true}).waitFor();
 await drag([target],right,'destination-folder');
 await page.waitForFunction(value=>document.querySelector('#links-target-directory')?.value===value,target);
 await right.getByText('同名冲突',{exact:true}).waitFor();
 await drag([second],left,'single-source');
 await left.getByRole('checkbox',{name:'选择来源：另一个文件.txt',exact:true}).waitFor();
 await drag([file],right,'invalid-destination-file');
 await page.getByText(/右侧只接收文件夹/).waitFor();check(await right.getByRole('textbox').inputValue()===target,'File drop changed the destination');
 await drag([source,target],right,'multiple-destination-folders');
 await page.getByText(/右侧每次只接收一个目标文件夹/).waitFor();check(await right.getByRole('textbox').inputValue()===target,'Multiple folder drop changed the destination');
 check((await readdir(target)).length===1,'OLE drop created unconfirmed links');
 check(await readFile(join(target,'拖入文件.txt'),'utf8')==='Existing destination must remain unchanged.','OLE drop overwrote an existing entry');
 const recorded=await page.evaluate(async()=>JSON.parse(await window.go.main.App.DirectoryLinks('list','{}')));check(recorded.length===0,'OLE drop created link history without confirmation');
 report.nativeOle=true;report.mixedSources=true;report.singleSource=true;report.targetFolder=true;report.invalidTargetsPreserveSelection=true;report.conflictVisible=true;report.noImplicitCreation=true;
 await page.screenshot({path:resolve(output,'ole-drag-result.png')});await writeFile(resolve(output,'result.json'),JSON.stringify(report,null,2),'utf8');
 await page.getByRole('button',{name:'清空源列表',exact:true}).click();
 await page.evaluate(async()=>{const api=window.go.main.App;const value=await api.Snapshot();await api.SaveSettings({...value.settings,startup:{...value.settings.startup,keepUIRunning:false}});void api.WindowCommand('close');});
}catch(error){await writeFile(resolve(output,'node-error.txt'),String(error.stack||error),'utf8');await writeFile(resolve(output,'result-partial.json'),JSON.stringify(report,null,2),'utf8');if(page)await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});process.exitCode=1;}
finally{if(browser)await browser.close().catch(()=>{});}
