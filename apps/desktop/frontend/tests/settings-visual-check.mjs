// Production components with in-memory data; no system changes.
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE||'playwright');
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||'_temp/settings-visual-check');
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});
const report=[],errors=[];
try {
 for(const [width,language,theme] of [[1100,'zh-CN','light'],[1100,'en-US','dark'],[520,'zh-CN','dark'],[520,'en-US','light'],[390,'zh-CN','light'],[390,'en-US','dark']]) {
  const page=await browser.newPage({viewport:{width,height:900},reducedMotion:'reduce',colorScheme:theme});
  page.on('pageerror',e=>errors.push(String(e)));
  for(const fixture of ['shell-preview','capture-settings-preview','module-preview','selection-preview','wintray-preview','ai-settings-preview','permission-settings-preview']) {
   await page.goto(`${process.env.WINONE_LAYOUT_BASE||'http://127.0.0.1:5198'}/tests/${fixture}.html?language=${language}&theme=${theme}`);
   await page.locator('h1').waitFor();
   await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);
   const tabs=await page.getByRole('tablist').count()?await page.getByRole('tablist').first().getByRole('tab').allTextContents():[];
   for(let index=0;index<Math.max(1,tabs.length);index++) {
    if(tabs.length)await page.getByRole('tab',{name:tabs[index],exact:true}).click();
    await page.evaluate(()=>Promise.all([document.fonts.ready,...document.getAnimations().map(a=>a.finished)]));
    const scroll=await page.evaluate(()=>{const e=document.querySelector('[data-slot=page-scroll]')||document.scrollingElement;return {height:e.clientHeight,total:e.scrollHeight};});
    const stops=Math.max(1,Math.ceil(scroll.total/scroll.height));
    for(let position=0;position<stops;position++) {
     await page.evaluate(pos=>{const e=document.querySelector('[data-slot=page-scroll]')||document.scrollingElement;e.scrollTop=pos*(e.clientHeight-60);},position);
     await page.evaluate(()=>new Promise(requestAnimationFrame));
     const file=`${fixture}-${width}-${language}-${theme}-tab${index}-${position}.png`;
     await page.mouse.move(0,0);
     await page.screenshot({path:resolve(output,file)});
     const overflow=await page.evaluate(()=>[...document.querySelectorAll('html,body,main,[data-slot=page-scroll],.module-section')].filter(e=>e.clientWidth&&e.scrollWidth>e.clientWidth+1).map(e=>e.className||e.tagName));
     report.push({file,fixture,width,language,theme,tab:tabs[index]||'default',position,overflow});
    }
   }
  }
  await page.close();
 }
 await writeFile(resolve(output,'report.json'),JSON.stringify({report,errors},null,2));
 if(errors.length||report.some(r=>r.overflow.length))throw Error('Visual page check failed; see report.json');console.log('PASS: '+report.length+' page/tab/scroll views');
} finally {await browser.close();}
