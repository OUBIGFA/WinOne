// Production components, synthetic settings and an isolated browser. This
// Checks reading hierarchy as geometry, not just lack of scrollbars. Page and
// tab coverage is complemented by stateful provider/import/desktop drivers.
import {createRequire} from "node:module";
import {mkdir,writeFile} from "node:fs/promises";
import {resolve} from "node:path";
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||"_temp/settings-hierarchy");
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});
const base=process.env.WINONE_LAYOUT_URL||"http://127.0.0.1:5198";
const report=[],errors=[];
try {
 for(const language of ["zh-CN","en-US"])for(const theme of ["light","dark"])for(const width of [520,800,1280]) {
  const context=await browser.newContext({viewport:{width,height:900},colorScheme:theme,reducedMotion:"reduce"});
  const page=await context.newPage();page.on("pageerror",error=>errors.push(String(error)));
  for(const fixture of ["module-preview","capture-settings-preview","permission-settings-preview","shell-preview","selection-preview","ai-settings-preview","wintray-preview"]) {
   await page.goto(`${base}/tests/${fixture}.html?language=${language}&theme=${theme}`);
   await page.locator("h1").waitFor();
   await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);
   const tabs=await page.getByRole("tab").allTextContents();
   for(let index=-1;index<tabs.length;index++) {
    if(index>=0){const tab=page.getByRole("tab",{name:tabs[index],exact:true});if(!await tab.count())continue;await tab.click();}
    await page.evaluate(()=>Promise.all([document.fonts.ready,...document.getAnimations().map(animation=>animation.finished)]));
    const result=await page.evaluate(()=>{
     const roots=[...document.querySelectorAll('html,body,main,[data-slot="page-scroll"],.module-section')];
     const overflow=roots.filter(e=>e.clientWidth&&e.scrollWidth>e.clientWidth+1).map(e=>e.className||e.tagName);
     const clippedTabs=[...document.querySelectorAll('[role="tab"] span')].filter(e=>e.clientWidth&&e.scrollWidth>e.clientWidth+1).map(e=>e.textContent);
     const spacing=[];
     const visible=e=>e.getBoundingClientRect().height>0&&!e.closest('[hidden]');
     for(const e of document.querySelectorAll('.settings-group > .settings-group-title,.dashboard-heading-copy > h2')) {
      const next=e.nextElementSibling;if(!next||!visible(e)||!visible(next))continue;
      const gap=next.getBoundingClientRect().top-e.getBoundingClientRect().bottom;
      const min=e.matches('h2')?8:16;
      if(gap<min-1)spacing.push({text:e.textContent,gap,min});
     }
     for(const field of document.querySelectorAll('[data-slot="field"],.settings-stack-compact')){
      const label=field.querySelector(':scope > label'),control=label?.nextElementSibling;
      if(!label||!control||!visible(label)||!visible(control))continue;
      const a=label.getBoundingClientRect(),b=control.getBoundingClientRect();
      if(b.top>=a.bottom-1&&b.top-a.bottom<5)spacing.push({text:label.textContent,gap:b.top-a.bottom,min:6});
     }
     return {overflow,clippedTabs,spacing};
    });
    report.push({fixture,language,theme,width,tab:index,...result});
   }
   if(width===1280) {
    if(tabs.length&&fixture!=="permission-settings-preview")await page.getByRole("tab",{name:tabs[0],exact:true}).click();
    await page.evaluate(()=>{const scroll=document.querySelector('[data-slot="page-scroll"]');if(scroll)scroll.scrollTop=0;else window.scrollTo(0,0);});
    await page.screenshot({path:resolve(output,`${fixture}-${language}-${theme}.png`),fullPage:true});
   }
  }
  await context.close();
 }
 // Scaling checks use independent contexts so device scale and focus state
 // cannot leak between cases. Text zoom exercises reflow, not just pixels.
 for(const scale of [1.25,1.5,2]) {
  const context=await browser.newContext({viewport:{width:1000,height:800},deviceScaleFactor:scale,reducedMotion:"reduce"});
  const page=await context.newPage();
  await page.goto(`${base}/tests/selection-preview.html`);
  await page.getByRole("tab",{name:"工具栏",exact:true}).click();
  await page.evaluate(scale=>document.documentElement.style.fontSize=`${16*scale}px`,scale);
  const edit=page.getByRole("button",{name:"编辑 解释",exact:true});
  await edit.focus();await page.keyboard.press("Enter");
  await page.getByLabel("AI 指令",{exact:true}).waitFor();
  const overflow=await page.locator('[data-slot="page-scroll"]').evaluate(e=>e.scrollWidth>e.clientWidth+1);
  report.push({fixture:"selection-zoom",scale,overflow:overflow?["page-scroll"]:[],clippedTabs:[]});
  await context.close();
 }
 await writeFile(resolve(output,"report.json"),JSON.stringify({report,errors},null,2));
 if(errors.length||report.some(r=>r.overflow.length||r.clippedTabs.length||r.spacing?.length))throw Error("Settings hierarchy failed; inspect report.json");
 console.log(`PASS: ${report.length} settings views; zh/en, light/dark, 520/800/1280 px; 125/150/200% text zoom and keyboard editing`);
} finally {await browser.close();}
