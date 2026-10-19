// Real production Chat UI with isolated Host/file fixtures. Exercises the
// user's regression paths, not source-string assertions or the live profile.
import {createRequire} from 'node:module';
import {mkdir,readFile,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {pathToFileURL} from 'node:url';
import {createServer as httpServer} from 'node:http';
import {execFileSync} from 'node:child_process';
import assert from 'node:assert/strict';
const root=resolve('.'),frontend=join(root,'apps/desktop/frontend'),output=resolve(process.env.WINONE_LAYOUT_OUTPUT||'_temp/claude-recording/chat-ui-check');
execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);await mkdir(join(output,'browser-temp'),{recursive:true});process.env.TEMP=process.env.TMP=join(output,'browser-temp');
const source='# Project notes\n\nA readable, editable preview.\n\n'+('Document paragraph.\n\n'.repeat(25));
const files=httpServer((req,res)=>{
 res.setHeader('Access-Control-Allow-Origin','*');res.setHeader('Content-Type','application/json');
 const url=new URL(req.url,'http://fixture'),directory=url.searchParams.get('path')||'C:/fixture/Alpha';
 if(url.pathname==='/files.list.json')res.end(JSON.stringify({directory,entries:[{name:'README.md',path:directory+'/README.md',directory:false,size:200},{name:'docs',path:directory+'/docs',directory:true,size:0}]}));
 else if(url.pathname==='/files.index.json')res.end(JSON.stringify({files:['README.md','src/example.ts','docs/guide.md'],truncated:false}));
 else if(url.pathname==='/files.search.json')res.end(JSON.stringify({results:[{relPath:'README.md',matches:[{lineNumber:3,lineText:'A readable preview',submatches:[{start:2,end:10}]}]}],totalMatches:1,totalFiles:1,truncated:false}));
 else {res.setHeader('Content-Type','text/plain; charset=utf-8');res.end(source);}
});await new Promise(done=>files.listen(0,'127.0.0.1',done));const origin=`http://127.0.0.1:${files.address().port}`;
const {createServer}=await import(pathToFileURL(join(frontend,'node_modules/vite/dist/node/index.js')));
const server=await createServer({root:frontend,configFile:join(frontend,'vite.config.ts'),server:{host:'127.0.0.1',port:5219,strictPort:true}});await server.listen();
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE||join(root,'dist/resources/tools/playwright-1.58.2/node_modules/playwright'));
const browser=await chromium.launch({headless:true,channel:'msedge'}),results=[],errors=[];let page;
const settle=async()=>page.evaluate(async()=>{await document.fonts.ready;await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);});
async function open(theme='light',width=1280,extra='alignment&repair',height=1000){
 await page?.close();page=await browser.newPage({viewport:{width,height},reducedMotion:'reduce'});page.setDefaultTimeout(15000);page.on('pageerror',error=>errors.push(error.message));
 await page.goto(`http://127.0.0.1:5219/tests/chat-experience-preview.html?${extra}&theme=${theme}&language=en-US&previewOrigin=${encodeURIComponent(origin)}`);
 await page.getByRole('textbox',{name:'Message',exact:true}).waitFor();await settle();
}
try{
 for(const theme of ['light','dark'])for(const width of [1280,800]){
  await open(theme,width);
  await page.locator('.chat-generated-files').getByRole('button',{name:'README.md',exact:true}).waitFor();
  const initial=await page.evaluate(()=>{
   const main=document.querySelector('.chat-main'),composer=document.querySelector('.chat-composer'),context=document.querySelector('.chat-project-context'),stack=document.querySelector('.chat-composer-stack'),handle=document.querySelector('.chat-composer-resize'),input=document.querySelector('.chat-composer textarea');
   const props=el=>{const s=getComputedStyle(el);return {background:s.backgroundColor,color:s.color,border:s.borderTopColor};};
   return {main:props(main),input:props(composer),handle:props(handle),placeholder:getComputedStyle(input,'::placeholder').color,inputColor:getComputedStyle(input).color,contextHeight:context.getBoundingClientRect().height,backplate:{background:getComputedStyle(stack,'::before').backgroundColor,left:getComputedStyle(stack,'::before').left,right:getComputedStyle(stack,'::before').right,bottom:getComputedStyle(stack,'::before').bottom},invalidTokens:[...document.querySelectorAll('.cindy-chat-ui')].flatMap(el=>['--background','--foreground','--border','--muted-foreground'].filter(name=>!CSS.supports('color',getComputedStyle(el).getPropertyValue(name))).map(name=>name)),requests:window.chatExperienceCalls.filter(call=>call.method==='file.stat')};
  });
  assert.equal(initial.main.background,theme==='light'?'rgb(252, 252, 252)':'rgb(25, 25, 25)');assert.deepEqual(initial.invalidTokens,[]);assert.notEqual(initial.placeholder,initial.inputColor);
  assert.equal(initial.backplate.left,'-3px');assert.equal(initial.backplate.right,'-3px');assert.equal(initial.backplate.bottom,'-3px');assert.equal(initial.contextHeight,38);
  assert.ok(initial.requests.every(call=>call.params.paths.every(path=>!path.endsWith('userId')&&!path.endsWith('getValue()'))));
  const resize=page.locator('.chat-workspace > [data-group] > [data-slot="resizable-handle"]');
  await resize.focus();for(let i=0;i<32;i++)await page.keyboard.press('ArrowLeft');await settle();
  assert.ok((await page.locator('.chat-sidebar').boundingBox()).width>=223,'Dragging the left sidebar collapsed it');
  const clippedTools=await page.locator('.chat-composer-toolbar').evaluate(toolbar=>{
   const bounds=toolbar.getBoundingClientRect();
   return [...toolbar.querySelectorAll('button')].filter(button=>{
    const rect=button.getBoundingClientRect();
    return rect.width&&rect.height&&(rect.left<bounds.left-1||rect.right>bounds.right+1||rect.top<bounds.top-1||rect.bottom>bounds.bottom+1);
   }).map(button=>button.getAttribute('aria-label')||button.textContent);
  });
  assert.deepEqual(clippedTools,[],'Narrow composer clips a toolbar action');
  const nav=page.locator('.chat-scroll-controls'),track=page.locator('.chat-transcript .lobe-scroll-scrollbar[data-orientation="vertical"]');
   const rail=await nav.boundingBox(),reading=await page.locator('.chat-reading-pane').boundingBox(),composerBox=await page.locator('.chat-input-area').boundingBox(),body=await page.locator('.chat-conversation-body').boundingBox();
   assert.ok(Math.abs(reading.width-body.width)<=1,'Navigation still consumes a separate column');assert.ok(rail.y+rail.height<=composerBox.y+1,'Navigation overlaps the input area');
   if(await track.count()){const b=await track.boundingBox();if(b&&b.height>0)assert.ok(rail.x+rail.width+2<=b.x,'Navigation covers the scrollbar');}
  const chip=page.locator('.chat-generated-files').getByRole('button',{name:'README.md',exact:true});await chip.scrollIntoViewIfNeeded();await chip.click();
  const panel=page.locator('.chat-side-panel');await panel.locator('.chat-file-preview').waitFor();await panel.locator('.cm-editor').waitFor();
  const preview=await panel.locator('.chat-file-preview').evaluate(el=>({background:getComputedStyle(el).backgroundColor,foreground:getComputedStyle(el).color}));assert.notEqual(preview.background,'rgba(0, 0, 0, 0)');
  const add=panel.getByRole('button',{name:'Add tab',exact:true});await add.hover();
  const tooltip=page.getByRole('tooltip');await tooltip.waitFor();
  const tip=await tooltip.evaluate(el=>{const popup=el.closest('.cindy-panel-popup')||el;return {background:getComputedStyle(popup).backgroundColor,color:getComputedStyle(popup).color};});assert.notEqual(tip.color,tip.background);assert.equal(tip.color,'rgb(255, 255, 255)');
  await page.mouse.move(0,0);await panel.getByRole('tab',{name:'Files & resources',exact:true}).click();await panel.getByRole('tab',{name:'Resources',exact:true}).click();
  await panel.getByText('Review project',{exact:true}).waitFor();assert.equal(await panel.getByRole('alert').count(),0);
  assert.equal(await panel.getByRole('button',{name:'Add tab',exact:true}).isVisible(),false,'Library must not create tabs');
  await panel.getByRole('tab',{name:'Files',exact:true}).click();
  await panel.getByText('Select a file to preview',{exact:true}).waitFor();
  const filter=panel.getByPlaceholder('Filter files…');await filter.fill('README');await panel.getByText('README.md',{exact:true}).last().waitFor();
  assert.equal(await panel.getByRole('alert').count(),0);
  await panel.getByRole('tab',{name:'Tabs',exact:true}).click();await add.click();await page.getByRole('menuitem',{name:/^Review(?:\s|$)/}).click();
  await panel.locator('[data-cindy-review] header').waitFor();await settle();
  const review=await panel.locator('[data-cindy-review] header').evaluate(el=>{const notice=el.querySelector('div.mt-2'),bounds=notice?.getBoundingClientRect();return {width:el.clientWidth,scrollWidth:el.scrollWidth,overlap:bounds?[...el.querySelectorAll('button')].some(button=>button.getBoundingClientRect().height>0&&button.getBoundingClientRect().bottom>bounds.top+1):false};});
  assert.ok(review.scrollWidth<=review.width+1,'Review toolbar overflows');assert.equal(review.overlap,false,'Commit/push overlaps the read-only notice');
  await page.mouse.move(0,0);await page.screenshot({path:join(output,`chat-${theme}-${width}.png`)});results.push({theme,width,initial,preview,tip,review});
 }
 await open('light',1280,'alignment&repair&running=true');
 const header=page.locator('[data-slot="cindy-work"]').last();
 const group=header.locator('..');await group.locator('[data-live-work-preview]').waitFor();
 assert.equal(await group.locator('[data-live-work-preview] [data-live-work-activity="tool"]').count(),5);
 await header.click();await group.locator('.process-body').waitFor();assert.ok(await group.locator('.process-body [data-live-work-activity="tool"]').count()>5);
 await header.click();await group.locator('[data-live-work-preview]').waitFor();assert.equal(await group.locator('[data-live-work-preview] [data-live-work-activity="tool"]').count(),5);
 results.push({scenario:'Cindy running latest-five / expand / fold'});
 // Check the actual nested Markdown root; parent-only gray tokens used to be
 // reset by its cindy-chat-ui class, leaving PowerShell payloads near black.
 for(const theme of ['light','dark']){
  await open(theme,1280,'repair&running=true');const work=page.locator('[data-slot=cindy-work]').last();await work.click();
  const row=page.getByRole('button',{name:'powershell Write-Output repair-7',exact:true});await row.click();
  const payload=row.locator('..').locator('.chat-tool-payload');await payload.waitFor();
  await payload.evaluate(node=>{const v=node.closest('.chat-transcript-viewport');v.scrollTop+=node.getBoundingClientRect().top-v.getBoundingClientRect().top-64;});await settle();
  const palette=await payload.evaluate(node=>{const text=node.querySelector('.msg-markdown'),main=document.querySelector('.chat-main'),sample=document.createElement('span');sample.style.color='var(--muted-foreground)';node.append(sample);const value={text:getComputedStyle(text).color,muted:getComputedStyle(sample).color,body:getComputedStyle(main).color};sample.remove();return value;});assert.equal(palette.text,palette.muted);assert.notEqual(palette.text,palette.body);
  await page.mouse.move(0,0);const before=await row.evaluate(node=>({background:getComputedStyle(node).backgroundColor,opacity:getComputedStyle(node).opacity,transform:getComputedStyle(node).transform}));await row.hover();const after=await row.evaluate(node=>({background:getComputedStyle(node).backgroundColor,opacity:getComputedStyle(node).opacity,transform:getComputedStyle(node).transform}));assert.deepEqual(after,before);
  await page.mouse.move(0,0);await page.screenshot({path:join(output,`work-payload-${theme}.png`)});results.push({scenario:'muted PowerShell / static hover',theme,palette});
 }
 await open('dark',1280,'repair');
 const first=page.locator('[data-slot="cindy-work"]').first();await first.click();
 const firstGroup=first.locator('..');await firstGroup.locator('.process-body').waitFor();
 // A short expanded group fits the viewport. Folding it must keep the reader's
 // position; only a genuinely long group is allowed to navigate to its header.
 assert.equal(await firstGroup.locator('[data-process-long="true"]').count(),0);
 const transcript=page.locator('.chat-transcript-viewport');
 await transcript.evaluate(el=>{el.scrollTop=60;});await settle();
 const shortBefore=await transcript.evaluate(el=>el.scrollTop);
 await first.click();await firstGroup.locator('.process-body').waitFor({state:'hidden'});await settle();
 const shortAfter=await transcript.evaluate(el=>el.scrollTop);
 assert.ok(Math.abs(shortAfter-shortBefore)<1,'Short process collapse changed the reading position');
 results.push({scenario:'short process collapse keeps reading position',shortBefore,shortAfter});
 await first.click();await firstGroup.locator('[data-live-work-activity="thinking"]').click();
 await firstGroup.locator('[data-process-long="true"]').waitFor();
 const collapse=page.locator('.process-collapse-action');await collapse.waitFor();
 await page.locator('.chat-transcript-viewport').evaluate(el=>{el.scrollTop=900;});await settle();
 const placement=await collapse.evaluate(button=>{const process=document.querySelector('.process-body[data-process-long=true]'),body=process.getBoundingClientRect(),content=process.querySelector('.process-body-content').getBoundingClientRect(),r=button.getBoundingClientRect(),footer=document.querySelector('.chat-input-area').getBoundingClientRect();return {rightInset:body.right-r.right,clearance:r.left-content.right,bottom:r.bottom,footerTop:footer.top,border:getComputedStyle(button).borderTopWidth};});
 assert.ok(placement.rightInset>=0&&placement.rightInset<=4);assert.ok(placement.clearance>=2);assert.ok(placement.bottom<placement.footerTop);assert.equal(placement.border,'1px');
 await page.mouse.move(0,0);await page.screenshot({path:join(output,'process-collapse-edge.png')});
 await page.locator('.process-body[data-process-long=true]').first().evaluate(node=>{const viewport=node.closest('.chat-transcript-viewport'),footer=document.querySelector('.chat-input-area');viewport.scrollTop+=node.getBoundingClientRect().bottom-footer.getBoundingClientRect().top+8;});await settle();await collapse.waitFor({state:'hidden'});
 await page.locator('.chat-transcript-viewport').evaluate(el=>{el.scrollTop=900;});await collapse.waitFor();await collapse.click();await settle();
 const anchor=await first.evaluate(el=>{const viewport=el.closest('.chat-transcript-viewport');return el.getBoundingClientRect().top-viewport.getBoundingClientRect().top;});assert.ok(Math.abs(anchor-8)<3,'Long collapse did not restore its heading');
 results.push({scenario:'long process collapse navigation',anchor});
 await open('light',1280,'liveThinking&running=true');
 const liveThought=page.locator('[data-live-work-activity=thinking]');await liveThought.click();await page.locator('[data-process-long=true]').waitFor();
 await page.locator('.chat-transcript-viewport').evaluate(view=>{const body=view.querySelector('[data-process-long=true]');view.scrollTop+=body.getBoundingClientRect().top-view.getBoundingClientRect().top+240;});
 await page.locator('.process-collapse-action').waitFor();await page.mouse.move(0,0);await page.screenshot({path:join(output,'live-thinking-collapse.png')});await page.locator('.process-collapse-action').click();await page.locator('.process-collapse-action').waitFor({state:'hidden'});
 assert.equal(await liveThought.getAttribute('aria-expanded'),'false');results.push({scenario:'live thinking preview has the same long-content collapse action'});
 for(const theme of ['light','dark'])for(const [width,height] of [[1280,940],[800,940],[800,600]]){
  await open(theme,width,'alignment&repair&audit&running=true',height);
  await page.getByRole('textbox',{name:'Message',exact:true}).fill('Draft with every send control visible');
  await page.getByRole('button',{name:'Show chat details',exact:true}).click();
  const panel=page.locator('.chat-side-panel');await panel.getByRole('tab',{name:'Files & resources',exact:true}).click();
  await panel.getByRole('tab',{name:'Files',exact:true}).waitFor();
  const major=await panel.locator('.chat-panel-major-tabs').boundingBox();assert.ok(major.height>=34&&major.height<=36,'Primary sections need the bordered tab geometry');
  const tabs=await panel.evaluate(panel=>{const major=panel.querySelector('.chat-panel-major-tabs'),minor=panel.querySelector('.chat-library-tabs'),sample=el=>({width:el.getBoundingClientRect().width,radius:parseFloat(getComputedStyle(el).borderTopLeftRadius),border:getComputedStyle(el).borderTopWidth,tabs:[...el.querySelectorAll('[role=tab]')].map(tab=>({width:tab.getBoundingClientRect().width,align:getComputedStyle(tab).textAlign}))});return {panelWidth:panel.getBoundingClientRect().width,major:sample(major),minor:sample(minor)};});
  for(const level of [tabs.major,tabs.minor]){assert.ok(Math.abs(level.width-(tabs.panelWidth-16))<=1);assert.ok(Math.abs(level.tabs[0].width-level.tabs[1].width)<=1);assert.ok(level.tabs.every(tab=>tab.align==='center'));assert.equal(level.border,'1px');}assert.ok(tabs.major.radius<=4);assert.ok(tabs.minor.radius>10);
  const viewport=page.locator('.chat-transcript-viewport');await viewport.evaluate(node=>{node.scrollTop=100;});await settle();
  const geometry=()=>page.evaluate(()=>{
   const rect=selector=>{const r=document.querySelector(selector).getBoundingClientRect();return {x:r.x,y:r.y,right:r.right,bottom:r.bottom,height:r.height};};
   const toolbar=document.querySelector('.chat-composer-toolbar'),bounds=toolbar.getBoundingClientRect();
   const clipped=[...toolbar.querySelectorAll('button')].filter(button=>{const r=button.getBoundingClientRect();return r.width&&r.height&&(r.left<bounds.left-1||r.right>bounds.right+1||r.top<bounds.top-1||r.bottom>bounds.bottom+1);}).map(button=>button.getAttribute('aria-label')||button.textContent);
   const input=document.querySelector('.chat-composer'),stack=document.querySelector('.chat-composer-stack'),v=document.querySelector('.chat-transcript-viewport');
   const nav=document.querySelector('.chat-scroll-controls'),navBox=nav.getBoundingClientRect(),overlap=r=>r.width>0&&r.height>0&&r.left<navBox.right-1&&r.right>navBox.left+1&&r.top<navBox.bottom-1&&r.bottom>navBox.top+1;
   const visibleBox=node=>{const box=node.getBoundingClientRect();if(!node.closest('.client-thread'))return box;const view=v.getBoundingClientRect();return {width:box.width,height:Math.max(0,Math.min(box.bottom,view.bottom)-Math.max(box.top,view.top)),left:Math.max(box.left,view.left),right:Math.min(box.right,view.right),top:Math.max(box.top,view.top),bottom:Math.min(box.bottom,view.bottom)};};
   const collisions=[...document.querySelectorAll('.client-thread :is(button,a,p,pre,h1,h2,h3),.chat-input-area,.chat-header-title,.chat-header>button')].filter(node=>!nav.contains(node)&&overlap(visibleBox(node))).map(node=>node.className||node.tagName);
   return {rail:rect('.chat-scroll-controls'),reading:rect('.chat-reading-pane'),body:rect('.chat-conversation-body'),footer:rect('.chat-input-area'),viewport:rect('.chat-transcript-viewport'),scrollTop:v.scrollTop,collisions,clipped,radius:parseFloat(getComputedStyle(input).borderBottomRightRadius),outerRadius:parseFloat(getComputedStyle(stack,'::before').borderBottomRightRadius),horizontal:document.documentElement.scrollWidth-innerWidth};
  });
  const before=await geometry();assert.deepEqual(before.clipped,[],'Narrow composer clipped a send/model/tool button');assert.ok(before.horizontal<=1);
  assert.equal(before.outerRadius,before.radius+3,'Backing and editor must share concentric bottom corners');
  const grip=page.getByRole('separator',{name:'Resize message input'});await grip.focus();for(let i=0;i<12;i++)await grip.press('ArrowUp');await settle();
  const after=await geometry();assert.ok(Math.abs(after.reading.right-after.body.right)<=1,'Navigation still reserves a separate column');assert.deepEqual(after.collisions,[],'Navigation overlaps a message or component');
  assert.deepEqual(after.clipped,[]);assert.equal(after.viewport.height,before.viewport.height);assert.ok(Math.abs(after.scrollTop-before.scrollTop)<1,'Resizing the input moved the transcript');
  await page.evaluate(()=>document.activeElement?.blur());await page.mouse.move(0,0);await page.getByRole('tooltip').waitFor({state:'hidden'});await settle();
  await page.screenshot({path:join(output,`send-controls-${theme}-${width}-${height}.png`)});results.push({scenario:'all send controls / tall composer / fixed library / independent navigation',theme,width,height,before,after});
 }
 assert.deepEqual(errors,[]);
 console.log(JSON.stringify({scenarios:results.length,output,errors}));
}catch(error){errors.push(String(error));await page?.screenshot({path:join(output,'failure.png')});throw error;}
finally{await writeFile(join(output,'report.json'),JSON.stringify({results,errors},null,2));await browser.close();await server.close();files.closeAllConnections();await new Promise(done=>files.close(done));}
