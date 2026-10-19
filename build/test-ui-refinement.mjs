// Rendered regressions for intrinsic gesture rails and stationary tab lines.
// The production App uses an in-memory Host; native acceptance is separate.
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {pathToFileURL} from 'node:url';
import assert from 'node:assert/strict';
const root=resolve('.'),frontend=join(root,'apps/desktop/frontend');
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||'_temp/ui-refinement-check');
const {execFileSync}=await import('node:child_process');
execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);
await mkdir(output,{recursive:true});
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE||join(root,'dist/resources/tools/playwright-1.58.2/node_modules/playwright'));
const {createServer}=await import(pathToFileURL(join(frontend,'node_modules/vite/dist/node/index.js')));
const server=await createServer({root:frontend,configFile:join(frontend,'vite.config.ts'),server:{host:'127.0.0.1',port:0,hmr:false}});
await server.listen();
const base='http://127.0.0.1:'+server.httpServer.address().port;
const browser=await chromium.launch({headless:true,channel:'msedge'}),results=[],errors=[];
try{
 for(const language of ['zh-CN','en-US'])for(const width of [520,900,1280])for(const motion of ['no-preference','reduce']){
  const page=await browser.newPage({viewport:{width,height:940},reducedMotion:motion});
  page.on('pageerror',e=>errors.push(e.message));
  await page.goto(base+'/tests/settings-design-preview.html?page=convenience&language='+language);
  const en=language==='en-US';
  await page.getByRole('tab',{name:en?'Mouse gestures':'鼠标手势',exact:true}).click();
  await page.locator('.stroke-action-row').first().waitFor();
  await page.evaluate(async()=>{await document.fonts.ready;});
  const tabs=page.getByRole('tablist',{name:en?'Mouse gestures':'鼠标手势',exact:true}).getByRole('tab');
  // Sample the outgoing underline during its actual opacity transition. Its
  // geometry must remain the selected geometry, including on wrapped bands.
  const samples=await tabs.evaluateAll(async nodes=>{
   const line=n=>{const s=getComputedStyle(n,'::after');return {left:s.left,right:s.right,bottom:s.bottom,width:s.width,height:s.height};};
   const results=[];
   for(let i=0;i<nodes.length*2;i++){
    const previous=nodes.find(n=>n.getAttribute('aria-selected')==='true');
    const before=line(previous),next=nodes[(nodes.indexOf(previous)+1)%nodes.length];
    next.click();
    for(let frame=0;frame<4;frame++){
     await new Promise(requestAnimationFrame);
     results.push({before,after:line(previous),reduced:matchMedia('(prefers-reduced-motion: reduce)').matches,duration:getComputedStyle(previous,'::after').transitionDuration});
    }
   }
   return results;
  });
  for(const sample of samples){assert.deepEqual(sample.after,sample.before,'Outgoing tab underline moved');if(sample.reduced)assert.equal(sample.duration,'0s');}
  await page.getByRole('tab',{name:en?'Action packages':'动作包',exact:true}).click();
  await page.locator('.stroke-package-name').first().click();
  const name=en?'Explorer folder · Global':'资源管理器文件夹·全局';
  await page.locator('#stroke-package-name').fill(name);
  const rail=await page.locator('.stroke-package-layout').evaluate(async layout=>{
   await document.fonts.ready;await new Promise(requestAnimationFrame);
   const nav=layout.querySelector('nav'),button=nav.querySelector('.stroke-package-name'),editor=nav.nextElementSibling;
   const n=nav.getBoundingClientRect(),e=editor.getBoundingClientRect(),s=getComputedStyle(button),range=document.createRange();range.selectNodeContents(button);
   return {title:button.textContent,titleWidth:button.clientWidth,textWidth:button.scrollWidth,height:range.getBoundingClientRect().height,lineHeight:parseFloat(s.lineHeight),railWidth:n.width,editorWidth:e.width,stacked:e.top>=n.bottom-1,overflow:document.documentElement.scrollWidth-innerWidth};
  });
  assert.ok(rail.textWidth<=rail.titleWidth+1,'A normal package title was truncated despite available space: '+JSON.stringify({width,language,...rail}));
  assert.ok(rail.height<=rail.lineHeight+1,'Package title wrapped: '+JSON.stringify(rail));
  assert.ok(rail.overflow<=1,'Page overflowed');
  await page.screenshot({path:join(output,'packages-'+language+'-'+width+'-'+motion+'.png'),fullPage:true});
  // A genuinely long custom name stays single-line and keeps its full tooltip.
  await page.locator('#stroke-package-name').fill(name.repeat(15));
  const long=await page.locator('.stroke-package-name').first().evaluate(n=>{const r=document.createRange();r.selectNodeContents(n);return {height:r.getBoundingClientRect().height,line:parseFloat(getComputedStyle(n).lineHeight),title:n.title,text:n.textContent,overflow:document.documentElement.scrollWidth-innerWidth};});
  assert.equal(long.title,long.text);assert.ok(long.height<=long.line+1);assert.ok(long.overflow<=1);
  results.push({width,language,motion,rail,samples:samples.length});await page.close();
 }
 assert.deepEqual(errors,[]);
 await writeFile(join(output,'report.json'),JSON.stringify({results,errors},null,2));
 console.log(JSON.stringify({scenarios:results.length,animationSamples:results.reduce((n,r)=>n+r.samples,0),errors}));
}finally{await browser.close();await server.close();}
