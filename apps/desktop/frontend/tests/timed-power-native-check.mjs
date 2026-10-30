// Real Rust UI + Host IPC; only reminder-only tasks may be armed by this driver.
// Entry: build/test-timed-power.ps1 (fresh profile, every other module disabled).
import {createRequire} from 'node:module';
import {writeFile,readFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {execFileSync} from 'node:child_process';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2),errors=[],views=[],checks=[];
let browser,page;
try{
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(r=>setTimeout(r,100));}}
 if(!browser)throw Error('Owned UI did not expose the test connection');
 while(Date.now()<deadline){page=browser.contexts().flatMap(c=>c.pages()).find(p=>p.url().includes('winone.localhost'));if(page)break;await new Promise(r=>setTimeout(r,100));}
 if(!page)throw Error('Owned UI missing');
 page.setDefaultTimeout(10000);page.on('pageerror',e=>errors.push(String(e)));
 await page.waitForFunction(()=>performance.getEntriesByName('winone-ui-ready').length>0);
 const nativeViewport=await page.evaluate(()=>({width:innerWidth,height:innerHeight}));
 const call=(method,params={})=>page.evaluate(async({method,params})=>JSON.parse(await window.go.main.App.TimedPower(method,JSON.stringify(params))),{method,params});
 const open=async()=>{await page.locator('.app-navigation [data-page-id="convenience"]').click();await page.getByRole('tab',{name:/^(定时关机|Shutdown timer)$/}).click();await page.getByRole('switch',{name:/^(强制关闭程序|Force close applications)$/}).waitFor();};
 const field=name=>page.getByRole('spinbutton',{name,exact:true});
 const fill=async(h,m,s)=>{for(const [name,value]of [['时',h],['分',m],['秒',s]])await field(name).fill(String(value));};
 const save=async()=>{await page.getByRole('button',{name:'保存设置',exact:true}).click();await page.getByText('已保存。保存不会开始或改变任务。',{exact:true}).waitFor();};
 const startSafe=async()=>{assert.equal((await call('settings.get')).dryRun,true);assert.equal(await page.getByRole('switch',{name:'仅提醒，不执行电源操作',exact:true}).isChecked(),true);await page.getByRole('button',{name:'开始计时',exact:true}).click();await page.getByText('正在计时',{exact:true}).waitFor();};
 const moduleSwitch=()=>page.getByRole('switch',{name:'启用定时关机',exact:true});
 const settle=async()=>page.evaluate(async()=>{await document.fonts.ready;document.activeElement?.blur();await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));});
 const initial=await call('settings.get');assert.equal(initial.force,true);assert.equal(initial.dryRun,false);
 const host=await page.evaluate(()=>window.go.main.App.Snapshot());
 assert.equal(host.settings.startup.runAtLogon,false);assert.equal(!!host.settings.startup.preferAdministrator,false);
 assert.ok(host.modules.filter(m=>m.id!=='timed-power').every(m=>m.state==='disabled'),'Every other module must remain disabled');
 await open();
 assert.deepEqual(await page.getByRole('tablist').first().getByRole('tab').allTextContents(),['置顶功能','鼠标手势','文件与目录链接','定时关机']);
 await page.getByRole('button',{name:'开始计时',exact:true}).click();
 assert.match(await page.getByRole('dialog').textContent(),/未保存内容可能丢失/);
 await page.getByRole('button',{name:'继续编辑',exact:true}).click();
 assert.equal((await call('snapshot')).active,false);
 checks.push('default force; closing confirmation does not arm');
 for(const [label,h,m] of [['15 分钟','0','15'],['30 分钟','0','30'],['1 小时','1','0'],['1 小时 30 分钟','1','30'],['2 小时','2','0'],['2 小时 30 分钟','2','30']]){
  await page.getByRole('button',{name:label,exact:true}).click();
  assert.deepEqual(await Promise.all(['时','分','秒'].map(name=>field(name).inputValue())),[h,m,'0']);
 }
 checks.push('all six quiet outlined presets fill the three fields on click');
 await fill(2,30,0);await save();assert.equal((await call('settings.get')).delaySeconds,9000);
 await field('分').fill('15');await page.locator('.app-navigation [data-page-id="overview"]').click();
 await page.getByRole('dialog').waitFor();await page.getByRole('button',{name:'继续编辑',exact:true}).click();assert.equal(await field('分').inputValue(),'15');
 await page.getByRole('button',{name:'撤销修改',exact:true}).click();assert.equal(await field('分').inputValue(),'30');
 checks.push('typed 2h30m; real Host save; navigation and undo preserve all three segments');
 // Sample actual descendants, not only the Host. These are focused idle/active
 // samples, not a full bundled-module or input-to-photon benchmark.
 const sample=name=>execFileSync('powershell.exe',['-NoProfile','-File',resolve('build/measure-resources.ps1'),'-RootProcessIds',process.env.WINONE_COMPONENTS_PID,'-OutputPath',join(output,name+'.json'),'-DurationSeconds','8','-Scenario','timed-power-'+name],{windowsHide:true,timeout:30000});
 sample('idle-resources');
 await page.getByRole('switch',{name:'仅提醒，不执行电源操作',exact:true}).click();
 await fill(0,1,0);await save();await startSafe();assert.equal((await call('snapshot')).task.dryRun,true);
 sample('active-resources');
 await page.getByRole('button',{name:'取消任务',exact:true}).click();await page.getByText('任务已取消',{exact:true}).waitFor();
 await fill(0,0,2);await save();await startSafe();await page.getByText('提醒已完成，未执行电源操作',{exact:true}).waitFor();
 let state=await call('snapshot');assert.equal(state.state,'reminded');assert.equal(state.active,false);
 checks.push('countdown reminder-only executes; active task cancels; idle/active process-family samples');
 await page.getByRole('radio',{name:'指定时刻',exact:true}).click();
 const target=new Date(Date.now()+6000);await fill(target.getHours(),target.getMinutes(),target.getSeconds());await save();
 const clock=[target.getHours(),target.getMinutes(),target.getSeconds()].map(n=>String(n).padStart(2,'0')).join(':');
 assert.equal((await call('settings.get')).timeOfDay,clock);
 await startSafe();await page.getByText('提醒已完成，未执行电源操作',{exact:true}).waitFor();
 state=await call('snapshot');assert.equal(new Date(state.deadline).getSeconds(),target.getSeconds());
 checks.push('specified local time preserves seconds through UI, Host persistence and actual reminder');
 await page.getByRole('radio',{name:'每周重复',exact:true}).click();await fill(23,59,37);await save();await startSafe();
 state=await call('snapshot');assert.equal(state.task.mode,'weekly');assert.equal(new Date(state.deadline).getSeconds(),37);
 await moduleSwitch().click();
 await page.getByText('功能已关闭，仍可编辑和保存设置。',{exact:true}).waitFor();
 await moduleSwitch().click();
 // The button is visible even while disabled. Wait for the completed module
 // transition rather than querying a gate which is still applying the change.
 await page.getByText('功能已关闭，仍可编辑和保存设置。',{exact:true}).waitFor({state:'hidden'});
 await page.getByText('已开启',{exact:true}).waitFor();
 assert.equal(await moduleSwitch().isChecked(),true);
 assert.equal((await call('snapshot')).active,false);
 checks.push('weekly second precision; disabling cancels; enabling never resumes');
 // All visuals use real bundled UI. Check wide/narrow views, every time mode,
 // both languages/themes, and the actual Windows frame for representative views.
 for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark']){
  await page.evaluate(async({language,theme})=>{const app=window.go.main.App,s=await app.Snapshot();await app.SaveSettings({...s.settings,language,theme});},{language,theme});
  await page.reload({waitUntil:'domcontentloaded'});await page.waitForFunction(()=>performance.getEntriesByName('winone-ui-ready').length>0);await open();
  for(const width of [520,1100]){
   await page.setViewportSize({width,height:1000});
   for(const [mode,label]of [['countdown',language==='zh-CN'?'倒计时':'Countdown'],['time',language==='zh-CN'?'指定时刻':'Time of day'],['weekly',language==='zh-CN'?'每周重复':'Weekly']]){
    await page.getByRole('radio',{name:label,exact:true}).click();
    await page.locator('[data-slot="page-scroll"]').evaluate(e=>{e.scrollTop=0;});await page.mouse.move(0,0);await settle();
    const metrics=await page.evaluate(()=>{
     const visible=e=>e.getClientRects().length&&e.clientWidth>0;
     const overflow=[...document.querySelectorAll('html,body,[data-slot="page-scroll"],[data-slot="page-content"],fieldset')].filter(e=>visible(e)&&e.scrollWidth>e.clientWidth+1).map(e=>e.getAttribute('aria-label')||e.tagName);
     const parts=[...document.querySelectorAll('fieldset[aria-label] input[type="number"]')].filter(visible).map(e=>{
      const b=e.getBoundingClientRect(),style=getComputedStyle(e),label=document.querySelector(`label[for="${CSS.escape(e.id)}"]`),unit=label.querySelector('[aria-hidden="true"]'),u=unit.getBoundingClientRect();
      return{x:b.x,y:b.y,width:b.width,height:b.height,inputColor:style.color,unit:{text:unit.textContent,x:u.x,y:u.y,width:u.width,height:u.height,color:getComputedStyle(label).color},textEnd:b.right-parseFloat(style.paddingRight)};
     });
     const presets=[...document.querySelectorAll('.timed-power-presets button')].filter(visible).map(e=>{const s=getComputedStyle(e);return{text:e.textContent,color:s.color,background:s.backgroundColor,border:s.borderColor,fontWeight:s.fontWeight,height:e.getBoundingClientRect().height};});
     return{overflow,parts,presets};
    });
    assert.deepEqual(metrics.overflow,[]);assert.equal(metrics.parts.length,3);
    assert.ok(metrics.parts.every(p=>Math.abs(p.y-metrics.parts[0].y)<1),'Time fields must stay on one row');
    assert.ok(metrics.parts.every(p=>Math.abs(p.width-metrics.parts[0].width)<1),'Time fields share width');
    for(const p of metrics.parts){
     assert.ok(p.unit.x>p.textEnd&&p.unit.x+p.unit.width<p.x+p.width,'Unit stays inside the right edge, clear of digits');
     assert.ok(Math.abs((p.unit.y+p.unit.height/2)-(p.y+p.height/2))<1,'Unit is vertically centered in the input');
     assert.notEqual(p.unit.color,p.inputColor,'Unit must remain secondary to the editable digits');
    }
    if(mode==='countdown'){
     assert.equal(metrics.presets.length,6,'Every quick duration remains available');
     for(const preset of metrics.presets){
      assert.equal(preset.color,metrics.parts[0].unit.color,'Presets share the muted unit tone');
      assert.equal(preset.fontWeight,'400');assert.notEqual(preset.border,'rgba(0, 0, 0, 0)','Presets have a visible quiet button boundary');
      assert.ok(preset.height>=32,'Muted shortcuts must retain their hit area');
     }
    }
    const file=`${language}-${theme}-${width}-${mode}.png`;await page.screenshot({path:join(output,file)});views.push({language,theme,width,mode,file,...metrics});
   }
  }
  const undo=page.getByRole('button',{name:language==='zh-CN'?'撤销修改':'Discard changes',exact:true});if(await undo.isEnabled())await undo.click();
 }
 await page.evaluate(async()=>{const app=window.go.main.App,s=await app.Snapshot();await app.SaveSettings({...s.settings,language:'zh-CN',theme:'light'});});
 await page.reload({waitUntil:'domcontentloaded'});await page.waitForFunction(()=>performance.getEntriesByName('winone-ui-ready').length>0);await open();
 if(process.env.WINONE_WINDOW_SHOT_EXE){
  await page.setViewportSize(nativeViewport);
  await page.getByRole('radio',{name:'倒计时',exact:true}).click();
  await page.getByRole('button',{name:'2 小时 30 分钟',exact:true}).click();
  await page.locator('[data-slot="page-scroll"]').evaluate(e=>{e.scrollTop=0;});
  await page.mouse.move(0,0);await settle();execFileSync(process.env.WINONE_WINDOW_SHOT_EXE,[process.env.WINONE_COMPONENTS_PID,join(output,'native-window.png'),'-wait','250'],{windowsHide:true,timeout:15000});
  await page.getByRole('button',{name:'撤销修改',exact:true}).click();
 }
 const disk=JSON.parse(await readFile(join(process.env.WINONE_COMPONENTS_PROFILE,'configuration.json'),'utf8')).committed;
 assert.equal(disk.values['timed-power'].timeOfDay,'23:59:37');assert.equal(disk.values['timed-power'].dryRun,true);assert.equal(disk.builtinVersion,5);
 assert.deepEqual(errors,[]);
 await writeFile(join(output,'result.json'),JSON.stringify({checks,views,errors,realPowerActions:false,source:'bundled Rust UI + real Host; isolated profile'},null,2));
 await page.evaluate(()=>{void window.go.main.App.WindowCommand('close');});
}catch(error){
 await writeFile(join(output,'node-error.txt'),String(error.stack||error));
 if(page)await page.screenshot({path:join(output,'failure.png')}).catch(()=>{});
 process.exitCode=1;
}finally{await browser?.close();}
