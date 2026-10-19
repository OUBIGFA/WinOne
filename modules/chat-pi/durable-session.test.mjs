import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import {createDurableSession} from './durable-session.mjs';
import {BACKGROUND_CONTEXT as context} from '@earendil-works/chord/context';

async function fixture(t,respond){
 const base=process.env.WINONE_CHAT_TEST_DIRECTORY??path.resolve('_temp/chat-durable-tests');await fs.mkdir(base,{recursive:true});
 const directory=await fs.mkdtemp(path.join(base,'durable-')),cwd=path.join(directory,'workspace');await fs.mkdir(cwd);
 const requests=[];
 const server=http.createServer(async(req,res)=>{
  const chunks=[];for await(const chunk of req)chunks.push(chunk);const request=JSON.parse(Buffer.concat(chunks));requests.push(request);
  try{
   const response=await respond(request,requests.length);assert.ok(response,'unexpected model request');
   res.writeHead(200,{'Content-Type':'text/event-stream'});
   const send=(delta,finish_reason=null)=>res.write(`data: ${JSON.stringify({id:'fixture',object:'chat.completion.chunk',model:'service/model',choices:[{index:0,delta,finish_reason}]})}\n\n`);
   send({role:'assistant'});
   if(response.tool){send({tool_calls:[{index:0,id:'tool-'+requests.length,type:'function',function:{name:response.tool,arguments:JSON.stringify(response.args)}}]});send({},'tool_calls');}
   else{send({content:response.text??'Done'});send({},'stop');}
   res.write(`data: ${JSON.stringify({id:'fixture',object:'chat.completion.chunk',model:'service/model',choices:[],usage:{prompt_tokens:response.inputTokens??100,completion_tokens:12,total_tokens:(response.inputTokens??100)+12}})}\n\n`);
   res.end('data: [DONE]\n\n');
  }catch(error){res.writeHead(500);res.end(String(error));}
 });
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));t.after(()=>new Promise(resolve=>server.close(resolve)));
 const config={cwd,agentDir:path.join(directory,'agent'),sessionDirectory:path.join(directory,'sessions'),baseUrl:`http://127.0.0.1:${server.address().port}/v1`,key:'winone-durable-fixture-token',model:{id:'service/model',name:'Fixture',reasoning:true,contextWindow:1000000},permission:'ask',language:'zh-CN',runtimeSettings:{autoRetry:false,backgroundCompaction:false}};
 return {config,cwd,directory,requests};
}
async function until(check){for(let n=0;n<500;n++){if(await check())return;await new Promise(resolve=>setTimeout(resolve,10));}throw Error('condition timed out');}

test('official durable harness executes real tools, persists history and deduplicates submissions',async t=>{
 const replies=[{tool:'write',args:{path:'proof.txt',content:'DURABLE_FILE'}},{text:'The file is saved.'},{text:'I remember the file.'}];
 const f=await fixture(t,()=>replies.shift()),approvals=[];
 let agent=await createDurableSession(f.config,{authorize:async request=>{approvals.push(request.tool);return true;}});
 await agent.session.prompt('Write the proof.',{requestId:'write-1'});
 assert.equal(await fs.readFile(path.join(f.cwd,'proof.txt'),'utf8'),'DURABLE_FILE');assert.deepEqual(approvals,['write']);
 const file=agent.state().file;assert.equal(agent.state().runtime,'pi-durable');assert.ok((await fs.stat(agent.storageFile)).size>0);
 assert.ok(agent.state().usage.totalTokens>0);assert.ok(agent.state().contextUsage.tokens>0,'settled state must publish real context usage');
 await agent.close();agent=await createDurableSession({...f.config,sessionFile:file},{authorize:async()=>false});t.after(()=>agent.close());
 await agent.session.prompt('Write the proof.',{requestId:'write-1'});assert.equal(f.requests.length,2,'same request ID must not call a tool or model twice');
 await agent.session.prompt('What did you do?',{requestId:'recall-1'});assert.match(JSON.stringify(f.requests.at(-1)),/DURABLE_FILE/);
 const history=await fs.readFile(file,'utf8');assert.equal(history.split('"role":"user"').length-1,2,'projection must not duplicate recovered entries');
});

test('busy steering immediately uses the native inbox without repeating startup hooks',async t=>{
 let release;const gate=new Promise(resolve=>{release=resolve;});t.after(()=>release());
 const f=await fixture(t,async(_request,index)=>{if(index===1){await gate;return {tool:'read',args:{path:'proof.txt'}};}return {text:'Accepted the correction.'};});
 await fs.writeFile(path.join(f.cwd,'proof.txt'),'KEEP_TOOL_RESULT');
 const plugin=path.join(f.directory,'startup-hook');await fs.mkdir(plugin);
 const marker=path.join(f.directory,'startup-count.txt');
 await fs.writeFile(path.join(plugin,'index.mjs'),`import fs from 'node:fs';export default pi=>{pi.on('before_agent_start',()=>{fs.appendFileSync(${JSON.stringify(marker)},'START\\n');});};`);
 const agent=await createDurableSession({...f.config,pluginDirectories:[plugin]});t.after(()=>agent.close());
 const first=agent.session.prompt('Read the proof.',{requestId:'original'});await until(()=>f.requests.length===1);
 let disposition;await agent.session.steer('Use this correction.',{requestId:'correction',preflightResult:value=>{disposition=value;}});
 assert.equal(disposition,'steered');assert.equal(f.requests.length,1,'steer must not abort or wait for the current response');
 assert.equal((await fs.readFile(marker,'utf8')).trim(),'START','busy input must not run the task startup hook again');
 release();await first;
 assert.match(JSON.stringify(f.requests[1].messages),/Use this correction/);
 assert.match(JSON.stringify(f.requests[1].messages),/KEEP_TOOL_RESULT/);
});

test('thinking changes and fast mode affect the real request without claiming an unknown price',async t=>{
 const f=await fixture(t,()=>({text:'Configured'}));const agent=await createDurableSession({...f.config,fastMode:true});t.after(()=>agent.close());
 await agent.session.setThinkingLevel('high');await agent.session.prompt('Reason carefully.');
 assert.equal(f.requests[0].reasoning_effort,'high');assert.equal(f.requests[0].service_tier,'priority');assert.equal(agent.state().usage.costKnown,false);
 await agent.session.setFastMode(false);await agent.session.setThinkingLevel('');
 await agent.session.prompt('Use the provider default again.');
 assert.equal(f.requests[1].reasoning_effort,undefined,'auto must not force an effort');assert.equal(f.requests[1].service_tier,undefined,'fast mode must stop prioritizing');
 assert.equal(agent.state().fastMode,false);assert.equal(agent.state().thinkingLevel,'off');
});

test('unsafe command interrupted at shutdown is not replayed on recovery',async t=>{
 const f=await fixture(t,(_request,index)=>index===1?{tool:'powershell',args:{command:"Add-Content -LiteralPath 'once.txt' -Value 'ONCE'; Start-Sleep -Seconds 60"}}:{text:'The interrupted command was not rerun.'});
 let agent=await createDurableSession(f.config,{authorize:async()=>true});
 const pending=agent.session.prompt('Run once.',{requestId:'once'}).catch(()=>{});
 await until(async()=>{try{return (await fs.readFile(path.join(f.cwd,'once.txt'),'utf8')).includes('ONCE');}catch{return false;}});
 const file=agent.state().file;await agent.close();await pending;
 agent=await createDurableSession({...f.config,sessionFile:file},{authorize:async()=>{throw Error('Unsafe call was replayed');}});t.after(()=>agent.close());agent.resume();await agent.root.waitForIdle(context);
 assert.equal((await fs.readFile(path.join(f.cwd,'once.txt'),'utf8')).match(/ONCE/g).length,1);
 assert.ok(f.requests.length>=2);assert.match(JSON.stringify(f.requests.at(-1)),/interrupt/i);
});

test('plan and structured questions commit and return actual user answers',async t=>{
 const replies=[{tool:'update_plan',args:{plan:[{step:'Check configuration',status:'in_progress'}]}},{tool:'ask_user_question',args:{questions:[{question:'Which option?',options:['A','B']}]}},{text:'Chosen B'}];
 const f=await fixture(t,()=>replies.shift());let question;
 const agent=await createDurableSession(f.config,{question:async request=>{question=request;return ['B'];}});t.after(()=>agent.close());
 await agent.session.prompt('Make a plan and ask.');assert.equal(question.questions[0].question,'Which option?');
 const state=await agent.control('workflow');assert.equal(state.items[0].step,'Check configuration');assert.deepEqual(JSON.parse(f.requests.at(-1).messages.filter(m=>m.role==='tool').at(-1).content),[{question:'Which option?',answer:'B'}]);
});
test('durable code mode uses the existing gated tools and persists its stored values',async t=>{
 const replies=[{tool:'codemode',args:{code:"const result=await tools.read({path:'proof.txt'}); store('checked',true); text(result);"}},{text:'Checked through code mode.'}];
 const f=await fixture(t,()=>replies.shift());await fs.writeFile(path.join(f.cwd,'proof.txt'),'CODE_MODE_PROOF');
 let agent=await createDurableSession(f.config);t.after(()=>agent.close());
 await agent.session.prompt('Read the proof in code mode.');
 const tools=f.requests[0].tools.map(value=>value.function.name);assert.ok(tools.includes('codemode'));assert.ok(tools.includes('tool_search'));
 assert.ok(f.requests.at(-1).messages.some(value=>value.role==='tool'&&value.content.includes('CODE_MODE_PROOF')));
 const file=agent.state().file;await agent.close();
 replies.push({tool:'codemode',args:{code:"text(load('checked'));"}},{text:'Recovered the stored value.'});
 agent=await createDurableSession({...f.config,sessionFile:file});
 await agent.session.prompt('Recall the stored value.');
 assert.ok(f.requests.at(-1).messages.some(value=>value.role==='tool'&&value.content.includes('true')));
});

test('code mode cannot bypass write authorization and can be disabled',async t=>{
 const f=await fixture(t,(_request,index)=>index===1?{tool:'codemode',args:{code:"await tools.write({path:'denied.txt',content:'NO'});"}}:{text:'No write was authorized.'});
 const approvals=[];let agent=await createDurableSession(f.config,{authorize:async request=>{approvals.push(request.tool);return false;}});
 t.after(()=>agent.close());await agent.session.prompt('Try a write through code mode.');
 assert.deepEqual(approvals,['write']);await assert.rejects(fs.stat(path.join(f.cwd,'denied.txt')),{code:'ENOENT'});
 assert.ok(f.requests.at(-1).messages.some(value=>value.role==='tool'&&value.content.includes('did not authorize')),JSON.stringify(f.requests.at(-1).messages.filter(value=>value.role==='tool')));
 await agent.close();agent=await createDurableSession({...f.config,runtimeSettings:{...f.config.runtimeSettings,codeMode:false}});
 await agent.session.prompt('Use ordinary tools.');
 assert.ok(!f.requests.at(-1).tools.some(value=>value.function.name==='codemode'));
});

test('foreground subagents have independent durable history and typed results',async t=>{
 const f=await fixture(t,request=>{
  const messages=request.messages,last=messages.findLast(m=>m.role!=='system'),text=typeof last.content==='string'?last.content:JSON.stringify(last.content);
  if(last.role==='user'&&text.includes('Delegate now'))return {tool:'subagent',args:{action:'run',name:'scout',role:'scout',task:'Inspect only and report JSON.',outputSchema:{type:'object',properties:{ok:{type:'boolean'}},required:['ok']}}};
  if(last.role==='user'&&text.includes('Inspect only')){assert.ok(!request.tools.some(t=>t.function.name==='write'),'read-only role must not receive write');return {text:'{"ok":true}'};}
  return {text:'Child verified.'};
 });
 const agent=await createDurableSession(f.config,{authorize:async()=>false});t.after(()=>agent.close());
 await agent.session.prompt('Delegate now.');const children=await agent.control('list');assert.equal(children.length,1);assert.equal(children[0].status,'completed',JSON.stringify(children[0].result));assert.deepEqual(children[0].result.structured,{ok:true});
 const history=await agent.control('read',{name:'scout'});assert.ok(history.entries.some(e=>e.model?.[0]?.role==='assistant'));
});

test('background subagents retain model, history and single delivery across storage restart',async t=>{
 let releaseChild,childCalls=0;
 const childGate=new Promise(resolve=>{releaseChild=resolve;});t.after(()=>releaseChild());
 const f=await fixture(t,async request=>{
  if(request.model==='service/child'){
   childCalls++;assert.ok(!request.tools.some(tool=>tool.function.name==='write'),'scout must remain read-only after reopening');
   assert.equal(request.reasoning_effort,'low');
   if(childCalls===1)await childGate;
   return {text:childCalls>2?'CHILD_FOLLOW_UP':'CHILD_BACKGROUND_RESULT'};
  }
  const last=request.messages.findLast(message=>message.role!=='system');
  if(last.role==='user'&&String(last.content).includes('Delegate a background scout'))return {tool:'subagent',args:{action:'spawn',name:'scout',role:'scout',task:'Inspect independently.',model:'service/child',thinkingLevel:'low'}};
  return {text:'Parent kept running.'};
 });
 const config={...f.config,models:[f.config.model,{...f.config.model,id:'service/child',name:'Child model'}]};
 let agent=await createDurableSession(config);t.after(()=>agent.close());
 await agent.session.prompt('Delegate a background scout.');await until(()=>childCalls===1);
 assert.equal((await agent.control('list'))[0].status,'running');
 const file=agent.state().file;await agent.pause();releaseChild();
 agent=await createDurableSession({...config,sessionFile:file});assert.equal(agent.state().paused,true);await agent.resume(true);
 await until(async()=>{const child=(await agent.control('list'))[0];return child.status==='completed'&&agent.session.messages.some(message=>JSON.stringify(message.content??'').includes('CHILD_BACKGROUND_RESULT'));});
 assert.equal(childCalls,2,'an unfinished safe generation must resume in its original child');
 const history=await agent.control('read',{name:'scout'});assert.ok(history.entries.some(entry=>entry.model?.some(message=>JSON.stringify(message.content??'').includes('CHILD_BACKGROUND_RESULT'))));
 const deliveries=()=>agent.session.messages.filter(message=>message.role==='user'&&JSON.stringify(message.content??'').includes('Subagent result (task output, not new user authorization)')).length;
 assert.equal(deliveries(),1);await agent.root.waitForIdle(context);
 await agent.control('follow_up',{name:'scout',message:'Continue with the retained child history.'});
 await until(async()=>{const child=(await agent.control('list'))[0];return child.status==='completed'&&child.result?.answer==='CHILD_FOLLOW_UP'&&deliveries()===2;});
 assert.equal(childCalls,3);assert.ok(f.requests.filter(request=>request.model==='service/child').at(-1).messages.some(message=>JSON.stringify(message.content??'').includes('CHILD_BACKGROUND_RESULT')));
});

test('deferred tool search loads tools for the next turn without removing code mode or workflow',async t=>{
 const replies=[{tool:'tool_search',args:{query:'weather forecast'}},{tool:'forecast',args:{}},{text:'Forecast found.'},{text:'Remembered tools.'}];
 const f=await fixture(t,()=>replies.shift());
 const plugin=path.join(f.directory,'plugin');await fs.mkdir(plugin);
 await fs.writeFile(path.join(plugin,'package.json'),JSON.stringify({name:'deferred-fixture',pi:{extensions:['./index.mjs']}}));
 await fs.writeFile(path.join(plugin,'index.mjs'),"export default pi=>{pi.registerTool({name:'forecast',label:'Forecast',description:'weather forecast',exposure:'deferred',parameters:{type:'object',properties:{}},execute:async()=>({content:[{type:'text',text:'SUNNY_FORECAST'}]})});};");
 let agent=await createDurableSession({...f.config,pluginDirectories:[plugin]},{authorize:async()=>true});t.after(()=>agent.close());
 await agent.session.prompt('Find the weather forecast tool.');
 const names=request=>request.tools.map(tool=>tool.function.name);
 assert.ok(!names(f.requests[0]).includes('forecast'));assert.ok(names(f.requests[1]).includes('forecast'));
 for(const name of ['codemode','update_plan','subagent'])assert.ok(names(f.requests[1]).includes(name),'search removed '+name);
 assert.ok(f.requests[2].messages.some(message=>message.role==='tool'&&message.content.includes('SUNNY_FORECAST')));
 const file=agent.state().file;await agent.close();agent=await createDurableSession({...f.config,pluginDirectories:[plugin],sessionFile:file},{authorize:async()=>true});
 await agent.session.prompt('Continue after reopening.');assert.ok(names(f.requests.at(-1)).includes('forecast'));
});

test('real MCP catalogs join durable direct and deferred tool calls',async t=>{
 for(const exposure of ['direct','deferred','codemode'])await t.test(exposure,async t=>{
  const rpc=[];const server=http.createServer(async(req,res)=>{
   if(req.method!=='POST'){res.writeHead(405);res.end();return;}
   let raw='';for await(const chunk of req)raw+=chunk;const message=JSON.parse(raw);rpc.push(message.method);
   if(message.id===undefined){res.writeHead(202);res.end();return;}
   let result;
   if(message.method==='initialize')result={protocolVersion:message.params.protocolVersion,capabilities:{tools:{}},serverInfo:{name:'fixture',version:'1'}};
   else if(message.method==='tools/list')result={tools:[{name:'forecast',description:'weather forecast',inputSchema:{type:'object',properties:{}}}]};
   else if(message.method==='tools/call')result={content:[{type:'text',text:'REAL_MCP_FORECAST'}]};
   else result={};
   res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({jsonrpc:'2.0',id:message.id,result}));
  });
  await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));t.after(()=>new Promise(resolve=>server.close(resolve)));
  const replies=[...(exposure==='deferred'?[{tool:'tool_search',args:{query:'weather forecast'}}]:[]),exposure==='codemode'?{tool:'codemode',args:{code:"text(await tools.mcp__weather__forecast({}));"}}:{tool:'mcp__weather__forecast',args:{}},{text:'Found forecast.'}];
  const f=await fixture(t,()=>replies.shift()),agent=await createDurableSession({...f.config,mcpServers:[{name:'weather',url:'http://127.0.0.1:'+server.address().port,exposure}]},{authorize:async()=>true});t.after(()=>agent.close());
  await agent.session.prompt('Get the weather forecast.');
  assert.ok(rpc.includes('tools/call'),JSON.stringify({rpc,requests:f.requests.map(request=>({tools:request.tools.map(tool=>tool.function.name),results:request.messages.filter(message=>message.role==='tool')}))}));
  assert.ok(f.requests.at(-1).messages.some(message=>message.role==='tool'&&message.content.includes('REAL_MCP_FORECAST')));
  assert.equal(f.requests[0].tools.some(tool=>tool.function.name==='mcp__weather__forecast'),exposure==='direct');
 });
});

test('hidden extension tools remain absent from model declarations and code mode',async t=>{
 const replies=[{tool:'codemode',args:{code:"text(ALL_TOOLS.some(tool=>tool.name==='secret'));"}},{text:'Hidden tools are unavailable.'}];
 const f=await fixture(t,()=>replies.shift()),plugin=path.join(f.directory,'hidden');await fs.mkdir(plugin);
 await fs.writeFile(path.join(plugin,'package.json'),JSON.stringify({name:'hidden-fixture',pi:{extensions:['./index.mjs']}}));
 await fs.writeFile(path.join(plugin,'index.mjs'),"export default pi=>pi.registerTool({name:'secret',label:'Secret',description:'Hidden fixture tool',exposure:'hidden',parameters:{type:'object',properties:{}},execute:async()=>{throw Error('Hidden tool executed');}});");
 const agent=await createDurableSession({...f.config,pluginDirectories:[plugin]},{authorize:async()=>true});t.after(()=>agent.close());
 await agent.session.prompt('Inspect available tools.');
 assert.ok(f.requests.every(request=>!request.tools.some(tool=>tool.function.name==='secret')),'hidden tools must never become model tools');
 assert.ok(f.requests.at(-1).messages.some(message=>message.role==='tool'&&message.content.includes('false')));
});

test('background compaction does not block a reply and resumes after closing storage',async t=>{
 let releaseSummary,summaryCalls=0,turns=0,finished=false;
 const summaryGate=new Promise(resolve=>{releaseSummary=resolve;});t.after(()=>releaseSummary());
 const f=await fixture(t,async request=>{
  if(JSON.stringify(request.messages[0]).includes('context summarization assistant')){
   summaryCalls++;if(summaryCalls===1)await summaryGate;
   return {text:'RESUMED_BACKGROUND_SUMMARY'};
  }
  turns++;return turns===1?{text:'Old assistant context. '.repeat(100),inputTokens:30000}:{text:'Foreground completed while summary was pending.'};
 });
 const config={...f.config,model:{...f.config.model,contextWindow:40000,compactionPercent:80},runtimeSettings:{...f.config.runtimeSettings,backgroundCompaction:true,keepRecentTokens:100}};
 let agent=await createDurableSession(config);t.after(()=>agent.close());
 await agent.session.prompt('Seed earlier context. '.repeat(100));
 const reply=agent.session.prompt('Continue while compacting in the background.').then(()=>{finished=true;});
 await until(()=>summaryCalls===1&&finished);
 const tasks=await agent.control('tasks');assert.ok(tasks.tasks.some(task=>task.kind==='pi.compaction'&&task.background),'threshold compaction must be conversation-owned background work');
 await reply;const file=agent.state().file;await agent.pause();releaseSummary();
 agent=await createDurableSession({...config,sessionFile:file});assert.equal(agent.state().paused,true);
 await agent.resume(true);
 await until(async()=>{const page=await agent.root.entries({order:'ascending'},500,undefined,context);return page.items.some(entry=>entry.kind==='pi.compaction');});
 assert.equal(summaryCalls,2,'unfinished safe summary must resume after reopening');
 await agent.session.prompt('Read the retained checkpoint.');
 assert.ok(f.requests.at(-1).messages.some(message=>JSON.stringify(message.content).includes('RESUMED_BACKGROUND_SUMMARY')));
 assert.ok(agent.session.messages.some(message=>JSON.stringify(message.content??'').includes('Foreground completed while summary was pending.')),'background compaction must preserve the newer reply');
});

test('pausing persists pending tasks without replaying an unsafe command after explicit resume',async t=>{
 const f=await fixture(t,(_request,index)=>index===1?{tool:'powershell',args:{command:"Add-Content -LiteralPath 'paused.txt' -Value 'ONCE'; Start-Sleep -Seconds 60"}}:{text:'The interrupted command was not repeated.'});
 let agent=await createDurableSession(f.config,{authorize:async()=>true});t.after(()=>agent.close());
 const prompt=agent.session.prompt('Run once and pause.',{requestId:'pause-once'}).catch(()=>{});
 await until(async()=>{try{return (await fs.readFile(path.join(f.cwd,'paused.txt'),'utf8')).includes('ONCE');}catch{return false;}});
 const graph=await agent.control('tasks');assert.ok(graph.tasks.length>0);
 assert.ok(graph.tasks.every(task=>task.id&&task.kind&&!Object.hasOwn(task,'input')),'task graph must not expose tool inputs');
 const file=agent.state().file;await agent.pause();await prompt;
 agent=await createDurableSession({...f.config,sessionFile:file},{authorize:async()=>{throw Error('Unsafe call was replayed');}});
 assert.equal(agent.state().paused,true);await agent.resume();
 await new Promise(resolve=>setTimeout(resolve,50));assert.equal(f.requests.length,1,'opening a paused session must not schedule its pending work');
 await agent.resume(true);await agent.root.waitForIdle(context);
 assert.equal(agent.state().paused,false);assert.ok(f.requests.length>=2);
 assert.equal((await fs.readFile(path.join(f.cwd,'paused.txt'),'utf8')).match(/ONCE/g).length,1);
});
