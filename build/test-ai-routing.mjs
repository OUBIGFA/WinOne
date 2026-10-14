import {createRequire} from 'node:module';
import {spawn} from 'node:child_process';
import {mkdir,readFile,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import http from 'node:http';
import {randomBytes} from 'node:crypto';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const root=resolve(process.env.WINONE_LAYOUT_OUTPUT||'_temp/20261011-thread-takeover/routing-layout');await mkdir(root,{recursive:true});
const profile=join(root,'profile');await mkdir(profile,{recursive:true});
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
  const layout=await content.evaluate(()=>{const view=document.getElementById('view-routing'),cards=[...document.querySelectorAll('.rt-gtpl-card')],box=document.querySelector('.rt-gtpl-cards').getBoundingClientRect();const bounds=cards.map(card=>card.getBoundingClientRect());return {upstreamTemplates:cards.every(node=>getComputedStyle(node).borderRadius==='10px'&&getComputedStyle(node).paddingLeft==='12px')&&getComputedStyle(document.querySelector('.rt-gtpl-cards')).display==='grid',height:view.getBoundingClientRect().height,scrollHeight:document.documentElement.scrollHeight,viewport:innerHeight,overflow:document.documentElement.scrollWidth-innerWidth,cards:cards.length,centerOffset:(Math.min(...bounds.map(r=>r.left))+Math.max(...bounds.map(r=>r.right)))/2-(box.left+box.right)/2,scrolling:[...view.querySelectorAll('*')].filter(e=>e.clientHeight>0&&e.scrollHeight>e.clientHeight+2&&/auto|scroll/.test(getComputedStyle(e).overflowY)).map(e=>e.className)};});
  if(!layout.upstreamTemplates||layout.cards!==3||layout.overflow>1||Math.abs(layout.height-layout.viewport)>2||layout.scrolling.length)throw Error('Routing layout failed: '+JSON.stringify({language,theme,width,...layout}));
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
 await writeFile(join(root,'report.json'),JSON.stringify({editor,layouts,adapterOverride:!!process.env.WINONE_ROUTING_ADAPTER_FILE,browserErrors:errors},null,2));console.log(JSON.stringify({editor,layouts:layouts.length,browserErrors:0}));
}finally{await browser.close();child.stdin.end();parentServer.closeAllConnections();await new Promise(done=>parentServer.close(done));}

