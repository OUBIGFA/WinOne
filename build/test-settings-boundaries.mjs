// Settings boundary checks use the real App with an in-memory Host. Forced
// colors are browser-emulated; this never changes Windows contrast settings.
// Run from the repository root: node build/test-settings-boundaries.mjs
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {pathToFileURL} from 'node:url';
import {execFileSync} from 'node:child_process';
import assert from 'node:assert/strict';

const frontend=resolve('apps/desktop/frontend');
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||'_temp/settings-boundaries/review');
execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);
await mkdir(join(output,'browser-temp'),{recursive:true});
process.env.TEMP=process.env.TMP=join(output,'browser-temp');
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE||resolve('dist/resources/tools/playwright-1.58.2/node_modules/playwright'));
const {createServer}=await import(pathToFileURL(join(frontend,'node_modules/vite/dist/node/index.js')));
const server=await createServer({root:frontend,configFile:join(frontend,'vite.config.ts'),server:{host:'127.0.0.1',port:5198,strictPort:true}});
await server.listen();
const browser=await chromium.launch({headless:true,...(process.env.WINONE_CHROMIUM?{executablePath:process.env.WINONE_CHROMIUM}:{channel:'msedge'})});
const results=[],errors=[];
let page;
async function open({id='settings',width=1000,language='zh-CN',theme='light',scale=1,forcedColors='none'}={}){
 await page?.close();
 page=await browser.newPage({viewport:{width,height:920},colorScheme:theme,forcedColors,reducedMotion:'reduce'});
 page.setDefaultTimeout(10000);
 page.on('pageerror',error=>errors.push(String(error)));
 await page.goto(`http://127.0.0.1:5198/tests/settings-design-preview.html?page=${id}&theme=${theme}&language=${language}`);
 await page.locator(`[data-settings-page="${id}"]`).waitFor();
 if(id==='settings')await page.locator('#theme-dark:not([data-disabled]):not(:disabled)').waitFor();
 await page.evaluate(async scale=>{document.documentElement.style.fontSize=`${16*scale}px`;await document.fonts.ready;await new Promise(requestAnimationFrame);},scale);
}
try{
 // Compare real computed colors, not source text: a theme alias or a portal
 // escaping the settings scope would otherwise go unnoticed. Pure grayscale
 // is the user contract; contrast remains a separate accessibility check.
 for(const theme of ['light','dark']){
  await open({theme});
  const metrics=await page.evaluate(()=>{
   const style=getComputedStyle(document.documentElement);
   const names=['settings-canvas','settings-surface','settings-well','settings-hover','settings-ink','settings-muted','settings-accent','settings-selected','settings-selected-text','settings-input-edge','destructive'];
   const rgb=value=>value.startsWith('#')?[1,3,5].map(i=>parseInt(value.slice(i,i+2),16)):value.match(/[\d.]+/g).slice(0,3).map(Number);
   const colors=Object.fromEntries(names.map(name=>[name,rgb(style.getPropertyValue('--'+name).trim())]));
   const luminance=channels=>channels.map(v=>v/255).map(v=>v<=.04045?v/12.92:((v+.055)/1.055)**2.4).reduce((sum,v,i)=>sum+v*[.2126,.7152,.0722][i],0);
   const contrast=(a,b)=>{const x=luminance(a),y=luminance(b);return (Math.max(x,y)+.05)/(Math.min(x,y)+.05);};
   return {colors,contrast:{text:contrast(colors['settings-ink'],colors['settings-canvas']),help:contrast(colors['settings-muted'],colors['settings-canvas']),emptyRadio:contrast(colors['settings-input-edge'],colors['settings-well']),input:contrast(colors['settings-input-edge'],colors['settings-surface']),focus:contrast(colors['settings-accent'],colors['settings-canvas']),marker:contrast(colors['settings-canvas'],colors['settings-accent'])}};
  });
  // Chrome is neutral; 'destructive' is a meaning colour and must stay coloured
  // so errors and failed requests keep their signal.
  for(const [name,[r,g,b]] of Object.entries(metrics.colors)){
   if(name==='destructive'){assert.ok(!(r===g&&g===b),`${theme}/${name} must keep its meaning colour`);continue;}
   assert.ok(r===g&&g===b,`${theme}/${name} must be grayscale`);
  }
  for(const [name,value] of Object.entries(metrics.contrast))assert.ok(value>=(name==='text'||name==='help'?4.5:3),`${theme}/${name} contrast ${value}`);
  // The current page tab is marked by its underline and label weight, never by
  // a filled block that competes with the settings controls below it.
  const activeTab=page.locator('[data-slot="tabs-list"][data-variant="default"] > [data-slot="tabs-trigger"][data-active]').first();
  const tabStyle=await activeTab.evaluate(tab=>{const style=getComputedStyle(tab),mark=getComputedStyle(tab,'::after');return {background:style.backgroundColor,mark:mark.opacity,markHeight:parseFloat(mark.height)};});
  assert.equal(tabStyle.background,'rgba(0, 0, 0, 0)','The current page must not be marked by a filled block');
  assert.equal(tabStyle.mark,'1','The current page keeps a visible underline');
  assert.ok(tabStyle.markHeight>=2,`The underline must stay visible, got ${tabStyle.markHeight}`);
  const themeGroup=page.getByRole('radiogroup',{name:'主题',exact:true});
  const before=await themeGroup.boundingBox();
  await themeGroup.getByRole('radio',{name:'跟随系统',exact:true}).click();
  const after=await themeGroup.boundingBox();
  for(const key of ['x','y','width','height'])assert.ok(Math.abs(before[key]-after[key])<=1,'Choosing another segment must not shift its geometry');
  await page.getByRole('combobox',{name:'起始页面',exact:true}).click();
  const popup=page.locator('[data-slot="select-content"]');await popup.waitFor();
  const popupStyle=await popup.evaluate(el=>{const s=getComputedStyle(el);return {background:s.backgroundColor,border:parseFloat(s.borderTopWidth),color:s.color};});
  assert.equal(popupStyle.border,1);assert.equal(popupStyle.background,theme==='dark'?'rgb(35, 35, 35)':'rgb(255, 255, 255)');
  await page.keyboard.press('Escape');
  results.push({scenario:'grayscale / contrast / stable segment / portalled menu',theme,...metrics,popupStyle});

  await open({id:'ai-services',theme,width:520});
  await page.getByRole('button',{name:'模型设置：shared-model',exact:true}).click();
  const dialog=page.getByRole('dialog');await dialog.waitFor();
  const dialogStyle=await dialog.evaluate(el=>{const s=getComputedStyle(el);return {background:s.backgroundColor,border:parseFloat(s.borderTopWidth),width:el.clientWidth,scrollWidth:el.scrollWidth};});
  assert.equal(dialogStyle.background,theme==='dark'?'rgb(35, 35, 35)':'rgb(255, 255, 255)');assert.equal(dialogStyle.border,1);assert.ok(dialogStyle.scrollWidth<=dialogStyle.width+1);
  assert.equal(await dialog.getByRole('radio').count(),9,'All three model capabilities retain default/on/off choices');
  await page.screenshot({path:join(output,`model-dialog-${theme}.png`)});
  await page.keyboard.press('Escape');await dialog.waitFor({state:'hidden'});
  results.push({scenario:'portalled model editor / complete capabilities / narrow dialog',theme,dialogStyle});
 }
 for(const theme of ['light','dark']){
  await open({theme,forcedColors:'active'});
  const group=page.locator('[aria-labelledby="theme-label"]');
  const checked=group.locator('[role="radio"][aria-checked="true"]');
  const colors=await checked.evaluate(radio=>({background:getComputedStyle(radio).backgroundColor,dot:getComputedStyle(radio.querySelector('[data-slot="radio-group-indicator"] span')).backgroundColor}));
  assert.notEqual(colors.dot,colors.background,'The selected radio mark must survive forced colors');
  await checked.focus();await page.keyboard.press('ArrowLeft');
  const focused=group.locator('[role="radio"]:focus-visible');
  await focused.waitFor();
  const focus=await focused.evaluate(radio=>{const style=getComputedStyle(radio.closest('.settings-choice'));return {outline:style.outlineStyle,width:parseFloat(style.outlineWidth)};});
  assert.notEqual(focus.outline,'none');assert.ok(focus.width>=2);
  assert.equal(await group.locator('[aria-checked="true"]').count(),1);
  await page.screenshot({path:join(output,`forced-colors-${theme}.png`)});
  results.push({scenario:'forced-colors',theme,colors,focus});
 }
 for(const width of [390,1000])for(const language of ['zh-CN','en-US']){
  await open({width,language,scale:width===390?2:1});
  await page.evaluate(()=>{
   const app=window.go.main.App,save=app.SaveSettings;
   // A manually released promise tests a slow Host without inventing latency
   // or changing the application's timeout and concurrency behavior.
   window.settingsSaveProbe={calls:0};
   app.SaveSettings=value=>{window.settingsSaveProbe.calls++;return new Promise((resolve,reject)=>{window.settingsSaveProbe.finish=()=>save(value).then(resolve,reject);});};
  });
  const choices=page.getByRole('radiogroup',{name:language==='zh-CN'?'主题':'Theme',exact:true});
  const darkChoice=choices.getByRole('radio',{name:language==='zh-CN'?'深色':'Dark',exact:true});
  const lightChoice=choices.getByRole('radio',{name:language==='zh-CN'?'浅色':'Light',exact:true});
  await darkChoice.click();
  const status=page.getByRole('status').filter({hasText:language==='zh-CN'?'正在保存…':'Saving…'});
  await status.waitFor();
  const bounds=await status.boundingBox(),caption=await page.locator('[data-slot="window-titlebar"]').boundingBox();
  assert.ok(bounds.y>=caption.y+caption.height&&bounds.y+bounds.height<=920,'Saving feedback must stay in the visible page');
  assert.ok(bounds.x>=0&&bounds.x+bounds.width<=width,'Saving feedback must fit the narrow pane');
  assert.equal(await lightChoice.getAttribute('aria-checked'),'true','Unconfirmed choices must not look saved');
  assert.equal(await darkChoice.isDisabled(),true);
  // Playwright refuses to click a disabled control, so force the click to
  // reproduce a real attempt on an unconfirmed choice during a slow save.
  await page.locator('[aria-labelledby="theme-label"] .settings-choice').first().click({force:true});
  assert.equal(await page.evaluate(()=>window.settingsSaveProbe.calls),1,'Disabled choices must not submit another save');
  await page.screenshot({path:join(output,`saving-${width}-${language}.png`)});
  await page.evaluate(()=>window.settingsSaveProbe.finish());
  await page.waitForFunction(()=>document.documentElement.dataset.theme==='dark');
  await status.waitFor({state:'hidden'});
  assert.equal(await darkChoice.getAttribute('aria-checked'),'true');
  const overflow=await page.locator('[data-settings-page="settings"]').evaluate(root=>[document.documentElement,root,root.parentElement].filter(e=>e.scrollWidth>e.clientWidth+1).map(e=>({element:e.className||e.tagName,client:e.clientWidth,scroll:e.scrollWidth})));
  assert.deepEqual(overflow,[]);
  results.push({scenario:'slow-save',width,language,scale:width===390?2:1,feedback:bounds});
 }
}catch(error){
 errors.push(String(error));
 await page?.screenshot({path:join(output,'failure.png')}).catch(error=>errors.push('Failure screenshot: '+error));
}finally{
 await writeFile(join(output,'report.json'),JSON.stringify({results,errors},null,2));
 await browser.close();await server.close();
 console.log(JSON.stringify({scenarios:results.length,errors:errors.length,output}));
 if(errors.length)process.exitCode=1;
}
