// Opt-in real public-relay/browser interop. Sends only a generated test file.
// No user data or desktop profile is read. This is not a two-network/NAT test.
import {spawn,execFileSync} from 'node:child_process';
import {createInterface} from 'node:readline';
import {createRequire} from 'node:module';
import {mkdir,writeFile,readFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import assert from 'node:assert/strict';
const [bundleArg,outputArg,consent]=process.argv.slice(2);
if(!bundleArg||!outputArg||consent!=='--public')throw Error('Usage: node build/test-interconnect-croc-web.mjs <bundle> <ignored-output> --public');
const output=resolve(outputArg);
execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);
await mkdir(output,{recursive:true});
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE||resolve('dist/resources/tools/playwright-1.58.2/node_modules/playwright'));
const worker=spawn(join(resolve(bundleArg),'WinOne.Interconnect.Transfer.exe'),[],{windowsHide:true,stdio:['pipe','pipe','pipe']});
let sequence=0,browser,page;const pending=new Map();
createInterface({input:worker.stdout}).on('line',line=>{const value=JSON.parse(line),item=pending.get(value.id);if(!item)return;pending.delete(value.id);clearTimeout(item.timer);value.error?item.reject(Error(value.error)):item.resolve(value.result);});
worker.on('exit',()=>{for(const item of pending.values()){clearTimeout(item.timer);item.reject(Error('Transfer worker exited'));}pending.clear();});
const call=(method,params=null)=>new Promise((resolve,reject)=>{const id=String(++sequence),timer=setTimeout(()=>{pending.delete(id);reject(Error('Worker timeout: '+method));},10000);pending.set(id,{resolve,reject,timer});worker.stdin.write(JSON.stringify({id,method,params})+'\n');});
try{
 const source=join(output,'winone-croc-browser-test.txt'),content='WinOne generated browser interoperability test. 非用户文件。';
 await writeFile(source,content);
 await call('start',{mode:'send',paths:[source],dataDir:join(output,'sender'),options:{transport:'auto',disableLocal:true,noPrompt:true}});
 let state;const deadline=Date.now()+60000;
 while(Date.now()<deadline){state=await call('snapshot');if(state.browserUrl)break;if(state.status!=='running')throw Error(state.error||state.status);await new Promise(resolve=>setTimeout(resolve,200));}
 assert.ok(state.browserUrl,'Public relay did not produce a browser receive URL');
 browser=await chromium.launch({headless:true,channel:'msedge'});
 const context=await browser.newContext({viewport:{width:390,height:844},acceptDownloads:true});
 // Exercise the phone-style Blob/download fallback, not a desktop-only picker.
 await context.addInitScript(()=>{delete window.showDirectoryPicker;delete window.showSaveFilePicker;});
 page=await context.newPage();
 await page.goto(state.browserUrl,{timeout:60000});
 const accept=page.getByRole('button',{name:/^(Accept files|Download files)$/});
 // The hosted deployment labels its phone download action “Download files”;
 // the locked source uses “Accept files”. Both lead to the real download.
 await accept.waitFor({timeout:60000});
 await page.screenshot({path:join(output,'offer.png'),fullPage:true});
 const download=page.waitForEvent('download',{timeout:60000});
 await accept.click();
 const received=await download,destination=join(output,'received.txt');
 await received.saveAs(destination);
 assert.equal(await readFile(destination,'utf8'),content);
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
 await page.screenshot({path:join(output,'received.png'),fullPage:true});
 await writeFile(join(output,'result.json'),JSON.stringify({passed:true,officialWebReceiver:true,publicRelay:true,localDiscoveryDisabled:true,fileBytesMatch:true,twoNetworkTest:false},null,2));
 console.log('croc official web receiver: public relay download matches generated file');
}catch(error){
 await writeFile(join(output,'result.json'),JSON.stringify({passed:false,error:String(error),twoNetworkTest:false},null,2));
 if(page){await page.screenshot({path:join(output,'failure.png'),fullPage:true});await writeFile(join(output,'failure.txt'),await page.locator('body').innerText());}
 throw error;
}finally{await call('stop').catch(()=>{});worker.stdin.end();if(worker.exitCode===null)worker.kill();await browser?.close();}
