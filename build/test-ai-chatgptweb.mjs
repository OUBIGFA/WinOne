// Explicit integration with the author's plugin and local test website. Pass
// a reviewed checkout; the only adapter substitutes its origin, never auth.
import {mkdir,writeFile,readFile,readdir,cp,access} from 'node:fs/promises';
import {resolve,join,relative} from 'node:path';
import {pathToFileURL} from 'node:url';
import {spawn} from 'node:child_process';
const [checkoutArg,outputArg,executableArg]=process.argv.slice(2),checkout=resolve(checkoutArg),output=resolve(outputArg);
if(!relative(resolve('_temp'),output)||relative(resolve('_temp'),output).startsWith('..'))throw Error('Choose a new repository _temp directory');
await mkdir(output,{recursive:false});
const {fixture,browserExecutable}=await import(pathToFileURL(join(checkout,'test/helpers.mjs'))),site=await fixture();
// Explicit test options let the cold-start case use the plugin's own browser
// discovery/installation instead of always bypassing it with an executable.
const options=process.env.WINONE_CHATGPT_WEB_TEST_OPTIONS ? JSON.parse(process.env.WINONE_CHATGPT_WEB_TEST_OPTIONS) : {profilesDir:join(output,'profiles'),executablePath:await browserExecutable(),headless:true,browserMode:'keep-open',pollMs:50,settleMs:150};
options.profilesDir??=join(output,'profiles');
// A real user cannot finish authorization before the window is shown. Hold
// the local site's session until the native-window assertion confirms this.
const visibleMarker=!options.headless&&!options.executablePath ? join(output,'browser-visible.ready') : '';
if(visibleMarker)site.state.authenticated=false;
const authorize=visibleMarker ? setInterval(()=>{void access(visibleMarker).then(()=>{site.state.authenticated=true;}).catch(()=>{});},25) : null;
// Match Magpie's installed-code layout. Browser assets belong to the code
// tree; credentials and profiles are siblings in the private runtime home.
const dataDirectory=join(output,'runtime'),entry=join(dataDirectory,'third-party','plugins','node_modules','chatgptweb-local-fixture');await mkdir(entry,{recursive:true});
const author=join(entry,'author');await mkdir(author);
for(const name of ['dist','node_modules'])await cp(join(checkout,name),join(author,name),{recursive:true});
if((await readdir(checkout)).includes('.playwright-browsers'))await cp(join(checkout,'.playwright-browsers'),join(author,'.playwright-browsers'),{recursive:true});
await cp(join(checkout,'package.json'),join(author,'package.json'));
await writeFile(join(entry,'package.json'),JSON.stringify({name:'chatgptweb-local-fixture',type:'module',main:'index.mjs'}));
await writeFile(join(entry,'index.mjs'),[
 `import {createPluginHooks} from ${JSON.stringify(pathToFileURL(join(author,'dist/plugin.js')).href)};`,
 `import {BrowserBridge} from ${JSON.stringify(pathToFileURL(join(author,'dist/bridge.js')).href)};`,
 `import {parseOptions} from ${JSON.stringify(pathToFileURL(join(author,'dist/options.js')).href)};`,
 `export default {server:async(_input,options)=>createPluginHooks(options,new BrowserBridge(parseOptions(options),${JSON.stringify(site.url)}))};`
].join('\n'));
try{
 const code=await new Promise((done,reject)=>{
  const child=spawn(process.env.WINONE_GO_EXE||'go',['test','./internal/ai','-run','^TestChatGPTWebPluginSignInAndReply$','-count=1','-v','-timeout=360s'],{stdio:'inherit',env:{...process.env,WINONE_AI_PLUGIN_TEST_EXE:resolve(executableArg),WINONE_CHATGPT_WEB_DATA:dataDirectory,WINONE_CHATGPT_WEB_FIXTURE:entry,WINONE_CHATGPT_WEB_OPTIONS:JSON.stringify(options),WINONE_CHATGPT_WEB_VISIBLE_MARKER:visibleMarker,TEMP:output,TMP:output}});
  child.on('error',reject);child.on('exit',done);
 });
 if(!Number.isInteger(code))throw Error('Browser integration runner exited without a status');
 await writeFile(join(output,'test-result.json'),JSON.stringify({exitCode:code}));
 process.exitCode=code;
}finally{
 if(authorize)clearInterval(authorize);
 // The plugin intentionally preserves its isolated login browser. Close only
 // the fixture's profiles after checking short-lived WinOne workers.
 const {chromium}=await import(pathToFileURL(join(checkout,'node_modules/playwright/index.mjs')));
 for(const name of await readdir(options.profilesDir).catch(()=>[])){
  let browser;try{const [port,path]=(await readFile(join(options.profilesDir,name,'DevToolsActivePort'),'utf8')).trim().split(/\r?\n/);browser=await chromium.connectOverCDP(`ws://127.0.0.1:${port}${path}`,{timeout:1000});const session=await browser.newBrowserCDPSession();await session.send('Browser.close');}catch{/* Owned profile already stopped. */}finally{await browser?.close().catch(()=>{});}
 }
 await site.close();
}
