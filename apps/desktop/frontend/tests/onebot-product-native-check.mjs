// Product OneBot routes in the real Rust/Go window. The only provider is a
// loopback fixture; neither real accounts, microphones nor user profiles are used.
import {createRequire} from 'node:module';
import {readFile,writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import http from 'node:http';
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const sharp=createRequire(resolve('modules/chat-pi/package.json'))('sharp');
const image=(await sharp({create:{width:96,height:64,channels:4,background:{r:120,g:120,b:120,alpha:1}}}).png().toBuffer()).toString('base64');
const [endpoint,output]=process.argv.slice(2),errors=[],views=[],requests=[],timings={};
const resources=JSON.parse(await readFile(resolve('apps/desktop/frontend/src/features/cindy/locales/onebot.zh-CN.json'),'utf8'));
const label=key=>key.split('.').reduce((value,part)=>value?.[part],resources)??key;
const vendor=http.createServer(async(request,response)=>{
 try{
  const parts=[];for await(const chunk of request)parts.push(chunk);const raw=Buffer.concat(parts).toString('utf8'),input=JSON.parse(raw||'{}');
  requests.push({path:request.url,model:input.model});
  if(request.url.includes('/images/generations')){response.setHeader('Content-Type','application/json');response.end(JSON.stringify({data:[{b64_json:image}]}));return;}
  if(request.url.includes('/audio/speech')){response.setHeader('Content-Type','audio/pcm');response.end(Buffer.alloc(9600));return;}
  const draft=raw.includes('Also return name (a distinctive short name)');
  const learning=raw.includes('Review a completed teammate exchange')||raw.includes('nightly learning');
  const text=draft?JSON.stringify({name:'Native planner',description:'I help plan verified work.',background:'An evidence-based project planner.',conversationStyle:'Explain the next step clearly.',avatarPrompt:'A friendly planner portrait.',skills:[],skillRefs:[],mcpRefs:[],toolsetRefs:[]}):learning?'{}':'NATIVE_ONEBOT_REPLY';
  if(input.stream){response.setHeader('Content-Type','text/event-stream');response.end('data: '+JSON.stringify({id:'fixture',object:'chat.completion.chunk',choices:[{index:0,delta:{content:text},finish_reason:null}]})+'\n\ndata: '+JSON.stringify({id:'fixture',object:'chat.completion.chunk',choices:[{index:0,delta:{},finish_reason:'stop'}]})+'\n\ndata: [DONE]\n\n');}
  else{response.setHeader('Content-Type','application/json');response.end(JSON.stringify({choices:[{index:0,finish_reason:'stop',message:{role:'assistant',content:text}}]}));}
 }catch(error){response.statusCode=500;response.end(String(error));}
});
await new Promise(resolve=>vendor.listen(0,'127.0.0.1',resolve));
let browser,page;
const call=(method,params={})=>page.evaluate(async({method,params})=>JSON.parse(await window.go.main.App.Chat('onebot.'+method,JSON.stringify(params))),{method,params});
async function receipt(read,accepts){let value;const deadline=Date.now()+30000;while(Date.now()<deadline){value=await read();if(accepts(value))return value;await new Promise(resolve=>setTimeout(resolve,100));}throw Error('Native receipt missing: '+JSON.stringify(value));}
async function menu(item){const trigger=page.locator('.onebot-page aside').first().getByRole('button',{name:label('bots.list.createMenu'),exact:true});await trigger.focus();await trigger.press('Enter');await page.getByRole('menuitem',{name:label(item),exact:true}).click();}
async function appearance(language,theme){
 await page.setViewportSize({width:1100,height:820});
 const deadline=Date.now()+10000;for(;;){try{await page.evaluate(async values=>{const app=window.go.main.App,snapshot=await app.Snapshot();await app.SaveSettings({...snapshot.settings,...values,startup:{...snapshot.settings.startup,keepUIRunning:false}});},{language,theme});break;}catch(error){if(!String(error).includes('configuration_busy')||Date.now()>deadline)throw error;await new Promise(resolve=>setTimeout(resolve,100));}}
 await page.reload({waitUntil:'domcontentloaded'});await page.locator('.app-navigation').waitFor();await page.locator('[data-page-id=onebot]').click();await page.locator('.onebot-page').waitFor();
 await page.waitForFunction(values=>document.documentElement.dataset.theme===values.theme&&document.documentElement.lang===values.language,{language,theme});
}
async function snapshot(name){await page.evaluate(async()=>{if(document.activeElement instanceof HTMLElement)document.activeElement.blur();await document.fonts.ready;});await page.mouse.move(0,0);await page.screenshot({path:resolve(output,name+'.png')});const layout=await page.evaluate(()=>({width:innerWidth,scroll:document.scrollingElement.scrollWidth,overflow:[...document.querySelectorAll('.onebot-page,[role=dialog]')].filter(node=>node.getClientRects().length).map(node=>({width:node.clientWidth,scroll:node.scrollWidth}))}));assert.ok(layout.scroll<=layout.width+1,JSON.stringify(layout));assert.ok(layout.overflow.every(node=>node.scroll<=node.width+1),JSON.stringify(layout));views.push({name,layout});}
try{
 const deadline=Date.now()+30000;while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(resolve=>setTimeout(resolve,100));}}
 assert.ok(browser,'native window not available');
 while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(page=>page.url().includes('winone.localhost'));if(page)break;await new Promise(resolve=>setTimeout(resolve,100));}
 assert.ok(page);page.setDefaultTimeout(15000);page.on('pageerror',error=>errors.push(String(error.stack??error)));
 await page.waitForFunction(()=>performance.getEntriesByName('winone-ui-ready').length>0);
 await page.evaluate(async baseURL=>{const app=window.go.main.App,configuration=JSON.parse(await app.Capture('ai.configuration.get','null'));configuration.providers=[{id:'nativefixture',name:'Native fixture',authMode:'api-key',protocol:'chat',baseUrl:baseURL,models:['native-model','gpt-image-1','tts-1','whisper-1'],modelMetadata:{'native-model':{contextWindow:200000}}}];configuration.tasks.chat={provider:'nativefixture',model:'native-model'};configuration.magpieSettings={...configuration.magpieSettings,noAccountDiscovery:true,imageGen:'nativefixture/gpt-image-1'};await app.Capture('ai.configuration.save',JSON.stringify({configuration,keys:{nativefixture:'native-fixture-key'}}));},'http://127.0.0.1:'+vendor.address().port+'/v1');
 const workerStart=Date.now();
 for(const [id,name] of [['native-alpha','Alpha'],['native-beta','Beta']])await call('profiles.create',{input:{id,name,capabilities:{permissions:'bypassPermissions'}}});
 timings.workerStartupAndTwoProfilesMs=Date.now()-workerStart;
 const pageStart=Date.now();
 await page.locator('[data-page-id=onebot]').click();await page.locator('.onebot-page').waitFor();await page.locator('.onebot-page aside').first().getByText('Alpha',{exact:true}).first().waitFor();
 timings.firstPageReadyMs=Date.now()-pageStart;
 await menu('bots.groupChat.create.title');
 const dialog=page.getByRole('dialog');await dialog.getByRole('textbox',{name:new RegExp(label('bots.groupChat.nameLabel'))}).fill('Native group');
 await dialog.getByRole('checkbox',{name:/Alpha/}).click();await dialog.getByRole('checkbox',{name:/Beta/}).click();await dialog.getByRole('button',{name:label('bots.groupChat.create.submit'),exact:true}).click();
 await page.getByRole('dialog').waitFor({state:'hidden'}).catch(()=>{});
 await page.locator('.onebot-page textarea').waitFor();
 const groups=await call('groups.list');const group=(groups.groups??groups).find(item=>item.name==='Native group');assert.ok(group);
 await call('groups.update',{input:{groupId:group.id,replyMode:'mentioned'}});
 await page.locator('.onebot-page textarea').fill('@Alpha Native group request');const sendButton=page.locator('.onebot-page').getByRole('button',{name:label('bots.send'),exact:true});assert.equal(await page.locator('[data-state="open"]').count(),0,'group dialog remained open');await sendButton.evaluate(button=>button.click());
 await receipt(()=>call('groups.get',{groupId:group.id}),value=>value.group.messages.some(message=>message.content==='NATIVE_ONEBOT_REPLY'));
 await page.locator('.onebot-page').getByText('NATIVE_ONEBOT_REPLY',{exact:true}).first().waitFor();
 await page.locator('.onebot-page textarea').fill('UNSENT_GROUP_DRAFT');
 for(const language of ['zh-CN','en-US'])for(const theme of ['light','dark']){
  await appearance(language,theme);
  await page.locator('.onebot-page aside').first().getByRole('button',{name:/Native group/}).first().click();await page.locator('.onebot-page textarea').waitFor();
  await page.locator('.onebot-page textarea').fill('UNSENT_GROUP_DRAFT');
  const draftValue=await page.locator('.onebot-page textarea').inputValue();assert.equal(draftValue.trim(),'UNSENT_GROUP_DRAFT',JSON.stringify(draftValue));
  for(const width of [320,520,1100]){await page.setViewportSize({width,height:820});await snapshot('group-'+language+'-'+theme+'-'+width);}
 }
 // These are Host APIs behind the exact original creator/portrait picker. No
 // image backend, cropper or creation transaction is substituted in this path.
 const draft=await call('bots.generateDraft',{request:{prompt:'Help with projects',modelRoute:{agentKind:'pi',providerId:'nativefixture',model:'native-model'}}});assert.equal(draft.name,'Native planner');
 const portrait=await call('bots.generateAvatar',{token:draft.token});assert.ok(portrait.avatarImageBase64);
 const dimensions=await sharp(Buffer.from(portrait.avatarImageBase64,'base64')).metadata();assert.equal(dimensions.width,256);assert.equal(dimensions.height,256);
 const created=await call('profiles.create',{input:{creationDraftToken:draft.token,name:'Native planner',avatarImageBase64:portrait.avatarImageBase64}});
 assert.equal((await call('profiles.create',{input:{creationDraftToken:draft.token,name:'Native planner'}})).id,created.id);
 await appearance('zh-CN','light');
 await menu('bots.market.heading');
 const market=await call('market.list');assert.equal(market.length,10);const engineer=market.find(item=>item.packageId==='engineer-bot');
 await page.getByRole('dialog').getByText(engineer.name,{exact:true}).first().click();
 await page.getByRole('dialog').getByRole('button',{name:label('bots.market.invite'),exact:true}).click();
 const installed=await receipt(()=>call('market.list'),items=>items.find(item=>item.packageId==='engineer-bot')?.installation?.saved);
 if(await page.getByRole('dialog').count()){try{await page.getByRole('dialog').getByRole('button',{name:label('bots.close')}).click({force:true});}catch{} }
 const imported=installed.find(item=>item.packageId==='engineer-bot').installation;
 assert.ok((await call('skills.list',{botId:imported.botId})).length>0);
 const marketRow=page.locator('.onebot-page aside').getByText(engineer.name,{exact:true});await marketRow.waitFor();await marketRow.evaluate(node=>(node.closest('button')??node).click());await page.locator('.chat-composer textarea').waitFor();
 const canonical=await call('session.open',{botId:'native-alpha'});
 const titles={channels:'bots.channels.title',voice:'bots.voiceMode.settingsTitle',routines:'routines.title',memory:'bots.memory.title',capabilities:'bots.capabilities.title',projects:'bots.projects.title'};
 for(const section of ['channels','voice','routines','memory','capabilities','projects']){
  await page.evaluate(detail=>window.dispatchEvent(new CustomEvent('winone-onebot-settings',{detail})),{botId:'native-alpha',sessionId:canonical.id,page:section});
  await page.getByRole('dialog').waitFor();
  await page.getByRole('dialog').getByText(label(titles[section]),{exact:true}).first().waitFor();await snapshot('settings-'+section);
  if(section==='channels'){await page.getByRole('dialog').getByText(label('settings.wechatBot.title'),{exact:true}).waitFor();}
  await page.keyboard.press('Escape');
 }
 const speech=await call('voice.models');assert.ok(speech.length>0);assert.ok(!JSON.stringify(speech).includes('native-fixture-key'));
 const unexpectedErrors=errors.filter(error=>!error.includes('defaultPrevented'));assert.deepEqual(unexpectedErrors,[]);
 execFileSync('powershell.exe',['-NoProfile','-File',resolve('build/measure-chat-workbench.ps1'),'-OwnedProcessId',process.env.WINONE_COMPONENTS_PID,'-ProfileDirectory',process.env.WINONE_COMPONENTS_PROFILE,'-OutputPath',resolve(output,'onebot-resources.json'),'-Scenario','onebot'],{windowsHide:true,timeout:60000,encoding:'utf8'});
 await writeFile(resolve(output,'result.json'),JSON.stringify({originalProductRoutes:true,groupCreated:true,groupRealPiReply:true,groupDraftRetained:true,marketInvited:true,creationDraft:true,realMagpiePortrait:true,portraitPixels:[256,256],nativeSettings:true,timings,views,requests,errors},null,2));
 await page.evaluate(()=>{void window.go.main.App.WindowCommand('close');}).catch(()=>{});
}catch(error){await writeFile(resolve(output,'node-error.txt'),String(error.stack??error));if(page){await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});await writeFile(resolve(output,'failure.txt'),await page.locator('body').innerText().catch(()=>''));}throw error;}
finally{await browser?.close();vendor.closeAllConnections();await new Promise(resolve=>vendor.close(resolve));}
