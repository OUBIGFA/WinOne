import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||'_temp/settings-experience');await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,channel:'msedge'}),report=[],errors=[];
const base=process.env.WINONE_LAYOUT_BASE||'http://127.0.0.1:5198';
const check=(ok,message)=>{if(!ok)throw Error(message);};
try{
 const page=await browser.newPage({reducedMotion:'reduce'});page.on('pageerror',error=>errors.push(String(error)));
 for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark'])for(const width of [520,1280]){
  const zh=language==='zh-CN';await page.setViewportSize({width,height:900});
  await page.goto(`${base}/tests/ai-settings-preview.html?manyProviders=1&language=${language}&theme=${theme}`);
  const overview=page.getByRole('region',{name:zh?'已启用供应商':'Enabled providers'});await overview.locator('button').first().waitFor();await page.evaluate(()=>document.fonts.ready);
  const result=await overview.evaluate(root=>({buttons:root.querySelectorAll('button').length,width:root.clientWidth,overflow:root.scrollWidth-root.clientWidth,clipped:[...root.querySelectorAll('button')].filter(e=>{const r=e.getBoundingClientRect();return r.left<0||r.right>innerWidth;}).length}));
  const tabs=page.getByRole('tablist',{name:zh?'AI 服务':'AI services'});
  const sizes=await tabs.getByRole('tab').evaluateAll(nodes=>nodes.map(node=>node.getBoundingClientRect().height));
  check(result.buttons===11&&!result.overflow&&!result.clipped,'Provider overview lost or clipped providers: '+JSON.stringify(result));check(sizes.every(height=>height>=40),'Primary tabs are below the restored readable size');
  await page.locator('[data-slot=page-scroll]').evaluate(node=>node.scrollTop=0);const before=await tabs.boundingBox();await page.getByRole('tab',{name:zh?'默认 AI 服务':'Default AI service',exact:true}).click();
  await page.locator('[data-slot=page-scroll]').evaluate(node=>node.scrollTop=0);const after=await tabs.boundingBox();check(Math.abs(before.y-after.y)<1,'Overview moved when changing service tabs');
  await page.screenshot({path:resolve(output,`providers-${language}-${theme}-${width}.png`)});
  await page.getByRole('tab',{name:zh?'路由':'Routing',exact:true}).click();
  const label=page.locator('.ai-routing-toggle-label');const aligned=await label.evaluate(root=>{const icon=root.querySelector('svg').getBoundingClientRect(),text=root.querySelector('span').getBoundingClientRect();return {offset:Math.abs(icon.top+icon.height/2-text.top-text.height/2),sameLine:icon.right<=text.left};});
  check(aligned.offset<1&&aligned.sameLine,'Routing icon and label are not on one line');
  report.push({language,theme,width,overview:result,tabHeights:sizes,routingLabel:aligned});
 }
 await page.setViewportSize({width:1280,height:900});await page.goto(`${base}/tests/directory-links-preview.html`);
 const source=page.getByRole('region',{name:'源文件夹',exact:true}),target=page.getByRole('region',{name:'目标文件夹',exact:true});
 await source.getByRole('textbox').fill('C:\\source');await source.getByRole('textbox').press('Enter');
 await target.getByRole('textbox').fill('C:\\links');await target.getByRole('textbox').press('Enter');
 await source.getByRole('checkbox',{name:'选择来源：document-1.txt',exact:true}).check();
 await page.getByRole('combobox',{name:'链接类型',exact:true}).click();await page.getByRole('option',{name:'符号链接',exact:true}).click();
 const tint=async locator=>locator.evaluate(node=>getComputedStyle(node.closest('.directory-links-entry')).backgroundColor);
 const sourceTint=await tint(source.getByRole('checkbox',{name:'选择来源：document-1.txt',exact:true})),targetTint=await tint(target.getByText('document-1.txt',{exact:true}));
 check(sourceTint===targetTint,'Source and destination type backgrounds differ');
 const handle=page.getByRole('separator',{name:'调整左右视窗高度'}),panes=page.locator('.directory-links-panes');
 await handle.focus();await handle.press('End');check((await panes.boundingBox()).height===4000,'Chosen pane height is still clamped to the viewport');
 await page.reload();await handle.waitFor();check((await panes.boundingBox()).height===4000,'Large pane height was not remembered');
 await handle.press('Home');const integration=page.getByRole('region',{name:'资源管理器集成'});await integration.scrollIntoViewIfNeeded();
 check(await integration.getByRole('switch',{name:'资源管理器集成',exact:true}).isVisible(),'Explorer integration switch is unavailable');
 check(await integration.getByRole('switch').count()===1&&await integration.getByRole('checkbox').count()===0,'Explorer integration has extra controls');
 check(await integration.evaluate(node=>getComputedStyle(node).borderBottomWidth)==='0px','Explorer integration adds a duplicate separator');
 await page.screenshot({path:resolve(output,'links-history-and-integration.png')});
 check(!errors.length,errors.join('\n'));await writeFile(resolve(output,'report.json'),JSON.stringify({report,linkTypeTint:sourceTint,paneMaximum:4000,errors},null,2));console.log('PASS: 8 provider layouts, large primary tabs, inline routing label, paired link tints, 4000px remembered height, expanded integration');
}finally{await browser.close();}
