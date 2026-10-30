// Runs production links UI against the explicit in-memory host fixture.
// Filesystem identity/transaction tests live with the Go manager and Host.
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {pathToFileURL} from 'node:url';
const output=resolve(process.env.WINONE_LAYOUT_OUTPUT||'_temp/directory-history-archive/browser');
await mkdir(output,{recursive:true});
const frontend=resolve('apps/desktop/frontend');
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE||resolve('dist/resources/tools/playwright-1.58.2/node_modules/playwright'));
const {createServer}=await import(pathToFileURL(join(frontend,'node_modules/vite/dist/node/index.js')));
const server=await createServer({root:frontend,configFile:join(frontend,'vite.config.ts'),server:{host:'127.0.0.1',port:0,hmr:false}});
await server.listen();
const port=server.httpServer.address().port;
const browser=await chromium.launch({headless:true,channel:'msedge'}),results=[],errors=[];
let page;
try{
 for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark'])for(const width of [520,900,1280]){
  page=await browser.newPage({viewport:{width,height:900},colorScheme:theme,reducedMotion:'reduce'});page.setDefaultTimeout(10000);
  page.on('pageerror',error=>errors.push(String(error)));
  await page.goto(`http://127.0.0.1:${port}/tests/settings-design-preview.html?page=convenience&language=${language}&theme=${theme}`);
  const zh=language==='zh-CN';
  await page.getByRole('tab',{name:zh?'文件与目录链接':'File and folder links',exact:true}).click();
  await page.getByRole('tab',{name:zh?'查找已有链接':'Find existing links',exact:true}).click();
  const exportButton=page.getByRole('button',{name:zh?'导出选中记录':'Export selected records',exact:true});
  assert.equal(await exportButton.isDisabled(),true);
  await page.getByText(zh?'选择所有记录':'Select all records',{exact:true}).click();
  await exportButton.click();
  await page.getByText(/WinOne-links-backup.json/).waitFor();
  await page.getByRole('button',{name:zh?'导入历史备份':'Import history backup',exact:true}).click();
  const dialog=page.getByRole('dialog');await dialog.waitFor();
  await dialog.getByText('Imported record',{exact:false}).waitFor();
  const geometry=await dialog.evaluate(element=>({width:element.clientWidth,scroll:element.scrollWidth,page:document.documentElement.scrollWidth,viewport:innerWidth}));
  assert.ok(geometry.scroll<=geometry.width+1&&geometry.page<=geometry.viewport+1,JSON.stringify(geometry));
  assert.equal(await page.evaluate(()=>window.settingsDesignFixture.calls.some(call=>call.method==='links.history.import')),false);
  await dialog.getByText(zh?'查看跳过原因':'Show skipped entries',{exact:true}).click();
  await page.screenshot({path:join(output,`${language}-${theme}-${width}.png`)});
  await dialog.getByRole('button',{name:zh?'确认导入记录':'Import verified records',exact:true}).click();
  await dialog.waitFor({state:'hidden'});
  await page.getByText(zh?'已导入 1 条记录，真实文件和链接未改变。':'Imported 1 records. Real files and links remain unchanged.',{exact:true}).waitFor();
  const calls=await page.evaluate(()=>window.settingsDesignFixture.calls.filter(call=>call.method.startsWith('links.history.')));
  assert.equal(calls.find(call=>call.method==='links.history.export').params.ids.length,4);
  assert.deepEqual(calls.find(call=>call.method==='links.history.import').params,{token:'archive-preview',confirmed:true});
  results.push({language,theme,width,geometry,calls});await page.close();
 }
 assert.deepEqual(errors,[]);
 await writeFile(join(output,'result.json'),JSON.stringify({results,errors},null,2));
 console.log(`PASS: ${results.length} history export/import layout and confirmation scenarios`);
}catch(error){
 await page?.screenshot({path:join(output,'failure.png')}).catch(()=>{});
 await writeFile(join(output,'failure.json'),JSON.stringify({error:String(error),errors,text:await page?.locator('body').innerText().catch(()=>''),calls:await page?.evaluate(()=>window.settingsDesignFixture?.calls).catch(()=>[])},null,2));throw error;
}finally{await browser.close();await server.close();}
