// Original Cindy review UI under the production CSP. Actual Git writes and
// revision protection are independently covered by internal/chat's tests.
import {createRequire} from 'node:module';
import {mkdir,readFile,writeFile} from 'node:fs/promises';
import {join,resolve} from 'node:path';
import {execFileSync} from 'node:child_process';
import {createServer} from 'node:http';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [base,folder]=process.argv.slice(2),output=resolve(folder);
assert.equal(new URL(base).hostname,'127.0.0.1');execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);await mkdir(output,{recursive:true});
const policy=(await readFile('apps/desktop/frontend/index.html','utf8')).match(/content="(default-src[^"]+)"/)[1];
const labels=JSON.parse(await readFile('apps/desktop/frontend/src/features/cindy/panel-ui/en.json','utf8'));
const png=await readFile('internal/branding/logo.png'),document='# Project notes\n\nA **rendered** document.\n\n![Diagram](picture.png)\n\n| Item | State |\n| --- | --- |\n| Review | Ready |\n';
const server=createServer((req,res)=>{const url=new URL(req.url,'http://localhost');res.setHeader('Access-Control-Allow-Origin','*');res.setHeader('Cache-Control','no-store');if(url.pathname.endsWith('.png')){res.setHeader('Content-Type','image/png');res.end(png);}else if(url.pathname==='/review.diff'){const path=url.searchParams.get('path');res.setHeader('Content-Type','text/plain');res.end(`diff --git a/${path} b/${path}\n--- a/${path}\n+++ b/${path}\n@@ -1 +1,2 @@\n-const before = 1;\n+const after = 2;\n+export { after };\n`);}else{res.setHeader('Content-Type','text/plain; charset=utf-8');res.end(document);}});
await new Promise(done=>server.listen(0,'127.0.0.1',done));const origin=`http://127.0.0.1:${server.address().port}`;
const browser=await chromium.launch({headless:true,channel:'msedge'}),results=[];let page;
try{
 for(const width of [960,1540])for(const theme of ['light','dark']){
  page=await browser.newPage({viewport:{width,height:960},reducedMotion:'reduce'});const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.goto(`${base}/tests/chat-experience-preview.html?alignment=1&review=1&language=en-US&theme=${theme}&previewOrigin=${encodeURIComponent(origin)}`);
  await page.getByRole('region',{name:'Files and changes'}).waitFor();
  // Vite may reload once when it first optimizes these lazy dependencies.
  // Settle that development-only navigation before enforcing the desktop CSP.
  for(let attempt=0;;attempt++){try{await page.evaluate(async()=>{await Promise.all([import('/src/features/chat-file-preview.tsx'),import('/src/features/chat-review.tsx')]);const highlighter=await import('/src/features/cindy/review/DiffViewer/highlight.ts');await highlighter.highlightLine('javascript','const ready = true;');await document.fonts.ready;});await page.waitForLoadState('networkidle');break;}catch(error){if(attempt>=2||!/Execution context was destroyed/.test(String(error)))throw error;await page.waitForLoadState('domcontentloaded');await page.getByRole('region',{name:'Files and changes'}).waitFor();}}
  await page.evaluate(policy=>{window.reviewPolicyViolations=[];document.addEventListener('securitypolicyviolation',event=>window.reviewPolicyViolations.push({directive:event.effectiveDirective,blocked:event.blockedURI,source:event.sourceFile,line:event.lineNumber}));const meta=document.createElement('meta');meta.httpEquiv='Content-Security-Policy';meta.content=policy;document.head.append(meta);},policy);
  await page.getByRole('region',{name:'Files and changes'}).locator('.chat-generated-files').getByRole('button',{name:'README.md',exact:true}).click();
  const panel=page.locator('.chat-side-panel');
  const resizeLabel=await page.evaluate(async()=>(await import('/src/i18n.ts')).i18n.t('chat.resizeDetails'));
  const divider=await page.getByRole('separator',{name:resizeLabel,exact:true}).boundingBox(),before=await panel.boundingBox(),targetWidth=width===960?280:680;
  assert(divider&&before);await page.mouse.move(divider.x+divider.width/2,divider.y+divider.height/2);await page.mouse.down();await page.mouse.move(divider.x+divider.width/2+before.width-targetWidth,divider.y+divider.height/2,{steps:8});await page.mouse.up();
  await page.waitForFunction(target=>Math.abs(document.querySelector('.chat-side-panel').getBoundingClientRect().width-target)<4,targetWidth);
  await panel.getByRole('button',{name:labels.rightSidebar.tabs.addAria,exact:true}).click();await page.getByRole('menuitem',{name:/^Review/}).click();
  const review=panel.locator('[data-cindy-review]');await review.locator('[data-review-file-id="unstaged:example.ts"]').waitFor({timeout:15000});
  await review.locator('[data-review-markdown-preview] h1').waitFor();
  await review.locator('[data-review-file-id="unstaged:picture.png"] img').first().waitFor();
  await page.waitForFunction(()=>[...document.querySelectorAll('[data-cindy-review] img')].every(img=>img.naturalWidth>0));
  const metrics=await review.evaluate(el=>({panelWidth:el.getBoundingClientRect().width,font:getComputedStyle(el).fontFamily,files:el.querySelectorAll('[data-review-file-id]').length,markdown:el.querySelectorAll('[data-review-markdown-preview]').length,images:el.querySelectorAll('img').length,horizontalOverflow:el.scrollWidth-el.clientWidth}));
  assert.equal(metrics.files,3);assert.equal(metrics.markdown,1);assert(metrics.images>=3);assert(metrics.horizontalOverflow<=1);
  const file=review.locator('[data-review-file-id="unstaged:example.ts"]');await file.hover();await file.getByRole('button',{name:'Stage',exact:true}).click();
  await file.waitFor({state:'hidden'});assert(await page.evaluate(()=>window.chatExperienceCalls.some(call=>call.method==='review.mutate'&&call.params.mutation.action==='stage'&&call.params.mutation.revision==='review-1')));
  await review.getByRole('button',{name:labels.rightSidebar.review.sourceAria||'Select review source',exact:true}).click();
  await page.getByRole('menuitem',{name:/^Staged/}).click();
  const staged=review.locator('[data-review-file-id="staged:example.ts"]');await staged.waitFor();await staged.hover();await staged.getByRole('button',{name:'Unstage',exact:true}).click();await staged.waitFor({state:'hidden'});
  await review.getByRole('button',{name:'Select review source',exact:true}).click();await page.getByRole('menuitem',{name:/^Unstaged/}).click();
  await file.waitFor();await file.hover();await file.getByRole('button',{name:'Revert hunk',exact:true}).click();const dialog=page.getByRole('alertdialog');await dialog.waitFor();
  const modal=await dialog.evaluate(el=>{const style=getComputedStyle(el),scrim=getComputedStyle(el.closest('.modal-scrim'));return {background:style.backgroundColor,radius:style.borderRadius,border:style.borderTopWidth,scrim:scrim.backgroundColor};});
  assert.notEqual(modal.background,'rgba(0, 0, 0, 0)','Cindy dialog lost its opaque panel');assert.notEqual(modal.scrim,'rgba(0, 0, 0, 0)','Cindy dialog lost its scrim');assert.equal(modal.border,'1px');assert(parseFloat(modal.radius)>=12);
  const segmented=review.locator('.cindy-segmented').first();assert.equal(await segmented.evaluate(el=>getComputedStyle(el).borderRadius),'9999px');
  await page.screenshot({path:join(output,`review-confirm-${width}-${theme}.png`)});await dialog.getByRole('button',{name:'Cancel',exact:true}).click();
  assert(!await page.evaluate(()=>window.chatExperienceCalls.some(call=>call.method==='review.mutate'&&call.params.mutation.action==='revert')));
  await page.screenshot({path:join(output,`review-${width}-${theme}.png`)});
  const violations=await page.evaluate(()=>window.reviewPolicyViolations);assert.deepEqual(violations,[]);assert.deepEqual(errors,[]);results.push({width,theme,...metrics,violations});await page.close();page=null;
 }
 await writeFile(join(output,'report.json'),JSON.stringify(results,null,2));console.log(`Cindy review: ${results.length} layout, text/image preview, stage/unstage, source and confirmation cases passed.`);
}catch(error){if(page&&!page.isClosed()){await page.screenshot({path:join(output,'failure.png')});await writeFile(join(output,'failure.txt'),await page.locator('body').innerText());await writeFile(join(output,'failure-dom.html'),await page.content());}throw error;}
finally{await browser.close();server.closeAllConnections();await new Promise(done=>server.close(done));}
