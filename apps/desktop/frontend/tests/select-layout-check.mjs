// Real shared selects, isolated fixture settings and browser. No host calls.
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE||'playwright');
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||'_temp/select-layout');await mkdir(output,{recursive:true});
const base=process.env.WINONE_LAYOUT_BASE||'http://127.0.0.1:5198';
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});
const report=[],errors=[];
try{for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark'])for(const width of [520,1100]){
 const en=language==='en-US';
 const page=await browser.newPage({viewport:{width,height:850},reducedMotion:'reduce',colorScheme:theme});page.on('pageerror',e=>errors.push(String(e)));
 const go=async(name)=>{await page.goto(base+'/tests/'+name+'.html?theme='+theme+'&language='+language);await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);};
 const inspect=async(name,trigger,keyboard=false)=>{
  await trigger.click();const popup=page.locator('[data-slot=select-content][data-open]');await popup.waitFor();await page.mouse.move(0,0);
  const file=[name,language,theme,width].join('-')+'.png';await page.screenshot({path:resolve(output,file),animations:'disabled'});
  const box=await trigger.boundingBox();
  const result=await popup.evaluate((e,anchor)=>{const p=e.getBoundingClientRect(),style=getComputedStyle(e);const rows=[...e.querySelectorAll('[role=option]')];return {padding:parseFloat(style.paddingLeft),contained:p.left>=7&&p.right<=innerWidth-7&&p.top>=7&&p.bottom<=innerHeight-7,separated:p.top>=anchor.y+anchor.height+5||p.bottom<=anchor.y-5,overflow:e.scrollWidth>e.clientWidth+1,rows:rows.map(row=>({height:row.getBoundingClientRect().height,clipped:row.scrollWidth>row.clientWidth+1})),background:style.backgroundColor};},box);
  if(!result.contained||!result.separated||result.padding<4||result.overflow||result.rows.some(r=>r.height<35||r.clipped))throw Error('Menu geometry: '+JSON.stringify({name,language,theme,width,...result}));
  report.push({file,name,language,theme,width,...result});
  if(keyboard){const options=await page.getByRole('option').allTextContents();await page.keyboard.press('Home');await page.keyboard.press('ArrowDown');await page.keyboard.press('Enter');await popup.waitFor({state:'hidden'});if(!(await trigger.innerText()).includes(options[1]))throw Error('Keyboard selection did not update the field');}
  else{await page.keyboard.press('Escape');await popup.waitFor({state:'hidden'});}
 };
 await go('ai-settings-preview');await inspect('protocol',page.getByRole('combobox',{name:en?'API protocol':'API 协议',exact:true}),true);
 await page.getByRole('button',{name:en?'Rename service':'重命名服务',exact:true}).click();
 await page.getByLabel(en?'Service name':'服务名称',{exact:true}).fill('A provider with a long descriptive name for shared translation and conversation');
 await page.keyboard.press('Enter');
 await inspect('long-provider',page.getByRole('combobox',{name:en?'Service':'服务',exact:true}));
 await page.getByRole('tab',{name:en?'Account providers':'账号供应商',exact:true}).click();await inspect('accounts',page.getByRole('combobox',{name:en?'Account':'账号',exact:true}));
 await go('capture-settings-preview');await page.getByRole('tab',{name:/OCR/}).click();await inspect('languages',page.getByRole('combobox',{name:en?'Target language':'目标语言',exact:true}),true);
 await go('selection-preview');await page.getByRole('tab',{name:en?'Toolbar':'工具栏',exact:true}).click();await page.getByRole('button',{name:en?'Edit Explain':'编辑 解释',exact:true}).click();await inspect('icons',page.getByRole('combobox',{name:en?'Icon':'图标',exact:true}));
 await go('wintray-preview');await inspect('order',page.locator('.wintray-program-row [role=combobox]').first());await inspect('mode',page.locator('#program-mode'),true);
 await page.close();
}await writeFile(resolve(output,'report.json'),JSON.stringify({report,errors},null,2));if(errors.length)throw Error(errors.join('; '));console.log('PASS: '+report.length+' menus; spacing, bounds, long names, themes, languages and keyboard selection');}finally{await browser.close();}
