// Production bundle in independent browser profiles. A real local tracker
// isolates feature tests from public-server outages; --public-trackers selects
// the public defaults. Capture devices are fixtures, not physical hardware.
const fs = require('node:fs'), path = require('node:path'), http = require('node:http'), crypto = require('node:crypto'), assert = require('node:assert/strict');
const root = path.resolve(__dirname, '../..');
const output = path.resolve(root, process.argv[2] || '_temp/flashchat-complete');
if (!output.startsWith(path.join(root, '_temp') + path.sep)) throw Error('Output must be in repository _temp');
fs.mkdirSync(path.join(output, 'scratch'), {recursive:true});
process.env.TEMP = process.env.TMP = process.env.TMPDIR = path.join(output, 'scratch');
const {chromium} = require(process.env.WINONE_PLAYWRIGHT_MODULE || path.join(root, 'dist/resources/tools/playwright-1.58.2/node_modules/playwright'));
const assets = path.join(root, 'apps/desktop-mygo/pages/flashchat/assets');
const types = {'.js':'application/javascript','.css':'text/css','.svg':'image/svg+xml','.html':'text/html','.json':'application/json','.png':'image/png','.aac':'audio/aac','.mp3':'audio/mpeg'};
const report = {checks:[], errors:[], diagnostics:[]};
const bounded=(promise,label,ms=15000)=>Promise.race([promise,new Promise((_,reject)=>{const timer=setTimeout(()=>reject(Error(label+' timed out')),ms);timer.unref()})]);
async function main() {
 const publicTrackers=process.argv.includes('--public-trackers');
 const rtcOverride=process.env.WINONE_TEST_RTC_CONFIG ? JSON.parse(process.env.WINONE_TEST_RTC_CONFIG) : null;
 const Tracker=require(path.join(root,'modules/flashchat/node_modules/bittorrent-tracker/server'));
 const tracker=publicTrackers?null:new Tracker({udp:false,http:false,ws:true,stats:false});
 if(tracker)await new Promise((resolve,reject)=>{tracker.once('error',reject);tracker.listen(0,'127.0.0.1',resolve)});
 const trackerUrl=tracker?'ws://127.0.0.1:'+tracker.http.address().port:null;
 report.discovery=publicTrackers?'public trackers':'local WebTorrent tracker';
 const server = http.createServer((req,res) => {
   let name = decodeURIComponent(new URL(req.url, 'http://localhost').pathname).slice(1);
   if (!name || /^(public|private)\//.test(name) || ['settings','about','disclaimer'].includes(name)) name='index.html';
   const file=path.resolve(assets,name);
   if (!file.startsWith(assets+path.sep) || !fs.existsSync(file) || !fs.statSync(file).isFile()) {res.writeHead(404).end();return}
   res.setHeader('Content-Type',types[path.extname(file)]||'application/octet-stream');fs.createReadStream(file).pipe(res);
 });
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 const origin='http://127.0.0.1:'+server.address().port;
 const browser=await chromium.launch({channel:'msedge',headless:true,args:['--use-fake-device-for-media-stream','--use-fake-ui-for-media-stream','--auto-select-desktop-capture-source=Entire screen']});
 const check=name=>{report.checks.push(name);fs.writeFileSync(path.join(output,'progress.json'),JSON.stringify(report,null,2));console.log('PASS '+name)};
 const create=async(name,language='en-US',theme='light',width=1000)=>{
   const tx=key=>language.startsWith('zh')?(require(path.join(root,'modules/flashchat/src/config/zh-CN.json'))[key]||key):key;
   const context=await browser.newContext({viewport:{width,height:800},permissions:['camera','microphone','clipboard-read','clipboard-write'],acceptDownloads:true});
   context.setDefaultTimeout(30000);context.setDefaultNavigationTimeout(30000);
   await context.addInitScript(({name,language,theme})=>{
     window.__winoneState={language,theme};window.__winoneLegacyNickname=name;
     window.__testPCs=[];const Original=window.RTCPeerConnection;
     window.RTCPeerConnection=class extends Original {constructor(...args){super(...args);window.__testPCs.push(this)}};
   },{name,language,theme});
   const page=await context.newPage();page.on('pageerror',error=>report.errors.push(error.message));
   page.on('console',message=>{if(['error','warning'].includes(message.type()))report.diagnostics.push({name,type:message.type(),text:message.text()})});
   page.on('requestfailed',request=>report.diagnostics.push({name,url:request.url(),failure:request.failure()}));
   if(trackerUrl || rtcOverride){
     await page.goto(origin+'/settings');
     await page.getByText(tx('Custom network settings'),{exact:true}).click();
     if(trackerUrl)await page.getByLabel(tx('Pairing servers (one WebSocket URL per line)'),{exact:true}).fill(trackerUrl);
     await page.getByLabel(tx('WebRTC configuration (JSON)'),{exact:true}).fill(JSON.stringify(rtcOverride || {iceServers:[]}));
     await page.getByRole('button',{name:tx('Apply network settings'),exact:true}).click();
     await page.getByText(tx('Network settings saved'),{exact:true}).waitFor();
   }
   return page;
 };
 const send=async(page,text)=>{const input=page.getByPlaceholder('Your message').first();await input.fill(text);await input.press('Enter')};
 const waitText=(page,text)=>page.getByText(text,{exact:true}).first().waitFor({timeout:60000});
 try {
   for (const [language,theme,width] of [['zh-CN','light',1000],['zh-CN','dark',560],['en-US','light',560],['en-US','dark',1000]]) {
     const page=await create('Layout',language,theme,width);
     for(const route of ['', 'settings','about','disclaimer']){
       await page.goto(origin+'/'+route);await page.locator('header').waitFor();
       await page.waitForFunction(()=>!document.querySelector('[aria-label="Loading"]'));
       assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
       await page.screenshot({path:path.join(output,`${route||'home'}-${language}-${theme}-${width}.png`)});
     }
     await page.goto(origin);await page.locator('.Home').waitFor();
     await page.getByRole('button',{name:language.startsWith('zh')?'打开菜单':'Open menu',exact:true}).click();
     assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
     check(`layout ${language} ${theme} ${width}`);await page.context().close();
   }
   // Exercise exported files through the UI in fresh browser profiles. A
   // successful import must preserve the identity, keys and preferences.
   const profile=await create('Profile fixture');
   await profile.goto(origin+'/settings');
   const typingPreference=profile.getByRole('checkbox',{name:'Show active typing indicators',exact:true});
   await typingPreference.click();
   // Settings commits to IndexedDB before changing the controlled checkbox.
   await profile.waitForFunction(()=>[...document.querySelectorAll('input[type=checkbox]')].some(input=>input.closest('label')?.textContent==='Show active typing indicators'&&!input.checked));
   const exportProfile=async(page,name)=>{
     const pending=page.waitForEvent('download');
     await page.getByRole('button',{name:'Export profile data',exact:true}).click();
     const download=await pending, file=path.join(output,name);
     await download.saveAs(file);return {file,data:JSON.parse(fs.readFileSync(file,'utf8'))};
   };
   const exported=await exportProfile(profile,'profile-source.json');
   const restored=await create('Import fixture');
   await restored.goto(origin+'/settings');
   await restored.locator('input[type=file]').setInputFiles(exported.file);
   await waitText(restored,'Profile successfully imported');
   await restored.reload();
   const imported=await exportProfile(restored,'profile-restored.json');
   // Never print exported verification keys in an assertion diagnostic.
   assert.deepEqual(Object.keys(imported.data).sort(),Object.keys(exported.data).sort());
   for(const key of Object.keys(exported.data))assert.ok(JSON.stringify(imported.data[key])===JSON.stringify(exported.data[key]),'imported profile field differs: '+key);
   check('profile file round trip and persisted identity');
   await restored.getByRole('button',{name:'Delete all data and restart',exact:true}).click();
   await restored.getByRole('button',{name:'Cancel',exact:true}).click();
   assert.equal((await exportProfile(restored,'profile-cancelled.json')).data.userId,exported.data.userId);check('profile deletion cancellation');
   await profile.goto(origin);
   await profile.getByRole('button',{name:'Get embed code',exact:true}).click();
   const embed=await profile.getByRole('dialog').innerText();
   assert.ok(embed.includes('https://chitchatter.im/public/')&&embed.includes('https://chitchatter.im/sdk.js')&&!embed.includes(origin));
   await profile.getByRole('dialog').getByRole('button',{name:'Close',exact:true}).click();check('portable iframe and SDK embedding');
   await profile.getByRole('button',{name:'Show QR Code',exact:true}).click();
   await profile.getByRole('dialog').locator('canvas').waitFor();
   await profile.getByRole('button',{name:'Dismiss',exact:true}).click();check('QR code opens and closes');
   await profile.getByRole('button',{name:'fullscreen',exact:true}).click();
   await profile.waitForFunction(()=>!!document.fullscreenElement);
   await profile.locator('button.MuiFab-root[aria-label="show room controls"]').click();
   await profile.getByRole('button',{name:'fullscreen',exact:true}).click();
   await profile.waitForFunction(()=>!document.fullscreenElement);check('fullscreen enter and exit');
   await Promise.all([profile.context().close(),restored.context().close()]);
   const a=await create('Ada'), b=await create('Babbage'), c=await create('Curie');
   const room='winone-check-'+crypto.randomUUID(),url=origin+'/public/'+room;
   await Promise.all([a.goto(url),b.goto(origin)]);
   await b.getByLabel('Room name (generated on your device)',{exact:true}).fill('https://chitchatter.im/public/'+room);
   await b.getByRole('button',{name:'Join public room',exact:true}).click();
   await send(a,'public before joining');await waitText(b,'public before joining');
   await send(b,'public reply');await waitText(a,'public reply');check('two-way public messages through pasted invitation');
   if(rtcOverride?.iceTransportPolicy==='relay'){
     // Chromium can expose the selected pair before its byte counters catch up
     // with messages already delivered to JavaScript.
     await a.waitForFunction(async()=>{for(const pc of window.__testPCs){if(pc.connectionState!=='connected')continue;const stats=await pc.getStats();for(const entry of stats.values()){if(entry.type==='transport'&&entry.selectedCandidatePairId){const pair=stats.get(entry.selectedCandidatePairId);if(pair.bytesSent>0&&pair.bytesReceived>0)return true}}}return false},null,{timeout:15000});
     report.selectedPaths=await a.evaluate(async()=>{const paths=[];for(const pc of window.__testPCs){if(pc.connectionState!=='connected')continue;const stats=await pc.getStats();for(const entry of stats.values()){if(entry.type==='transport'&&entry.selectedCandidatePairId){const pair=stats.get(entry.selectedCandidatePairId);paths.push({local:stats.get(pair.localCandidateId)?.candidateType,remote:stats.get(pair.remoteCandidateId)?.candidateType,bytesSent:pair.bytesSent,bytesReceived:pair.bytesReceived})}}}return paths});
     assert.ok(report.selectedPaths.length>0&&report.selectedPaths.every(p=>p.local==='relay'&&p.bytesSent>0&&p.bytesReceived>0));check('messages traversed public relay');
   }
   await c.goto(url);await waitText(c,'public before joining');await waitText(c,'public reply');check('public history backfill');
   await a.getByRole('button',{name:'Peer list',exact:true}).click();
   const bobButton=a.getByRole('button',{name:/Babbage/}).first();
   await bobButton.focus();await bobButton.press('Enter');
   const privateDialog=a.getByRole('dialog').filter({has:a.getByText('Their public key',{exact:true})});
   await privateDialog.getByPlaceholder('Your message').fill('only for Babbage');
   await privateDialog.getByPlaceholder('Your message').press('Enter');
   await privateDialog.getByRole('button',{name:'Close',exact:true}).click();
   await b.getByRole('button',{name:'Peer list',exact:true}).click();
   await b.getByRole('button',{name:/Ada/}).first().click();
   await waitText(b,'only for Babbage');
   assert.equal(await c.getByText('only for Babbage',{exact:true}).count(),0);
   await b.getByRole('dialog').getByRole('button',{name:'Close',exact:true}).click();
   await bobButton.click();await waitText(a,'only for Babbage');
   await privateDialog.getByRole('button',{name:'Close',exact:true}).click();
   await Promise.all([a.getByRole('button',{name:'Close peer list',exact:true}).click(),b.getByRole('button',{name:'Close peer list',exact:true}).click()]);
   check('keyboard private conversation, isolation and retained history');
   await a.getByRole('button',{name:'Copy current URL',exact:true}).click();
   assert.equal(await a.evaluate(()=>navigator.clipboard.readText()),'https://chitchatter.im/public/'+room);check('portable invitation');
   await a.getByRole('button',{name:'toggle camera',exact:true}).click();
   await b.waitForFunction(()=>[...document.querySelectorAll('video')].some(v=>v.srcObject&&v.videoWidth>0),null,{timeout:30000});check('camera received by peer');
   await a.getByRole('button',{name:'call',exact:true}).click();
   await b.waitForFunction(async()=>{for(const pc of window.__testPCs){if(pc.connectionState!=='connected')continue;for(const s of (await pc.getStats()).values()){if(s.type==='inbound-rtp'&&s.kind==='audio'&&s.bytesReceived>0)return true}}return false},null,{timeout:30000});check('microphone received by peer');
   await a.getByRole('button',{name:'call',exact:true}).click();
   await a.getByRole('button',{name:'toggle camera',exact:true}).click();
   await b.waitForFunction(()=>![...document.querySelectorAll('video')].some(v=>v.srcObject),null,{timeout:15000});check('camera stopped');
   await a.getByRole('button',{name:'share screen',exact:true}).click();
   await b.waitForFunction(()=>[...document.querySelectorAll('video')].some(v=>v.srcObject&&v.videoWidth>0),null,{timeout:30000});check('shared screen received by peer');
   await a.getByRole('button',{name:'share screen',exact:true}).click();
   await b.waitForFunction(()=>![...document.querySelectorAll('video')].some(v=>v.srcObject),null,{timeout:15000});check('screen sharing stopped');
   const bytes=Buffer.from('WinOne encrypted file verification\n'.repeat(128));
   await a.locator('#file-upload').setInputFiles({name:'verification.txt',mimeType:'text/plain',buffer:bytes});
   await b.getByRole('button',{name:'Peer list',exact:true}).click();
   const download=b.waitForEvent('download',{timeout:60000});
   // Keep a failed button action from turning shutdown into an unhandled
   // download rejection that obscures the original failure.
   download.catch(()=>{});
   await b.getByRole('button',{name:'Download shared files',exact:true}).click({timeout:30000});
   const received=await download;await received.saveAs(path.join(output,'received.txt'));
   assert.deepEqual(fs.readFileSync(path.join(output,'received.txt')),bytes);check('encrypted file downloaded without corruption');
   await a.getByRole('button',{name:'Share files',exact:true}).click();
   await b.getByRole('button',{name:'Download shared files',exact:true}).waitFor({state:'hidden',timeout:15000});check('file offer cancelled');
   console.log('START room screenshot');
   await a.screenshot({path:path.join(output,'room.png'),timeout:15000});
   // Reload the public participant: the persisted profile must reconnect.
   // Private rooms deliberately clear URL secrets and ask again on reload.
   await b.reload();
   await b.getByRole('button',{name:'Peer list',exact:true}).click();
   await b.getByRole('button',{name:/Ada/}).first().waitFor({timeout:60000});
   await b.getByRole('button',{name:'Close peer list',exact:true}).click();
   await send(b,'message after reconnect');await waitText(a,'message after reconnect');
   check('reload reconnect with persisted identity');
   // Separate private sessions must not receive public history or each other's
   // messages when their passwords differ.
   const privateRoom='private-'+crypto.randomUUID();
   console.log('START private rooms');
   await Promise.all([a.goto(origin+'/private/'+privateRoom+'#secret=one',{waitUntil:'domcontentloaded'}),b.goto(origin+'/private/'+privateRoom+'#secret=one',{waitUntil:'domcontentloaded'}),c.goto(origin+'/private/'+privateRoom+'#secret=two',{waitUntil:'domcontentloaded'})]);
   console.log('PASS private room navigation');
   // Private rooms intentionally do not backfill earlier messages. Wait for
   // the peer before sending; a message typed while alone has no recipient.
   // ICE "connected" precedes the data channel and application handshake.
   // Wait for the peer's identity in both lists before testing private delivery.
   await Promise.all([a.getByRole('button',{name:'Peer list',exact:true}).click(),b.getByRole('button',{name:'Peer list',exact:true}).click()]);
   await a.getByRole('button',{name:/Babbage/}).first().waitFor({timeout:60000});
   await b.getByRole('button',{name:/Ada/}).first().waitFor({timeout:60000});
   await Promise.all([a.getByRole('button',{name:'Close peer list',exact:true}).click(),b.getByRole('button',{name:'Close peer list',exact:true}).click()]);
   await send(a,'private secret');await waitText(b,'private secret');
   assert.equal(await c.getByText('private secret',{exact:true}).count(),0);check('private password isolation');
   assert.deepEqual(report.errors,[]);check('no page errors');
 } catch(error) {
   console.error('CHECK FAILED',error);
   report.failure=error.stack;
   report.pages=[];
   for(const context of browser.contexts()) for(const page of context.pages()) {
     const state=await bounded(page.evaluate(()=>({text:document.body.innerText,connections:(window.__testPCs||[]).map(p=>({connection:p.connectionState,ice:p.iceConnectionState,signaling:p.signalingState}))})),'failure state').catch(error=>({diagnosticError:String(error)}));
     report.pages.push({url:page.url(),frames:page.frames().map(frame=>frame.url()),workers:context.serviceWorkers().map(worker=>worker.url()),...state});
     await page.screenshot({path:path.join(output,'failure-'+report.pages.length+'.png'),timeout:10000}).catch(()=>{});
   }
   throw error;
 } finally {
   fs.writeFileSync(path.join(output,'result.json'),JSON.stringify(report,null,2));
   // Try every cleanup even when browser shutdown stalls. Preserve the
   // original check failure and report cleanup failures separately.
   const cleanup=await Promise.allSettled([
     bounded(browser.close(),'browser shutdown'),
     bounded(new Promise(resolve=>{server.close(resolve);server.closeAllConnections()}),'server shutdown'),
     tracker?bounded(new Promise((resolve,reject)=>tracker.close(error=>error?reject(error):resolve())),'tracker shutdown'):Promise.resolve(),
   ]);
   const failures=cleanup.filter(result=>result.status==='rejected');
   if(failures.length){report.cleanupErrors=failures.map(result=>String(result.reason));fs.writeFileSync(path.join(output,'result.json'),JSON.stringify(report,null,2));if(!report.failure)throw failures[0].reason}
 }
}
main().catch(error=>{report.failure=report.failure||error.stack;fs.writeFileSync(path.join(output,'result.json'),JSON.stringify(report,null,2));console.error(error);process.exit(1)});
