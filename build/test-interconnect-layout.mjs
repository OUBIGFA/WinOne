// Real production App with the isolated in-memory desktop adapter, not a UI redraw.
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {pathToFileURL} from 'node:url';
import {execFileSync} from 'node:child_process';
import assert from 'node:assert/strict';
const root=resolve('.'),frontend=join(root,'apps/desktop/frontend');
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||'_temp/interconnect-layout');
execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);
await mkdir(output,{recursive:true});
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE||join(root,'dist/resources/tools/playwright-1.58.2/node_modules/playwright'));
const {createServer,preview}=await import(pathToFileURL(join(frontend,'node_modules/vite/dist/node/index.js')));
const server=await createServer({root:frontend,configFile:join(frontend,'vite.config.ts'),server:{host:'127.0.0.1',port:0,hmr:false}});
await server.listen();
const base='http://127.0.0.1:'+server.httpServer.address().port;
const built=await preview({root:frontend,configFile:join(frontend,'vite.config.ts'),preview:{host:'127.0.0.1',port:0}});
const builtBase='http://127.0.0.1:'+built.httpServer.address().port;
const browser=await chromium.launch({headless:true,channel:'msedge'}),results=[],errors=[];
try{
 for(const language of ['zh-CN','en-US'])for(const width of [520,900,1280])for(const theme of ['light','dark']){
  const page=await browser.newPage({viewport:{width,height:900},reducedMotion:'reduce'}),en=language==='en-US';
  // The fixture uses Vite, while the child document enforces production CSP.
  // Serve its actual build at the original URL to preserve the origin-checked
  // theme/native bridge without allowing inline development styles.
  await page.route('**/*',async route=>{
   const url=new URL(route.request().url());
   if(url.pathname==='/interconnect-chat.html'||url.pathname.startsWith('/assets/')){
    await route.fulfill({response:await route.fetch({url:builtBase+url.pathname+url.search})});
   }else await route.continue();
  });
  page.on('pageerror',error=>errors.push(error.message));
  await page.goto(base+'/tests/settings-design-preview.html?page=interconnect&language='+language+'&theme='+theme);
  const start=page.getByRole('button',{name:en?'Send':'发送',exact:true});
  await start.waitFor();await page.evaluate(()=>document.fonts.ready);
  assert.deepEqual(await page.getByRole('tablist',{name:en?'Interconnect':'互联助手',exact:true}).getByRole('tab').allTextContents(),en?['File transfer','Stored sharing','Port forwarding','General']:['文件互传','托管分享','端口穿透','常规']);
  const footer=page.locator('[data-slot=settings-footer]');
  const saveSettings=footer.getByRole('button',{name:en?'Save settings':'保存设置',exact:true});
  await saveSettings.waitFor({timeout:5000});
  assert.equal(await saveSettings.isEnabled(),false,'Unchanged settings can be saved');
  await page.getByRole('tab',{name:en?'General':'常规',exact:true}).click();
  await page.getByRole('spinbutton',{name:en?'Port':'端口',exact:true}).fill('5422');
  await page.getByRole('switch',{name:en?'Notify when a transfer completes or fails':'传输完成或失败时通知',exact:true}).click();
  assert.equal(await saveSettings.isEnabled(),true,'Changing tabs lost the settings draft');
  await saveSettings.click();
  await page.getByText(en?'Saved':'已保存',{exact:true}).waitFor();
  assert.equal(await saveSettings.isEnabled(),false,'Saved settings remain dirty');
  assert.equal(await page.getByRole('spinbutton',{name:en?'Port':'端口',exact:true}).inputValue(),'5422');
  await page.getByRole('spinbutton',{name:en?'Port':'端口',exact:true}).fill('5423');
  await footer.getByRole('button',{name:en?'Discard changes':'撤销修改',exact:true}).click();
  assert.equal(await page.getByRole('spinbutton',{name:en?'Port':'端口',exact:true}).inputValue(),'5422');
  const footerBounds=await footer.boundingBox();
  assert.ok(footerBounds.y+footerBounds.height<=900&&footerBounds.y>800,'Save bar is not fixed at the bottom');
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'Stopped page overflows');
  await page.getByRole('tab',{name:en?'File transfer':'文件互传',exact:true}).click();
  await page.getByRole('button',{name:en?'Choose files':'选择文件',exact:true}).click();
  await start.click();
  const receiveQR=page.getByRole('img',{name:en?'Cross-network browser receive QR code':'跨网网页接收二维码',exact:true});
  await receiveQR.waitFor();
  const receiveTabs=page.getByRole('tablist',{name:en?'Receive method':'接收方式',exact:true});
  assert.equal(await receiveTabs.count(),1,'Receiving routes must use a tab switcher');
  const scanTab=receiveTabs.getByRole('tab',{name:en?'Scan to receive':'扫码接收',exact:true});
  const lanTab=receiveTabs.getByRole('tab',{name:en?'LAN direct':'局域网直连',exact:true});
  const lanQR=page.getByRole('img',{name:en?'LAN direct receive QR code':'局域网直连接收二维码',exact:true});
  assert.equal(await scanTab.getAttribute('aria-selected'),'true','Scan to receive must be selected by default');
  assert.equal(await lanQR.count(),0,'LAN QR must not be visible alongside the default QR');
  const qrBounds=await receiveQR.boundingBox(),routeBounds=await receiveQR.locator('..').boundingBox();
  assert.ok(qrBounds.width>=200&&qrBounds.y>=40&&qrBounds.y+qrBounds.height<850,'Sending did not directly reveal a prominent QR code');
  assert.ok(Math.abs(qrBounds.x+qrBounds.width/2-routeBounds.x-routeBounds.width/2)<2,'QR code is not centered in the receive area');
  await page.screenshot({path:join(output,[language,width,theme,'receive'].join('-')+'.png'),fullPage:true});
  await receiveTabs.scrollIntoViewIfNeeded();
  const tabsBounds=await receiveTabs.boundingBox();
  const tabsContainerBounds=await receiveTabs.locator('..').boundingBox();
  assert.ok(Math.abs(tabsBounds.x+tabsBounds.width/2-tabsContainerBounds.x-tabsContainerBounds.width/2)<2,'Receive tabs are not centered as a group');
  await lanTab.click();await lanQR.waitFor();
  assert.equal(await receiveQR.count(),0,'Cross-network QR remains visible in the LAN tab');
  assert.equal(await lanTab.getAttribute('aria-selected'),'true');
  await page.waitForTimeout(1600); // Cross two production polling ticks.
  assert.equal(await lanTab.getAttribute('aria-selected'),'true','Task refresh reset the selected route');
  const switchedBounds=await receiveTabs.boundingBox();
  assert.ok(Math.abs(tabsBounds.x-switchedBounds.x)<2&&Math.abs(tabsBounds.y-switchedBounds.y)<2,'Switching receiving methods moves the tabs');
  const lanBounds=await lanQR.boundingBox(),lanRouteBounds=await lanQR.locator('..').boundingBox();
  assert.ok(Math.abs(lanBounds.x+lanBounds.width/2-lanRouteBounds.x-lanRouteBounds.width/2)<2,'LAN QR is not centered');
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'LAN tab overflows');
  await page.screenshot({path:join(output,[language,width,theme,'receive-lan'].join('-')+'.png'),fullPage:true});
  await lanTab.focus();await page.keyboard.press('ArrowLeft');await page.keyboard.press('Enter');
  await receiveQR.waitFor();
  assert.equal(await scanTab.getAttribute('aria-selected'),'true','Keyboard cannot switch back to scan receive');
  assert.equal(await lanQR.count(),0);
  await page.getByText(en?'Text':'文本',{exact:true}).click();
  await page.getByRole('textbox',{name:en?'Text':'文本',exact:true}).fill('互联助手 / Interconnect — preserved draft');
  await page.getByRole('button',{name:en?'Add and send':'添加并发送',exact:true}).click();
  const sends=await page.evaluate(()=>window.settingsDesignFixture.calls.filter(call=>['interconnect.transfer_start','interconnect.share_add_text'].includes(call.method)));
  assert.deepEqual(sends.map(call=>call.method),['interconnect.transfer_start','interconnect.share_add_text'],'Sending text started a second server');
  assert.equal(await page.getByRole('img',{name:en?'Cross-network browser receive QR code':'跨网网页接收二维码',exact:true}).count(),1);
  assert.equal(await lanQR.count(),0);
  const routes=await page.locator('.interconnect-receive-route h3').allTextContents();
  assert.equal(routes[0],en?'Scan to receive (any network)':'扫码接收（不限网络）','croc must be the primary receiving route');
  await page.getByText(en?'Manage shared content · 2':'管理共享内容 · 2',{exact:true}).click();
  await page.getByText('互联助手 / Interconnect — preserved draft',{exact:true}).first().waitFor({timeout:8000});
  await page.getByRole('button',{name:en?'Save as':'另存为',exact:true}).first().click({timeout:8000});
  await page.getByRole('dialog').waitFor();
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'Save dialog overflows');
  await page.getByRole('dialog').getByRole('button',{name:en?'Close':'关闭',exact:true}).last().click();
  await page.getByRole('dialog').waitFor({state:'hidden'});
  const overflow=await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth);
  assert.ok(overflow<=0,'Running page overflows: '+overflow);
  await page.screenshot({path:join(output,[language,width,theme].join('-')+'.png'),fullPage:true});
  results.push({language,width,theme,overflow});
  await page.getByRole('button',{name:en?'Stop service':'停止服务',exact:true}).first().click();
  {
   const tabName=en?'Stored sharing':'托管分享';
   await page.getByRole('tab',{name:tabName,exact:true}).click();
   const panel=page.getByRole('tabpanel').filter({visible:true}).last();
   await panel.getByRole('button',{name:en?'Choose files':'选择文件',exact:true}).click();
   await panel.getByRole('button',{name:en?'Upload encrypted copy':'上传加密副本',exact:true}).click();
   await panel.getByText(en?'Completed':'已完成',{exact:true}).waitFor();
   await panel.getByText(en?'Advanced options':'高级选项',{exact:true}).click();
   await panel.getByText(en?'Network settings':'网络设置',{exact:true}).click();
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,tabName+' overflows');
   await page.screenshot({path:join(output,[language,width,theme,'store'].join('-')+'.png'),fullPage:true});
   await panel.getByRole('button',{name:en?'Revoke share':'撤销分享',exact:true}).click();await panel.getByText(en?'Revoked':'已撤销',{exact:true}).waitFor();
  }
  for(const kind of ['share','tunnel']){
  {
   const tabName=en?'Port forwarding':'端口穿透';
   await page.getByRole('tab',{name:tabName,exact:true}).click();
   const panel=page.getByRole('tabpanel').filter({visible:true}).last();
   await panel.getByRole('textbox',{name:en?'Target host and port':'目标主机与端口',exact:true}).fill('127.0.0.1:8080');
   await panel.getByRole('button',{name:en?'Start service':'启动服务',exact:true}).click();
   await panel.getByText(en?'Ready':'可用',{exact:true}).first().waitFor({timeout:8000});
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,tabName+' overflows');
   await page.screenshot({path:join(output,[language,width,theme,'tunnel'].join('-')+'.png'),fullPage:true});
   await panel.getByRole('button',{name:en?'Stop service':'停止服务',exact:true}).click();
   await panel.getByRole('button',{name:en?'Remove record':'移除记录',exact:true}).click();
  }
  }
  await page.getByRole('tab',{name:en?'General':'常规',exact:true}).click();
  await page.getByText(en?'Default receive folder':'默认接收目录',{exact:true}).waitFor();
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'General overflows');
  await page.screenshot({path:join(output,[language,width,theme,'general'].join('-')+'.png'),fullPage:true});
  const order=await page.locator('[data-page-id]').evaluateAll(nodes=>nodes.map(node=>node.getAttribute('data-page-id')));
  assert.equal(order[order.indexOf('chat')+1],'flash-chat','Flash Chat must immediately follow Chat');
  assert.equal(await page.getByRole('tab',{name:en?'Flash Chat':'闪聊',exact:true}).count(),0,'Chat remains in transfer tabs');
  await page.locator('[data-page-id=flash-chat]').click();
  const flash=page.frameLocator('iframe[title="'+(en?'Flash Chat':'闪聊')+'"]');
  await flash.getByRole('button',{name:en?'Join room':'加入房间',exact:true}).waitFor();
  const brand=await flash.locator('.flash-chat-brand').evaluate(node=>{const [icon,title]=node.children;return {icon:icon.getBoundingClientRect().y,title:title.getBoundingClientRect().y};});
  assert.ok(Math.abs(brand.icon-brand.title)<8,'Flash Chat brand is not horizontal');
  const checkWelcome=async()=>{
   const layout=await flash.locator('.flash-chat-entry').evaluate(node=>{
    const center=element=>{const rect=element.getBoundingClientRect();return rect.x+rect.width/2;};
    const description=node.querySelector('.flash-chat-welcome > p');
    return {entry:center(node),brand:center(node.querySelector('.flash-chat-brand')),description:center(description),align:getComputedStyle(description).textAlign};
   });
   assert.ok(Math.abs(layout.entry-layout.brand)<2,'Flash Chat icon and title are not centered together');
   assert.ok(Math.abs(layout.entry-layout.description)<2&&layout.align==='center','Flash Chat welcome description is not centered');
  };
  await checkWelcome();
  await flash.getByRole('button',{name:en?'Join room':'加入房间',exact:true}).click();
  await checkWelcome();
  await flash.getByPlaceholder(en?'Room ID':'房间标识',{exact:true}).waitFor();
  await flash.getByPlaceholder(en?'Room password':'房间密码',{exact:true}).waitFor();
  assert.equal(await flash.locator('.flash-chat-lobby').evaluate(node=>getComputedStyle(node).display),'grid','Flash Chat stylesheet was not loaded');
  assert.equal(await page.locator('[data-slot=settings-footer]').count(),0,'Flash Chat must not have the settings footer');
  assert.equal(await page.locator('[data-slot=page-scroll]').evaluate(node=>node.scrollHeight>node.clientHeight),false,'Flash Chat outer page scrolls');
  await page.screenshot({path:join(output,[language,width,theme,'flash-chat'].join('-')+'.png'),fullPage:true});
  await page.close();
 }
 // The second settings write can fail after the LAN write succeeded. Keep
 // the remaining draft, protect navigation, then retry only that failed write.
 const failure=await browser.newPage({viewport:{width:900,height:700}});
 await failure.goto(base+'/tests/settings-design-preview.html?page=interconnect&failure=interconnect.general.save');
 await failure.getByRole('tab',{name:'常规',exact:true}).click();
 await failure.getByRole('spinbutton',{name:'端口',exact:true}).fill('5430');
 const notification=failure.getByRole('switch',{name:'传输完成或失败时通知',exact:true});
 await notification.click();
 await failure.getByRole('button',{name:'保存设置',exact:true}).click();
 await failure.getByText(/设置读取失败，请重试。（隔离测试）/).waitFor();
 assert.equal(await notification.getAttribute('aria-checked'),'false','Failed save discarded the draft');
 await failure.locator('[data-page-id=settings]').click();
 await failure.getByRole('dialog').waitFor();
 await failure.getByRole('button',{name:'继续编辑',exact:true}).click();
 await failure.evaluate(()=>window.settingsDesignFixture.recover());
 await failure.getByRole('button',{name:'保存设置',exact:true}).click();
 await failure.getByText('已保存',{exact:true}).waitFor();
 const saves=await failure.evaluate(()=>window.settingsDesignFixture.calls.filter(call=>['interconnect.preferences.save','interconnect.general.save'].includes(call.method)));
 assert.deepEqual(saves.map(call=>call.method),['interconnect.preferences.save','interconnect.general.save','interconnect.general.save'],'Retry repeated a completed save');
 await failure.screenshot({path:join(output,'save-retry.png')});
 await failure.close();
 assert.deepEqual(errors,[]);
 await writeFile(join(output,'results.json'),JSON.stringify({results,errors},null,2));
 console.log('Interconnect UI: '+results.length+' layouts and start/publish/save/stop flows passed');
}finally{await browser.close();await server.close();await new Promise(resolve=>built.httpServer.close(resolve));}
