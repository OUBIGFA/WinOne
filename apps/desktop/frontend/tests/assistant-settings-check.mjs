// Production settings with in-memory fixtures; never changes system settings.
import {createRequire} from "node:module";
import {mkdir,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
import assert from "node:assert/strict";
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||"_temp/assistant-settings-check");
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});
const base=process.env.WINONE_LAYOUT_BASE||"http://127.0.0.1:5198";
const report=[],errors=[];
const settle=page=>page.evaluate(()=>Promise.all([document.fonts.ready,...document.getAnimations().map(animation=>animation.finished.catch(()=>{}))]));
try{
 for(const [width,language,theme] of [[1100,"zh-CN","light"],[1100,"en-US","dark"],[520,"zh-CN","dark"],[390,"en-US","light"]]){
  const page=await browser.newPage({viewport:{width,height:1000},colorScheme:theme,reducedMotion:"reduce"});
  page.on("pageerror",error=>errors.push(String(error)));
  const zh=language==="zh-CN";
  await page.goto(base+'/tests/ai-settings-preview.html?language='+language+'&theme='+theme+'&manyModels=1');
  await page.getByLabel(zh?"API 基础地址":"API base URL",{exact:true}).waitFor();await settle(page);
  const summary=page.getByRole("region",{name:zh?"已启用供应商":"Enabled providers"});
  const originalText=await summary.innerText();
  const tabs=page.getByRole("tablist").first();const initial=await tabs.boundingBox();
  for(const title of zh?["账号供应商","默认 AI 服务","本地网关","自定义供应商"]:["Account providers","Default AI service","Local gateway","Custom providers"]){
   await page.getByRole("tab",{name:title,exact:true}).click();await settle(page);
   assert.equal(await summary.innerText(),originalText,"provider summary changed with tab");
   const rect=await tabs.boundingBox();assert.ok(Math.abs(rect.y-initial.y)<1&&Math.abs(rect.x-initial.x)<1,"tabs moved");
   await page.screenshot({path:resolve(output,width+'-'+language+'-'+theme+'-'+title+'.png')});
  }
  if(width===1100&&zh){
   const list=page.locator('[title="拖动右下角可调整列表高度"]');await list.scrollIntoViewIfNeeded();
   let box=await list.boundingBox();assert.ok(box.height>=390,"default list too short");
   await page.mouse.move(box.x+box.width-4,box.y+box.height-4);await page.mouse.down();await page.mouse.move(box.x+box.width-4,box.y+box.height+86,{steps:12});await page.mouse.up();
   await page.waitForFunction(()=>Number(localStorage.getItem('winone.ai-model-height.v1.one'))>450);
   const saved=await page.evaluate(()=>Number(localStorage.getItem('winone.ai-model-height.v1.one')));
   await page.getByRole('tab',{name:'账号供应商',exact:true}).click();await page.getByRole('tab',{name:'自定义供应商',exact:true}).click();
   assert.equal(await list.evaluate(e=>parseFloat(e.style.height)),saved,'model height did not survive remount');
   const sidebar=page.locator('[data-panel-id="app-sidebar"]');
   const handle=page.getByRole('separator',{name:'调整侧栏宽度'});await handle.scrollIntoViewIfNeeded();
   box=await handle.boundingBox();await page.mouse.move(box.x+box.width/2,box.y+box.height/2);await page.mouse.down();await page.mouse.move(64,box.y+box.height/2,{steps:20});await page.mouse.up();
   await page.waitForFunction(()=>document.querySelector('.shell')?.hasAttribute('data-navigation-compact'));
   const stored=await page.evaluate(()=>JSON.parse(localStorage.getItem('winone.sidebar.v1')));assert.ok(stored.width<=70,'sidebar did not reach icon width');
   box=await handle.boundingBox();await page.mouse.move(box.x+box.width/2,box.y+box.height/2);await page.mouse.down();await page.mouse.move(215,box.y+box.height/2,{steps:20});await page.mouse.up();
   await page.waitForFunction(()=>!document.querySelector('.shell')?.hasAttribute('data-navigation-compact'));
   await page.reload();await page.getByLabel('API 基础地址',{exact:true}).waitFor();
   const restored=await handle.boundingBox();assert.ok(restored.x>200&&restored.x<225,'sidebar width did not survive reload');
  }
  for(const fixture of ['shell-preview','selection-preview','capture-settings-preview','wintray-preview','module-preview']){
   await page.goto(base+'/tests/'+fixture+'.html?language='+language+'&theme='+theme);await page.locator('h1').waitFor();await settle(page);
   const pageTabs=await page.getByRole('tablist').count()?await page.getByRole('tablist').first().getByRole('tab').allTextContents():[];
   for(let i=-1;i<pageTabs.length;i++){
    if(i>=0)await page.getByRole('tablist').first().getByRole('tab',{name:pageTabs[i],exact:true}).click();await settle(page);
    const overflow=await page.evaluate(()=>[...document.querySelectorAll('html,body,main,[data-slot="page-scroll"]')].filter(e=>e.clientWidth&&e.scrollWidth>e.clientWidth+1).map(e=>e.tagName));
    assert.deepEqual(overflow,[],fixture+' horizontal overflow');
   }
   await page.screenshot({path:resolve(output,width+'-'+language+'-'+theme+'-'+fixture+'.png')});
  }
  await page.goto(base+'/tests/selection-preview.html?assistant=quick&language='+language+'&theme='+theme);
  await page.getByRole('button',{name:zh?'打开快捷助手':'Open quick assistant'}).waitFor();
  await page.getByRole('tab',{name:zh?'常规':'General',exact:true}).click();
  const pin=page.getByRole('switch',{name:zh?'默认置顶快捷助手':'Keep Quick assistant on top by default'});
  assert.equal(await pin.getAttribute('aria-checked'),'false');await pin.click();
  await page.getByRole('switch',{name:zh?'显示快捷助手的思考过程':'Show reasoning in Quick assistant'}).click();
  await page.getByRole('button',{name:zh?'保存设置':'Save settings',exact:true}).click();
  await settle(page);await page.screenshot({path:resolve(output,width+'-'+language+'-quick-general.png')});
  await page.getByRole('tab',{name:zh?'工具栏':'Toolbar',exact:true}).click();
  await page.getByRole('button',{name:zh?'添加自定义功能':'Add custom action'}).click();
  await page.getByRole('combobox',{name:zh?'图标':'Icon',exact:true}).click();
  await page.getByRole('option').first().waitFor();
  assert.ok(await page.getByRole('option').count()>=24,'custom icon selection is too small');
  await page.getByRole('option',{name:zh?'星形':'Astroid',exact:true}).click();
  await settle(page);await page.screenshot({path:resolve(output,width+'-'+language+'-quick-custom.png')});
  report.push({width,language,theme,tabsStable:true,overflow:false,quickPreferences:true});await page.close();
 }
 assert.deepEqual(errors,[],'page errors');
 await writeFile(resolve(output,'report.json'),JSON.stringify(report,null,2));console.log(JSON.stringify(report));
}finally{await browser.close();}
