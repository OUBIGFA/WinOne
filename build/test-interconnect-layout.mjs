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
const {createServer}=await import(pathToFileURL(join(frontend,'node_modules/vite/dist/node/index.js')));
const server=await createServer({root:frontend,configFile:join(frontend,'vite.config.ts'),server:{host:'127.0.0.1',port:0,hmr:false}});
await server.listen();
const base='http://127.0.0.1:'+server.httpServer.address().port;
const browser=await chromium.launch({headless:true,channel:'msedge'}),results=[],errors=[];
try{
 for(const language of ['zh-CN','en-US'])for(const width of [520,900])for(const theme of ['light','dark']){
  const page=await browser.newPage({viewport:{width,height:900},reducedMotion:'reduce'}),en=language==='en-US';
  page.on('pageerror',error=>errors.push(error.message));
  await page.goto(base+'/tests/settings-design-preview.html?page=interconnect&language='+language+'&theme='+theme);
  const start=page.getByRole('button',{name:en?'Start sharing':'开始共享',exact:true});
  await start.waitFor();await page.evaluate(()=>document.fonts.ready);
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'Stopped page overflows');
  await start.click();
  await page.getByRole('button',{name:en?'Add files':'添加文件',exact:true}).click();
  try {
   await page.getByText(en?'Publish text':'发布文本',{exact:true}).click({timeout:5000});
  } catch (error) {
   await page.screenshot({path:join(output,'failure.png'),fullPage:true});
   console.error({language,width,theme,layout:await page.locator('.settings-section-block,.settings-group-body,summary').evaluateAll(nodes=>nodes.map(node=>({tag:node.tagName,cls:node.className,rect:node.getBoundingClientRect().toJSON(),columns:getComputedStyle(node).gridTemplateColumns}))) });
   throw error;
  }
  await page.getByRole('textbox',{name:en?'Publish text':'发布文本',exact:true}).fill('互联助手 / Interconnect — preserved draft');
  await page.getByRole('button',{name:en?'Publish':'发布',exact:true}).click();
  await page.getByText('互联助手 / Interconnect — preserved draft',{exact:true}).first().waitFor();
  await page.getByRole('button',{name:en?'Save as':'另存为',exact:true}).first().click();
  await page.getByRole('dialog').waitFor();
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'Save dialog overflows');
  await page.getByRole('dialog').getByRole('button',{name:en?'Close':'关闭',exact:true}).last().click();
  await page.getByRole('dialog').waitFor({state:'hidden'});
  const overflow=await page.evaluate(()=>document.documentElement.scrollWidth-innerWidth);
  assert.ok(overflow<=0,'Running page overflows: '+overflow);
  await page.screenshot({path:join(output,[language,width,theme].join('-')+'.png'),fullPage:true});
  results.push({language,width,theme,overflow});
  await page.getByRole('button',{name:en?'Stop sharing':'停止共享',exact:true}).click();
  await start.waitFor();
  for(const stored of [false,true]){
   const tabName=stored?(en?'Stored sharing':'托管分享'):(en?'Code transfer':'口令传输');
   await page.getByRole('tab',{name:tabName,exact:true}).click();
   const panel=page.getByRole('tabpanel').filter({visible:true});
   await panel.getByRole('button',{name:en?'Choose files':'选择文件',exact:true}).click();
   await panel.getByRole('button',{name:stored?(en?'Upload encrypted copy':'上传加密副本'):(en?'Generate code and send':'生成口令并发送'),exact:true}).click();
   await panel.getByText(en?'Completed':'已完成',{exact:true}).waitFor();
   await panel.getByText(en?'Network settings':'网络设置',{exact:true}).click();
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,tabName+' overflows');
   await page.screenshot({path:join(output,[language,width,theme,stored?'store':'transfer'].join('-')+'.png'),fullPage:true});
   if(stored){await panel.getByRole('button',{name:en?'Revoke share':'撤销分享',exact:true}).click();await panel.getByText(en?'Revoked':'已撤销',{exact:true}).waitFor();}
  }
  for(const kind of ['share','tunnel']){
   const tabName=kind==='share'?(en?'Live sharing':'持续分享'):(en?'Port forwarding':'端口穿透');
   await page.getByRole('tab',{name:tabName,exact:true}).click();
   const panel=page.getByRole('tabpanel').filter({visible:true});
   if(kind==='share')await panel.getByRole('button',{name:en?'Choose files':'选择文件',exact:true}).click();
   await panel.getByRole('button',{name:en?'Start service':'启动服务',exact:true}).click();
   await panel.getByText(en?'Ready':'可用',{exact:true}).waitFor();
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,tabName+' overflows');
   await page.screenshot({path:join(output,[language,width,theme,kind].join('-')+'.png'),fullPage:true});
   await panel.getByRole('button',{name:en?'Stop service':'停止服务',exact:true}).click();
   await panel.getByRole('button',{name:en?'Resume service':'恢复服务',exact:true}).click();
   await panel.getByText(en?'Ready':'可用',{exact:true}).waitFor();
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
  await page.screenshot({path:join(output,[language,width,theme,'flash-chat'].join('-')+'.png'),fullPage:true});
  await page.close();
 }
 assert.deepEqual(errors,[]);
 await writeFile(join(output,'results.json'),JSON.stringify({results,errors},null,2));
 console.log('Interconnect UI: '+results.length+' layouts and start/publish/save/stop flows passed');
}finally{await browser.close();await server.close();}
