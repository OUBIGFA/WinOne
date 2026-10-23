// Native mouse input, observed through the real page and window geometry.
import {createRequire} from 'node:module';
import {readFile,writeFile,access} from 'node:fs/promises';
import {resolve} from 'node:path';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE),run=promisify(execFile);
const [endpoint,output]=process.argv.slice(2),wait=ms=>new Promise(done=>setTimeout(done,ms));
const check=(condition,message)=>{if(!condition)throw Error(message);};
let browser,page;const report={actions:[],errors:[]};
try{
 const deadline=Date.now()+30000;while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await wait(100);}}
 check(browser,'隔离窗口未启动');
 while(Date.now()<deadline){page=browser.contexts().flatMap(c=>c.pages()).find(p=>p.url().includes('winone.localhost'));if(page)break;await wait(100);}
 check(page,'页面未加载');page.setDefaultTimeout(10000);page.on('pageerror',e=>report.errors.push(String(e)));
 await page.waitForFunction(()=>performance.getEntriesByName('winone-ui-ready').length>0);
 async function input(name,action,point,delta={}){
  const scale=await page.evaluate(()=>devicePixelRatio),request=resolve(output,name+'.native-input.json');
  const spec={taskRoot:resolve(output),targetExe:resolve(output,'WinOne.exe'),targetPid:Number(process.env.WINONE_COMPONENTS_PID),expectedTargetElevated:process.env.WINONE_COMPONENTS_ELEVATED==='1',action,x:Math.round(point.x*scale),y:Math.round(point.y*scale),dx:delta.x||0,dy:delta.y||0,resultPath:resolve(output,name+'.json')};
  await writeFile(request,JSON.stringify(spec));
  if(!spec.expectedTargetElevated)await run('powershell.exe',['-NoProfile','-ExecutionPolicy','Bypass','-File',resolve('build/component-native-input.ps1'),'-Specification',request],{windowsHide:true,timeout:15000});
  const until=Date.now()+15000;let complete=false;
  while(Date.now()<until){const error=await readFile(request+'.error.txt','utf8').catch(()=>null);if(error)throw Error(error);if(await access(spec.resultPath).then(()=>true,()=>false)){complete=true;break;}await wait(100);}
  check(complete,'原生输入未完成');const result=JSON.parse(await readFile(spec.resultPath,'utf8'));report.actions.push({name,...result});return result;
 }
 const center=async locator=>{const b=await locator.boundingBox();return {x:b.x+b.width/2,y:b.y+b.height/2};};
 await input('navigation','click',await center(page.locator('nav [data-page-id="capture"]')));
 await page.locator('.capture-directory-setting').waitFor();
 // Populate the existing real list; all entries are inert in this fresh profile.
 await page.evaluate(async()=>{
  const api=window.go.main.App,current=JSON.parse(await api.WinTray('settings.get','null'));
  current.settings.managedApps=Array.from({length:80},(_,i)=>({id:'input-fixture-'+i,name:'程序 '+(i+1),exePath:'C:\\Missing-UI-Fixture\\app'+i+'.exe',args:'',runOnStartup:false,collectTrayIcon:false,launchViaLogonTask:false,launchHiddenInBackground:false,trayBehavior:{autoMinimizeAndHideOnLaunch:false,closeDelaySeconds:0},schedule:{enabled:false,startDelayMinutes:0,autoExitMinutes:0,frequencyEnabled:false,frequencyDays:1,frequencyRuns:1}}));
  let job=JSON.parse(await api.WinTray('settings.save',JSON.stringify({revision:current.revision,settings:current.settings})));
  const until=Date.now()+10000;while(job.state==='running'&&Date.now()<until){await new Promise(done=>setTimeout(done,50));job=JSON.parse(await api.WinTray('jobs.get',JSON.stringify({id:job.id})));}
  if(job.state!=='completed')throw Error('列表夹具未保存');
 });
 await input('program-navigation','click',await center(page.locator('nav [data-page-id="wintray"]')));
 const list=page.locator('.wintray-program-list');await list.waitFor();await page.locator('.wintray-program-row').last().waitFor({state:'attached'});
 check(await list.evaluate(e=>e.scrollHeight>e.clientHeight),'程序列表未产生滚动');
 await input('wheel','wheel',await center(list));
 await page.waitForFunction(()=>document.querySelector('.wintray-program-list')?.scrollTop>0);
 const moved=await input('title-drag','drag',await center(page.locator('[data-slot="window-branding"]')),{x:30,y:20});
 check(moved.after.left-moved.before.left>=20&&moved.after.top-moved.before.top>=10,'标题栏拖动没有移动窗口');
 const corner=await page.evaluate(()=>({x:innerWidth-3,y:innerHeight-3}));
 const resized=await input('resize','drag',corner,{x:-60,y:-60});
 check((resized.before.right-resized.before.left)-(resized.after.right-resized.after.left)>=40,'窗口宽度没有缩小');
 check((resized.before.bottom-resized.before.top)-(resized.after.bottom-resized.after.top)>=40,'窗口高度没有缩小');
 await input('navigation-after-resize','click',await center(page.locator('nav [data-page-id="capture"]')));
 await page.locator('.capture-directory-setting').waitFor();
 check(report.errors.length===0,'页面出现错误');report.passed=true;
 await writeFile(resolve(output,'result.json'),JSON.stringify(report,null,2));
 await page.evaluate(async()=>{const api=window.go.main.App,s=await api.Snapshot();await api.SaveSettings({...s.settings,startup:{...s.settings.startup,keepUIRunning:false}});void api.WindowCommand('close');});
}catch(error){await writeFile(resolve(output,'node-error.txt'),String(error.stack||error));await writeFile(resolve(output,'result-partial.json'),JSON.stringify(report,null,2));process.exitCode=1;}
finally{if(browser)await browser.close().catch(()=>{});}
