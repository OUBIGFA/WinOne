import http from 'node:http';
import {mkdir,writeFile,readFile} from 'node:fs/promises';
import {join} from 'node:path';
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';

// Real isolated Host -> shared gateway -> local vendor -> durable Worker.
// Disposable fixture credentials never leave the loopback server.
export async function checkAISync(page,output){
 const requests=[],LF=String.fromCharCode(10);let leaked=false,phase=0,mcpCalls=0,mcpHeaderSeen=false;
 const server=http.createServer(async(req,res)=>{
  leaked ||= !!req.headers['x-winone-caller-key'];
  let raw='';for await(const chunk of req)raw+=chunk;
  if(req.url==='/mcp'){
   mcpHeaderSeen ||= req.headers['x-winone-mcp-fixture']==='native-header';
   if(req.method!=='POST'){res.writeHead(405);res.end();return;}
   const message=JSON.parse(raw);if(message.id===undefined){res.writeHead(202);res.end();return;}
   let result={};if(message.method==='initialize')result={protocolVersion:message.params.protocolVersion,capabilities:{tools:{}},serverInfo:{name:'fixture',version:'1'}};
   if(message.method==='tools/list')result={tools:[{name:'forecast',description:'Native MCP forecast',inputSchema:{type:'object',properties:{}}}]};
   if(message.method==='tools/call'){mcpCalls++;result={content:[{type:'text',text:'NATIVE_MCP_PROOF'}]};}
   res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({jsonrpc:'2.0',id:message.id,result}));return;
  }
  if(req.url.endsWith('/models')){res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({data:req.url.startsWith('/large/')?Array.from({length:2500},(_,index)=>({id:'fixture-'+String(index).padStart(4,'0')})):[{id:'gpt-4o'}]}));return;}
  const body=JSON.parse(raw);requests.push(body);
  const tools=body.tools??[],canAsk=tools.some(value=>(value.function?.name??value.name)==='ask_user_question');
  let delta={content:'Native answer accepted'},finish='stop';
  if(canAsk&&phase<7){
   const operations=[
    {name:'update_plan',args:{plan:[{step:'保留原配置',status:'completed'},{step:'验证用户回答',status:'in_progress'}]}},
    {name:'ask_user_question',args:{questions:[{question:'原生提问验证',options:['保留配置','重新设置']},{question:'备注'}]}},
    {name:'codemode',args:{code:"store('nativeProof',true); text('NATIVE_CODE_MODE_PROOF');"}},
    {name:'update_plan',args:{plan:[{step:'保留原配置',status:'completed'},{step:'验证用户回答',status:'completed'}]}},
    {name:'powershell',args:{command:"Add-Content -LiteralPath 'pause-proof.txt' -Value 'ONCE'; Start-Sleep -Seconds 60"}},
    {name:'codemode',args:{code:"text(await tools.mcp__fixture__forecast({}));"}},
    {name:'subagent',args:{action:'spawn',name:'native-scout',role:'scout',task:'Inspect only; return the fixture answer.'}},
   ];
   const operation=operations[phase++];delta={tool_calls:[{index:0,id:'native-operation-'+phase,type:'function',function:{name:operation.name,arguments:JSON.stringify(operation.args)}}]};finish='tool_calls';
  }
  res.writeHead(200,{'Content-Type':'text/event-stream'});
  for(const chunk of [{id:'native',object:'chat.completion.chunk',choices:[{index:0,delta,finish_reason:null}]},{id:'native',object:'chat.completion.chunk',choices:[{index:0,delta:{},finish_reason:finish}]}])res.write('data: '+JSON.stringify(chunk)+LF+LF);
  res.end('data: [DONE]'+LF+LF);
 });
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 try{
  const base='http://127.0.0.1:'+server.address().port+'/v1';
  const tabs=page.getByRole('tablist',{name:'AI 服务',exact:true});await tabs.getByRole('tab',{name:'自定义供应商',exact:true}).click();
  await page.getByRole('button',{name:'添加自定义供应商',exact:true}).click();
  await page.getByRole('textbox',{name:'服务名称',exact:true}).fill('Native curl fixture');
  const command='curl '+base+'/chat/completions -H '+JSON.stringify('Authorization: Bearer native-curl-fixture-key')+" -d '"+JSON.stringify({model:'gpt-4o'})+"'";
  await page.getByRole('textbox',{name:'API 基础地址',exact:true}).evaluate((input,text)=>{const data=new DataTransfer();data.setData('text/plain',text);input.dispatchEvent(new ClipboardEvent('paste',{bubbles:true,cancelable:true,clipboardData:data}));},command);
  await page.getByText('已将 cURL 示例填入草稿，请检查后保存。',{exact:true}).waitFor();
  assert.equal(await page.getByRole('textbox',{name:'API 基础地址',exact:true}).inputValue(),base);
  assert.equal(await page.getByLabel('API 密钥',{exact:true}).inputValue(),'native-curl-fixture-key');
  await page.getByRole('checkbox',{name:/gpt-4o/}).waitFor();
  await page.getByRole('button',{name:'保存 AI 设置',exact:true}).click();await page.getByRole('status').filter({hasText:'已保存'}).waitFor();
  const configuration=await page.evaluate(async()=>JSON.parse(await window.go.main.App.Capture('ai.configuration.get','null')));
  const provider=configuration.providers.find(value=>value.name==='Native curl fixture');assert.ok(provider);assert.deepEqual(provider.models,['gpt-4o']);
  assert.equal(JSON.stringify(configuration).includes('native-curl-fixture-key'),false,'Saved key remains write-only');
  const workspace=join(output,'question-workspace'),history=join(output,'question-history');await mkdir(workspace);await mkdir(history);
  const conversation=await page.evaluate(async params=>JSON.parse(await window.go.main.App.Chat('conversation.new',JSON.stringify(params))),{directory:workspace,saveDirectory:history,title:'Native questions',model:{provider:provider.id,model:'gpt-4o'}});
  await page.locator('.app-navigation [data-page-id="chat"]').click();
  await page.getByRole('button',{name:'Chat 设置',exact:true}).click();
  const settings=page.getByRole('dialog',{name:'Chat 设置',exact:true});
  await settings.getByRole('checkbox',{name:'后台压缩',exact:true}).check();
  await settings.getByRole('spinbutton',{name:'同时运行的子代理上限',exact:true}).fill('2');
  await settings.getByRole('tab',{name:'MCP',exact:true}).click();
  await settings.getByRole('button',{name:'添加 MCP 服务',exact:true}).click();
  await settings.getByRole('textbox',{name:'服务名称',exact:true}).last().fill('fixture');
  await settings.getByRole('combobox',{name:'连接方式',exact:true}).last().selectOption('url');
  await settings.getByRole('textbox',{name:'服务 URL',exact:true}).last().fill(base.replace('/v1','/mcp'));
  await settings.getByRole('combobox',{name:'工具调用方式',exact:true}).last().selectOption('codemode');
  await settings.locator('summary').last().click();
  await settings.getByRole('textbox',{name:'环境变量（JSON 对象）',exact:true}).last().fill('{"WINONE_MCP_FIXTURE":"1"}');
  await settings.getByRole('textbox',{name:'请求头（JSON 对象）',exact:true}).last().fill('{"X-WinOne-MCP-Fixture":"native-header"}');
  const toolModes=settings.getByRole('textbox',{name:'单个工具调用方式（JSON）',exact:true}).last();
  await toolModes.fill('{"forecast":');assert.equal(await settings.getByRole('button',{name:'保存设置',exact:true}).isDisabled(),true);
  await toolModes.fill('{"forecast":"codemode"}');
  await settings.getByRole('button',{name:'保存设置',exact:true}).click();
  await settings.waitFor({state:'hidden'});
  const savedSettings=await page.evaluate(async()=>JSON.parse(await window.go.main.App.Chat('settings.get','null')));
  assert.equal(savedSettings.backgroundCompaction,true);assert.equal(savedSettings.maxSubagents,2);
  assert.equal(savedSettings.mcpServers.find(server=>server.name==='fixture').exposure,'codemode');
  assert.deepEqual(savedSettings.mcpServers.find(server=>server.name==='fixture').toolExposure,{forecast:'codemode'});
  assert.deepEqual(savedSettings.mcpServers.find(server=>server.name==='fixture').env,{WINONE_MCP_FIXTURE:'1'});
  await page.getByRole('button',{name:'Native questions',exact:true}).click();
  await page.getByRole('textbox',{name:'消息',exact:true}).fill('Ask the fixture questions');
  await page.getByRole('button',{name:'发送',exact:true}).click();
  const question=page.getByRole('region',{name:'等待你的回答',exact:true});await question.waitFor();
  const plan=page.getByRole('region',{name:'计划',exact:true});await plan.waitFor();
  assert.equal(await plan.getByRole('button').getAttribute('aria-expanded'),'false');
  await plan.getByRole('button').click();await plan.getByText('保留原配置',{exact:true}).waitFor();
  assert.equal(await question.getByRole('button',{name:'提交答案',exact:true}).isDisabled(),true);
  await page.screenshot({path:join(output,'native-questions.png')});
  await question.getByRole('button',{name:'保留配置',exact:true}).click();
  await question.getByRole('textbox',{name:'备注',exact:true}).fill('本地真实链路验证');
  await question.getByRole('button',{name:'提交答案',exact:true}).click();
  await page.getByRole('button',{name:/允许一次/}).click();
  const until=async predicate=>{const deadline=Date.now()+15000;while(!await predicate()){if(Date.now()>deadline)throw Error('Native pause fixture timed out');await new Promise(resolve=>setTimeout(resolve,30));}};
  await until(async()=>{try{return (await readFile(join(workspace,'pause-proof.txt'),'utf8')).includes('ONCE');}catch{return false;}});
  await page.getByRole('button',{name:'暂停任务',exact:true}).click();
  await page.getByRole('button',{name:'恢复任务',exact:true}).waitFor();
  const paused=await page.evaluate(async id=>JSON.parse(await window.go.main.App.Chat('state',JSON.stringify({id}))),conversation.id);
  assert.equal(paused.paused,true);assert.equal(paused.streaming,false);assert.ok(paused.taskGraph.tasks.length>0);
  await page.getByRole('textbox',{name:'消息',exact:true}).fill('暂停期间保留的草稿');
  await page.getByRole('button',{name:/^任务 · /}).click();
  await page.getByRole('list',{name:'任务',exact:true}).waitFor();
  await page.screenshot({path:join(output,'native-tasks-paused.png')});
  await page.keyboard.press('Escape');
  await page.getByRole('button',{name:'恢复任务',exact:true}).click();
  await page.getByRole('button',{name:/允许一次/}).click();
  await page.getByText('Native answer accepted',{exact:true}).last().waitFor();
  assert.equal(await page.getByRole('textbox',{name:'消息',exact:true}).inputValue(),'暂停期间保留的草稿');
  assert.equal((await readFile(join(workspace,'pause-proof.txt'),'utf8')).match(/ONCE/g).length,1);
  assert.equal(mcpCalls,1);assert.equal(mcpHeaderSeen,true);assert.ok(requests.flatMap(value=>value.messages??[]).some(value=>value.role==='tool'&&value.content?.includes('NATIVE_MCP_PROOF')));
  const answer=requests.flatMap(value=>value.messages??[]).find(value=>value.role==='tool'&&value.content?.includes('本地真实链路验证'));
  assert.ok(answer,'The vendor received actual answers');assert.equal(leaked,false);
  assert.ok(requests.flatMap(value=>value.messages??[]).some(value=>value.role==='tool'&&value.content?.includes('NATIVE_CODE_MODE_PROOF')));
  await plan.getByText('计划已完成',{exact:true}).waitFor();
  assert.equal(await plan.getByRole('button').getAttribute('aria-expanded'),'false');
  await until(async()=>{const list=await page.evaluate(async id=>JSON.parse(await window.go.main.App.Chat('agents',JSON.stringify({id,action:'list'}))),conversation.id);return list.some(child=>child.name==='native-scout'&&child.status==='completed');});
  await page.getByRole('button',{name:'子代理 · 1',exact:true}).click();
  await page.getByRole('button',{name:'查看消息 · native-scout',exact:true}).click();
  const child=page.getByRole('dialog',{name:'查看消息 · native-scout',exact:true});
  await child.getByText('Native answer accepted',{exact:true}).waitFor();
  await child.getByRole('textbox',{name:'发给子代理的消息',exact:true}).fill('Continue the native child inspection.');
  await child.getByRole('button',{name:'发送后续消息',exact:true}).click();
  await until(async()=>await child.getByRole('textbox',{name:'发给子代理的消息',exact:true}).inputValue()==='');
  await child.getByRole('button',{name:'刷新消息',exact:true}).click();
  const childLayout=await child.evaluate(node=>({height:node.getBoundingClientRect().height,viewport:innerHeight,refresh:node.querySelector('button:not([data-slot=dialog-close])').getBoundingClientRect().height,scoped:node.classList.contains('desktop-ui-portal')}));
  assert.ok(childLayout.refresh<=44,'Child refresh button must keep its compact height: '+JSON.stringify(childLayout));
  assert.equal(childLayout.scoped,true,'Child messages retain upstream portal styles');
  await page.screenshot({path:join(output,'native-child-messages.png')});
  const resize=join(output,'resize-child-window.ps1');
  await writeFile(resize,[
   'param([int]$OwnerPID,[string]$ExpectedExe,[int]$Width,[int]$Height)',
   '$ErrorActionPreference="Stop"',
   '$p=Get-Process -Id $OwnerPID',
   'if($p.Path -ne $ExpectedExe){throw "The window does not belong to this check"}',
   'Add-Type -TypeDefinition @\'',
   'using System;using System.Runtime.InteropServices;public static class ChildWindowSize{[DllImport("user32.dll")]public static extern bool SetProcessDPIAware();[DllImport("user32.dll",SetLastError=true)]public static extern bool SetWindowPos(IntPtr h,IntPtr a,int x,int y,int w,int height,uint flags);}',
   '\'@',
   '[void][ChildWindowSize]::SetProcessDPIAware()',
   'if(-not [ChildWindowSize]::SetWindowPos($p.MainWindowHandle,[IntPtr]::Zero,40,40,$Width,$Height,0x14)){throw "Window resize failed"}',
  ].join(LF),'utf8');
  execFileSync('powershell.exe',['-NoProfile','-File',resize,'-OwnerPID',process.env.WINONE_COMPONENTS_PID,'-ExpectedExe',join(output,'WinOne.exe'),'-Width','800','-Height','600'],{windowsHide:true});
  await page.evaluate(async()=>{await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);});
  await child.getByRole('button',{name:'发送后续消息',exact:true}).scrollIntoViewIfNeeded();
  const shortLayout=await child.evaluate(node=>{const box=node.getBoundingClientRect();return {width:innerWidth,height:innerHeight,x:box.x,y:box.y,bottom:box.bottom,overflow:node.scrollWidth-node.clientWidth};});
  assert.ok(shortLayout.y>=0&&shortLayout.bottom<=shortLayout.height&&shortLayout.overflow<=1,'Child dialog fits the short native window: '+JSON.stringify(shortLayout));
  await page.screenshot({path:join(output,'native-child-short.png')});
  await child.getByRole('button',{name:'关闭',exact:true}).last().click();
  assert.equal(await page.getByRole('textbox',{name:'消息',exact:true}).inputValue(),'暂停期间保留的草稿');
  await page.screenshot({path:join(output,'native-question-answered.png')});
  // A saved API key reaches the actual balance worker without needing an
  // enabled model or account. This loopback vendor intentionally has no quota API.
  const keyBalance=await page.evaluate(async id=>JSON.parse(await window.go.main.App.Capture('ai.accounts.quota',JSON.stringify({provider:id}))),provider.id);
  assert.deepEqual(keyBalance.quotas??[],[]);
  await page.locator('.app-navigation [data-page-id="ai-services"]').click();
  const providerTabs=page.getByRole('tablist',{name:'AI 服务',exact:true});
  await providerTabs.getByRole('tab',{name:'用量统计',exact:true}).click();
  await page.getByText('额度与余额',{exact:true}).waitFor();
  await providerTabs.getByRole('tab',{name:'自定义供应商',exact:true}).click();
  const catalog=await page.evaluate(async()=>JSON.parse(await window.go.main.App.Capture('ai.accounts.catalog','{}')));
  const mantle=catalog.presets.find(item=>item.id==='bedrock-mantle');assert.ok(mantle);assert.equal(mantle.regions.length,17);
  await page.getByRole('button',{name:'从服务预设添加',exact:true}).click();
  const picker=page.getByRole('dialog',{name:'选择服务预设',exact:true});
  await picker.getByRole('textbox',{name:'搜索供应商',exact:true}).fill('Mantle');
  await picker.getByRole('button',{name:'Amazon Bedrock (Mantle)',exact:true}).click();
  const protocols=[['chat','OpenAI Chat Completions'],['responses','OpenAI Responses'],['anthropic','Anthropic Messages']];
  for(const region of mantle.regions){
   await page.getByRole('combobox',{name:'地区或套餐',exact:true}).click();await page.getByRole('option',{name:region.name,exact:true}).click();
   for(const [protocol,label] of protocols){
    await page.getByRole('combobox',{name:'API 协议',exact:true}).click();await page.getByRole('option',{name:label,exact:true}).click();
    assert.equal(await page.getByRole('textbox',{name:'API 基础地址',exact:true}).inputValue(),region[protocol]);
   }
  }
  await page.getByRole('button',{name:'保存 AI 设置',exact:true}).click();await page.getByRole('status').filter({hasText:'已保存'}).waitFor();
  const mantleSaved=await page.evaluate(async()=>JSON.parse(await window.go.main.App.Capture('ai.configuration.get','null')));
  assert.ok(mantleSaved.providers.some(item=>item.preset==='bedrock-mantle'&&item.presetRegion===mantle.regions.at(-1).id&&item.protocol==='anthropic'&&item.baseUrl===mantle.regions.at(-1).anthropic));
  await page.screenshot({path:join(output,'native-mantle.png')});
  await writeFile(join(output,'balance-mantle-result.json'),JSON.stringify({native:true,savedKeyBalanceWorker:true,allowancesVisible:true,mantleRegions:17,mantleProtocols:3,saved:true,awsRequest:false},null,2),'utf8');
  await page.getByRole('button',{name:'添加自定义供应商',exact:true}).click();
  await page.getByRole('textbox',{name:'服务名称',exact:true}).fill('Native large catalog');
  await page.getByRole('textbox',{name:'API 基础地址',exact:true}).fill(base.replace('/v1','/large/v1'));
  const discoveryStarted=performance.now();
  await page.getByRole('button',{name:'获取模型列表',exact:true}).click();
  const finalModel=page.getByRole('checkbox',{name:/fixture-2499/});await finalModel.waitFor();
  const discoveryMs=performance.now()-discoveryStarted;
  assert.equal(await page.getByRole('checkbox',{name:/fixture-/}).count(),2500);
  await page.getByRole('button',{name:'全选',exact:true}).click();
  await page.getByRole('button',{name:'保存 AI 设置',exact:true}).click();await page.getByRole('status').filter({hasText:'已保存'}).waitFor();
  const largeSaved=await page.evaluate(async()=>JSON.parse(await window.go.main.App.Capture('ai.configuration.get','null')));
  const large=largeSaved.providers.find(item=>item.name==='Native large catalog');assert.equal(large.models.length,2500);assert.equal(large.availableModels.length,2500);
  await finalModel.scrollIntoViewIfNeeded();await page.screenshot({path:join(output,'native-model-2500.png')});
  await writeFile(join(output,'large-catalog-result.json'),JSON.stringify({native:true,models:2500,allEnabled:true,saved:true,lastModelReachable:true,discoveryMs},null,2),'utf8');
  await writeFile(join(output,'ai-sync-result.json'),JSON.stringify({native:true,assetMode:process.env.WINONE_NATIVE_PRODUCTION_ASSETS==='1'?'embedded production assets':'live Vite review',curlDraft:true,keyWriteOnly:true,question:conversation.id,actualAnswers:true,codeMode:true,mcpCodeMode:true,plan:true,durableSettings:true,pauseResume:true,taskGraph:true,childHistory:true,childFollowUp:true,childLayout,draftRetained:true,unsafeCommandExecutedOnce:true,noPrivateHeaderLeak:true,requests:requests.length},null,2),'utf8');
 }finally{await new Promise(resolve=>server.close(resolve));}
}
