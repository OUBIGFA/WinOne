// Exercise actual React layout under the production CSP and isolated file I/O.
import {createRequire} from 'node:module';
import {createServer} from 'node:http';
import {readFile,mkdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {execFileSync} from 'node:child_process';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [base,folder]=process.argv.slice(2),output=resolve(folder);
assert.equal(new URL(base).hostname,'127.0.0.1');
execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);await mkdir(output,{recursive:true});
const policy=(await readFile('apps/desktop/frontend/index.html','utf8')).match(/content="(default-src[^"]+)"/)[1];
const png=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aHfoAAAAASUVORK5CYII=','base64');
const pdfText='BT /F1 20 Tf 50 700 Td (WinOne PDF proof) Tj ET';
const pdfObjects=['<< /Type /Catalog /Pages 2 0 R >>','<< /Type /Pages /Kids [3 0 R] /Count 1 >>','<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>','<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>',`<< /Length ${pdfText.length} >>\nstream\n${pdfText}\nendstream`];
let pdf='%PDF-1.4\n';const offsets=[0];for(const [index,obj] of pdfObjects.entries()){offsets.push(pdf.length);pdf+=`${index+1} 0 obj\n${obj}\nendobj\n`;}const xref=pdf.length;pdf+=`xref\n0 6\n0000000000 65535 f \n${offsets.slice(1).map(n=>String(n).padStart(10,'0')+' 00000 n ').join('\n')}\ntrailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF`;
const files=createServer((req,res)=>{res.setHeader('Access-Control-Allow-Origin','*');res.setHeader('Cache-Control','no-store');if(req.url==='/sample.pdf'){res.setHeader('Content-Type','application/pdf');res.end(pdf);}else if(req.url==='/picture.png'){res.setHeader('Content-Type','image/png');res.end(png);}else{res.setHeader('Content-Type','text/plain');res.end(req.url==='/README.md'?'# Project notes\n\nA readable **document**.\n\n- First item\n- Second item':'const answer = 42;\nconsole.log(answer);\n');}});
await new Promise(resolve=>files.listen(0,'127.0.0.1',resolve));const origin=`http://127.0.0.1:${files.address().port}`;
const browser=await chromium.launch({headless:true,...(process.env.WINONE_BROWSER_EXE?{executablePath:process.env.WINONE_BROWSER_EXE}:{channel:'msedge'})});
const failures=[],results=[];
async function check(name,body){try{results.push({name,...await body()});}catch(error){failures.push({name,error:error.message});}}
try{
 const page=await browser.newPage({viewport:{width:1280,height:900},reducedMotion:'reduce'});
 // Vite needs its development styles during initial load. Apply the exact
 // production policy afterwards, before opening any editor or media preview.
 // Fulfilling HTML through route() would change Chromium's address-space
 // classification and incorrectly block loopback requests in this probe.
 const applyPolicy=()=>page.evaluate(policy=>{const meta=document.createElement('meta');meta.httpEquiv='Content-Security-Policy';meta.content=policy;document.head.appendChild(meta);},policy);
 const url=(welcome=false,theme='light',language='en-US')=>`${base}/tests/chat-experience-preview.html?language=${language}&theme=${theme}&previewOrigin=${encodeURIComponent(origin)}${welcome?'&welcome':''}`;
 const box=selector=>page.locator(selector).boundingBox();
 const settle=()=>page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
 await page.goto(url());await page.locator('.chat-markdown h2').waitFor();await page.waitForFunction(()=>document.querySelector('.chat-composer textarea')?.value==='保留这段草稿');await page.evaluate(()=>document.fonts.ready);await settle();await applyPolicy();
 await check('composer overlays transcript without moving it with a solid boundary',async()=>{const before=await box('.chat-transcript'),scroll=await page.locator('.chat-transcript-viewport').evaluate(e=>e.scrollTop),bottom=await box('.chat-composer');const handle=page.locator('.chat-composer-resize').first();await handle.focus();await handle.press('ArrowUp');await handle.press('ArrowUp');await settle();const after=await box('.chat-transcript'),grown=await box('.chat-composer');assert.ok(Math.abs(before.height-after.height)<1,'transcript height changed');assert.ok(Math.abs(await page.locator('.chat-transcript-viewport').evaluate(e=>e.scrollTop)-scroll)<1,'transcript scrolled');assert.ok(Math.abs(bottom.y+bottom.height-grown.y-grown.height)<1,'composer bottom moved');assert.equal(await page.locator('.chat-input-area').evaluate(e=>getComputedStyle(e,'::before').backgroundImage),'none');return {before,after};});
 await check('project entry stays on one line',async()=>{const label=page.getByRole('button',{name:'Open or create project',exact:true});const geometry=await label.evaluate(e=>{const text=e.querySelector('span');return {height:e.getBoundingClientRect().height,whiteSpace:text&&getComputedStyle(text).whiteSpace,overflow:text&&getComputedStyle(text).textOverflow};});assert.equal(geometry.whiteSpace,'nowrap');assert.equal(geometry.overflow,'ellipsis');assert.ok(geometry.height<42);return geometry;});
 await check('left divider remains visible',async()=>{const divider=await page.locator('[data-slot=resizable-handle]').first().evaluate(e=>({width:e.getBoundingClientRect().width,color:getComputedStyle(e).backgroundColor}));assert.ok(divider.width>=1,'divider shrank below one pixel');return divider;});
 await check('code and tables reflow completely within the reading column',async()=>{const code=page.locator('.chat-answer-content pre').last();assert.equal(await code.evaluate(e=>getComputedStyle(e).whiteSpace),'pre-wrap');assert.ok(await page.locator('.desktop-markdown-codebar').count());assert.equal(await page.locator('.chat-answer-content table').evaluate(e=>getComputedStyle(e).tableLayout),'fixed');return {};});
 await page.getByRole('button',{name:'Show chat details',exact:true}).click();await page.locator('.client-tools-grid').waitFor();await settle();
 await check('original BrowserPanel capsules keep labels and icons intact',async()=>{const metrics=await page.locator('.client-browser-tab').evaluateAll(tabs=>tabs.map(tab=>{const label=tab.querySelector('[role=tab]>span'),style=getComputedStyle(tab),close=tab.querySelector('.desktop-icon-control svg');return {text:label.textContent,width:label.clientWidth,natural:label.scrollWidth,border:style.borderTopWidth,radius:style.borderRadius,icon:close.getBoundingClientRect().width};}));assert.ok(metrics.length);for(const tab of metrics){assert.ok(tab.width>=tab.natural-1,'short tab text was clipped: '+tab.text);assert.equal(tab.border,'1px');assert.ok(tab.icon>=12);}return {metrics};});
 await page.getByRole('button',{name:/^Files.*Ctrl/}).click();
 const menuAction=async(name)=>{await page.getByRole('button',{name:'Tab actions',exact:true}).click();await page.getByRole('menuitem',{name,exact:true}).click();};
 await check('file opens and fills side panel',async()=>{await page.getByRole('button',{name:/README.md.*120 B/}).click();await page.getByRole('heading',{name:'Project notes'}).waitFor();const panel=await box('.chat-side-panel'),preview=await box('.chat-file-preview');assert.ok(preview.width>panel.width-2);assert.ok(preview.height>panel.height-110);return {panel,preview};});
 await check('source editor works under strict CSP and saves the edited text',async()=>{
  await menuAction('View source');const editor=page.locator('.cm-content');await editor.waitFor();
  const styles=await page.locator('.cm-scroller').evaluate(e=>({display:getComputedStyle(e).display,height:e.getBoundingClientRect().height,root:e.getRootNode().constructor.name}));
  assert.equal(styles.display,'flex');assert.equal(styles.root,'ShadowRoot');assert.ok(styles.height>200);
  await editor.fill('# Edited document\n\nSaved from the file tab.');await menuAction('Save file');
  await page.waitForFunction(()=>window.chatExperienceCalls.some(c=>c.method==='file.save'&&c.params.text.includes('Edited document')));
  await menuAction('View preview');await page.getByRole('heading',{name:'Edited document'}).waitFor();return styles;
 });
 await check('dirty file retains edits when close is cancelled',async()=>{
  await menuAction('View source');await page.locator('.cm-content').fill('Unsaved text');await page.getByRole('button',{name:'Close README.md',exact:true}).click();
  await page.getByRole('dialog').waitFor();await page.getByRole('button',{name:'Cancel',exact:true}).click();assert.equal(await page.locator('.cm-content').innerText(),'Unsaved text');
  await page.getByRole('button',{name:'Close README.md',exact:true}).click();await page.getByRole('button',{name:'Discard changes',exact:true}).click();await page.getByRole('tab',{name:'README.md',exact:true}).waitFor({state:'hidden'});return {};
 });
 await check('local image passes production image policy',async()=>{await page.getByRole('tab',{name:'Files',exact:true}).click();await page.getByRole('button',{name:/picture.png.*120 B/}).click();await page.locator('.chat-image-preview img').waitFor();await page.waitForFunction(()=>document.querySelector('.chat-image-preview img')?.naturalWidth>0,{},{timeout:4000});return {};});
 await page.screenshot({path:join(output,'panel.png')});
 await check('PDF paints and exposes selectable text under production policy',async()=>{await page.getByRole('tab',{name:'Files',exact:true}).click();await page.getByRole('button',{name:/sample.pdf.*120 B/}).click();await page.locator('[data-pdf-rendered=true]').waitFor();await page.locator('.textLayer').getByText('WinOne PDF proof').waitFor();return {pages:await page.locator('.pdfViewer .page').count()};});
 for(const theme of ['light','dark'])await check(`upstream geometry with requested surface colors: ${theme}`,async()=>{
  await page.goto(url(false,theme)+'&sample=reading');await page.locator('.desktop-conversation-message').first().waitFor();await page.getByRole('button',{name:'Show chat details',exact:true}).click();await page.locator('.client-tools-grid').waitFor();await settle();
  const reference=await browser.newPage({viewport:{width:1280,height:900},reducedMotion:'reduce'});
  try{
   await reference.goto(`${base}/tests/chat-kit-reference-preview.html?theme=${theme}`);await reference.locator('.client-browser-tab').waitFor();
   const styles=p=>p.evaluate(()=>{
    const read=(selector,properties)=>{const s=getComputedStyle(document.querySelector(selector));return Object.fromEntries(properties.map(key=>[key,s[key]]));};
    return {tab:read('.client-browser-tab',['backgroundColor','borderRadius','borderTopWidth','padding','fontWeight']),icon:read('[data-desktop-action=\"client.tab.add\"]',['width','height']),user:read('.desktop-conversation-message--user>div:first-child',['borderRadius','padding']),heading:read('.desktop-ai-markdown h2',['fontSize','fontWeight','lineHeight','marginBottom']),code:read('.desktop-ai-markdown pre',['padding','lineHeight','backgroundColor'])};
   });
   const actual=await styles(page),expected=await styles(reference);assert.deepEqual(actual,expected);
   await page.screenshot({path:join(output,`upstream-${theme}.png`)});return actual;
  }finally{await reference.close();}
 });
 for(const width of [720,1280])for(const theme of ['light','dark'])for(const language of ['zh-CN','en-US']){
  await page.evaluate(()=>localStorage.removeItem('winone.chat.composer-height.v1'));await page.setViewportSize({width,height:900});await page.goto(url(true,theme,language));await page.locator('.chat-composer').waitFor();await page.evaluate(()=>document.fonts.ready);await settle();
  await check(`welcome grows upward: ${width} ${theme} ${language}`,async()=>{const before=await box('.chat-composer'),logoBefore=await box('.chat-welcome-logo');const handle=page.locator('.chat-composer-resize').first(),grip=await handle.boundingBox();await page.mouse.move(grip.x+grip.width/2,grip.y+grip.height/2);await page.mouse.down();await page.mouse.move(grip.x+grip.width/2,grip.y-250,{steps:12});await page.mouse.up();await settle();const after=await box('.chat-composer'),logoAfter=await box('.chat-welcome-logo');assert.ok(Math.abs((logoAfter.y-logoBefore.y)-(after.y-before.y))<1,'logo did not follow the editor: '+JSON.stringify({before,after,logoBefore,logoAfter}));assert.ok(logoAfter.y+logoAfter.height<after.y,'logo was covered');assert.ok(after.height>before.height);assert.ok(after.y<before.y);const stage=await box('.chat-welcome-stage'),docked=await box('.chat-welcome-composer');assert.ok(docked.y+docked.height<=stage.y+stage.height-12,'welcome exceeded the bottom boundary');assert.ok(after.y>=60,'composer crossed header');await handle.focus();for(let i=0;i<30;i++)await handle.press('ArrowUp');const maximum=await box('.chat-composer');assert.ok(maximum.y>=60);assert.ok(Math.abs(maximum.y+maximum.height-stage.y-stage.height+12)<2,'maximum editor did not reach bottom boundary: '+JSON.stringify({stage,before,after,maximum}));assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));await page.screenshot({path:join(output,`welcome-${width}-${theme}-${language}.png`)});return {before,after,maximum};});
 }
 await writeFile(join(output,'report.json'),JSON.stringify({results,failures},null,2));console.log(JSON.stringify({checks:results.length+failures.length,failures},null,2));assert.equal(failures.length,0);
}finally{await browser.close();files.close();}
