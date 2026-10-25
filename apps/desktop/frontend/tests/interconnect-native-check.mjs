// Real Rust window and Host, launched only by build/test-desktop-components.ps1
// in its private desktop/profile. Signaling is local; no real settings change.
import {createRequire} from 'node:module';
import {writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import {pathToFileURL} from 'node:url';
import {createServer} from 'node:http';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2);
let browser,page,tracker,relay;
try{
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(resolve=>setTimeout(resolve,100));}}
 if(!browser)throw Error('Isolated WebView2 did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(page=>page.url().includes('winone.localhost'));if(page)break;await new Promise(resolve=>setTimeout(resolve,100));}
 if(!page)throw Error('Isolated settings document did not load');
 const errors=[];page.on('pageerror',error=>errors.push(String(error)));
 const consoleErrors=[];page.on('console',message=>{if(message.type()==='error')consoleErrors.push(message.text())});
 // Match the existing native component runner: startup navigation must settle
 // before exercising page changes, rather than racing the initial Home load.
 await page.waitForFunction(()=>performance.getEntriesByName('winone-ui-ready').length>0);
 const nav=page.getByRole('button',{name:'互联助手',exact:true});
 await nav.waitFor({state:'visible',timeout:10000});
 await nav.click();
 await page.getByRole('tab',{name:'文件互传',exact:true}).waitFor({state:'visible',timeout:10000});
 const generalTab=page.getByRole('tab',{name:'常规',exact:true});
 await generalTab.waitFor({state:'visible',timeout:10000});
 await generalTab.click();
 await page.waitForTimeout(200);
 const afterGeneral=await page.locator('[data-slot=page-scroll]').getAttribute('data-page');
 await page.waitForTimeout(2500);
 const afterWait=await page.locator('[data-slot=page-scroll]').getAttribute('data-page');
 const activeTab=await page.locator('[role=tab][data-active]').allTextContents();
 const pane=page.locator('[role=tabpanel]:visible').last();
 const paneText=(await pane.innerText().catch(()=>'' )).slice(0,500);
 const paneHtml=(await pane.innerHTML().catch(()=>'' )).slice(0,500);
 const generalVisible=await page.getByText('默认接收目录',{exact:true}).isVisible().catch(()=>false);
 const bridge=await page.evaluate(async()=>{try{return {value:await window.go.main.App.Interconnect('get','{}')}}catch(error){return {error:String(error)}}});
 const generalBridge=await page.evaluate(async()=>{try{return {value:await window.go.main.App.Interconnect('general.get','{}')}}catch(error){return {error:String(error)}}});
 await writeFile(resolve(output,'general-debug.json'),JSON.stringify({afterGeneral,afterWait,activeTab,paneText,paneHtml,generalVisible,bridge,generalBridge,consoleErrors,body:(await page.locator('body').innerText()).slice(0,700)}));
 await page.locator('#interconnect-port').waitFor({state:'visible',timeout:5000});
 await page.locator('#interconnect-port').fill('0');
 const notify=page.getByRole('switch',{name:'传输完成或失败时通知',exact:true});
 await notify.click();
 const expected=await notify.getAttribute('aria-checked');
 const footer=page.locator('[data-slot=settings-footer]');
 await footer.getByRole('button',{name:'保存设置',exact:true}).click();
 await page.getByText('已保存',{exact:true}).waitFor();
 const saved=await page.evaluate(async()=>({
  lan:JSON.parse(await window.go.main.App.Interconnect('get','{}')),
  general:JSON.parse(await window.go.main.App.Interconnect('general.get','{}')),
 }));
 assert.equal(saved.lan.settings.port,0);
 assert.equal(saved.lan.running,false,'Saving settings starts sharing');
 assert.equal(String(saved.general.notify),expected);
 const before=await footer.boundingBox();
 await page.locator('[data-slot=page-scroll]').evaluate(node=>{node.scrollTop=node.scrollHeight;});
 assert.deepEqual(await footer.boundingBox(),before,'Scrolling moves the settings footer');
 await page.screenshot({path:resolve(output,'interconnect-save.png')});
 await page.locator('[data-page-id=settings]').click();
 await page.locator('[data-page-id=interconnect]').click();
 await page.getByRole('tab',{name:'常规',exact:true}).click();
 await notify.waitFor({state:'visible',timeout:8000});
 assert.equal(await notify.getAttribute('aria-checked'),expected,'Reopening lost saved settings');
 // Use the real Host and worker while keeping all files and the relay outage
 // inside this runner's isolated profile. The picker is not mocked.
 const source=resolve(output,'native-shared.txt');await writeFile(source,'native phone share');
 relay=createServer((req,res)=>{res.writeHead(503);res.end('isolated relay outage');});
 await new Promise(resolve=>relay.listen(0,'127.0.0.1',resolve));
 const task=await page.evaluate(async spec=>JSON.parse(await window.go.main.App.Interconnect('transfer_start',JSON.stringify(spec))),{mode:'share_server',paths:[source],derpMapURL:'http://127.0.0.1:'+relay.address().port});
 await page.getByRole('tab',{name:'文件互传',exact:true}).click();
 const receiveTabs=page.getByRole('tablist',{name:'接收方式',exact:true});
 await receiveTabs.getByRole('tab',{name:'局域网直连',exact:true}).click({timeout:15000});
 await page.getByRole('heading',{name:'局域网直连（可选）',exact:true}).waitFor({timeout:15000});
 const phone=page.getByRole('button',{name:'复制局域网链接',exact:true});await phone.scrollIntoViewIfNeeded();
 await page.getByRole('img',{name:'局域网直连接收二维码',exact:true}).waitFor();
 const tasks=await page.evaluate(async()=>JSON.parse(await window.go.main.App.Interconnect('transfer_list','{}')));
 const phoneURL=new URL(tasks.find(item=>item.id===task.id).state.share.info.url);
 assert.equal(phoneURL.protocol,'http:');assert.ok(phoneURL.searchParams.get('key'));
 await page.getByText('WinOne 持续分享',{exact:true}).click();
 await page.getByText('跨网暂不可用',{exact:true}).waitFor();
 assert.equal(await page.getByRole('button',{name:'复制 WinOne 分享地址',exact:true}).count(),0,'Unavailable relay advertises a code');
 await page.getByText('WinOne 持续分享',{exact:true}).click();
 await page.getByRole('radio',{name:'文本',exact:true}).click();
 await page.getByRole('textbox',{name:'文本',exact:true}).fill('real host file then text');
 await page.getByRole('button',{name:'添加并发送',exact:true}).click();
 await page.getByText('管理共享内容 · 2',{exact:true}).waitFor();
 const afterAppend=await page.evaluate(async()=>JSON.parse(await window.go.main.App.Interconnect('transfer_list','{}')));
 assert.equal(afterAppend.length,1,'Appending text started a second share');
 assert.equal(afterAppend[0].state.share.info.url,phoneURL.toString());
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
 await page.screenshot({path:resolve(output,'persistent-share-phone.png')});
 await page.getByRole('button',{name:'停止服务',exact:true}).click();
 await page.evaluate(async id=>window.go.main.App.Interconnect('transfer_remove',JSON.stringify({id})),task.id);
 await page.locator('[data-page-id=flash-chat]').click();
 assert.equal(await page.locator('[data-page-id=flash-chat] .lucide-messages-circle').count(),1);
 assert.equal(await page.locator('[data-slot=settings-footer]').count(),0);
 const frame=page.frameLocator('iframe[title="闪聊"]');
 await frame.locator('.flash-chat-welcome .lucide-messages-circle').waitFor();
 const welcome=await frame.locator('.flash-chat-entry').evaluate(node=>{
  const center=element=>{const rect=element.getBoundingClientRect();return rect.x+rect.width/2;};
  return {entry:center(node),brand:center(node.querySelector('.flash-chat-brand')),description:center(node.querySelector('.flash-chat-welcome > p'))};
 });
 assert.ok(Math.abs(welcome.entry-welcome.brand)<2&&Math.abs(welcome.entry-welcome.description)<2,'Native Flash Chat welcome is not centered');
 await page.screenshot({path:resolve(output,'flash-chat-lobby.png')});
 const {Server}=await import(pathToFileURL(resolve(process.env.WINONE_TRACKER_MODULE,'index.js')));
 tracker=new Server({udp:false,http:false,ws:{clientTracking:true},stats:false});
 await new Promise(resolve=>tracker.listen(0,'127.0.0.1',resolve));
 await frame.getByRole('button',{name:'网络设置',exact:true}).click();
 await frame.getByRole('textbox',{name:'信令服务（每行一个 WebSocket 地址）',exact:true}).fill('ws://127.0.0.1:'+tracker.ws.address().port);
 await frame.getByRole('textbox',{name:'WebRTC 配置（JSON，可设置 STUN / TURN）',exact:true}).fill(JSON.stringify({iceServers:[]}));
 await frame.getByRole('dialog').getByRole('button',{name:'关闭',exact:true}).last().click();
 await frame.getByRole('button',{name:'创建房间',exact:true}).click();
 await frame.getByRole('button',{name:'房间信息',exact:true}).waitFor();
 await frame.getByRole('textbox',{name:'输入消息，支持 Markdown 和代码块',exact:true}).fill('房主可以独自发送');
 await frame.getByRole('button',{name:'发送消息',exact:true}).click();
 await frame.getByRole('log').getByText('房主可以独自发送',{exact:true}).waitFor();
 assert.equal(await frame.locator('html').evaluate(node=>node.scrollWidth>innerWidth||node.scrollHeight>innerHeight),false);
 const composer=await frame.locator('.flash-chat-composer').boundingBox();
 const frameBounds=await page.locator('iframe[title="闪聊"]').boundingBox();
 assert.ok(composer.y+composer.height<=frameBounds.y+frameBounds.height);
 await page.screenshot({path:resolve(output,'flash-chat-room.png')});
 await frame.getByRole('button',{name:'房间信息',exact:true}).click();
 await frame.getByRole('dialog').getByRole('button',{name:'离开房间',exact:true}).click();
 await frame.getByRole('button',{name:'加入房间',exact:true}).waitFor();
 assert.deepEqual(errors,[]);
 await writeFile(resolve(output,'result.json'),JSON.stringify({passed:true,realHostSave:true,reopenPreservesSettings:true,saveDoesNotStartShare:true,fixedFooter:true,officialIcon:true,roomLifecycle:true,soloSend:true,fileThenTextSameShare:true,errors},null,2));
 // Only the isolated fixture opts out of background retention. Wait for the
 // host's configuration transaction instead of racing its busy protection.
 await page.waitForFunction(async()=>{
  const api=window.go.main.App;
  try{const snapshot=await api.Snapshot();await api.SaveSettings({...snapshot.settings,startup:{...snapshot.settings.startup,keepUIRunning:false}});return true;}
  catch(error){if(String(error).includes('(configuration_busy)'))return false;throw error;}
 },null,{timeout:10000,polling:100});
 await writeFile(resolve(output,'cleanup-stage.txt'),'closing native window');
 await page.evaluate(()=>{void window.go.main.App.WindowCommand('close');});
 await writeFile(resolve(output,'cleanup-stage.txt'),'native close dispatched');
}catch(error){
 await writeFile(resolve(output,'node-error.txt'),String(error.stack||error));
 if(page){await page.screenshot({path:resolve(output,'failure.png')});await writeFile(resolve(output,'failure.txt'),await page.locator('body').innerText());}
 throw error;
}finally{
 // CDP disconnect does not own the native browser. Explicitly close the test
 // servers' keep-alive sockets so their shutdown cannot outlive the driver.
 await writeFile(resolve(output,'cleanup-stage.txt'),'closing tracker');
 for(const socket of tracker?.ws?.clients??[])socket.terminate();
 if(tracker)await new Promise(resolve=>tracker.close(resolve));
 await writeFile(resolve(output,'cleanup-stage.txt'),'closing relay');
 if(relay){relay.closeAllConnections();await new Promise(resolve=>relay.close(resolve));}
 await writeFile(resolve(output,'cleanup-stage.txt'),'disconnecting CDP');
 await browser?.close();
 await writeFile(resolve(output,'cleanup-stage.txt'),'cleanup complete');
}
