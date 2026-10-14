// Production components with in-memory data; no system changes.
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE||'playwright');
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||'_temp/settings-dialog-check');
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});
const report=[],errors=[];
try {
 for(const [language,theme] of [['zh-CN','light'],['en-US','dark'],['zh-CN','dark'],['en-US','light']])for(const width of [520,1100]) {
  const en=language==='en-US';
  const page=await browser.newPage({viewport:{width,height:900},reducedMotion:'reduce',colorScheme:theme});
  page.on('pageerror',e=>errors.push(String(e)));
  const go=async(fixture,query='')=>{await page.goto(`${process.env.WINONE_LAYOUT_BASE||'http://127.0.0.1:5198'}/tests/${fixture}.html?language=${language}&theme=${theme}&${query}`);await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);};
  const snap=async(name)=>{await page.evaluate(()=>document.fonts.ready);await page.mouse.move(0,0);const file=`${width}-${language}-${theme}-${name}.png`;await page.screenshot({path:resolve(output,file),animations:'disabled'});report.push({file,name,width,language,theme});};
  await go('ai-settings-preview','multipleKeys');
  await page.getByRole('button',{name:en?'Add from service presets':'从服务预设添加',exact:true}).click();
  await page.getByRole('button',{name:'OpenAI',exact:true}).waitFor();
  await snap('preset-list');
  await page.getByLabel(en?'Search providers':'搜索供应商',{exact:true}).fill('unmatched-provider');await snap('preset-empty');
  await page.getByLabel(en?'Search providers':'搜索供应商',{exact:true}).fill('');
  await page.setViewportSize({width,height:380});await snap('preset-short');
  report[report.length-1].dialog=await page.getByRole('dialog').evaluate(e=>{const r=e.getBoundingClientRect();return {top:r.top,bottom:r.bottom,viewport:innerHeight,overflow:e.scrollWidth>e.clientWidth+1};});
  await page.setViewportSize({width,height:900});await page.keyboard.press('Escape');
 await page.getByRole('button',{name:en?'Rename service':'重命名服务',exact:true}).click();
  await page.getByLabel(en?'Service name':'服务名称',{exact:true}).fill('A provider name with a long but meaningful description for translation');
 await page.keyboard.press('Enter');
  await page.getByRole('combobox',{name:en?'Service':'服务',exact:true}).click();await snap('long-provider-option');await page.keyboard.press('Escape');
  await page.getByLabel(en?'API key 1':'API 密钥 1',{exact:true}).scrollIntoViewIfNeeded();await snap('keys');
  await go('selection-preview');await page.getByRole('tab',{name:en?'Toolbar':'工具栏',exact:true}).click();await page.getByRole('button',{name:en?'Edit Explain':'编辑 解释',exact:true}).click();
  await page.getByLabel(en?'AI instruction':'AI 指令',{exact:true}).scrollIntoViewIfNeeded();await snap('action-editor');
  await go('wintray-preview');await page.locator('.wintray-program-detail').waitFor();
  for(let tab=0;tab<2;tab++){await page.locator('.wintray-program-detail [role=tab]').nth(tab).click();const pane=page.locator('.wintray-program-detail');const size=await pane.evaluate(e=>({height:e.clientHeight,total:e.scrollHeight}));for(let y=0;y<Math.ceil(size.total/(size.height-40));y++){await pane.evaluate((e,y)=>e.scrollTop=y*(e.clientHeight-40),y);await snap(`program-tab${tab}-${y}`);}}
  await page.getByRole('tab',{name:en?'Startup settings':'启动设置',exact:true}).click();await page.getByRole('button',{name:en?'Clear programs and restore':'清空托管程序并还原',exact:true}).click();await page.getByRole('dialog').waitFor();await snap('program-reset');await page.keyboard.press('Escape');
  await go('ui-preview');await page.getByRole('button',{name:'标记未保存'}).click();await page.getByRole('button',{name:'离开设置'}).click();await page.getByRole('dialog').waitFor();await snap('unsaved-dialog');
  await page.close();
 }
 await writeFile(resolve(output,'report.json'),JSON.stringify({report,errors},null,2));if(errors.length||report.some(r=>r.dialog&&(r.dialog.top<15||r.dialog.bottom>r.dialog.viewport-15||r.dialog.overflow)))throw Error('Dialog viewport check failed; see report.json');console.log('PASS: '+report.length+' dialog, menu, action editor and program detail views');
}finally{await browser.close();}
