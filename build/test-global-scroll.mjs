// Reflow, real scrollbar allocation, and empty-list centering use production
// components. Host calls stay in the existing in-memory browser fixtures.
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {pathToFileURL} from 'node:url';
import {execFileSync} from 'node:child_process';
const output=resolve(process.argv[2]||'_temp/global-scroll'),frontend=resolve('apps/desktop/frontend');
execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);
await mkdir(output,{recursive:true});
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE||resolve('dist/resources/tools/playwright-1.58.2/node_modules/playwright'));
const {createServer}=await import(pathToFileURL(join(frontend,'node_modules/vite/dist/node/index.js')));
const server=await createServer({root:frontend,configFile:join(frontend,'vite.config.ts'),server:{host:'127.0.0.1',port:0,hmr:false}});
await server.listen();
const browser=await chromium.launch({headless:true,channel:'msedge'}),results=[];let page;
const base=server.resolvedUrls.local[0];
const settle=()=>page.evaluate(async()=>{await document.fonts.ready;await new Promise(done=>requestAnimationFrame(()=>requestAnimationFrame(done)));});
async function check(root,label){
 await settle();
 const metrics=await root.evaluate(root=>{
  const bars=[],gutter=[],clipped=[];
  for(const el of [root,...root.querySelectorAll('*')]){
   if(!(el instanceof HTMLElement)||!el.clientWidth||!el.getClientRects().length||el.closest('[hidden],[aria-hidden=true]'))continue;
   const s=getComputedStyle(el);if(s.visibility==='hidden')continue;
   const detail={tag:el.tagName,class:el.className,width:el.clientWidth,scroll:el.scrollWidth};
   if(s.overflowX==='scroll'||s.overflowX==='auto'&&el.scrollWidth>el.clientWidth+1)bars.push(detail);
   if(s.scrollbarGutter.includes('stable')&&el.scrollHeight<=el.clientHeight+1)gutter.push(detail);
   if(el.matches('.chat-dialog-field,.chat-project-permissions,.chat-dialog-location,.settings-group,.wintray-list-heading')&&el.scrollWidth>el.clientWidth+1)clipped.push(detail);
  }
  return {bars,gutter,clipped};
 });
 assert.deepEqual(metrics,{bars:[],gutter:[],clipped:[]},label);return metrics;
}
try{
 for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark'])for(const width of [520,900,1280]){
  page=await browser.newPage({viewport:{width,height:940},reducedMotion:'reduce'});
  await page.addInitScript(()=>{window.layoutErrors=[];window.addEventListener('error',event=>window.layoutErrors.push(event.message));});
  await page.goto(base+`tests/settings-design-preview.html?page=wintray&empty-programs&language=${language}&theme=${theme}`);
  await page.locator('.wintray-program-empty').waitFor();
  for(const height of [940,600]){
   await page.setViewportSize({width,height});await settle();
   const center=await page.locator('.wintray-program-empty').evaluate(node=>{
    const list=node.parentElement,heading=list.querySelector('.wintray-list-heading'),s=getComputedStyle(list),b=list.getBoundingClientRect(),h=heading.getBoundingClientRect();
    const text=document.createRange();text.selectNodeContents(node);const t=text.getBoundingClientRect();
    const left=b.left+parseFloat(s.paddingLeft),right=b.right-parseFloat(s.paddingRight);
    const top=h.bottom+parseFloat(s.rowGap),bottom=b.bottom-parseFloat(s.paddingBottom);
    return {dx:(t.left+t.right-left-right)/2,dy:(t.top+t.bottom-top-bottom)/2,scroll:list.scrollHeight-list.clientHeight};
   });
   assert(Math.abs(center.dx)<2&&Math.abs(center.dy)<2,JSON.stringify({width,height,language,theme,center}));
   assert(center.scroll<=1,'Empty list unexpectedly scrolls');
   const layout=await check(page.locator('[data-slot=page-content]'),'program list');
   results.push({page:'empty-programs',width,height,language,theme,center,...layout});
  }
  await page.screenshot({path:join(output,`empty-${width}-${language}-${theme}.png`)});
  assert.deepEqual(await page.evaluate(()=>window.layoutErrors),[]);await page.close();
  page=await browser.newPage({viewport:{width,height:800},reducedMotion:'reduce'});
  await page.goto(base+`tests/chat-experience-preview.html?welcome&language=${language}&theme=${theme}`);
  const labels=await page.evaluate(async language=>(await import('/src/locales/chat.ts')).chatMessages[language==='zh-CN'?'zh':'en'],language);
  await page.getByRole('button',{name:labels.openOrCreateProject,exact:true}).click();
  const dialog=page.getByRole('dialog');await dialog.waitFor();
  await page.evaluate(()=>{window.go.main.App.PickDirectory=async()=> 'C:/fixture/'+ 'LongFolderWithChinese中文/'.repeat(12);});
  await dialog.getByRole('button',{name:labels.chooseLocation,exact:true}).click();await settle();
  for(const create of [false,true]){
   await dialog.getByRole('checkbox').setChecked(create);
   await dialog.getByRole('textbox').first().fill('Project 项目 '.repeat(15));
   const layout=await check(dialog,'long project dialog');
   results.push({page:'project-dialog',width,language,theme,create,...layout});
  }
  await page.screenshot({path:join(output,`project-${width}-${language}-${theme}.png`)});
  await page.close();
 }
 await writeFile(join(output,'report.json'),JSON.stringify({results},null,2),'utf8');
 console.log(`Global scroll: ${results.length} cases passed.`);
}catch(error){if(page&&!page.isClosed())await page.screenshot({path:join(output,'failure.png')});await writeFile(join(output,'failure.json'),JSON.stringify({error:String(error),results},null,2),'utf8');throw error;}
finally{await browser.close();await server.close();}
