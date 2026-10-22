// Real WinOne -> Rust bridge -> Host -> bundled file search and Pi resources.
// All files and configuration belong to the runner's isolated output directory.
import {createRequire} from 'node:module';
import {mkdir,writeFile,readFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,folder]=process.argv.slice(2),output=resolve(folder),workspace=join(output,'workspace');
let browser,page;const evidence=[],errors=[];
try{
 await mkdir(join(workspace,'docs'),{recursive:true});
 await writeFile(join(workspace,'README.md'),'# Native preview\n\nA file from the isolated workspace.\n');
 await writeFile(join(workspace,'AGENTS.md'),'# Native resource fixture\n\nNo external actions.\n');
 await writeFile(join(workspace,'docs','guide.md'),'Search this native fixture.\n');
 await writeFile(join(output,'private.txt'),'Outside the saved workspace; no preview grant.');
 for(let i=0;i<80;i++)await writeFile(join(workspace,`file-${i}.txt`),'native stat fixture');
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(done=>setTimeout(done,100));}}
 if(!browser)throw Error('Native WinOne test connection did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(page=>page.url().includes('winone.localhost'));if(page)break;await new Promise(done=>setTimeout(done,100));}
 if(!page)throw Error('Native WinOne document not found');page.setDefaultTimeout(15000);page.on('pageerror',error=>errors.push(error.message));
 await page.locator('.app-navigation').waitFor({timeout:30000});
 const result=await page.evaluate(async workspace=>{
  const call=async(method,params={})=>JSON.parse(await window.go.main.App.Chat(method,JSON.stringify(params)));
  const conversation=await call('conversation.new',{directory:workspace,title:'Native file check'});
  const id=conversation.id;
  const activity=await call('activity');
  const resolvedFile=await call('file.resolve',{id,path:'README.md'}),resolvedDirectory=await call('file.resolve',{id,path:'docs'});
  let denied=false;try{await call('file.resolve',{id,path:workspace+'/../private.txt'});}catch{denied=true;}
  // This runs before the renderer opens that conversation and with no model.
  const resources=await call('resources',{id});
  const stats=await call('file.stat',{id,paths:[...Array.from({length:80},(_,i)=>`file-${i}.txt`),'docs','missing.txt']});
  const index=await call('files.index',{id,path:''});const names=await fetch(index.url).then(response=>response.json());
  const search=await call('files.search',{id,path:'',query:'native fixture',caseSensitive:false,maxMatches:20,searchId:'native-proof'});const matches=await fetch(search.url).then(response=>response.json());
  const preview=await call('file.preview',{id,path:'README.md'});const original=await fetch(preview.url).then(response=>response.text());
  await call('file.save',{id,path:'README.md',revision:preview.revision,text:original+'\nSaved through the real bridge.\n'});
  return {id,activity,resolvedFile,resolvedDirectory,denied,resources,stats,index:names,search:matches};
 },workspace);
 assert.equal(typeof result.activity,'object');assert.equal(result.resolvedFile.path,join(workspace,'README.md'));assert.equal(result.resolvedDirectory.path,join(workspace,'docs'));assert.ok(result.denied,'Native resolve admitted an ungranted external file');
 assert.ok(result.resources.agents.some(item=>item.path.endsWith('AGENTS.md')));
 assert.equal(result.stats.files.filter(item=>item.file&&!item.file.directory).length,80);
 assert.ok(result.stats.files.some(item=>item.file?.directory));assert.ok(result.stats.files.some(item=>item.missing));
 assert.ok(result.index.files.includes('README.md'));assert.ok(result.search.totalMatches>0);
 assert.match(await readFile(join(workspace,'README.md'),'utf8'),/Saved through the real bridge/);
 await writeFile(join(output,'native-api-evidence.json'),JSON.stringify(result,null,2));
 evidence.push({scenario:'native file stat / directory / missing / index / search / resource-before-open / save',...result});
 await page.locator('.app-navigation [data-page-id="chat"]').click();
 await page.getByRole('button',{name:'Native file check',exact:true}).click();
 const extra=page.getByRole('button',{name:/^(附件与工具|Attachments and tools)$/});await extra.click();
 await page.getByRole('menuitem',{name:/^(引用工作文件|Reference a work file)$/}).click();
 const panel=page.locator('.chat-side-panel');await panel.getByText(/选择一个文件进行预览|Select a file to preview/).waitFor();
 const filter=panel.getByPlaceholder(/筛选文件|Filter files/);await filter.fill('README');
 await panel.getByText('README.md',{exact:true}).click();await panel.locator('.cm-editor').waitFor();
 assert.equal(await panel.getByRole('alert').count(),0);
 await page.mouse.move(0,0);await page.screenshot({path:join(output,'native-file-browser.png')});
 evidence.push({scenario:'native file tab / filter / preview',panel:await panel.boundingBox()});
 await panel.getByRole('tab',{name:/^(资源|Resources)$/}).click();
 const agent=panel.getByRole('button',{name:/AGENTS\.md/}).filter({hasText:workspace}).last();
 await agent.waitFor();assert.equal(await panel.getByRole('alert').count(),0);
 await page.mouse.move(0,0);await page.screenshot({path:join(output,'native-resources.png')});
 await agent.click();await panel.getByText('Native resource fixture',{exact:true}).waitFor();
 evidence.push({scenario:'native resources tab and resource preview without a configured model'});
 // The same current UI is checked in both colors, without changing system settings.
 await page.evaluate(async()=>{const app=window.go.main.App,value=await app.Snapshot();await app.SaveSettings({...value.settings,theme:'dark',startup:{...value.settings.startup,keepUIRunning:false}});});
 await page.reload({waitUntil:'domcontentloaded'});
 await page.locator('.app-navigation').waitFor();
 // WinOne records theme on data-theme; the .dark class is not its state.
 await page.waitForFunction(()=>document.documentElement.dataset.theme==='dark');
 await page.locator('.app-navigation [data-page-id="chat"]').click();
 await page.getByRole('button',{name:'Native file check',exact:true}).click();
 await page.locator('.chat-composer textarea').waitFor();
 await page.waitForFunction(()=>document.querySelector('.chat-desktop-surface')?.getAttribute('data-theme')==='dark');
 const palette=await page.evaluate(()=>{
  const main=document.querySelector('.chat-main'),composer=document.querySelector('.chat-composer');
  return {main:getComputedStyle(main).backgroundColor,composer:getComputedStyle(composer).backgroundColor,foreground:getComputedStyle(main).color};
 });
 assert.equal(palette.main,'rgb(25, 25, 25)');assert.equal(palette.composer,'rgb(35, 35, 35)');
 evidence.push({scenario:'native dark surfaces follow data-theme',palette});
 await page.mouse.move(0,0);await page.screenshot({path:join(output,'native-dark.png')});
 // Archive changes use the real catalog and durable Host, not a mocked row.
 // Keep standalone collapse set so removing its duplicate heading cannot hide
 // archived tasks with no way to expand them.
 const archiveIDs=await page.evaluate(async workspace=>{
  const call=async(method,params)=>JSON.parse(await window.go.main.App.Chat(method,JSON.stringify(params)));
  const project=await call('project.add',{path:workspace,name:'Archive project',permission:'ask'});
  const member=await call('conversation.new',{projectId:project.id,title:'Archived project task'});
  const solo=await call('conversation.new',{directory:workspace,title:'Archived standalone task'});
  for(const item of [member,solo])await call('conversation.update',{id:item.id,archived:true});
  localStorage.setItem('winone.chat.sections.v1',JSON.stringify({conversations:true}));
  return {member:member.id,solo:solo.id};
 },workspace);
 await page.reload({waitUntil:'domcontentloaded'});await page.locator('.app-navigation').waitFor();
 await page.locator('.app-navigation [data-page-id="chat"]').click();
 const sidebar=page.locator('.chat-sidebar');
 await sidebar.getByRole('button',{name:/^(查看归档|View archive)$/}).click();
 assert.equal(await sidebar.getByRole('heading',{name:/^(已归档|Archived)$/}).count(),1);
 assert.equal(await sidebar.getByRole('heading',{name:/^(项目|Projects|对话|Chats)$/}).count(),0);
 const archivedRows=[];
 for(const id of Object.values(archiveIDs)){
  const row=sidebar.locator(`[data-session-id="${id}"] .chat-sidebar-row`);await row.waitFor();
  const placement=await row.evaluate(row=>({padding:getComputedStyle(row).paddingLeft,leading:row.firstElementChild.className,overflow:row.scrollWidth>row.clientWidth}));
  assert.equal(placement.padding,'10px','Project rows must not indent task names');
  assert.equal(placement.leading,'chat-sidebar-title','Idle rows must not reserve an icon column');
  assert.equal(placement.overflow,false);archivedRows.push(placement);
 }
 await page.mouse.move(0,0);await page.screenshot({path:join(output,'native-archived.png')});
 await sidebar.getByRole('button',{name:/^(返回对话|Back to chats)$/}).click();
 await sidebar.getByRole('heading',{name:/^(项目|Projects)$/}).waitFor();
 assert.equal(await sidebar.getByRole('button',{name:/^(展开对话|Expand Chats)$/}).getAttribute('aria-expanded'),'false');
 evidence.push({scenario:'single archive heading / standalone archive remains visible / no extra project-name indentation / collapse preference retained',archivedRows});
 assert.deepEqual(errors,[]);
 await writeFile(join(output,'result.json'),JSON.stringify({native:true,evidence,errors},null,2));
 await page.evaluate(()=>{void window.go.main.App.WindowCommand('close');});
}catch(error){
 if(page)await page.screenshot({path:join(output,'failure.png')}).catch(()=>{});
 await writeFile(join(output,'node-error.txt'),String(error)+'\n'+error.stack+'\n'+errors.join('\n'));process.exitCode=1;
}finally{await browser?.close();}
