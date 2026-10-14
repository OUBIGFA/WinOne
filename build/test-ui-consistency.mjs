// Real renderer checks for startup navigation, nested tabs, and shipped gesture
// defaults. All state is isolated in the browser context and the fixture bridge.
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {execFileSync} from 'node:child_process';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const base=process.env.WINONE_LAYOUT_BASE||'http://127.0.0.1:5198';
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||'_temp/ui-recheck-20261011/shared');
if(new URL(base).hostname!=='127.0.0.1')throw Error('Use the isolated local preview');
execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,channel:'msedge'}),results=[];
try{
 for(const width of [640,1120])for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark']){
  const page=await browser.newPage({viewport:{width,height:900},reducedMotion:'reduce'}),errors=[];
  page.on('pageerror',error=>errors.push(error.message));
  await page.goto(`${base}/tests/ui-consistency-preview.html?language=${language}&theme=${theme}`);
  const en=language==='en-US',rail=page.locator('.app-navigation');
  await rail.waitFor();
  await page.waitForFunction(()=>Math.round(document.querySelector('.app-navigation').getBoundingClientRect().width)===64);
  await page.getByRole('radio',{name:en?'Expanded by default':'默认展开'}).click();
  if(width>767)await page.waitForFunction(()=>document.querySelector('.app-navigation').getBoundingClientRect().width>180);
  await page.reload();await rail.waitFor();
  if(width>767)await page.waitForFunction(()=>document.querySelector('.app-navigation').getBoundingClientRect().width>180);
  await page.getByRole('radio',{name:en?'Collapsed by default':'默认收起'}).click();
  await page.waitForFunction(()=>Math.round(document.querySelector('.app-navigation').getBoundingClientRect().width)===64);
  await page.getByRole('button',{name:en?'Mouse gestures':'鼠标手势',exact:true}).click();
  await page.locator('.stroke-action-row').first().waitFor();
  const names=await page.locator('.stroke-action-copy>span:first-child').allTextContents();
  if(JSON.stringify(names.slice(0,3))!==JSON.stringify(['后退','前进','回到上一层']))throw Error('Wrong first three actions: '+names.slice(0,3));
  const enabled=await page.locator('.stroke-action-controls [role="switch"][aria-checked="true"]').count();
  if(enabled!==3)throw Error('Expected exactly three enabled gestures, got '+enabled);
  const bindings=await page.locator('.stroke-action-row').evaluateAll(rows=>Object.fromEntries(rows.filter(row=>['回到上一层','复制'].includes(row.querySelector('.stroke-action-copy>span').textContent)).map(row=>[row.querySelector('.stroke-action-copy>span').textContent,row.querySelector('[role="combobox"]').textContent])));
  if(!bindings['回到上一层']?.includes('↑')&&!bindings['回到上一层']?.includes('上'))throw Error('Up action binding unavailable: '+JSON.stringify(bindings));
  const tabs=page.locator('[role="tablist"] [role="tab"]'),tabMetrics=[];
  for(let index=0;index<await tabs.count();index++){
   await tabs.nth(index).click();
   const metrics=await tabs.nth(index).evaluate(node=>{const after=getComputedStyle(node,'::after'),style=getComputedStyle(node);return {text:node.textContent,selected:node.getAttribute('aria-selected'),opacity:after.opacity,height:after.height,width:after.width,background:style.backgroundColor,shadow:style.boxShadow};});
   if(metrics.selected!=='true'||metrics.opacity!=='1'||parseFloat(metrics.height)!==2||parseFloat(metrics.width)<=0||metrics.background!=='rgba(0, 0, 0, 0)')throw Error('Underline tab not rendered: '+JSON.stringify(metrics));
   tabMetrics.push(metrics);
  }
  await tabs.first().click();
  const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth);
  if(overflow||errors.length)throw Error(JSON.stringify({overflow,errors}));
  await page.screenshot({path:join(output,`gestures-${width}-${language}-${theme}.png`)});
  results.push({width,language,theme,defaultCollapsed:true,configuredStartup:true,firstActions:names.slice(0,3),enabled,bindings,tabs:tabMetrics});await page.close();
 }
 await writeFile(join(output,'report.json'),JSON.stringify(results,null,2));console.log(JSON.stringify({cases:results.length,passed:true}));
}finally{await browser.close();}
