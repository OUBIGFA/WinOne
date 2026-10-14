// Exercise the production shell with an unsaved form, without host operations.
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE||'playwright');
const base=process.env.WINONE_LAYOUT_BASE||'http://127.0.0.1:5198';
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||'_temp/navigation-layout');
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,executablePath:process.env.WINONE_CHROMIUM});
const results=[],errors=[];
try{
 for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark']){
  const page=await browser.newPage({viewport:{width:1100,height:900},reducedMotion:'reduce',colorScheme:theme});
  page.on('pageerror',e=>errors.push(String(e)));
  const en=language==='en-US',expand=en?'Expand sidebar':'展开侧栏',collapse=en?'Collapse sidebar':'收起侧栏';
  const url=`${base}/tests/ai-settings-preview.html?language=${language}&theme=${theme}`;
  await page.goto(url);
  await page.getByRole('button',{name:en?'Rename service':'重命名服务',exact:true}).click();
  await page.getByLabel(en?'Service name':'服务名称',{exact:true}).fill('Retain this unsaved draft');
  await page.keyboard.press('Enter');
  // The drawer hides background controls from assistive technology, but their
  // geometry still proves whether opening it changes the page width.
  const name=page.getByRole('combobox',{name:en?'Service':'服务',exact:true,includeHidden:true});
  const sidebar=page.locator('.app-navigation');
  const width=async()=>Math.round((await sidebar.boundingBox()).width);
  const inspectRail=async()=>{
   const offsets=await sidebar.evaluate(root=>{const r=root.getBoundingClientRect(),center=r.x+r.width/2;return [...root.querySelectorAll('button')].map(button=>{const b=button.getBoundingClientRect(),icon=button.querySelector('svg')?.getBoundingClientRect();return {label:button.getAttribute('aria-label'),buttonOffset:Math.abs(b.x+b.width/2-center),iconOffset:icon?Math.abs(icon.x+icon.width/2-center):0,width:b.width,height:b.height};});});
   if(offsets.some(item=>item.buttonOffset>1||item.iconOffset>1||item.width<43||item.height<43))throw Error('Icon rail alignment or hit area: '+JSON.stringify(offsets));
  };
  const divider=page.getByRole('separator',{name:en?'Resize sidebar':'调整侧栏宽度',exact:true});
  const box=await divider.boundingBox();
  await page.mouse.move(box.x,box.y+200);await page.mouse.down();await page.mouse.move(box.x+65,box.y+200,{steps:8});await page.mouse.up();
  const dragged=await width();if(dragged<300)throw Error('Sidebar did not follow pointer');
  await divider.focus();await page.keyboard.press('ArrowLeft');
  const savedWidth=await width();if(savedWidth>=dragged)throw Error('Sidebar did not respond to keyboard');
  const iconPositions=()=>sidebar.locator('button > svg').evaluateAll(icons=>icons.map(e=>{const r=e.getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2,width:r.width,height:r.height};}));
  const expandedIcons=await iconPositions();
  await page.getByRole('button',{name:collapse,exact:true}).click();
  if(await width()!==64)throw Error('Manual collapse did not leave an icon rail');
  const compactIcons=await iconPositions();
  if(compactIcons.length!==expandedIcons.length||compactIcons.some((icon,i)=>Object.keys(icon).some(key=>Math.abs(icon[key]-expandedIcons[i][key])>1)))throw Error('Collapse moved or resized navigation icons: '+JSON.stringify({expandedIcons,compactIcons}));
  await page.screenshot({path:resolve(output,language+'-'+theme+'-collapsed.png'),animations:'disabled'});
  await inspectRail();
  if(!(await name.textContent()).includes('Retain this unsaved draft'))throw Error('Collapse lost the draft');
  await page.getByRole('button',{name:expand,exact:true}).click();
  if(Math.abs(await width()-savedWidth)>1){console.log({savedWidth,actual:await width(),storage:await page.evaluate(()=>localStorage.getItem('winone.sidebar.v1'))});await page.screenshot({path:resolve(output,'restore-failure.png')});throw Error('Expand did not restore the chosen width');}
  await page.screenshot({path:resolve(output,`${language}-${theme}-expanded.png`)});
  await page.setViewportSize({width:520,height:900});
  await page.waitForFunction(()=>document.querySelector('.shell').hasAttribute('data-navigation-compact'));
  if(await width()!==64)throw Error('Narrow window did not use an icon rail');
  await inspectRail();
  for(const scale of [1.25,1.5,2]){await page.evaluate(scale=>document.documentElement.style.fontSize=`${16*scale}px`,scale);await inspectRail();}
  await page.evaluate(()=>document.documentElement.style.fontSize='');
  const contentBefore=await name.boundingBox();
  await page.getByRole('button',{name:expand,exact:true}).click();
  await page.getByRole('dialog').waitFor();
  const contentAfter=await name.boundingBox();
  if(contentBefore.x!==contentAfter.x||contentBefore.width!==contentAfter.width)throw Error('Narrow navigation squeezes the form');
  await page.screenshot({path:resolve(output,`${language}-${theme}-drawer.png`)});
  await page.keyboard.press('Escape');await page.getByRole('dialog').waitFor({state:'hidden'});
  if(!(await name.textContent()).includes('Retain this unsaved draft'))throw Error('Responsive navigation lost the draft');
  await page.screenshot({path:resolve(output,`${language}-${theme}-compact.png`)});
  await page.setViewportSize({width:1100,height:900});
  await page.waitForFunction(()=>!document.querySelector('.shell').hasAttribute('data-navigation-compact'));
  await page.waitForFunction(expected=>Math.abs(document.querySelector('.app-navigation').getBoundingClientRect().width-expected)<=1,savedWidth,{timeout:3000});
  await page.reload();await name.waitFor();if(Math.abs(await width()-savedWidth)>1)throw Error('Reload lost sidebar width');
  await page.getByRole('button',{name:collapse,exact:true}).click();await page.reload();await name.waitFor();
  if(await width()!==64)throw Error('Reload lost manual collapse');
  await page.goto(`${base}/tests/wintray-preview.html?language=${language}&theme=${theme}`);await page.setViewportSize({width:520,height:900});
  await page.locator('.wintray-program-row').first().waitFor();
  const clipped=await page.locator('.wintray-program-list').evaluate(list=>{const bounds=list.getBoundingClientRect();return [...list.querySelectorAll('button,[role=switch],[role=combobox]')].filter(e=>{const r=e.getBoundingClientRect();return r.width>1&&(r.left<bounds.left||r.right>bounds.right+1);}).map(e=>e.getAttribute('aria-label')||e.textContent);});
  results.push({language,theme,dragged,savedWidth,expandedIcons,compactIcons,clipped});
  await page.screenshot({path:resolve(output,`${language}-${theme}-programs.png`)});
  await page.close();
 }
 await writeFile(resolve(output,'report.json'),JSON.stringify({results,errors},null,2));
 if(errors.length||results.some(r=>r.clipped.length))throw Error('Navigation check failed; inspect report.json');
 console.log('PASS: pointer/keyboard resize, saved width, manual collapse, narrow navigation, draft retention and visible program controls in both languages/themes');
}finally{await browser.close();}
