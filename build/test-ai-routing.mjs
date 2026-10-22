import {createRequire} from 'node:module';
import {spawn} from 'node:child_process';
import {mkdir,readFile,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import http from 'node:http';
import {randomBytes} from 'node:crypto';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const root=resolve(process.env.WINONE_LAYOUT_OUTPUT||'_temp/20261011-thread-takeover/routing-layout');await mkdir(root,{recursive:true});
const profile=join(root,'profile-'+randomBytes(6).toString('hex'));await mkdir(profile,{recursive:true});
const child=spawn(process.env.WINONE_ROUTING_TEST_EXE,[],{windowsHide:true,env:{...process.env,USERPROFILE:profile,APPDATA:profile,LOCALAPPDATA:profile,XDG_CONFIG_HOME:profile,XDG_CACHE_HOME:profile},stdio:['pipe','pipe','pipe']});
const token=randomBytes(32).toString('hex');
child.stdin.write(JSON.stringify({operation:'routing.serve',key:token,magpieSettings:{lang:'zh-CN',theme:'light',noStats:true,noUsageStats:true},auxiliaryProviders:[{id:'fixture',name:'Fixture AI',chat:'http://127.0.0.1:1/v1',key:'fixture-key',models:['model']}]})+'\n');
const address=await new Promise((resolve,reject)=>{let raw='';child.stdout.on('data',chunk=>{raw+=chunk;const end=raw.indexOf('\n');if(end>=0){const value=JSON.parse(raw.slice(0,end));if(value.url)resolve(value.url);else reject(Error(JSON.stringify(value)));}});child.on('exit',code=>reject(Error('Router exited '+code)));});
const browser=await chromium.launch({headless:true,channel:'msedge'}),errors=[];
// Different sites exercise the same third-party frame restriction as the
// desktop's winone.localhost parent and its private 127.0.0.1 routing child.
const parentServer=http.createServer((request,response)=>{const query=new URL(request.url,'http://localhost').searchParams,language=query.get('language')||'zh-CN',theme=query.get('theme')||'light';const source=new URL(address);source.searchParams.set('view','routing');source.searchParams.set('key',token);source.searchParams.set('locale',language.startsWith('zh')?'zh':'en');response.setHeader('Content-Type','text/html');response.end(`<style>body{margin:0;padding:20px;box-sizing:border-box}iframe{display:block;width:100%;height:600px;border:0}</style><iframe src="${source.href}" onload="this.contentWindow.postMessage({type:'winone-routing-theme',language:'${language}',theme:'${theme}'},'${source.origin}')"></iframe><script>addEventListener('message',event=>{const frame=document.querySelector('iframe');if(event.source===frame.contentWindow&&event.origin==='${source.origin}'&&event.data?.type==='winone-routing-height')frame.style.height=event.data.height+'px';});</script>`);});
await new Promise(done=>parentServer.listen(0,'127.0.0.1',done));
try{
 const page=await browser.newPage({viewport:{width:1280,height:820},reducedMotion:'reduce'});page.on('pageerror',error=>errors.push(error.message));
 // During adapter development an older isolated binary can exercise the exact
 // new host script. Final bundle verification omits this override.
 if(process.env.WINONE_ROUTING_ADAPTER_FILE){const adapter=await readFile(process.env.WINONE_ROUTING_ADAPTER_FILE,'utf8');await page.route('**/winone-routing.js',route=>route.fulfill({contentType:'text/javascript',body:adapter}));}
 const layouts=[];
 for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark'])for(const width of [520,1280]){
  await page.setViewportSize({width,height:820});await page.goto(`http://localhost:${parentServer.address().port}/?language=${language}&theme=${theme}`);
  const embedded=page.frameLocator('iframe');await embedded.locator('#rtNewGroup').waitFor({timeout:8000});await embedded.locator('.rt-gtpl-card').first().waitFor();
  await page.waitForTimeout(150);
  const content=page.frames().find(frame=>frame.url().startsWith(address));
  await content.evaluate(async()=>{await document.fonts.ready;await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);});
  const clippedHeaders=await content.evaluate(()=>[...document.querySelectorAll('#view-routing .row-head')].filter(row=>row.getClientRects().length).filter(row=>{const box=row.getBoundingClientRect();return [...row.children].some(child=>child.getClientRects().length&&child.getBoundingClientRect().bottom>box.bottom+1);}).map(row=>row.textContent));
  if(clippedHeaders.length)throw Error('Routing header overlaps the following controls: '+JSON.stringify({language,theme,width,clippedHeaders}));
  const headerGaps=await content.evaluate(()=>[...document.querySelectorAll('#view-routing .row-head')].filter(row=>row.getClientRects().length).map(row=>({gap:parseFloat(getComputedStyle(row).columnGap),label:row.textContent})));
  if(headerGaps.some(row=>row.gap<8))throw Error('Routing header controls touch: '+JSON.stringify(headerGaps));
  const boundary=await content.evaluate(()=>({pages:window.magpie?.pages(),foreign:document.querySelectorAll('#side,header.top,footer.foot,#view-agents,#view-providers,#view-settings,#view-usage,#view-library,#view-sessions').length,scripts:[...document.scripts].map(script=>script.getAttribute('src')).filter(Boolean)}));
  if(JSON.stringify(boundary.pages)!==JSON.stringify(['routing'])||boundary.foreign||boundary.scripts.some(src=>/^(app|start|plugins)\.js$|plugins\/(usage|library|sessions|gateway)\/client\.js/.test(src)))throw Error('Routing must mount only its own upstream plugin: '+JSON.stringify(boundary));
  const layout=await content.evaluate(()=>{const view=document.getElementById('view-routing'),cards=[...document.querySelectorAll('.rt-gtpl-card')],box=document.querySelector('.rt-gtpl-cards').getBoundingClientRect();const bounds=cards.map(card=>card.getBoundingClientRect());return {flatTemplates:cards.every((node,index)=>getComputedStyle(node).borderRadius==='0px'&&(!index||bounds[index].top>=bounds[index-1].bottom-1)),readableTemplates:cards.every(node=>[...node.querySelectorAll('.desc,.mem')].every(text=>parseFloat(getComputedStyle(text).fontSize)>=14)),requestFirst:!!(view.querySelector('.rt').compareDocumentPosition(view.querySelector('.rt-gsec'))&Node.DOCUMENT_POSITION_FOLLOWING),height:view.getBoundingClientRect().height,scrollHeight:document.documentElement.scrollHeight,viewport:innerHeight,overflow:document.documentElement.scrollWidth-innerWidth,cards:cards.length,centerOffset:(Math.min(...bounds.map(r=>r.left))+Math.max(...bounds.map(r=>r.right)))/2-(box.left+box.right)/2,scrolling:[...view.querySelectorAll('*')].filter(e=>e.clientHeight>0&&e.scrollHeight>e.clientHeight+2&&/auto|scroll/.test(getComputedStyle(e).overflowY)).map(e=>e.className)};});
  if(!layout.flatTemplates||!layout.readableTemplates||!layout.requestFirst||layout.cards!==3||layout.overflow>1||Math.abs(layout.height-layout.viewport)>2||layout.scrolling.length)throw Error('Routing layout failed: '+JSON.stringify({language,theme,width,...layout}));
  layouts.push({language,theme,width,...layout});await page.screenshot({path:join(root,`routing-${language}-${theme}-${width}.png`),fullPage:true});
 }
 await page.goto(`http://localhost:${parentServer.address().port}`);const embedded=page.frameLocator('iframe');await embedded.locator('#rtNewGroup').waitFor({timeout:8000});await embedded.locator('#rtNewGroup').click();
 await page.waitForTimeout(400);await page.screenshot({path:join(root,'routing-editor.png')});
 const childFrame=page.frames().find(frame=>frame.url().startsWith(address));
 console.log(JSON.stringify(await childFrame.evaluate(()=>({boot:window.bootPrefs,language:document.documentElement.lang,locale:typeof locale==='undefined'?'missing':locale,heading:document.querySelector('.rt-gedit')?.innerText.slice(0,160)}))));
 // The embedded frame must be able to save through the same authenticated
 // origin; a cookie-less third-party frame would fail here with 401.
 const saved=await childFrame.evaluate(async()=>{const response=await fetch('/api/groups/save',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:'embedded-check',name:'Embedded check',members:['fixture/model'],routing:'order'})});return response.status;});
 if(saved!==200)throw Error('Embedded routing write failed with '+saved);
 if(errors.length)throw Error(errors.join('\n'));
 const editor=await embedded.locator('.rt-gedit').count();if(!editor)throw Error('Full routing group editor did not open');
 if(!await embedded.getByText('新建组',{exact:true}).count())throw Error('Chinese routing labels were not applied');
 // Exercise the real upstream picker and save buttons; API-only writes cannot
 // prove that the extracted shared widgets still work in their own document.
 await embedded.locator('.rt-gedit input').first().fill('UI routing check');
 await embedded.locator('.rt-gedit').getByRole('button',{name:'添加模型',exact:true}).click();
 await embedded.locator('#q').fill('model');
 await embedded.locator('#list li[data-i]').filter({hasText:'model'}).first().click();
 await embedded.locator('#q').press('Escape');
 await embedded.locator('.rt-gedit').getByRole('button',{name:'添加',exact:true}).click();
 await embedded.locator('.rt-gedit').waitFor({state:'hidden'});
 const persisted=await childFrame.evaluate(async()=>{const value=await(await fetch('/api/groups')).json();return value.groups.find(group=>group.name==='UI routing check');});
 if(!persisted?.members.includes('fixture/model'))throw Error('Model picker/save lost the group member: '+JSON.stringify(persisted));
 await embedded.locator('.suffix-segs').getByRole('button',{name:'关闭',exact:true}).click();
 const plain=await childFrame.evaluate(async()=>{const deadline=Date.now()+3000;while(Date.now()<deadline){const s=await(await fetch('/api/settings')).json();if(s.plainNames)return true;await new Promise(done=>setTimeout(done,30));}return false;});
 if(!plain)throw Error('Routing model-name setting did not take effect');
 await embedded.locator('#rtMask').click();
 if(await embedded.locator('#rtMask').getAttribute('aria-pressed')!=='true')throw Error('Routing account masking did not toggle');
 await embedded.locator('#rtNewGroup').click();
 await embedded.locator('.rt-gedit input').first().fill('Unsaved routing draft');
 await embedded.locator('.rt-gedit').getByRole('button',{name:'取消',exact:true}).click();
 await embedded.locator('.action-confirm').waitFor();
 await embedded.locator('.action-confirm').getByRole('button',{name:'取消',exact:true}).click();
 if(await embedded.locator('.rt-gedit input').first().inputValue()!=='Unsaved routing draft')throw Error('Canceling discard lost the routing draft');
 await embedded.locator('.rt-gedit').getByRole('button',{name:'取消',exact:true}).click();
 await embedded.locator('.action-confirm .primary').click();
 await embedded.locator('.rt-gedit').waitFor({state:'hidden'});
 // Populated groups and an open editor use different flex/grid paths from
 // the empty templates. Check both; empty-state screenshots missed collisions.
 const populated=[];
 for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark'])for(const width of [520,1280]){
  await page.setViewportSize({width,height:820});await page.goto(`http://localhost:${parentServer.address().port}/?language=${language}&theme=${theme}`);
  await embedded.locator('.rt-group').first().waitFor();await embedded.locator('#rtNewGroup').click();
  const frame=page.frames().find(frame=>frame.url().startsWith(address));
  await frame.evaluate(async()=>{await document.fonts.ready;await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);});
  const metrics=await frame.evaluate(()=>{
   const visible=node=>node.getClientRects().length&&getComputedStyle(node).visibility!=='hidden';
   const collisions=[...document.querySelectorAll('.rt-group,.rt-pool,.rt-gedit .fbrow,.rt-gsec .row-head')].filter(visible).flatMap(row=>{
    const children=[...row.children].filter(visible).map(node=>({name:node.className,box:node.getBoundingClientRect()}));
    return children.flatMap((a,i)=>children.slice(i+1).filter(b=>Math.min(a.box.right,b.box.right)-Math.max(a.box.left,b.box.left)>1&&Math.min(a.box.bottom,b.box.bottom)-Math.max(a.box.top,b.box.top)>1).map(b=>({row:row.className,first:a.name,second:b.name})));
   });
   const groups=[...document.querySelectorAll('.rt-group')].filter(visible).map(node=>node.getBoundingClientRect());
   const fields=[...document.querySelectorAll('.rt-gedit input,.rt-gedit .segs,.rt-gedit .fallback')].filter(visible);
   return {collisions,groups:groups.length,groupOverlap:groups.slice(1).some((box,i)=>box.top<groups[i].bottom-1),fields:fields.length,
    overflow:document.documentElement.scrollWidth-innerWidth,editorOverflow:document.querySelector('.rt-gedit').scrollWidth-document.querySelector('.rt-gedit').clientWidth};
  });
  if(metrics.collisions.length||metrics.groupOverlap||metrics.overflow>1||metrics.editorOverflow>1||metrics.groups<2||!metrics.fields)throw Error('Populated routing layout failed: '+JSON.stringify({language,theme,width,...metrics}));
  populated.push({language,theme,width,...metrics});await page.screenshot({path:join(root,`routing-groups-${language}-${theme}-${width}.png`),fullPage:true});
 }
 if(errors.length)throw Error(errors.join('\n'));
 await writeFile(join(root,'report.json'),JSON.stringify({editor,layouts,populated,adapterOverride:!!process.env.WINONE_ROUTING_ADAPTER_FILE,browserErrors:errors},null,2));console.log(JSON.stringify({editor,layouts:layouts.length,populated:populated.length,browserErrors:0}));
}finally{await browser.close();child.stdin.end();parentServer.closeAllConnections();await new Promise(done=>parentServer.close(done));}

