import {createRequire} from 'node:module';
import {resolve,join} from 'node:path';
import {pathToFileURL} from 'node:url';
import {mkdir,writeFile} from 'node:fs/promises';
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
// The real ChatPage fixture exercises the nested thinking expander and its
// reading pane. Output must stay outside tracked source.
const root=resolve('.'),frontend=join(root,'apps/desktop/frontend'),output=resolve(process.argv[2]||'_temp/chat-collapse');
execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);await mkdir(output,{recursive:true});
const {createServer}=await import(pathToFileURL(join(frontend,'node_modules/vite/dist/node/index.js')));
const server=await createServer({root:frontend,configFile:join(frontend,'vite.config.ts'),server:{host:'127.0.0.1',port:0,hmr:false}});await server.listen();
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE||join(root,'dist/resources/tools/playwright-1.58.2/node_modules/playwright'));
const browser=await chromium.launch({headless:true,channel:'msedge'}),results=[];
try{
 for(const width of [640,1120])for(const motion of ['reduce','no-preference']){
  const page=await browser.newPage({viewport:{width,height:900},reducedMotion:motion});
  await page.goto('http://127.0.0.1:'+server.httpServer.address().port+'/tests/chat-experience-preview.html');
  await page.locator('.chat-transcript-viewport').evaluate(node=>node.scrollTop=0);
  await page.locator('[data-slot=cindy-work]').first().click();await page.locator('[data-live-work-activity=thinking]').first().click();await page.locator('.process-body[data-process-long=true]').waitFor();
  const button=page.locator('.process-collapse-action'),viewport=page.locator('.chat-transcript-viewport');await button.waitFor();
  await page.waitForTimeout(100);const initial=await button.boundingBox();assert(initial);
  assert.equal(await button.evaluate(node=>{const r=node.getBoundingClientRect();return document.elementFromPoint(r.x+r.width/2,r.y+r.height/2)===node||node.contains(document.elementFromPoint(r.x+r.width/2,r.y+r.height/2));}),true,'collapse hidden behind input at first expansion');
  const observed=[];
  for(const scroll of [120,320,600]){await viewport.evaluate((node,value)=>node.scrollTop=value,scroll);await page.waitForTimeout(60);const box=await button.boundingBox();if(box){assert(Math.abs(box.y-initial.y)<1);observed.push(box.y);}}
  const transition=await button.evaluate(node=>getComputedStyle(node).transitionProperty);assert.equal(transition,'none');
  // Move the entire work above the viewport; the action disappears instead
  // of flying toward the work's old bottom. Extra space is test-only content.
  await viewport.evaluate(node=>{const spacer=document.createElement('div');spacer.style.height='1500px';node.firstElementChild.append(spacer);const body=node.querySelector('.process-body');node.scrollTop+=body.getBoundingClientRect().bottom-node.getBoundingClientRect().top+80;});
  await button.waitFor({state:'hidden'});
  await viewport.evaluate(node=>node.scrollTop=0);await button.waitFor();await page.waitForTimeout(50);await page.screenshot({path:join(output,width+'-'+motion+'.png')});await button.click();await button.waitFor({state:'hidden'});
  results.push({width,motion,initialY:initial.y,observed,transition,visibleImmediately:true,hiddenOutside:true});await page.close();
 }
 await writeFile(join(output,'result.json'),JSON.stringify(results,null,2));console.log('Collapse: 4 real-browser cases passed');
}finally{await browser.close();await server.close();}
