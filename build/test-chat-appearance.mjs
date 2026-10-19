// Font coverage, Cindy surface/fade, hover affordances, round navigation and
// welcome-to-conversation sizing all run against the actual production page.
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {execFileSync} from 'node:child_process';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [base,folder]=process.argv.slice(2),output=resolve(folder);
assert.equal(new URL(base).hostname,'127.0.0.1');execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,channel:'msedge'}),results=[];
try{
 for(const theme of ['light','dark']){
  const page=await browser.newPage({viewport:{width:1280,height:1000},reducedMotion:'reduce'}),errors=[],badFonts=[];
  page.on('pageerror',error=>errors.push(error.message));
  page.on('response',response=>{if(response.url().includes('.woff')&&response.status()>=400)badFonts.push(response.url());});
  await page.goto(`${base}/tests/chat-experience-preview.html?alignment&shell&theme=${theme}&language=en-US`);
  await page.getByText('I will check the project first.',{exact:true}).waitFor();
  const fontCoverage=await page.evaluate(async()=>{
   const families=[['Inter Variable',[100,200,300,400,500,600,700,800,900],['normal','italic'],'Abc 123'],['JetBrains Mono Variable',[100,200,300,400,500,600,700,800],['normal','italic'],'code 123'],['HarmonyOS Sans SC',[100,300,400,500,700,900],['normal'],'中文字体']];
   const results=[];
   for(const [family,weights,styles,text] of families)for(const weight of weights)for(const style of styles){
    const faces=await document.fonts.load(`${style} ${weight} 18px "${family}"`,text);
    if(!faces.length||faces.some(face=>face.status!=='loaded'))throw Error(`Missing font: ${family} ${style} ${weight}`);
    results.push({family,weight,style,faces:faces.length});
   }
   await document.fonts.ready;
   return results;
  });
  const appearance=await page.evaluate(()=>({font:getComputedStyle(document.body).fontFamily,page:getComputedStyle(document.querySelector('.chat-main')).backgroundColor,input:getComputedStyle(document.querySelector('.chat-composer')).backgroundColor,fade:getComputedStyle(document.querySelector('.chat-input-area'),'::before').backgroundImage,fadeHeight:getComputedStyle(document.querySelector('.chat-input-area'),'::before').height}));
  assert(appearance.font.includes('Inter Variable'));assert.equal(appearance.page,theme==='light'?'rgb(252, 252, 252)':'rgb(25, 25, 25)');assert.equal(appearance.input,theme==='light'?'rgb(255, 255, 255)':'rgb(35, 35, 35)');assert(appearance.fade.includes('linear-gradient'));assert.equal(appearance.fadeHeight,'32px');
  await page.mouse.move(0,0);
  const replies=page.locator('.desktop-conversation-message--assistant');
  for(const actions of await replies.locator('.desktop-conversation-message-actions').all())assert.equal(await actions.evaluate(node=>getComputedStyle(node).opacity),'0');
  const last=replies.last();await last.hover();
  assert.equal(await last.locator('.desktop-conversation-message-actions').evaluate(node=>getComputedStyle(node).opacity),'1');
  const source=last.locator('.chat-message-source');if(await source.count())assert(await source.evaluate(node=>node.closest('.desktop-conversation-message-actions')!==null));
  await page.mouse.move(0,0);await last.locator('.desktop-conversation-message-actions button').first().focus();
  assert.equal(await last.locator('.desktop-conversation-message-actions').evaluate(node=>getComputedStyle(node).opacity),'1');
  await page.getByRole('textbox',{name:'Message',exact:true}).fill('Check the next round.');await page.getByRole('button',{name:'Send',exact:true}).click();
  await page.getByText('The follow-up is complete.',{exact:true}).waitFor();
  assert(await page.locator('.chat-generated-files').evaluate(node=>!!(node.compareDocumentPosition([...document.querySelectorAll('.chat-markdown p')].find(item=>item.textContent==='The follow-up is complete.'))&Node.DOCUMENT_POSITION_PRECEDING)));
  const viewport=page.locator('.chat-transcript-viewport');
  await page.getByRole('button',{name:'Back to top',exact:true}).click();await page.waitForFunction(()=>document.querySelector('.chat-transcript-viewport').scrollTop<1);
  await page.getByRole('button',{name:'Next turn',exact:true}).click();
  await page.waitForFunction(()=>document.querySelector('.chat-transcript-viewport').scrollTop>20);
  await page.getByRole('button',{name:'Previous turn',exact:true}).click();
  const firstOffset=await viewport.evaluate(node=>node.scrollTop);assert(firstOffset<80);
  await page.getByRole('button',{name:'Go to bottom',exact:true}).click();
  await page.waitForFunction(()=>{const node=document.querySelector('.chat-transcript-viewport');return node.scrollHeight-node.clientHeight-node.scrollTop<2;});
  await page.screenshot({path:join(output,`chat-${theme}.png`)});
  // A tall welcome editor must not persist its geometry into a conversation.
  await page.goto(`${base}/tests/chat-experience-preview.html?alignment&shell&welcome&theme=${theme}&language=en-US`);
  const handle=page.getByRole('separator',{name:'Resize message input',exact:true});await handle.waitFor();
  for(let i=0;i<5;i++)await handle.press('ArrowUp');
  await page.waitForFunction(()=>document.querySelector('.chat-composer').getBoundingClientRect().height>300);
  const tall=(await page.locator('.chat-composer').boundingBox()).height;
  if(tall<=300){await page.screenshot({path:join(output,'welcome-size-failure.png')});console.log(await page.evaluate(()=>Object.fromEntries(['.chat-main','.chat-welcome','.chat-welcome-composer','.chat-composer'].map(selector=>[selector,document.querySelector(selector).getBoundingClientRect().toJSON()]))));}
  assert(tall>300,`Welcome composer did not grow: ${tall}`);
  await page.getByRole('textbox',{name:'Message',exact:true}).fill('Create a conversation.');await page.getByRole('button',{name:'Send',exact:true}).click();
  await page.getByText('The follow-up is complete.',{exact:true}).waitFor();
  await page.waitForFunction(()=>Math.abs(document.querySelector('.chat-composer').getBoundingClientRect().height-220)<1);
  const settled=await page.locator('.chat-composer').boundingBox();assert(Math.abs(settled.height-220)<1,`Composer retained welcome height ${settled.height}`);
  const main=await page.locator('.chat-main').boundingBox(),footer=await page.locator('.chat-input-area').boundingBox();assert(Math.abs(main.y+main.height-footer.y-footer.height)<1);
  assert.deepEqual(errors,[]);assert.deepEqual(badFonts,[]);results.push({theme,appearance,fontCoverage,tall,conversationHeight:settled.height});await page.close();
 }
 await writeFile(join(output,'report.json'),JSON.stringify(results,null,2));console.log('Chat appearance: both themes, all bundled font weights, hover, navigation and page transition passed.');
}finally{await browser.close();}
