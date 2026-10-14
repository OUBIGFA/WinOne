// Browser regressions for user-reported resize and attachment overflow.
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import path from 'node:path';
import {execFileSync} from 'node:child_process';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [base,folder]=process.argv.slice(2),output=path.resolve(folder);
assert.equal(new URL(base).hostname,'127.0.0.1');execFileSync('git',['check-ignore','--quiet','--no-index',path.join(output,'probe')]);await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,channel:'msedge'}),results=[],failures=[];
const settle=page=>page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
const geometry=page=>page.evaluate(()=>Object.fromEntries(['composer','composer-toolbar','transcript-viewport'].map(name=>[name,document.querySelector('.chat-'+name)?.getBoundingClientRect().toJSON()])));
try{
 for(const width of [640,1120,1600]){
  const page=await browser.newPage({viewport:{width,height:900},reducedMotion:'reduce'});
  await page.goto(`${base}/tests/chat-experience-preview.html?welcome`);await page.locator('.chat-composer').waitFor();await settle(page);
  const before=await geometry(page),handle=await page.locator('.chat-composer-resize').boundingBox();
  await page.mouse.move(handle.x+handle.width/2,handle.y+handle.height/2);await page.mouse.down();await page.mouse.move(handle.x+handle.width/2,handle.y+handle.height/2-45,{steps:6});await page.mouse.up();await settle(page);
  const after=await geometry(page),up=before.composer.top-after.composer.top,down=after.composer.bottom-before.composer.bottom;
  if(!(up>10&&down>10&&Math.abs(up-down)<3))failures.push(`welcome ${width}: edges did not grow together (${up}, ${down})`);
  await page.screenshot({path:path.join(output,`welcome-${width}.png`)});
  await page.goto(`${base}/tests/chat-experience-preview.html`);await page.locator('.chat-composer').waitFor();await settle(page);
  const separator=page.locator('.chat-composer-resize');for(let i=0;i<30;i++)await separator.press('ArrowDown');
  await page.getByRole('button',{name:'附件与工具',exact:true}).click();await page.getByRole('menuitem',{name:'添加附件',exact:true}).click();await page.locator('.cindy-attachment-card').last().waitFor();await settle(page);
  const attached=await geometry(page);
  if(attached['composer-toolbar'].bottom>attached.composer.bottom-2)failures.push(`attachments ${width}: toolbar is outside composer`);
  if(await page.locator('.chat-composer').evaluate(node=>node.scrollWidth>node.clientWidth+2))failures.push(`attachments ${width}: horizontal overflow`);
  await page.screenshot({path:path.join(output,`attachments-${width}.png`)});results.push({width,before,after,attached});await page.close();
 }
 await writeFile(path.join(output,'report.json'),JSON.stringify({results,failures},null,2));assert.deepEqual(failures,[]);console.log('Chat completion geometry passed at 3 widths.');
}finally{await browser.close();}
