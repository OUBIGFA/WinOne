// Genuine Windows OLE FileDrop source -> Wry coordinates -> receiving forms.
// All paths, processes and saved settings belong to the runner's fresh folder.
import {createRequire} from 'node:module';
import {copyFile,mkdir,readFile,writeFile,readdir} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
const run=promisify(execFile),{chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2),wait=ms=>new Promise(done=>setTimeout(done,ms));
const check=(condition,message)=>{if(!condition)throw Error(message);};
let browser,page;const report={nativeOle:false,probes:[],errors:[]};
try{
 const probe=join(output,'WinOne.OleDropProbe.exe');await copyFile(process.env.WINONE_OLE_DROP_PROBE,probe);
 const source=join(output,'sources'),folder=join(source,'拖入文件夹'),target=join(output,'destination');await mkdir(folder,{recursive:true});await mkdir(target,{recursive:true});
 const file=join(source,'拖入文件.txt'),program=join(source,'managed.cmd');await writeFile(file,'Owned drag fixture.');await writeFile(program,'@echo off\r\nexit /b 0\r\n');
 const deadline=Date.now()+30000;while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await wait(100);}}
 check(browser,'隔离窗口未启动');while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(value=>value.url().includes('winone.localhost'));if(page)break;await wait(100);}
 check(page,'隔离设置页面未加载');page.setDefaultTimeout(15000);page.on('pageerror',error=>report.errors.push(String(error)));
 await page.waitForFunction(()=>performance.getEntriesByName('winone-ui-ready').length>0);
 async function navigate(id){await page.locator(`nav [data-page-id="${id}"]`).click();}
 async function drag(paths,locator,name){
  await locator.scrollIntoViewIfNeeded();const box=await locator.boundingBox(),scale=await page.evaluate(()=>devicePixelRatio);
  const input={taskRoot:resolve(output),targetPid:Number(process.env.WINONE_COMPONENTS_PID),targetExe:resolve(output,'WinOne.exe'),paths,x:box.x+box.width/2,y:Math.min(box.y+box.height/2,(await page.evaluate(()=>innerHeight))-36),scale,resultPath:resolve(output,name+'-ole.json')};
  const spec=resolve(output,name+'-input.json');await writeFile(spec,JSON.stringify(input));await run(probe,[spec],{windowsHide:false,timeout:10000});const result=JSON.parse(await readFile(input.resultPath,'utf8'));check(result.state==='completed'&&!result.error,'原生拖放源失败：'+JSON.stringify(result));report.probes.push({name,...result});
 }
 await navigate('wintray');await page.getByRole('button',{name:'添加程序',exact:true}).waitFor();
 await drag([program],page.locator('.wintray-program-list'),'program-file');await page.getByRole('listitem',{name:'managed',exact:true}).waitFor();
 await drag([folder],page.locator('.wintray-program-list'),'program-rejects-folder');await page.getByText(/此处只接收文件/).waitFor();check(await page.locator('.wintray-program-row').count()===1,'目录被错误添加为程序');
 await page.getByRole('button',{name:'保存修改',exact:true}).click();await page.getByText('已保存',{exact:true}).waitFor();
 const saved=await page.evaluate(async()=>JSON.parse(await window.go.main.App.WinTray('settings.get','null')));check(saved.settings.managedApps.length===1&&saved.settings.managedApps[0].exePath===program,'拖入程序没有保存到隔离配置');
 await navigate('capture');await page.locator('.capture-directory-setting').waitFor();await drag([target],page.locator('.capture-directory-setting'),'capture-output-folder');await page.getByText(target,{exact:true}).waitFor();
 await drag([file],page.locator('.capture-directory-setting'),'capture-rejects-file');await page.getByText(/此处只接收文件夹/).waitFor();check((await page.locator('#capture-path').textContent()).trim()===target,'文件误投覆盖了输出目录');
 await page.locator('.settings-save-bar button').first().click();
 await navigate('convenience');await page.getByRole('tab',{name:/文件.*链接/}).click();
 const left=page.locator('[data-directory-links-drop="source"]'),right=page.locator('[data-directory-links-drop="target"]');await left.waitFor();
 await drag([file,folder],left.locator('.directory-links-file-list'),'mixed-link-sources');await left.getByRole('checkbox',{name:'选择来源：拖入文件.txt',exact:true}).waitFor();await left.getByRole('checkbox',{name:'选择来源：拖入文件夹',exact:true}).waitFor();
 await drag([target],right.locator('.directory-links-file-list'),'link-destination-folder');await page.waitForFunction(value=>document.querySelector('#links-target-directory')?.value===value,target);
 await drag([file],right.locator('.directory-links-file-list'),'link-rejects-destination-file');await page.getByText(/右侧只接收文件夹/).waitFor();check(await right.getByRole('textbox').inputValue()===target,'误投更改了链接位置');
 check((await readdir(target)).length===0,'拖入文件创建了未经确认的实际链接');
 await page.getByRole('button',{name:'清空源列表',exact:true}).click();await page.getByRole('tab',{name:'查找已有链接'}).click();
 await drag([source],page.locator('.directory-links-input-action').first(),'scan-folder');await page.waitForFunction(value=>document.querySelector('#links-scan-directory')?.value===value,source);
 await page.getByRole('tab',{name:'鼠标手势',exact:true}).click();await page.locator('.stroke-scene-row').first().waitFor();
 const names=await page.locator('.stroke-scene-label').allTextContents();check(names.length>2,'原生场景栏未加载');
 report.scenes=await page.locator('.stroke-scene-row').evaluateAll(rows=>rows.map(row=>{const label=row.querySelector('.stroke-scene-label'),toggle=row.querySelector('[role="switch"]');return {name:label.textContent,whiteSpace:getComputedStyle(label).whiteSpace,toggle:!!toggle};}));
 check(report.scenes.every(row=>row.whiteSpace==='nowrap'&&row.toggle),'场景名称换行或缺少左侧开关');
 await page.screenshot({path:resolve(output,'native-scenes.png')});
 const history=await page.evaluate(async()=>JSON.parse(await window.go.main.App.DirectoryLinks('list','{}')));check(history.length===0,'拖入意外写入了链接历史');
 check(report.errors.length===0,'原生页面出现错误：'+report.errors.join('\n'));report.nativeOle=true;report.historyUnchanged=true;report.savedProgram=true;report.validatedDirectoryFields=true;
 await writeFile(resolve(output,'result.json'),JSON.stringify(report,null,2));
 await page.evaluate(async()=>{const api=window.go.main.App;const value=await api.Snapshot();await api.SaveSettings({...value.settings,startup:{...value.settings.startup,keepUIRunning:false}});void api.WindowCommand('close');});
}catch(error){await writeFile(resolve(output,'node-error.txt'),String(error.stack||error));await writeFile(resolve(output,'result-partial.json'),JSON.stringify(report,null,2));if(page)await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});process.exitCode=1;}
finally{if(browser)await browser.close().catch(()=>{});}
