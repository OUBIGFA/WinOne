// Explicit integration with the author's plugin and local test website. Pass
// a reviewed checkout; the only adapter substitutes its origin, never auth.
import {mkdir,writeFile,readFile,readdir} from 'node:fs/promises';
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
const entry=join(output,'plugin');await mkdir(entry);
await writeFile(join(entry,'package.json'),JSON.stringify({name:'chatgptweb-local-fixture',type:'module',main:'index.mjs'}));
await writeFile(join(entry,'index.mjs'),[
 `import {createPluginHooks} from ${JSON.stringify(pathToFileURL(join(checkout,'dist/plugin.js')).href)};`,
 `import {BrowserBridge} from ${JSON.stringify(pathToFileURL(join(checkout,'dist/bridge.js')).href)};`,
 `import {parseOptions} from ${JSON.stringify(pathToFileURL(join(checkout,'dist/options.js')).href)};`,
 `export default {server:async(_input,options)=>createPluginHooks(options,new BrowserBridge(parseOptions(options),${JSON.stringify(site.url)}))};`
].join('\n'));
try{
 const code=await new Promise((done,reject)=>{
  const child=spawn(process.env.WINONE_GO_EXE||'go',['test','./internal/ai','-run','^TestChatGPTWebPluginSignInAndReply$','-count=1','-v','-timeout=360s'],{stdio:'inherit',env:{...process.env,WINONE_AI_PLUGIN_TEST_EXE:resolve(executableArg),WINONE_CHATGPT_WEB_FIXTURE:entry,WINONE_CHATGPT_WEB_OPTIONS:JSON.stringify(options),TEMP:output,TMP:output}});
  child.on('error',reject);child.on('exit',done);
 });
 process.exitCode=code||0;
}finally{
 // The plugin intentionally preserves its isolated login browser. Close only
 // the fixture's profiles after checking short-lived WinOne workers.
 const {chromium}=await import(pathToFileURL(join(checkout,'node_modules/playwright/index.mjs')));
 for(const name of await readdir(options.profilesDir).catch(()=>[])){
  let browser;try{const [port,path]=(await readFile(join(options.profilesDir,name,'DevToolsActivePort'),'utf8')).trim().split(/\r?\n/);browser=await chromium.connectOverCDP(`ws://127.0.0.1:${port}${path}`,{timeout:1000});const session=await browser.newBrowserCDPSession();await session.send('Browser.close');}catch{/* Owned profile already stopped. */}finally{await browser?.close().catch(()=>{});}
 }
 await site.close();
}
