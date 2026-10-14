import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||'_temp/20261011-thread-takeover/tab-hierarchy');await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,channel:'msedge'}),results=[],errors=[];
try{
 for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark']){
  const page=await browser.newPage({viewport:{width:1050,height:900},reducedMotion:'reduce'});page.on('pageerror',error=>errors.push(error.message));
  const zh=language==='zh-CN';await page.goto('http://127.0.0.1:5198/tests/tab-hierarchy-preview.html?language='+language+'&theme='+theme);
  const primary=page.getByRole('tablist',{name:zh?'便捷功能':'Convenience'});
  await primary.waitFor();
  const metric=async list=>list.evaluate(node=>{const tab=node.querySelector('[aria-selected=true]');return {variant:node.dataset.variant,background:getComputedStyle(node).backgroundColor,underline:getComputedStyle(tab,'::after').opacity,tabBackground:getComputedStyle(tab).backgroundColor,height:node.getBoundingClientRect().height};});
  const first=await metric(primary);if(first.variant!=='default'||first.underline!=='0'||first.background==='rgba(0, 0, 0, 0)')throw Error('Primary tab was not restored: '+JSON.stringify(first));
  for(const width of [420,650,1050]){
   await page.setViewportSize({width,height:900});
   const clipped=await primary.evaluate(node=>[...node.querySelectorAll('[role=tab]')].filter(tab=>{const text=tab.querySelector('span>span');if(!text)return false;const rect=text.getBoundingClientRect(),parent=tab.getBoundingClientRect(),style=getComputedStyle(text);return text.scrollWidth>text.clientWidth+1||rect.right>parent.right+1||style.textOverflow==='ellipsis';}).map(tab=>tab.textContent));
   if(clipped.length||await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1))throw Error('Primary label is shortened at '+width+': '+clipped.join(','));
  }
  for(const label of [zh?'置顶功能':'Always on Top',zh?'鼠标手势':'Mouse gestures',zh?'文件与目录链接':'File and folder links']){
   await primary.getByRole('tab',{name:label,exact:true}).click();
   const secondary=page.locator('[role=tabpanel]:visible [role=tablist]').first();await secondary.waitFor();const value=await metric(secondary);
   if(value.variant!=='line'||value.underline!=='1')throw Error('Secondary tabs did not show an underline: '+JSON.stringify({label,value}));
   if(label===(zh?'鼠标手势':'Mouse gestures')){
    await secondary.getByRole('tab',{name:zh?'画笔':'Pen',exact:true}).click();const toggle=page.getByRole('switch',{name:zh?'显示手势画笔':'Show gesture pen'});if(await toggle.getAttribute('aria-checked')!=='true')throw Error('New pen default off');await toggle.click();await page.getByRole('button',{name:zh?'保存鼠标手势设置':'Save mouse gesture settings'}).click();if(await toggle.getAttribute('aria-checked')!=='false')throw Error('Pen setting did not persist');await toggle.click();await page.getByRole('button',{name:zh?'保存鼠标手势设置':'Save mouse gesture settings'}).click();
   }
   results.push({language,theme,label,primary:first,secondary:value});
  }
  await page.screenshot({path:join(output,language+'-'+theme+'.png')});await page.close();
 }
 await writeFile(join(output,'report.json'),JSON.stringify({results,errors},null,2));if(errors.length)throw Error(errors.join('\n'));console.log(JSON.stringify({cases:results.length,penSwitch:true,errors:0}));
}finally{await browser.close();}

