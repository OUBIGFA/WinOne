// Production Chat, tool-snapshot dialogs and the real Shadow DOM editor.
// All Host calls, content and clipboard writes are isolated test fixtures.
// Usage: node build/test-chat-resilience-ui.mjs <ignored output directory>
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {mkdir,readFile,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {pathToFileURL} from 'node:url';
import {execFileSync} from 'node:child_process';
import {createServer} from 'node:http';
const output=resolve(process.argv[2]||'_temp/chat-resilience-ui/browser');
execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);
await mkdir(output,{recursive:true});
const frontend=resolve('apps/desktop/frontend');
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const {createServer:createViteServer}=await import(pathToFileURL(join(frontend,'node_modules/vite/dist/node/index.js')).href);
const documentText=await readFile(join(frontend,'tests/fixtures/chat-resilience.md'),'utf8');
const image=await readFile('internal/branding/logo.png');
const files=createServer((req,res)=>{
 res.setHeader('Access-Control-Allow-Origin','*');res.setHeader('Cache-Control','no-store');
 if(req.url?.includes('.png')){res.setHeader('Content-Type','image/png');res.end(image);}
 else if(req.url?.includes('files.')){res.setHeader('Content-Type','application/json');res.end(JSON.stringify({directory:'C:/fixture/Alpha',entries:[{name:'README.md',path:'C:/fixture/Alpha/README.md',directory:false,size:documentText.length}]}));}
 else {res.setHeader('Content-Type','text/plain; charset=utf-8');res.end(documentText);}
});
await new Promise(done=>files.listen(0,'127.0.0.1',done));
const previewOrigin=`http://127.0.0.1:${files.address().port}`;
const vite=await createViteServer({root:frontend,cacheDir:join(output,'vite-cache'),server:{host:'127.0.0.1',port:0},logLevel:'warn'});
await vite.listen();
const base=`http://127.0.0.1:${vite.httpServer.address().port}`;
const browser=await chromium.launch({headless:true,...(process.env.WINONE_BROWSER_EXE?{executablePath:process.env.WINONE_BROWSER_EXE}:{channel:'msedge'})});
let page;const report=[];
const settle=()=>page.evaluate(async()=>{await document.fonts.ready;await new Promise(done=>requestAnimationFrame(()=>requestAnimationFrame(done)));});
async function widthOf(selector){return page.locator(selector).evaluate(el=>el.getBoundingClientRect().width);}
async function noHorizontal(selector,label){
 await page.evaluate(label=>{window.fixturePhase=label;},label);
 const result=await page.locator(selector).evaluate(root=>{
  const bars=[],content=[];
  const inspect=node=>{for(const el of [node,...node.querySelectorAll('*')]){
   if(!(el instanceof HTMLElement)||!el.clientWidth||!el.getClientRects().length||el.closest('[aria-hidden=true],.cm-measure'))continue;
   const style=getComputedStyle(el);if(style.visibility==='hidden'||style.display==='none')continue;
   const value={tag:el.tagName,class:el.className,client:el.clientWidth,scroll:el.scrollWidth,overflow:style.overflowX};
   const hiddenScrollbar=style.scrollbarWidth==='none'||getComputedStyle(el,'::-webkit-scrollbar').display==='none';
   if(!hiddenScrollbar&&(['auto','scroll'].includes(style.overflowX)&&el.scrollWidth>el.clientWidth+1||style.overflowX==='scroll'))bars.push(value);
   if(el.matches('.msg-markdown,.chat-preview-markdown,.chat-math-fit,.chat-markdown-table,pre,.cm-editor,.cm-content,.cm-scroller,.cm-line,.cm-md-table-widget,.cm-md-mermaid-card,td,th')&&el.scrollWidth>el.clientWidth+1)content.push(value);
   if(el.shadowRoot)inspect(el.shadowRoot);
  }};
  inspect(root);return {bars,content};
 });
 report.push({label,...result});assert.deepEqual(result,{bars:[],content:[]},label+' contains horizontal scrolling or clipped content');
}
async function plateMatches(){
 const value=await page.locator('.chat-panel-major-tabs').evaluate(root=>{
  const plate=root.querySelector('.cindy-segmented-plate');
  const selected=root.querySelector('[aria-selected=true]');
  if(!plate||!selected)return {missing:true,html:root.innerHTML};
  const a=plate.getBoundingClientRect(),b=selected.getBoundingClientRect();
  return {left:Math.abs(a.left-b.left),width:Math.abs(a.width-b.width),height:root.getBoundingClientRect().height};
 });
 assert(!value.missing,JSON.stringify(value));assert(value.left<1&&value.width<1,JSON.stringify(value));assert.equal(value.height,28);return value;
}
async function expandWork(){
 const rows=page.locator('button[data-slot="cindy-work"][aria-expanded=false]');
 while(await rows.count())await rows.first().click();
}
try{
 for(const width of [1360,980])for(const theme of ['light','dark']){
  const language=width===1360?'zh-CN':'en-US',id=`${width}-${theme}`,errors=[];
  page=await browser.newPage({viewport:{width,height:960},reducedMotion:theme==='dark'?'reduce':'no-preference'});
  page.on('pageerror',error=>errors.push(error.message));
  if(process.env.WINONE_TRACE_RESIZE==='1')await page.addInitScript(()=>{
   const Native=window.ResizeObserver;window.fixtureResizes=[];
   window.ResizeObserver=class extends Native{constructor(callback){const source=new Error().stack;super((entries,observer)=>{window.fixtureResizes.push({source,targets:entries.map(entry=>({tag:entry.target.tagName,class:entry.target.className,width:entry.contentRect.width,height:entry.contentRect.height}))});window.fixtureResizes=window.fixtureResizes.slice(-8);callback.call(observer,entries,observer);});}};
  });
  await page.addInitScript(()=>{
   // Legacy saved expansion must no longer reopen the workbench on load.
   localStorage.setItem('react-resizable-panels:winone-chat-workbench-right:conversation:details',JSON.stringify({conversation:50,details:50}));
   localStorage.setItem('winone.chat.expanded-panels.v1',JSON.stringify({sidebar:240,right:390,bottom:280}));
   window.fixtureErrors=[];window.addEventListener('error',event=>window.fixtureErrors.push({message:event.message,phase:window.fixturePhase,resizes:window.fixtureResizes}));
   Object.defineProperty(navigator,'clipboard',{value:{writeText:async text=>{window.fixtureClipboard=text;}}});
  });
  const query=`resilience&running=true${width===980?'&planDone':''}&theme=${theme}&language=${language}&previewOrigin=${encodeURIComponent(previewOrigin)}`;
  await page.goto(`${base}/tests/chat-experience-preview.html?welcome&${query}`);
  await page.locator('.chat-composer').waitFor();await settle();
  const labels=await page.evaluate(async language=>(await import('/src/locales/chat.ts')).chatMessages[language==='zh-CN'?'zh':'en'],language);
  const closePanel=()=>page.locator('.chat-side-panel').getByRole('button',{name:language==='zh-CN'?'关闭侧栏':'Close sidebar',exact:true}).click();
  assert(await widthOf('#details')<1,'Initial session restored an expanded workbench');
  await page.getByRole('button',{name:labels.showDetails,exact:true}).click();
  await page.locator('.chat-panel-major-tabs').waitFor();await settle();
  assert(await widthOf('#details')>100);
  await page.locator('#chat-panel-documents-tab').click();await page.waitForTimeout(240);
  const tabs=await page.locator('.chat-panel-major-tabs').evaluate(root=>{
   const style=getComputedStyle(root),selected=root.querySelector('[aria-selected=true]'),canvas=document.createElement('canvas');canvas.width=canvas.height=1;
   const context=canvas.getContext('2d'),sample=value=>{context.clearRect(0,0,1,1);context.fillStyle=value;context.fillRect(0,0,1,1);return [...context.getImageData(0,0,1,1).data].slice(0,3);};
   return {borders:[style.borderTopWidth,style.borderRightWidth,style.borderBottomWidth,style.borderLeftWidth],background:sample(style.backgroundColor),foreground:sample(getComputedStyle(selected).color),plate:sample(getComputedStyle(root.querySelector('.cindy-segmented-plate')).backgroundColor),options:[...root.querySelectorAll('[role=tab]')].map(tab=>({width:tab.getBoundingClientRect().width,icon:!!tab.querySelector('svg'),nowrap:getComputedStyle(tab.querySelector('span')).whiteSpace}))};
  });
  assert.deepEqual(tabs.borders,['0px','0px','1px','0px']);
  for(const color of [tabs.background,tabs.foreground,tabs.plate])assert(Math.max(...color)-Math.min(...color)<2,'Primary tabs are not grayscale');
  assert(theme==='light'?tabs.background[0]>200&&tabs.foreground[0]<100:tabs.background[0]<100&&tabs.foreground[0]>200,'Primary tabs do not follow the workbench theme');
  assert(tabs.plate[0]!==tabs.background[0],'Selected tab has no visible state');
  assert.equal(tabs.options.length,2);assert(Math.abs(tabs.options[0].width-tabs.options[1].width)<1,'Primary tabs are not equal width');
  assert(tabs.options.every(tab=>tab.icon&&tab.nowrap==='nowrap'),'Primary tab icons or single-line labels missing');
  await plateMatches();
  const resize=page.getByRole('separator',{name:labels.resizeDetails,exact:true}),handle=await resize.boundingBox();
  await page.mouse.move(handle.x+handle.width/2,handle.y+handle.height/2);await page.mouse.down();
  const drag=[];
  for(const delta of [-60,50,110,-30,30]){await page.mouse.move(handle.x+delta,handle.y+handle.height/2);await settle();drag.push(await plateMatches());}
  await page.mouse.up();await settle();
  const adjusted=await widthOf('#details');
  await closePanel();await settle();assert(await widthOf('#details')<1);
  await page.getByRole('button',{name:labels.showDetails,exact:true}).click();await settle();assert(Math.abs(await widthOf('#details')-adjusted)<2,'Explicitly resized width was lost');
  // Opening an old session must close the workbench, without losing that width.
  await page.locator('[data-session-id=experience] .chat-sidebar-row').click();
  await page.locator('.chat-plan').first().waitFor();await settle();assert(await widthOf('#details')<1,'Switching to a saved session reopened the panel');
  await page.locator('[data-session-id=background][data-activity=running]').waitFor();
  await page.locator('[data-session-id=empty][data-activity=time]').waitFor();
  await page.getByRole('button',{name:labels.showDetails,exact:true}).click();
  await page.locator('#chat-panel-library-tab').click();await page.locator('.rsb-fbody-tree-pane').waitFor();await settle();
  const tree=page.locator('.rsb-fbody-tree-pane'),preview=page.locator('.rsb-fbody-compact');
  const left=await tree.boundingBox(),right=await preview.boundingBox();
  assert(left.x+left.width<=right.x&&right.width>=95,'File tree must precede the preview without overlap');
  const treeResize=page.getByRole('separator',{name:labels.resizeFileTree,exact:true});
  await treeResize.focus();await treeResize.press('ArrowLeft');await settle();const smaller=await widthOf('.rsb-fbody-tree-pane');
  await treeResize.press('ArrowRight');await settle();assert(await widthOf('.rsb-fbody-tree-pane')>smaller,'Right arrow does not grow the left file tree');
  const treeGrip=await treeResize.boundingBox();await page.mouse.move(treeGrip.x+treeGrip.width/2,treeGrip.y+treeGrip.height/2);await page.mouse.down();
  const beforeDrag=await widthOf('.rsb-fbody-tree-pane');await page.mouse.move(treeGrip.x-24,treeGrip.y+treeGrip.height/2,{steps:4});await page.mouse.up();await settle();
  assert(await widthOf('.rsb-fbody-tree-pane')<beforeDrag,'Dragging left does not shrink the left file tree');
  await noHorizontal('.chat-side-panel',id+'-file-tree');report.push({id,filePanes:{left,right,keyboardAndDrag:true}});
  await page.locator('.chat-side-panel').getByRole('button',{name:labels.hideDetails,exact:true}).click();await settle();
  await expandWork();
  const read=page.locator('.chat-work-entry button').filter({hasText:'README.md'}).first();await read.click();
  const dialog=page.getByRole('dialog');await dialog.locator('.chat-tool-document h1').waitFor();
  assert.equal(await dialog.locator('.chat-tool-document h1').innerText(),'Chat 文档预览');
  assert(await dialog.locator('.chat-tool-document table').count()>0);assert(await dialog.locator('.chat-tool-document li').count()>0);
  await dialog.locator('.chat-tool-document .katex-display').waitFor();await settle();
  await noHorizontal('[role=dialog]',id+'-read-snapshot');
  await dialog.locator('.chat-tool-file-actions').getByRole('button',{name:labels.fileSource,exact:true}).click();
  assert.equal(await dialog.locator('.chat-tool-document>pre').textContent(),documentText);
  await dialog.locator('.chat-tool-file-actions').getByRole('button',{name:labels.copy,exact:true}).click();
  assert.equal(await page.evaluate(()=>window.fixtureClipboard),documentText,'Copy did not preserve the operation snapshot');
  assert(!await dialog.locator('.chat-tool-raw').getAttribute('open'),'Raw protocol is expanded by default');
  await dialog.locator('.chat-tool-file-actions').getByRole('button',{name:labels.fileRendered,exact:true}).click();
  await dialog.getByRole('button',{name:labels.openCurrentFile,exact:true}).click();
  const editor=page.locator('.chat-side-panel .cindy-file-editor .cm-editor');await editor.waitFor({timeout:20000});
  await page.locator('.chat-side-panel .cm-md-table-widget').waitFor();await settle();
  await noHorizontal('.chat-side-panel',id+'-shadow-editor');
  const contrast=await editor.evaluate(root=>{
   const canvas=document.createElement('canvas');canvas.width=canvas.height=1;const context=canvas.getContext('2d');
   const luminance=color=>{context.clearRect(0,0,1,1);context.fillStyle=color;context.fillRect(0,0,1,1);const [r,g,b]=[...context.getImageData(0,0,1,1).data].slice(0,3).map(v=>{v/=255;return v<=.04045?v/12.92:((v+.055)/1.055)**2.4;});return .2126*r+.7152*g+.0722*b;};
   let background='';for(let node=root;node;node=node.parentElement??node.getRootNode()?.host){const color=getComputedStyle(node).backgroundColor;if(!/rgba\(.*,[ ]?0\)$/.test(color)&&color!=='transparent'){background=color;break;}}
   const bg=luminance(background);
   return [...root.querySelectorAll('.cm-md-heading-line span,.cm-line a,.cm-line code,.cm-line .tok-link')].filter(n=>n.textContent.trim()).map(n=>{const fg=luminance(getComputedStyle(n).color);return {text:n.textContent.slice(0,35),color:getComputedStyle(n).color,background,ratio:(Math.max(fg,bg)+.05)/(Math.min(fg,bg)+.05)};});
  });
  assert(contrast.length>0,'Editor heading contrast was not measured');
  assert(contrast.every(sample=>sample.ratio>=4.5),JSON.stringify({id,contrast}));
  report.push({id,editorContrast:contrast});
  const content=editor.locator('.cm-content');
  await page.evaluate(()=>{window.fixturePhase='draft-file-barrier';window.chatExperienceIO.hold('draft.save');window.chatExperienceIO.hold('file.save');});
  await page.locator('.chat-composer textarea').fill('Draft barrier regression');
  await page.waitForFunction(()=>window.chatExperienceCalls.some(call=>call.method==='draft.save'&&call.params.draft.text==='Draft barrier regression'));
  await page.getByRole('button',{name:labels.newChat,exact:true}).click();
  await content.focus();await content.press('Control+End');await page.keyboard.insertText('\nGuarded edit during draft save.');await settle();
  await page.evaluate(()=>window.chatExperienceIO.release('draft.save'));
  await page.getByText(labels.saveFileBeforeNavigate,{exact:true}).waitFor();
  assert(await widthOf('#details')>100,'Pending navigation discarded a newly edited file');
  assert.equal(await page.locator('[data-session-id=experience] .chat-sidebar-row').getAttribute('aria-current'),'page');
  await page.waitForFunction(()=>window.chatExperienceCalls.some(call=>call.method==='file.save'&&call.params.text.includes('Guarded edit during draft save.')));
  await page.evaluate(()=>window.chatExperienceIO.release('file.save'));await settle();
  await content.press('Control+Home');await page.locator('.chat-side-panel .cm-md-table-widget').waitFor();
  const cell=page.locator('.chat-side-panel .cm-md-table-widget th').first();
  // CodeMirror scrolls and rebuilds virtualized blocks after Ctrl+Home. Read
  // drag coordinates only once the target is visible and that frame is painted.
  await cell.scrollIntoViewIfNeeded();await settle();
  const oldWidth=await cell.evaluate(el=>el.getBoundingClientRect().width);
  const grip=await cell.locator('.cm-md-table-resize-handle').boundingBox();
  await page.mouse.move(grip.x+grip.width/2,grip.y+grip.height/2);await page.mouse.down();await page.mouse.move(grip.x+100,grip.y+grip.height/2,{steps:6});await page.mouse.up();await settle();
  assert(await cell.evaluate(el=>el.getBoundingClientRect().width)>oldWidth+1,'Table column drag no longer resizes its column');
  await noHorizontal('.chat-side-panel',id+'-resized-table');
  await content.focus();await content.press('Control+End');
  await page.keyboard.insertText('\nBrowser fixture edit.');
  await page.waitForFunction(()=>window.chatExperienceCalls.some(call=>call.method==='file.save'&&call.params.text.includes('Browser fixture edit.')),{},{timeout:8000});
  await content.press('Control+f');const search=page.locator('.chat-side-panel [role=search]');await search.waitFor();await search.locator('input').fill('Markdown');
  await page.waitForFunction(()=>/\d+\/\d+/.test(document.querySelector('.chat-side-panel [role=search]')?.textContent||''));
  await search.locator('input').press('Escape');
  await page.screenshot({path:join(output,`editor-${id}.png`)});
  await closePanel();await settle();
  // The reminder is Cindy's own floating/pinned card, not an update_plan JSON row.
  await page.locator('.chat-transcript-viewport').evaluate(el=>{el.scrollTop=el.scrollHeight;});
  const reminder=page.locator('.chat-plan-reminder');await reminder.waitFor();
  await reminder.getByRole('button').first().click();await page.locator('[data-plan-flyout-positioner]').waitFor();
  assert((await page.locator('[data-plan-flyout-positioner]').innerText()).includes('验证工作过程和文件预览'));
  await page.keyboard.press('Escape');await page.locator('[data-plan-flyout-positioner]').waitFor({state:'hidden'});
  await page.getByRole('button',{name:labels.sessionTools,exact:true}).click();
  await page.getByRole('button',{name:labels.compact,exact:true}).waitFor();
  await page.getByRole('button',{name:labels.pauseTask,exact:true}).waitFor();
  await page.keyboard.press('Escape');
  // A real narrow conversation and dialog, not just a hidden scrollbar rule.
  await page.getByRole('button',{name:labels.hideSidebar,exact:true}).click();
  await page.setViewportSize({width:360,height:960});await settle();
  await noHorizontal('.chat-main',id+'-narrow-transcript');
  await expandWork();await page.locator('.chat-work-entry button').filter({hasText:'notes.md'}).first().click();
  await page.getByRole('dialog').locator('.chat-tool-document h1').waitFor();await settle();
  await noHorizontal('[role=dialog]',id+'-narrow-write-snapshot');
  await page.screenshot({path:join(output,`snapshot-narrow-${id}.png`)});
  await page.keyboard.press('Escape');
  await page.setViewportSize({width,height:960});await page.getByRole('button',{name:labels.showSidebar,exact:true}).click();
  await page.getByRole('button',{name:labels.newChat,exact:true}).click();await settle();assert(await widthOf('#details')<1,'New chat opened the workbench');
  const windowErrors=await page.evaluate(()=>window.fixtureErrors);report.push({id,language,tabs,drag,adjusted,browserErrors:errors,windowErrors});assert.deepEqual(errors,[]);assert.deepEqual(windowErrors,[]);await page.close();
 }
 // Full Chat routing uses the same production message links and sidebar;
 // only native side effects and provider configuration are fixture boundaries.
 for(const width of [1360,980])for(const theme of ['light','dark']){
  const language=width===1360?'zh-CN':'en-US',id=`links-${width}-${theme}`,errors=[];
  page=await browser.newPage({viewport:{width,height:900},reducedMotion:theme==='dark'?'reduce':'no-preference'});
  page.on('pageerror',error=>errors.push(error.message));
  await page.addInitScript(()=>Object.defineProperty(navigator,'clipboard',{value:{writeText:async text=>{window.fixtureClipboard=text;}}}));
  await page.goto(`${base}/tests/chat-experience-preview.html?links&theme=${theme}&language=${language}&previewOrigin=${encodeURIComponent(previewOrigin)}`);
  const labels=await page.evaluate(async language=>(await import('/src/locales/chat.ts')).chatMessages[language==='zh-CN'?'zh':'en'],language);
  const source=page.locator('.chat-transcript-viewport').getByRole('link',{name:'source',exact:true});await source.waitFor();await settle();
  assert((await page.locator('.chat-model-trigger').innerText()).includes(labels.chooseModel),'Retired model remains in the composer');
  assert((await page.locator('.chat-message-source').innerText()).includes('Example Model'),'Historical model metadata was lost');
  await page.locator('[data-session-id=experience][data-activity=interrupted]').waitFor();await page.locator('[data-session-id=background][data-activity=running]').waitFor();
  for(const session of ['experience','background'])assert(await page.locator(`[data-session-id=${session}] .chat-sidebar-row`).evaluate(row=>row.firstElementChild.classList.contains('chat-sidebar-leading-status')),'Status is not at the leading edge');
  const animation=await page.locator('[data-session-id=background] [data-sidebar-right-status=running]').evaluate(el=>getComputedStyle(el).animationName);
  assert(theme==='dark'?animation==='none':animation!=='none','Running indicator does not honor reduced motion');
  await source.focus();await source.press('Shift+F10');await page.getByRole('menu').waitFor();
  for(const name of [labels.fileMenuSidebar,labels.openSystem,labels.revealFile,labels.fileMenuCopyFile,labels.copyPath,labels.fileMenuCopyLocation])await page.getByRole('menuitem',{name,exact:true}).waitFor();
  await page.getByRole('menuitem',{name:labels.fileMenuCopyLocation,exact:true}).click();
  await page.waitForFunction(()=>window.fixtureClipboard==='README.md:12:3');
  await page.waitForFunction(()=>document.activeElement?.textContent==='source');
  await source.click({button:'right'});await page.getByRole('menuitem',{name:labels.fileMenuCopyFile,exact:true}).click();
  await page.waitForFunction(()=>window.chatExperienceCalls.some(call=>call.method==='media.native.copy-file'&&call.params.path.replaceAll('\\','/').endsWith('/README.md')));
  await source.click({modifiers:['Shift']});await source.click({button:'middle'});
  await page.waitForFunction(()=>window.chatExperienceCalls.filter(call=>call.method==='file.open').length===2);
  assert(await widthOf('#details')<1,'Separate activation changed the reading pane');
  await source.click();await page.locator('.chat-side-panel .cm-editor:visible').waitFor();
  const denied=page.locator('.chat-transcript-viewport').getByRole('link',{name:'denied',exact:true});await denied.click();
  await denied.locator('..').getByRole('alert').waitFor();
  assert(!await page.evaluate(()=>window.chatExperienceCalls.some(call=>call.method==='file.preview'&&call.params.path.endsWith('private.txt'))),'Denied file reached preview');
  const socket='The socket connection was closed unexpectedly. For more information, pass `verbose: true` in the second argument to fetch()';
  await page.evaluate(socket=>window.chatExperienceIO.fail('file.open',socket),socket);await source.click({modifiers:['Shift']});
  const failure=source.locator('..').getByRole('alert');await failure.waitFor();assert.equal(await failure.innerText(),labels.error_network);assert.equal(await failure.getAttribute('title'),socket);
  assert.equal(await page.locator('.desktop-conversation-message--user').innerText(),'Keep the original request.');
  assert.equal(await page.locator('.chat-composer textarea').inputValue(),'保留这段草稿');
  await page.evaluate(()=>window.chatExperienceIO.fail('file.open'));
  const html=page.locator('.chat-transcript-viewport').getByRole('link',{name:'HTML',exact:true});await html.click({button:'right'});
  await page.getByRole('menuitem',{name:labels.fileMenuSidebarBrowser,exact:true}).click();await page.locator('.chat-browser:visible').waitFor();
  await page.waitForFunction(()=>window.chatExperienceCalls.filter(call=>call.method==='browser.native.create').length===1);
  await html.click({button:'right'});await page.getByRole('menuitem',{name:labels.fileMenuSource,exact:true}).click();await page.locator('.chat-side-panel .cm-editor:visible').waitFor();
  assert(await page.evaluate(()=>window.chatExperienceCalls.some(call=>call.method==='file.preview'&&call.params.path.endsWith('page.html'))));
  const folder=page.locator('.chat-transcript-viewport').getByRole('link',{name:'folder',exact:true});await folder.click();await page.locator('#chat-panel-library:visible .rsb-fbody-tree-pane').waitFor();
  await page.waitForFunction(()=>window.chatExperienceCalls.some(call=>call.method==='files.list'&&/docs[\\/]?$/.test(call.params.path)));
  await page.locator('.chat-side-panel').getByRole('button',{name:labels.hideDetails,exact:true}).click();
  await page.getByRole('button',{name:'Alpha',exact:true}).click();
  await page.reload({waitUntil:'domcontentloaded'});await source.waitFor();
  assert.equal(await page.getByRole('button',{name:'Alpha',exact:true}).getAttribute('aria-expanded'),'false','Project collapse was lost on remount');
  await page.getByRole('button',{name:labels.hideSidebar,exact:true}).click();await page.setViewportSize({width:360,height:900});await settle();
  await source.click({button:'right'});await page.getByRole('menu').waitFor();await noHorizontal('[role=menu]',id+'-narrow-menu');
  const menu=await page.getByRole('menu').boundingBox();assert(menu.x>=0&&menu.x+menu.width<=360.5,'File menu escaped the narrow viewport');
  await page.mouse.move(0,0);await page.screenshot({path:join(output,`${id}-menu.png`)});await page.keyboard.press('Escape');
  await noHorizontal('.chat-main',id+'-narrow-chat');assert.deepEqual(errors,[]);report.push({id,menu,animation,nativeSideEffects:'mocked',errors});await page.close();
 }
 await writeFile(join(output,'report.json'),JSON.stringify(report,null,2));
 console.log('Chat resilience UI: 8 browser cases passed (themes, locales, resize, file menus, missing models, collapse, activity, Markdown/editor safeguards, narrow layouts).');
}catch(error){
 if(page&&!page.isClosed()){await page.screenshot({path:join(output,'failure.png')});await writeFile(join(output,'failure-dom.txt'),await page.locator('body').innerText());}
 await writeFile(join(output,'failure.json'),JSON.stringify({error:String(error),report},null,2));throw error;
}finally{await browser.close();await vite.close();await new Promise(done=>files.close(done));}
