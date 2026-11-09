// Real peer/LAN processes and a mobile-sized browser. No public relay is used.
import {spawn,execFileSync} from 'node:child_process';
import {createInterface} from 'node:readline';
import {createServer as createTCPServer} from 'node:net';
import {createServer} from 'node:http';
import {createRequire} from 'node:module';
import {resolve,join} from 'node:path';
import {mkdir,writeFile,readFile} from 'node:fs/promises';
import assert from 'node:assert/strict';
const [bundleArg,outputArg]=process.argv.slice(2);
if(!bundleArg||!outputArg)throw Error('Usage: node build/test-interconnect-phone.mjs <interconnect-bundle> <ignored-output>');
const bundle=resolve(bundleArg),output=resolve(outputArg);
execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);
await mkdir(output,{recursive:true});
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE||resolve('dist/resources/tools/playwright-1.58.2/node_modules/playwright'));
const browser=await chromium.launch({headless:true,channel:'msedge'});
const decoder=await browser.newPage();
await decoder.addScriptTag({path:resolve('dist/resources/tools/playwright-1.58.2/node_modules/jsqr/dist/jsQR.js')});
async function decodeQR(source){
 return decoder.evaluate(async source=>{
  const image=new Image();image.src=source;await image.decode();
  const canvas=document.createElement('canvas');canvas.width=image.naturalWidth*2;canvas.height=image.naturalHeight*2;
  const context=canvas.getContext('2d');context.drawImage(image,0,0,canvas.width,canvas.height);
  const pixels=context.getImageData(0,0,canvas.width,canvas.height);
  const code=jsQR(pixels.data,canvas.width,canvas.height);if(!code)throw Error('Cannot decode rendered QR');return code.data;
 },source);
}
async function nativeQR(info,directory){
 const input=join(directory,'qr-info.json');await mkdir(directory,{recursive:true});await writeFile(input,JSON.stringify(info));
 execFileSync(process.env.WINONE_QR_RENDER_TEST_EXE,['-test.run=^TestPhoneQRRendering$','-test.timeout=30s'],{env:{...process.env,WINONE_PHONE_QR_INPUT:input},windowsHide:true});
 return decodeQR('data:image/png;base64,'+(await readFile(join(directory,'qr-native.png'))).toString('base64'));
}
const relay=createServer((req,res)=>{res.writeHead(503);res.end('isolated relay outage');});
await new Promise(resolve=>relay.listen(0,'127.0.0.1',resolve));
const occupied=createTCPServer(socket=>socket.end());
await new Promise(resolve=>occupied.listen(0,'127.0.0.1',resolve));
const children=[],errors=[],results=[];
function worker(){
 const process=spawn(join(bundle,'WinOne.Interconnect.Peer.exe'),[],{windowsHide:true,stdio:['pipe','pipe','pipe']});
 children.push(process);let sequence=0;const pending=new Map();
 createInterface({input:process.stdout}).on('line',line=>{const value=JSON.parse(line),item=pending.get(value.id);if(!item)return;pending.delete(value.id);clearTimeout(item.timer);value.error?item.reject(Error(value.error)):item.resolve(value.result);});
 process.on('exit',()=>{for(const item of pending.values()){clearTimeout(item.timer);item.reject(Error('Peer exited'));}pending.clear();});
 return (method,params=null)=>new Promise((resolve,reject)=>{const id=String(++sequence),timer=setTimeout(()=>{pending.delete(id);reject(Error('Peer timeout: '+method));},15000);pending.set(id,{resolve,reject,timer});process.stdin.write(JSON.stringify({id,method,params})+String.fromCharCode(10));});
}
try{
 const source=join(output,'phone-download.txt'),contents='WinOne phone download — 原始文件内容';
 await writeFile(source,contents);
 for(const allowWrites of [false,true]){
  const call=worker();
  await call('start',{mode:'share_server',paths:[source],bind:'127.0.0.1',port:occupied.address().port,dataDir:join(output,allowWrites?'writable':'readonly'),allowWrites,derpMapURL:'http://127.0.0.1:'+relay.address().port});
  let state;const deadline=Date.now()+15000;
  while(Date.now()<deadline){state=await call('snapshot');if(state.ready&&state.relayError)break;if(state.status!=='running')throw Error(state.error);await new Promise(resolve=>setTimeout(resolve,100));}
  assert.equal(state.ready,true);assert.ok(state.relayError);assert.equal(state.relayReady,false);
  const info=state.share.info;
  let link=await decodeQR('data:image/svg+xml,'+encodeURIComponent(info.qr));
  assert.equal(link,info.url,'Worker QR differs from the LAN link');
  if(process.env.WINONE_QR_RENDER_TEST_EXE){
   link=await nativeQR(info,join(output,allowWrites?'native-writable':'native-readonly'));
   assert.equal(link,info.url,'Native QR differs from the copied LAN link');
   // Characterize stale worker images: the selected URL must be authoritative.
   const changed={...info,url:info.url.replace(/key=[^&]+/,'key=qr-regression')};
   assert.equal(await nativeQR(changed,join(output,allowWrites?'native-stale-writable':'native-stale-readonly')),changed.url);
  }
  assert.notEqual(Number(new URL(link).port),occupied.address().port,'Busy port was not avoided');
  assert.ok(occupied.listening,'An existing listener was stopped');
  await call('add_text','text after sending a file');
  const appended=await call('snapshot');
  assert.equal(appended.share.info.url,link,'Adding text changed the LAN link');
  assert.equal(appended.share.items.length,2);
  assert.equal(appended.share.items.find(item=>item.kind==='text').content,'text after sending a file');
  assert.equal(new URL(link).protocol,'http:');assert.ok(new URL(link).searchParams.get('key'));
  const context=await browser.newContext({viewport:{width:390,height:844},acceptDownloads:true});
  const page=await context.newPage();page.on('pageerror',error=>errors.push(error.message));
  if(allowWrites)await page.route('**/api/share-info',async route=>{
   const response=await route.fetch(),info=await response.json();
   // Characterize the actual browser across a relay: the sender's addresses
   // differ from the receiver's gateway and must not capture download clicks.
   info.url='http://192.0.2.1:1234/';info.addresses=[{ip:'192.0.2.1',url:info.url,qr:''}];
   await route.fulfill({response,json:info});
  });
  await page.goto(link);
  await page.locator('[data-action=download]').first().waitFor();
  assert.equal(new URL(page.url()).search,'','Credential remains in browser URL');
  for(const selector of ['[data-open=textDialog]','[data-open=fileDialog]'])assert.equal(await page.locator(selector).isVisible(),allowWrites);
  for(const width of [320,390,768]){
   await page.setViewportSize({width,height:844});
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'Phone page overflows at '+width);
   await page.screenshot({path:join(output,(allowWrites?'writable':'readonly')+'-'+width+'.png'),fullPage:true});
  }
  const downloaded=page.waitForEvent('download');await page.locator('[data-action=download]').first().click();
  const download=await downloaded;const destination=join(output,allowWrites?'received-writable.txt':'received-readonly.txt');
  await download.saveAs(destination);assert.equal(await readFile(destination,'utf8'),contents);
  const status=await page.evaluate(async()=>{const response=await fetch('/api/text',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({content:'phone upload'})});return response.status;});
  assert.equal(status,allowWrites?200:403);
  // Exercise the phone's file picker and read the stored bytes back, not just
  // the text endpoint: a valid QR must authorize the actual transfer flow.
  if(allowWrites){
   await page.locator('[data-open=fileDialog]').click();
   const uploaded=page.waitForResponse(response=>response.url().endsWith('/api/upload')&&response.request().method()==='POST');
   await page.locator('#fileInput').setInputFiles({name:'from-phone.txt',mimeType:'text/plain',buffer:Buffer.from('手机上传后的完整文件')});
   const response=await uploaded;assert.equal(response.status(),200);
   const items=await response.json();assert.ok(items[0]?.id);
   await page.locator('#uploadTasks [data-state=completed]').waitFor();
   await page.locator('#uploadTasks [data-action=view]').click();
   const received=page.waitForEvent('download');
   await page.locator(`[data-action=download][data-id="${items[0].id}"]`).click();
   const file=await received;const saved=join(output,'phone-upload-roundtrip.txt');
   await file.saveAs(saved);assert.equal(await readFile(saved,'utf8'),'手机上传后的完整文件');
  }else{
   const uploadStatus=await page.evaluate(async()=>{const data=new FormData();data.append('file',new Blob(['read-only check']),'denied.txt');return (await fetch('/api/upload',{method:'POST',body:data})).status;});
   assert.equal(uploadStatus,403);
  }
  await call('stop');
  await page.locator('#shareEndedNotice').getByText('分享已停止',{exact:true}).waitFor();
  results.push({allowWrites,download:true,uploadRoundtrip:allowWrites,readOnlyUploadRejected:!allowWrites,qrDecoded:true,nativeQRDecoded:!!process.env.WINONE_QR_RENDER_TEST_EXE,credentialRedirect:true,relayFailureSurvived:true,stoppedNotice:true,autoPort:true,fileThenText:true,sameLink:true});
  await context.close();
 }
 assert.deepEqual(errors,[]);await writeFile(join(output,'results.json'),JSON.stringify({results,errors},null,2));
 console.log('Phone downloads: read-only and writable shares passed at 320/390/768px, including relay outage and stop');
}finally{
 for(const child of children){child.stdin.end();if(child.exitCode===null)child.kill();}
 await browser.close();await new Promise(resolve=>relay.close(resolve));await new Promise(resolve=>occupied.close(resolve));
}
