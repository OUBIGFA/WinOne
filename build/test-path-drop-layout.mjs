// Production UI with the existing in-memory fixture. Native OLE is verified
// separately; these events exercise receiving forms and browser layout only.
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {pathToFileURL} from 'node:url';
import assert from 'node:assert/strict';
const frontend=resolve('apps/desktop/frontend'),output=resolve(process.env.WINONE_LAYOUT_OUTPUT||'_temp/path-drop-layout/browser');
await mkdir(output,{recursive:true});
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE||resolve('dist/resources/tools/playwright-1.58.2/node_modules/playwright'));
const {createServer}=await import(pathToFileURL(join(frontend,'node_modules/vite/dist/node/index.js')));
const server=await createServer({root:frontend,configFile:join(frontend,'vite.config.ts'),server:{host:'127.0.0.1',port:5206,strictPort:true,hmr:false}});await server.listen();
const browser=await chromium.launch({headless:true,channel:'msedge'}),errors=[],results=[];
let page;
async function open(id,width,theme='light',extra=''){
 if(page){errors.push(...await page.evaluate(()=>window.layoutErrors));await page.close();}
 page=await browser.newPage({viewport:{width,height:900},reducedMotion:'reduce',colorScheme:theme});page.setDefaultTimeout(15000);page.on('pageerror',error=>errors.push(String(error)));
 // ResizeObserver errors are ErrorEvents, which Vite may handle before CDP
 // reports pageerror. Keep them in the result instead of treating a quiet
 // pageerror stream as proof that layout callbacks were error-free.
 await page.addInitScript(()=>{window.layoutErrors=[];window.addEventListener('error',event=>window.layoutErrors.push(event.message));});
 await page.goto(`http://127.0.0.1:5206/tests/settings-design-preview.html?page=${id}&theme=${theme}${extra}`);
 await page.locator(`[data-settings-page="${id}"]`).waitFor();
}
async function boundaries(){
 await page.locator('[data-slot="settings-actions"]').waitFor();
 await page.evaluate(()=>new Promise(requestAnimationFrame));
 const value=await page.evaluate(()=>{
  const content=document.querySelector('[data-slot="page-content"]'),rail=document.querySelector('[data-slot="settings-actions"]'),frame=document.querySelector('.settings-frame');
  const viewport=document.querySelector('[data-slot="page-scroll"]');
  const buttons=[...rail.querySelectorAll('button')].filter(button=>button.getClientRects().length);
  return {content:content.getBoundingClientRect().toJSON(),viewport:viewport.getBoundingClientRect().toJSON(),rail:rail.getBoundingClientRect().toJSON(),frame:frame.getBoundingClientRect().toJSON(),overflow:document.documentElement.scrollWidth>innerWidth,scroll:viewport.scrollWidth,buttonsOnTop:buttons.every(button=>{const box=button.getBoundingClientRect();return button.contains(document.elementFromPoint(box.x+box.width/2,box.y+box.height/2));})};
 });
 // Overlay actions intentionally share the content area; they must not reserve
 // a side lane or shorten the viewport as the former footer did.
 assert.ok(Math.abs(value.viewport.width-value.frame.width)<=1&&Math.abs(value.viewport.height-value.frame.height)<=1,`Action rail reserves content space: ${JSON.stringify(value)}`);
 assert.equal(value.buttonsOnTop,true,'Page content covers an action button');
 assert.ok(value.frame.right-value.rail.right>=11.5&&value.frame.right-value.rail.right<=40,'Rail needs a safe right inset');
 assert.equal(value.overflow,false);return value;
}
async function drop(locator,paths){await locator.evaluate((element,paths)=>element.dispatchEvent(new CustomEvent('winone:paths-drop',{bubbles:true,cancelable:true,detail:{paths}})),paths);}
try{
 for(const theme of ['light','dark'])for(const width of [520,1000,1600]){
  await open('wintray',width,theme);await page.getByRole('button',{name:'添加程序',exact:true}).waitFor();const programs=await boundaries();
  await drop(page.locator('.wintray-page'),['C:\\WinOne-test\\dropped.exe','C:\\WinOne-test\\job.ps1']);
  await page.getByRole('listitem',{name:'job',exact:true}).waitFor();
  assert.equal(await page.evaluate(()=>window.settingsDesignFixture.calls.some(call=>call.method==='wintray.settings.save')),false);
  await page.screenshot({path:join(output,`programs-${theme}-${width}.png`)});
  await open('capture',width,theme);await page.locator('.settings-save-bar button').first().waitFor();const capture=await boundaries();
  await drop(page.locator('.capture-directory-setting'),['C:\\WinOne-test\\DroppedFolder']);await page.getByText('C:\\WinOne-test\\DroppedFolder',{exact:true}).waitFor();
  await drop(page.locator('.capture-directory-setting'),['C:\\WinOne-test\\invalid.txt']);await page.getByText(/此处只接收文件夹/).waitFor();assert.equal(await page.locator('#capture-path').textContent(),'C:\\WinOne-test\\DroppedFolder');
  results.push({scenario:'right-side action rail / validated drops',theme,width,programs,capture});
 }
 for(const width of [520,1280]){
  await open('convenience',width,'dark');await page.getByRole('tab',{name:'鼠标手势',exact:true}).click();await page.locator('.stroke-scene-row').first().waitFor();
  const rows=await page.locator('.stroke-scene-row').evaluateAll(rows=>rows.map(row=>{const button=row.querySelector('button'),label=row.querySelector('.stroke-scene-label'),toggle=row.querySelector('[role="switch"]'),icon=row.querySelector('svg');return {name:label.textContent,whiteSpace:getComputedStyle(label).whiteSpace,label:label.getBoundingClientRect().toJSON(),toggle:toggle.getBoundingClientRect().toJSON(),icon:icon.classList.toString(),buttons:row.querySelectorAll('button').length};}));
  for(const row of rows){assert.equal(row.whiteSpace,'nowrap');assert.ok(Math.abs(row.label.y+row.label.height/2-row.toggle.y-row.toggle.height/2)<2);assert.ok(row.toggle.x>row.label.x);}
  const first=page.locator('.stroke-scene-row').first(),selection=await first.locator('button').first().getAttribute('aria-pressed');const toggle=first.getByRole('switch'),before=await toggle.getAttribute('aria-checked');await toggle.click();assert.notEqual(await toggle.getAttribute('aria-checked'),before);assert.equal(await first.locator('button').first().getAttribute('aria-pressed'),selection);
  await boundaries();await page.screenshot({path:join(output,`scenes-${width}.png`)});results.push({scenario:'single-line scene controls / task icons / independent toggle',width,rows});
 }
 await open('convenience',1000,'light','&history-states=1');await page.getByRole('tab',{name:/文件.*链接/}).click();await page.getByRole('tab',{name:'查找已有链接'}).click();
 await page.locator('.directory-links-history-group').first().waitFor();assert.equal(await page.locator('.directory-links-history-group').count(),3);assert.equal(await page.locator('.directory-links-history-group[open]').count(),0);
 await page.locator('.directory-links-history-group').filter({hasText:'example-junction'}).locator(':scope > summary').click();
 const invalid=await page.locator('.directory-links-record[data-invalid]').first().evaluate(element=>({color:getComputedStyle(element).color,kind:getComputedStyle(element.querySelector('.directory-links-kind')).color}));assert.equal(invalid.color,invalid.kind);
 await page.screenshot({path:join(output,'history-grouped.png')});await page.getByRole('button',{name:'一键删除失效历史'}).click();await page.getByText('已删除 1 条失效历史，真实文件和链接未改变。').waitFor();assert.equal(await page.locator('.directory-links-record[data-invalid]').count(),0);assert.equal(await page.locator('.directory-links-history-group').count(),2);
 results.push({scenario:'grouped history / muted invalid record / history-only clear',invalid});
 errors.push(...await page.evaluate(()=>window.layoutErrors));
 assert.deepEqual(errors,[]);await writeFile(join(output,'report.json'),JSON.stringify({results,errors},null,2));
 console.log(`Passed ${results.length} scenarios; ${errors.length} page errors.`);
}catch(error){await page?.screenshot({path:join(output,'failure.png')}).catch(()=>{});await writeFile(join(output,'failure.json'),JSON.stringify({error:String(error.stack||error),results,errors},null,2));throw error;}
finally{await page?.close();await browser.close();await server.close();}
