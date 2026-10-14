// Exercise the real desktop/Host/WebView/ConPTY against a test-owned repository.
// The only network endpoint is this process's loopback page and model fixture.
import {createRequire} from 'node:module';
import {mkdir,readFile,writeFile,stat,symlink} from 'node:fs/promises';
import {resolve} from 'node:path';
import {execFileSync} from 'node:child_process';
import http from 'node:http';
import {once} from 'node:events';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,outputArg]=process.argv.slice(2),output=resolve(outputArg);
const sampleResources=scenario=>{if(['1',scenario].includes(process.env.WINONE_WORKBENCH_RESOURCES))execFileSync('powershell.exe',['-NoProfile','-File',resolve('build/measure-chat-workbench.ps1'),'-OwnedProcessId',process.env.WINONE_COMPONENTS_PID,'-ProfileDirectory',process.env.WINONE_COMPONENTS_PROFILE,'-OutputPath',resolve(output,`resources-${scenario}.json`),'-Scenario',scenario],{windowsHide:true,encoding:'utf8',timeout:60000});};
if(new URL(endpoint).hostname!=='127.0.0.1'||!process.env.WINONE_COMPONENTS_PROFILE?.startsWith(output))throw Error('Use the isolated desktop launcher');
const project=resolve(output,'project'),archive=resolve(output,'history');await mkdir(project,{recursive:true});await mkdir(archive,{recursive:true});
const git=(...args)=>execFileSync('git',['-C',project,...args],{windowsHide:true,encoding:'utf8',env:{...process.env,GIT_CONFIG_GLOBAL:'NUL',GIT_CONFIG_NOSYSTEM:'1'}});
git('init');git('config','user.name','WinOne fixture');git('config','user.email','fixture@example.test');git('config','core.hooksPath','');
await writeFile(resolve(project,'note.txt'),'Original\n');await writeFile(resolve(project,'.gitignore'),'terminal-proof.txt\nlinked-skills/\n');
await writeFile(resolve(project,'picture.png'),Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aHfoAAAAASUVORK5CYII=','base64'));
// A complete single-page PDF exercises WebView2's actual document renderer.
const objects=['<< /Type /Catalog /Pages 2 0 R >>','<< /Type /Pages /Kids [3 0 R] /Count 1 >>','<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>','<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>'];
const stream='BT /F1 18 Tf 40 230 Td (WinOne PDF preview) Tj ET';objects.push(`<< /Length ${stream.length} >>\nstream\n${stream}\nendstream`);
let pdf='%PDF-1.4\n',offsets=[0];for(const [index,object] of objects.entries()){offsets.push(Buffer.byteLength(pdf));pdf+=`${index+1} 0 obj\n${object}\nendobj\n`;}const xref=Buffer.byteLength(pdf);pdf+=`xref\n0 ${objects.length+1}\n0000000000 65535 f \n`+offsets.slice(1).map(offset=>String(offset).padStart(10,'0')+' 00000 n \n').join('')+`trailer\n<< /Size ${objects.length+1} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF\n`;
await writeFile(resolve(project,'sample.pdf'),pdf);git('add','.');git('commit','-m','Initial');await writeFile(resolve(project,'note.txt'),'Modified\n');
const skillSource=resolve(output,'linked-skill-source'),skillAlias=resolve(project,'linked-skills');
await mkdir(resolve(skillSource,'fixture-skill'),{recursive:true});await writeFile(resolve(skillSource,'fixture-skill','SKILL.md'),'---\nname: fixture-skill\ndescription: Native linked resource preview fixture\n---\n\n# Linked skill preview\n\nThe complete linked resource is readable.\n');
await symlink(skillSource,skillAlias,'junction');
const server=http.createServer((req,res)=>{
 res.setHeader('Content-Type','text/html; charset=utf-8');
 res.end('<!doctype html><html><head><title>Workbench page</title></head><body style="font:24px sans-serif;padding:30px"><p id="target">Selected browser evidence</p><a href="/second">Next page</a><script>setTimeout(()=>{const range=document.createRange();range.selectNodeContents(document.getElementById("target"));getSelection().removeAllRanges();getSelection().addRange(range);},500);</script></body></html>');
});server.listen(0,'127.0.0.1');await once(server,'listening');
let browser,page;
try{
 const deadline=Date.now()+30000;while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await new Promise(done=>setTimeout(done,100));}}
 if(!browser)throw Error('WebView did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(c=>c.pages()).find(p=>p.url().includes('winone.localhost'));if(page)break;await new Promise(done=>setTimeout(done,100));}
 if(!page)throw Error('WinOne document missing');await page.getByRole('navigation').waitFor();
 const errors=[],consoleErrors=[];page.on('pageerror',error=>errors.push(String(error)));page.on('console',message=>{if(message.type()==='error')consoleErrors.push(message.text());});
 const call=(method,params={})=>page.evaluate(async({method,params})=>JSON.parse(await window.go.main.App.Chat(method,JSON.stringify(params))),{method,params});
 await page.evaluate(async({baseUrl})=>{const api=window.go.main.App,configuration=JSON.parse(await api.Capture('ai.configuration.get','null'));configuration.providers=[{id:'fixture',name:'Fixture',authMode:'api-key',protocol:'chat',baseUrl,models:['fixture-model'],hasKey:false}];configuration.tasks={default:{provider:'fixture',model:'fixture-model'},chat:{provider:'fixture',model:'fixture-model'}};await api.Capture('ai.configuration.save',JSON.stringify({configuration,keys:{fixture:'fixture-key'}}));},{baseUrl:`http://127.0.0.1:${server.address().port}/v1`});
 const preferences=await call('settings.get');await call('settings.save',{settings:{...preferences,skillDirectories:[skillAlias]}});
 const selected=await call('project.add',{path:project,name:'Workbench project'}),conversation=await call('conversation.new',{projectId:selected.id,title:'Workbench check',model:{provider:'fixture',model:'fixture-model'},saveDirectory:archive});
 await page.getByRole('button',{name:'Chat',exact:true}).click();await page.getByRole('button',{name:'Workbench check',exact:true}).click();await page.locator('.chat-composer textarea').waitFor();
 await page.getByRole('button',{name:'显示会话详情',exact:true}).click();await page.getByRole('button',{name:/^文件.*Ctrl/}).click();await page.getByRole('button',{name:/^note.txt/}).click();await page.locator('.cm-content').waitFor();
 await page.locator('.cm-content').fill('Edited in native preview\n');await page.locator('.cm-content').press('Control+s');
 await page.waitForFunction(()=>!document.querySelector('.chat-pane-tab-target small'));
 if(await readFile(resolve(project,'note.txt'),'utf8')!=='Edited in native preview\n')throw Error('Preview did not save actual file');
 await page.getByRole('tab',{name:'文件',exact:true}).click();await page.getByRole('button',{name:/^picture.png/}).click();await page.waitForFunction(()=>document.querySelector('.chat-image-preview img')?.naturalWidth===1);
 await page.getByRole('tab',{name:'文件',exact:true}).click();const pdfNavigation=page.waitForResponse(response=>response.url().endsWith('/sample.pdf'));await page.getByRole('button',{name:/^sample.pdf/}).click();await page.locator('[data-pdf-rendered=true]').waitFor();await page.locator('.textLayer').getByText('WinOne PDF preview').waitFor();
 const pdfResponse=await pdfNavigation;await pdfResponse.finished();if(pdfResponse.status()!==200||!pdfResponse.headers()['content-type']?.includes('application/pdf'))throw Error('Native PDF response was blocked');
 // The local PDF.js renderer must paint a page and expose selectable text.
 await page.waitForLoadState('networkidle',{timeout:10000});await writeFile(resolve(output,'pdf-frames.json'),JSON.stringify({consoleErrors,frames:page.frames().map(frame=>({url:frame.url()}))},null,2));await page.screenshot({path:resolve(output,'pdf-preview.png')});
 const resources=await call('resources',{id:conversation.id}),skill=resources.skills.find(value=>value.name==='fixture-skill');if(!skill)throw Error('Linked skill was not discovered by the actual Pi loader');
 const skillPreview=await call('file.preview',{id:conversation.id,path:skill.path});const skillContent=await page.request.get(skillPreview.url);if(skillContent.status()!==200||!(await skillContent.text()).includes('The complete linked resource is readable.'))throw Error('Discovered linked skill cannot be previewed');
 await page.getByRole('button',{name:'标签页操作',exact:true}).click();await page.getByRole('menuitem',{name:'资源',exact:true}).click();await page.getByRole('button',{name:/fixture-skill/}).click();await page.getByRole('heading',{name:'Linked skill preview',exact:true}).waitFor();
 const newTab=async name=>{await page.getByRole('button',{name:'新建标签页',exact:true}).click();if(name!=='浏览器')await page.locator('.client-tools-grid').getByRole('button',{name:new RegExp('^'+(name==='审查'?'更改':name))}).click();};
 await newTab('终端');await page.waitForFunction(()=>document.querySelector('.chat-terminal .xterm-helper-textarea')&&!document.querySelector('.chat-terminal [role=status]'));await page.locator('.chat-terminal .xterm-helper-textarea').focus();await page.keyboard.type('Set-Content terminal-proof.txt NATIVE_TERMINAL_OK');await page.keyboard.press('Enter');
 const terminalID=await page.evaluate(id=>JSON.parse(localStorage.getItem(`winone.chat.panel.v1:${id}`)).tabs.find(tab=>tab.kind==='terminal').id,conversation.id);
 let terminalProof='';const terminalDeadline=Date.now()+15000;while(Date.now()<terminalDeadline){try{terminalProof=await readFile(resolve(project,'terminal-proof.txt'),'utf8');}catch(error){if(error.code!=='ENOENT')throw error;}if(terminalProof.trim()==='NATIVE_TERMINAL_OK')break;await new Promise(done=>setTimeout(done,100));}
 if(terminalProof.trim()!=='NATIVE_TERMINAL_OK')throw Error('Keyboard input did not reach the real terminal');
 sampleResources('terminal');
 await page.getByRole('button',{name:'关闭 终端',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'关闭终端',exact:true}).click();
 await newTab('审查');await page.locator('.chat-review-files button').filter({hasText:'note.txt'}).waitFor();await page.locator('.chat-review-file').getByRole('button',{name:'暂存',exact:true}).click();
 await page.waitForFunction(()=>document.querySelector('.chat-review-diff')?.textContent.includes('没有文件改动'));
 if(!git('status','--porcelain').startsWith('M '))throw Error('Review did not stage the selected file');
 await page.getByRole('combobox',{name:'审查范围'}).click();await page.getByRole('option',{name:'已暂存',exact:true}).click();await page.locator('.chat-review-file').getByRole('button',{name:'取消暂存',exact:true}).click();
 await page.waitForFunction(()=>document.querySelector('.chat-review-diff')?.textContent.includes('没有文件改动'));
 if(!git('status','--porcelain').startsWith(' M'))throw Error('Review did not unstage the selected file');
 await newTab('浏览器');const address=page.getByRole('textbox',{name:'地址或搜索'});await address.fill(`http://127.0.0.1:${server.address().port}/`);await address.press('Enter');
 await page.getByRole('tab',{name:'Workbench page',exact:true}).waitFor();
 const browserID=await page.evaluate(id=>JSON.parse(localStorage.getItem(`winone.chat.panel.v1:${id}`)).tabs.find(tab=>tab.kind==='browser').id,conversation.id);
 const browserCall=(method,value={})=>page.evaluate(async args=>JSON.parse(await window.go.main.App.ChatBrowser(args.method,JSON.stringify(args.value))),{method,value:{sessionId:conversation.id,tabId:browserID,...value}});
 await page.evaluate(()=>{window.workbenchBrowserEvents=[];window.addEventListener('winone:chat-browser',event=>window.workbenchBrowserEvents.push(event.detail));});
 await page.waitForTimeout(700);await page.getByRole('button',{name:'评论页面',exact:true}).click();await page.getByPlaceholder('描述你希望修改的内容').fill('Keep this evidence');await page.getByRole('button',{name:'添加评论',exact:true}).click();
 await page.locator('.chat-attachments').getByText('#1 Keep this evidence',{exact:true}).waitFor();
 const draft=await call('draft.get',{id:conversation.id});const annotation=draft.attachments?.find(file=>file.annotation);
 if(!annotation||annotation.annotation.target.selectedText!=='Selected browser evidence'||(await stat(annotation.path)).size<100)throw Error('Native annotation screenshot/target was not persisted');
 await page.screenshot({path:resolve(output,'browser-comment.png')});
 sampleResources('browser');
 await browserCall('navigate',{url:`http://127.0.0.1:${server.address().port}/second`});
 let state;const navigationDeadline=Date.now()+10000;while(Date.now()<navigationDeadline){state=await browserCall('state');if(state.canGoBack&&!state.loading)break;await new Promise(done=>setTimeout(done,100));}if(!state?.canGoBack)throw Error('Browser history missing');
 await browserCall('back');await browserCall('terminate');state=await browserCall('reload');if(state.crashed)throw Error('Browser did not recover after release');
 if(errors.length)throw Error(errors.join('\n'));
 await page.getByRole('button',{name:'新建标签页',exact:true}).click();await page.locator('.client-tools-grid').getByRole('button',{name:'新建页面',exact:true}).click();await page.getByRole('textbox',{name:'文件名',exact:true}).fill('Created page.md');await page.getByRole('dialog').getByRole('button',{name:'创建',exact:true}).click();await page.getByRole('tab',{name:'Created page.md',exact:true}).waitFor();if((await stat(resolve(project,'Created page.md'))).size!==0)throw Error('New page was not created as an empty file');
 await writeFile(resolve(output,'result.json'),JSON.stringify({linkedSkillPreview:true,fileSaved:true,imageLoaded:true,pdfDelivered:true,newPageCreated:true,terminalInteractive:true,terminalCloseProtected:true,reviewStageUnstage:true,browserNavigation:true,browserCommentScreenshot:true,browserRecovery:true,errors},null,2));
 await call('draft.save',{id:conversation.id,draft:{text:'',attachments:[]}});await page.locator('.chat-composer textarea').fill('');
 execFileSync('powershell.exe',['-NoProfile','-File',resolve('apps/desktop/frontend/tests/native-window-state.ps1'),'-OwnedProcessId',process.env.WINONE_COMPONENTS_PID,'-Executable',resolve(output,'WinOne.exe'),'-Action','exit'],{windowsHide:true,encoding:'utf8',timeout:10000});
}catch(error){await writeFile(resolve(output,'node-error.txt'),String(error.stack||error));if(page){await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});await writeFile(resolve(output,'failure.txt'),await page.locator('body').innerText().catch(()=>''));}throw error;}
finally{await browser?.close();server.closeAllConnections();await new Promise(done=>server.close(done));}
