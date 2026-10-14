// Browser regression for the real shell. Fixtures never call the native Host.
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE||'playwright');
const base=process.env.WINONE_LAYOUT_BASE||'http://127.0.0.1:5178';
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||'_temp/shell-settings');
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,...(process.env.WINONE_CHROMIUM?{executablePath:process.env.WINONE_CHROMIUM}:{channel:'msedge'})});
const results=[],errors=[];
try{
 const page=await browser.newPage({viewport:{width:1100,height:800},reducedMotion:'reduce'});
 page.on('pageerror',error=>errors.push(String(error)));
 await page.goto(`${base}/tests/shell-preview.html`);
 const sidebar=page.locator('.app-navigation');await sidebar.waitFor();
 const expectWidth=async expected=>{await page.waitForFunction(expected=>Math.abs(document.querySelector('.app-navigation').getBoundingClientRect().width-expected)<=1,expected,{timeout:2000});await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));if(Math.abs((await sidebar.boundingBox()).width-expected)>1)throw Error(`Sidebar drifted from ${expected}px after layout settled`);};
 for(const width of [1100,520,1800,760,800,1100]){
  await page.setViewportSize({width,height:800});await expectWidth(64);results.push({case:'startup collapsed',viewport:width,width:(await sidebar.boundingBox()).width});
 }
 await page.getByRole('button',{name:'展开侧栏',exact:true}).click();await expectWidth(256);
 for(const width of [1600,520,1800]){await page.setViewportSize({width,height:800});await expectWidth(width<768?64:256);}
 await page.getByRole('button',{name:'收起侧栏',exact:true}).click();await expectWidth(64);
 for(const width of [520,1800,800]){await page.setViewportSize({width,height:800});await expectWidth(64);}
 const divider=page.getByRole('separator',{name:'调整侧栏宽度',exact:true});await divider.focus();await page.keyboard.press('ArrowRight');
 const resized=(await sidebar.boundingBox()).width;if(resized<=64)throw Error('Collapsed rail cannot be expanded with its handle');
 await sidebar.locator('nav button').nth(1).click();
 if(Math.abs((await sidebar.boundingBox()).width-resized)>1)throw Error('Navigation changed sidebar width');
 const top=page.getByRole('button',{name:'返回顶部',exact:true}),bottom=page.getByRole('button',{name:'返回底部',exact:true});
 await bottom.waitFor();
 const topBounds=await top.boundingBox(),bottomBounds=await bottom.boundingBox();
 if(topBounds.y+topBounds.height>bottomBounds.y||Math.abs(topBounds.x-bottomBounds.x)>1)throw Error('Scroll buttons are not stacked top then bottom');
 await bottom.click();await page.waitForFunction(()=>{const node=document.querySelector('[data-slot="page-scroll"]');return node.scrollHeight-node.clientHeight-node.scrollTop<=1;});
 await top.click();await page.waitForFunction(()=>document.querySelector('[data-slot="page-scroll"]').scrollTop===0);
 await page.setViewportSize({width:800,height:5000});await bottom.waitFor({state:'hidden'});
 await page.setViewportSize({width:800,height:800});await bottom.waitFor();
 results.push({case:'page scroll controls',verticalOrder:true,topAndBottom:true,hideWithoutOverflow:true});
 await page.getByRole('button',{name:'Chat',exact:true}).click();await bottom.waitFor({state:'hidden'});
 for(const name of ['切换 Chat 右栏','切换 Chat 左栏','切换 Chat 右栏','切换 Chat 左栏']){
  await page.getByRole('button',{name,exact:true}).click();await expectWidth(resized);
 }
 await page.getByRole('button',{name:'首页',exact:true}).click();await expectWidth(resized);
 results.push({case:'nested Chat-style panels',retainedWidth:resized,noPageScrollControls:true});
 await page.goto(`${base}/tests/wintray-preview.html`);await page.setViewportSize({width:1100,height:620});
 const detail=page.locator('.wintray-program-detail');await detail.waitFor();
 await detail.locator('button').first().focus();await bottom.waitFor();await bottom.click();
 await page.waitForFunction(()=>{const node=document.querySelector('.wintray-program-detail');return node.scrollTop>0&&node.scrollHeight-node.clientHeight-node.scrollTop<=1;});
 if(await page.locator('[data-slot="page-scroll"]').evaluate(node=>node.scrollTop)!==0)throw Error('Nested pane navigation scrolled the outer page');
 await top.click();await page.waitForFunction(()=>document.querySelector('.wintray-program-detail').scrollTop===0);
 results.push({case:'program detail scroll',independent:true});
 await page.goto(`${base}/tests/directory-links-preview.html`);
 await page.getByRole('tab',{name:'新建链接',exact:true}).waitFor();
 if(await page.getByRole('region',{name:'链接历史',exact:true}).count())throw Error('Create view exposes history');
 await page.getByRole('tab',{name:'查找已有链接',exact:true}).click();
 await page.getByRole('tabpanel',{name:'查找已有链接',exact:true}).getByRole('region',{name:'链接历史',exact:true}).waitFor();
 await page.getByRole('tab',{name:'新建链接',exact:true}).click();
 if(await page.getByRole('region',{name:'链接历史',exact:true}).count())throw Error('History remains after leaving find-existing');
 results.push({case:'directory link history',onlyFindExisting:true});
 await writeFile(resolve(output,'report.json'),JSON.stringify({results,errors},null,2));
 if(errors.length)throw Error(errors.join('\n'));
 console.log('PASS: sidebar breakpoints, remembered width, keyboard resize, nested panels, page/pane scroll controls and directory-history placement');
}finally{await browser.close();}
