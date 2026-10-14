// Read only review of the preceding isolated elevation fixture. No consent,
// link creation or Explorer menu changes occur in this visual check.
import {createRequire} from 'node:module';
import {writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2);
const wait=ms=>new Promise(resolve=>setTimeout(resolve,ms));
let browser,page;
try{
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await wait(100);}}
 if(!browser)throw Error('Isolated WebView2 did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(page=>page.url().includes('winone.localhost'));if(page)break;await wait(100);}
 await page.getByRole('navigation').waitFor();await page.evaluate(()=>window.go.main.App.Snapshot());
 await page.getByRole('button',{name:'便捷功能',exact:true}).click();
 await page.getByRole('tab',{name:/文件.*链接/}).click();
 const history=page.locator('.directory-links-history');await history.locator('.directory-links-record').first().waitFor();
 const records=await page.evaluate(async()=>JSON.parse(await window.go.main.App.DirectoryLinks('list','{}')));
 if(!records.some(row=>row.name==='isolated elevation'&&row.status==='removed'))throw Error('The elevated entry did not retain its final recycled state');
 await history.scrollIntoViewIfNeeded();await page.screenshot({path:resolve(output,'link-history.png')});
 await history.locator('article').first().locator('summary').click();
 await history.locator('article').first().scrollIntoViewIfNeeded();await page.screenshot({path:resolve(output,'link-details.png')});
 await writeFile(resolve(output,'result.json'),JSON.stringify({recycledHistoryVisible:true,records},null,2));
 await page.evaluate(async()=>{const api=window.go.main.App;const snapshot=await api.Snapshot();await api.SaveSettings({...snapshot.settings,startup:{...snapshot.settings.startup,keepUIRunning:false}});void api.WindowCommand('close');}).catch(error=>{if(!String(error).includes('closed'))throw error;});
}catch(error){await writeFile(resolve(output,'node-error.txt'),String(error.stack||error));process.exitCode=1;}finally{if(browser)await browser.close().catch(()=>{});}
