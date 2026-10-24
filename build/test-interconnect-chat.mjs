// Real browser/WebRTC sessions using an isolated local BitTorrent tracker.
// No fake chat transport, public signaling, or desktop configuration writes.
import {createRequire} from 'node:module';
import {resolve,join} from 'node:path';
import {pathToFileURL} from 'node:url';
import {mkdir,writeFile} from 'node:fs/promises';
import {randomUUID} from 'node:crypto';
import {execFileSync} from 'node:child_process';
import assert from 'node:assert/strict';
const root=resolve('.'),frontend=join(root,'apps/desktop/frontend');
const trackerPath=process.env.WINONE_TRACKER_MODULE;
if(!trackerPath)throw Error('Set WINONE_TRACKER_MODULE to an isolated bittorrent-tracker@11.2.3 package');
const {Server}=await import(pathToFileURL(resolve(trackerPath,'index.js')));
const tracker=new Server({udp:false,http:false,ws:true,stats:false});
await new Promise(resolve=>tracker.listen(0,'127.0.0.1',resolve));
const trackerURL='ws://127.0.0.1:'+tracker.ws.address().port;
// The production CSP deliberately rejects Vite's inline development styles.
// Exercise the shipped stylesheet files instead of bypassing that policy.
const {preview}=await import(pathToFileURL(join(frontend,'node_modules/vite/dist/node/index.js')));
const vite=await preview({root:frontend,configFile:join(frontend,'vite.config.ts'),preview:{host:'127.0.0.1',port:0}});
const base='http://127.0.0.1:'+vite.httpServer.address().port;
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE||join(root,'dist/resources/tools/playwright-1.58.2/node_modules/playwright'));
const output=resolve(process.env.WINONE_CHAT_OUTPUT||'_temp/interconnect-chat-check');
execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);
await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,channel:'msedge'}),errors=[];
const pages=[];
try{
 const room=randomUUID(),password=randomUUID();
 for(const name of ['Alice','Bob','Carol']){
  const context=await browser.newContext({viewport:{width:780,height:900},reducedMotion:'reduce'}),page=await context.newPage();pages.push(page);
  page.on('pageerror',e=>errors.push(e.message));
  await page.goto(base+'/interconnect-chat.html');
  await page.getByRole('tab',{name:'加入房间',exact:true}).click({timeout:5000});
  await page.getByRole('textbox',{name:'显示名称',exact:true}).fill(name);
  await page.getByRole('textbox',{name:'房间标识',exact:true}).fill(room);
  await page.getByRole('textbox',{name:'房间密码',exact:true}).fill(password);
  await page.getByText('网络设置',{exact:true}).click();
  await page.getByRole('textbox',{name:'信令服务（每行一个 WebSocket 地址）',exact:true}).fill(trackerURL);
  await page.getByRole('textbox',{name:'WebRTC 配置（JSON，可设置 STUN / TURN）',exact:true}).fill(JSON.stringify({iceServers:[]}));
  await page.getByRole('dialog').getByRole('button',{name:'关闭',exact:true}).last().click();
  await page.getByRole('dialog').waitFor({state:'hidden'});
  await page.getByRole('button',{name:'加入房间',exact:true}).click();
  await page.getByRole('button',{name:'离开房间',exact:true}).waitFor();
 }
 const [alice,bob,carol]=pages;
 for(const page of pages)await page.getByText('身份已校验',{exact:true}).nth(1).waitFor({timeout:30000});
 // A room must fill its viewport, and conversation rows must stay compact.
 // Legacy settings padding and nav grid stretching previously passed the
 // simple no-overflow check while wasting most of the conversation area.
 const workspace=await alice.locator('.flash-chat-workspace').boundingBox();
 assert.equal(workspace.x,0,'Room inherits the settings page gutter');
 assert.equal(workspace.y,0,'Room inherits the settings page top padding');
 const groupRow=await alice.getByRole('button',{name:'全部成员',exact:true}).boundingBox();
 assert.ok(groupRow.height<=80,'Conversation row stretches to fill the sidebar');
 const group='WebRTC group '+randomUUID();
 await alice.getByRole('textbox',{name:'输入消息，支持 Markdown 和代码块',exact:true}).fill(group);
 await alice.getByRole('button',{name:'发送消息',exact:true}).click();
 await bob.getByRole('log').getByText(group,{exact:true}).waitFor();await carol.getByRole('log').getByText(group,{exact:true}).waitFor();
 assert.equal(await alice.getByRole('textbox',{name:'输入消息，支持 Markdown 和代码块',exact:true}).inputValue(),'');
 const groupDraft='群聊草稿保留';
 await alice.getByRole('textbox',{name:'输入消息，支持 Markdown 和代码块',exact:true}).fill(groupDraft);
 await alice.getByRole('button',{name:/^私聊.*Bob/}).click();
 const direct='Private to Bob '+randomUUID();
 await alice.getByRole('textbox',{name:'输入消息，支持 Markdown 和代码块',exact:true}).fill(direct);
 await alice.getByRole('button',{name:'发送消息',exact:true}).click();
 // Bob opens the conversation only after the private send completed.
 await bob.getByRole('button',{name:/^私聊.*Alice/}).click();
 await bob.getByRole('log').getByText(direct,{exact:true}).waitFor();
 assert.equal(await carol.getByRole('log').getByText(direct,{exact:true}).count(),0);
 await alice.getByRole('button',{name:'全部成员',exact:true}).click();
 assert.equal(await alice.getByRole('textbox',{name:'输入消息，支持 Markdown 和代码块',exact:true}).inputValue(),groupDraft);
 await alice.getByRole('textbox',{name:'输入消息，支持 Markdown 和代码块',exact:true}).press('Shift+Enter');
 assert.equal(await alice.getByRole('textbox',{name:'输入消息，支持 Markdown 和代码块',exact:true}).inputValue(),groupDraft+'\n');
 await alice.getByRole('textbox',{name:'输入消息，支持 Markdown 和代码块',exact:true}).press('Enter');
 await carol.getByRole('log').getByText(groupDraft,{exact:true}).waitFor();
 const longMessage=Array.from({length:40},(_,index)=>'滚动验收第 '+(index+1)+' 段：阅读消息时，输入栏和会话列表保持原位。').join('\n\n');
 await alice.getByRole('textbox',{name:'输入消息，支持 Markdown 和代码块',exact:true}).fill(longMessage);
 await alice.getByRole('button',{name:'发送消息',exact:true}).click();
 await carol.getByRole('log').getByText('滚动验收第 40 段：阅读消息时，输入栏和会话列表保持原位。',{exact:true}).waitFor();
 const composerBefore=await carol.locator('.flash-chat-composer').boundingBox();
 assert.equal(await carol.getByRole('log').evaluate(node=>node.scrollHeight>node.clientHeight),true,'Long messages do not scroll inside the transcript');
 await carol.getByRole('log').evaluate(node=>{node.scrollTop=0;node.dispatchEvent(new Event('scroll'));});
 const followup='阅读中收到的新消息';
 await alice.getByRole('textbox',{name:'输入消息，支持 Markdown 和代码块',exact:true}).fill(followup);
 await alice.getByRole('button',{name:'发送消息',exact:true}).click();
 await carol.getByRole('log').getByText(followup,{exact:true}).waitFor({state:'attached'});
 assert.equal(await carol.getByRole('log').evaluate(node=>node.scrollTop),0,'An incoming message interrupts reading earlier messages');
 assert.deepEqual(await carol.locator('.flash-chat-composer').boundingBox(),composerBefore,'Scrolling moves the composer');
 for(const [index,page] of pages.entries()){assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);await page.screenshot({path:join(output,'peer-'+index+'.png'),fullPage:true});}
 await bob.getByRole('button',{name:'离开房间',exact:true}).click();await bob.getByRole('button',{name:'加入房间',exact:true}).waitFor();
 await bob.getByRole('button',{name:'加入房间',exact:true}).click();await bob.getByRole('button',{name:'离开房间',exact:true}).waitFor();
 // Exercise the isolated document's real theme and language bridge, including
 // the settings styles it must load independently of its parent window.
 const layouts=[];
 await carol.getByRole('button',{name:'离开房间',exact:true}).click();
 for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark'])for(const width of [320,520,900]){
  for(const [state,page] of [['room',alice],['join',carol]]){
   await page.setViewportSize({width,height:900});
   await page.evaluate(settings=>window.postMessage({type:'interconnect-theme',...settings},location.origin),{language,theme});
   await page.getByRole('button',{name:state==='room'?(language==='zh-CN'?'房间信息':'Room details'):(language==='zh-CN'?'加入房间':'Join room'),exact:true}).waitFor();
   await page.evaluate(async()=>{await document.fonts.ready;await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));});
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'Chat overflows: '+[language,theme,width,state]);
   if(state==='room'){
    const composer=await page.locator('.flash-chat-composer').boundingBox();
    assert.ok(composer.y+composer.height<=900,'Composer is below the viewport');
    assert.ok(900-composer.y-composer.height<=24,'Composer inherits a second bottom gutter');
    assert.equal(await page.evaluate(()=>document.documentElement.scrollHeight>innerHeight),false,'Room scrolls instead of its transcript');
    if(width<=520){
     await page.getByRole('button',{name:language==='zh-CN'?'房间成员':'Room members',exact:true}).click();
     const members=await page.locator('.flash-chat-sidebar').boundingBox();
     assert.equal(members.width,width,'Narrow member list inherits the old fixed sidebar width');
     await page.getByRole('button',{name:language==='zh-CN'?'全部成员':'Everyone',exact:true}).click();
    }
   }
   await page.screenshot({path:join(output,[language,theme,width,state].join('-')+'.png'),fullPage:true});
   layouts.push({language,theme,width,state});
  }
 }
 await writeFile(join(output,'layouts.json'),JSON.stringify(layouts,null,2));
 assert.deepEqual(errors,[]);await writeFile(join(output,'result.json'),JSON.stringify({passed:true,peers:3,checks:['real WebRTC','verified identities','group delivery','private isolation','unopened private conversation','draft cleared on delivery','per-conversation drafts','keyboard send and newline','leave and rejoin','compact conversation rows','independent transcript scrolling','reading position preserved','fixed composer','narrow member navigation','bilingual light/dark layouts'],errors},null,2));
 console.log('Real WebRTC chat: 3 peers, group/private delivery, identity and room lifecycle passed');
}catch(error){
 for(const [index,page] of pages.entries()){await page.screenshot({path:join(output,'failure-'+index+'.png'),fullPage:true});await writeFile(join(output,'failure-'+index+'.txt'),await page.locator('body').innerText());}
 await writeFile(join(output,'failure.json'),JSON.stringify({error:String(error),errors},null,2));throw error;
}finally{await browser.close();await new Promise(resolve=>vite.httpServer.close(resolve));await new Promise(resolve=>tracker.close(resolve));}
