// Real browser verification of the production Cindy workbench and document
// editor. The Host and all file content are isolated in-memory fixtures.
import {createRequire} from 'node:module';
import {mkdir,readFile,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {execFileSync} from 'node:child_process';
import {createServer} from 'node:http';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [base,folder]=process.argv.slice(2),output=resolve(folder);
assert.equal(new URL(base).hostname,'127.0.0.1');
execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);await mkdir(output,{recursive:true});
const labels=JSON.parse(await readFile('apps/desktop/frontend/src/features/cindy/panel-ui/en.json','utf8'));
const policy=(await readFile('apps/desktop/frontend/index.html','utf8')).match(/content="(default-src[^"]+)"/)[1];
const source='# Project notes\n\nRead this **important** document.\n\n| Feature | Status |\n| --- | --- |\n| Preview | Ready |\n| Search | Ready |\n\n![Local picture](picture.png)\n\n```mermaid\nflowchart LR\n  A[Read] --> B[Edit]\n```\n\nFinal paragraph.\n';
let externalSource=source;
const png=await readFile('internal/branding/logo.png');
const server=createServer((req,res)=>{res.setHeader('Access-Control-Allow-Origin','*');res.setHeader('Cache-Control','no-store');if(req.url?.endsWith('.png')){res.setHeader('Content-Type','image/png');res.end(png);}else{res.setHeader('Content-Type','text/plain; charset=utf-8');res.end(externalSource);}});
await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
const origin=`http://127.0.0.1:${server.address().port}`;
const browser=await chromium.launch({headless:true,channel:'msedge'}),results=[];let lastPage;
try{
 for(const width of [1440,900])for(const theme of ['light','dark']){
  externalSource=source;
  const page=await browser.newPage({viewport:{width,height:1000},reducedMotion:'reduce'}),errors=[];
  lastPage=page;
  page.on('pageerror',error=>errors.push(error.message));
  await page.goto(`${base}/tests/chat-experience-preview.html?alignment&shell&language=en-US&theme=${theme}&previewOrigin=${encodeURIComponent(origin)}`);
  const footer=page.getByRole('region',{name:'Files and changes'});await footer.waitFor();
  // Load Vite's development styles before enforcing the real desktop policy.
  // The editor and its widgets then execute under the unchanged production CSP.
  await page.evaluate(async()=>{await import('/src/features/chat-file-preview.tsx');await document.fonts.ready;});
  await page.evaluate(policy=>{window.cindyPolicyViolations=[];document.addEventListener('securitypolicyviolation',event=>window.cindyPolicyViolations.push({directive:event.effectiveDirective,blocked:event.blockedURI,source:event.sourceFile,line:event.lineNumber}));const meta=document.createElement('meta');meta.httpEquiv='Content-Security-Policy';meta.content=policy;document.head.append(meta);},policy);
  const chip=footer.locator('.chat-generated-files').getByRole('button',{name:'README.md',exact:true});await chip.scrollIntoViewIfNeeded();
  const chipBox=await chip.boundingBox(),changesBox=await footer.locator('.chat-transcript-changes').boundingBox();
  assert(changesBox.y-chipBox.y-chipBox.height>=12,'File chip touches the change-card border');
  await chip.click();assert.equal(await page.getByRole('menu').count(),0,'Left click opened a context menu');
  const panel=page.locator('.chat-side-panel');await panel.locator('[data-testid=right-sidebar-tab-bar]').waitFor();
  const editor=panel.locator('.cindy-file-editor .cm-editor');await editor.waitFor({timeout:10000});
  await panel.locator('.cm-md-table-widget').waitFor();
  const localImage=panel.locator('.cm-md-image-widget img');await localImage.waitFor();
  await page.waitForFunction(()=>{const host=document.querySelector('.cindy-file-editor'),image=host?.shadowRoot?.querySelector('.cm-md-image-widget img');return image?.naturalWidth>0;});
  const metrics=await editor.evaluate(el=>({font:getComputedStyle(el).fontFamily,background:getComputedStyle(el).backgroundColor,width:el.getBoundingClientRect().width,root:el.getRootNode().constructor.name,table:el.querySelectorAll('.cm-md-table-widget').length}));
  assert.equal(metrics.root,'ShadowRoot');assert(metrics.width>100);assert(metrics.table>0);
  // Exercise pointer selection, not only keyboard typing; a rendered Markdown
  // editor must remain selectable across its ShadowRoot boundary.
  await page.evaluate(()=>{document.body.style.userSelect='none';});
  const prose=editor.locator('.cm-line').filter({hasText:'Read this'}).first();
  const proseBox=await prose.boundingBox();
  await page.mouse.move(proseBox.x+5,proseBox.y+proseBox.height/2);
  await page.mouse.down();await page.mouse.move(proseBox.x+Math.min(190,proseBox.width-5),proseBox.y+proseBox.height/2,{steps:12});await page.mouse.up();
  const selection=await editor.evaluate(el=>(el.getRootNode().getSelection?.()??document.getSelection())?.toString());
  assert.ok(selection&&selection.length>5,'Rendered Markdown cannot select text by dragging');
  const originalEditor=await editor.elementHandle();
  externalSource=source+'\nExternal file change.\n';
  await page.evaluate(()=>window.chatExperienceIO.fileChanged());
  await editor.getByText('External file change.',{exact:true}).waitFor({timeout:5000});
  assert.ok(await originalEditor.evaluate(node=>node.isConnected),'Refresh rebuilt the editor');
  assert.equal(await editor.evaluate(el=>(el.getRootNode().getSelection?.()??document.getSelection())?.toString()),selection,'Refresh lost the text selection');
  // The document contains clickable image widgets. Focus the text surface
  // explicitly instead of clicking its center, which may open an image.
  const content=editor.locator('.cm-content');await content.focus();await content.press('Control+End');await page.keyboard.insertText('\nAdded during the browser check.');
  await page.waitForFunction(()=>window.chatExperienceCalls?.some(call=>call.method==='file.save'&&call.params.text.includes('Added during the browser check.')),{},{timeout:7000});
  await content.press('Control+f');const search=panel.getByRole('search');await search.waitFor();
  await search.locator('input').fill('Ready');await page.waitForFunction(()=>document.querySelector('.chat-side-panel [role=search]')?.textContent?.includes('1/2'));
  await search.locator('input').press('Escape');await search.waitFor({state:'hidden'});
  await panel.getByRole('button',{name:'View source',exact:true}).click();await panel.locator('.cindy-file-editor .cm-editor').waitFor();
  assert.equal(await panel.locator('.cm-md-table-widget').count(),0,'Source view still hides the Markdown table');
  await panel.getByRole('button',{name:'View preview',exact:true}).click();await panel.locator('.cindy-file-editor .cm-scroller').evaluate(el=>{el.scrollTop=0;});await panel.locator('.cm-md-table-widget').waitFor();
  await panel.getByRole('button',{name:labels.rightSidebar.tabs.addAria,exact:true}).click();
  for(const name of ['Review','Browser','Terminal','New page'])assert(await page.getByRole('menuitem',{name:new RegExp('^'+name+'(?:\\s|$)')}).isVisible(),`Missing ${name} entry`);
  await page.keyboard.press('Escape');
  await page.screenshot({path:join(output,`workbench-${width}-${theme}.png`)});
  const policyViolations=await page.evaluate(()=>window.cindyPolicyViolations);
  await writeFile(join(output,`policy-${width}-${theme}.json`),JSON.stringify(policyViolations,null,2));
  assert.deepEqual(policyViolations,[],'A widget violates the desktop content policy');
  assert.deepEqual(errors,[]);results.push({width,theme,...metrics,policyViolations});await page.close();
 }
 await writeFile(join(output,'report.json'),JSON.stringify(results,null,2));
 console.log(`Cindy workbench: ${results.length} document, image, save, search, source-mode and menu cases passed.`);
}catch(error){
 if(lastPage&&!lastPage.isClosed()){
  await lastPage.screenshot({path:join(output,'failure.png')});
  const layout=await lastPage.evaluate(()=>{const rows=[];for(const selector of ['.chat-side-panel','.chat-panel-contents','.chat-panel-content','.chat-file-preview','.cindy-file-document','.cindy-file-editor'])for(const el of document.querySelectorAll(selector)){const style=getComputedStyle(el),box=el.getBoundingClientRect();rows.push({selector,hidden:el.hidden,width:box.width,height:box.height,display:style.display,flex:style.flex});if(el.shadowRoot)for(const inner of el.shadowRoot.querySelectorAll('.select-text,.cm-editor,.cm-scroller')){const s=getComputedStyle(inner),r=inner.getBoundingClientRect();rows.push({selector:inner.className,width:r.width,height:r.height,display:s.display,flex:s.flex});}}return rows;});
  await writeFile(join(output,'failure.json'),JSON.stringify(layout,null,2));
 }
 throw error;
}finally{await browser.close();await new Promise(resolve=>server.close(resolve));}
