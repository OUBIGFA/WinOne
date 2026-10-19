// Real pointer drags exercise the three production splitters. No user files or
// settings are read: the preview supplies an isolated Host and browser profile.
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {execFileSync} from 'node:child_process';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [base,folder]=process.argv.slice(2),output=resolve(folder);
assert.equal(new URL(base).hostname,'127.0.0.1');
execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,channel:'msedge'}),results=[];
try{
 for(const viewport of [1440,720]){
  const page=await browser.newPage({viewport:{width:viewport,height:950},reducedMotion:'reduce'}),errors=[];
  page.on('pageerror',error=>errors.push(error.message));
  await page.goto(`${base}/tests/chat-experience-preview.html?alignment&shell&language=en-US`);
  await page.getByText('I will check the project first.',{exact:true}).waitFor();
  async function drag(name,panelSelector,widths,right=false){
   await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
   const handle=page.getByRole('separator',{name,exact:true});
   const panel=page.locator(panelSelector),start=await handle.boundingBox(),box=await panel.boundingBox();
   const origin=right?box.x+box.width:box.x,y=start.y+start.height/2;
   await page.mouse.move(start.x+start.width/2,y);await page.mouse.down();
   const samples=[];
   for(const width of widths){
    await page.mouse.move(origin+(right?-width:width),y,{steps:4});
    await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
    const actual=(await panel.boundingBox()).width;
    samples.push({expected:width,actual});
    assert(Math.abs(actual-width)<3,`${name}: drag snapped from ${width}px to ${actual}px`);
   }
   await page.mouse.up();results.push({viewport,name,samples});
  }
  await drag('Resize sidebar','#app-sidebar',[100,140,90,64,120]);
  await drag('Resize chat list','#sidebar',[150,110,60,20,0,50,130,190]);
  await page.getByRole('button',{name:'Hide chat list',exact:true}).click();
  await page.getByRole('button',{name:'Show chat details',exact:true}).click();
  await page.locator('.chat-side-panel').waitFor({state:'visible'});
  await drag('Resize chat details','#details',[200,140,70,20,0,70,150,220],true);
  const before=await page.locator('#details').boundingBox();
  await page.getByRole('button',{name:'Hide chat details',exact:true}).click();
  await page.getByRole('button',{name:'Show chat details',exact:true}).click();
  await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
  const restored=(await page.locator('#details').boundingBox()).width;
  assert(Math.abs(restored-before.width)<3,`Reopening lost the chosen width: ${before.width} -> ${restored}`);
  // A drag through zero must retain state and leave the same handle usable.
  await page.getByRole('button',{name:'Show chat list',exact:true}).click();
  assert(await page.getByRole('button',{name:'Chat settings',exact:true}).isVisible());
  const svg=await page.getByRole('button',{name:'Hide chat details',exact:true}).locator('svg').innerHTML();
  for(const name of ['Hide chat list','Expand sidebar']){
   assert.equal(await page.getByRole('button',{name,exact:true}).locator('svg').innerHTML(),svg,`${name}: sidebar icon differs`);
  }
  await page.screenshot({path:join(output,`sidebars-${viewport}.png`)});
  assert.deepEqual(errors,[]);await page.close();
 }
 await writeFile(join(output,'report.json'),JSON.stringify(results,null,2));
 console.log(`Continuous sidebar dragging: ${results.length} splitter cases passed.`);
}finally{await browser.close();}
