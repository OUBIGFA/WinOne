import {test} from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs/promises";
import path from "node:path";
import os from "node:os";
import http from "node:http";
import {createWinOneSession} from "./session.mjs";

async function fixture(t,replies){
 const root=await fs.mkdtemp(path.join(process.env.WINONE_CHAT_TEST_DIRECTORY??os.tmpdir(),"pi-session-"));
 const cwd=path.join(root,"workspace");await fs.mkdir(cwd);
 const requests=[];
 const server=http.createServer(async(req,res)=>{
  const chunks=[];for await(const chunk of req)chunks.push(chunk);
  const request=JSON.parse(Buffer.concat(chunks).toString("utf8"));requests.push(request);
  assert.equal(req.headers.authorization,"Bearer winone-fixture-session-token");
  const next=replies.shift();assert.ok(next,"unexpected extra model request");
  if(request.stream===false){res.writeHead(200,{"Content-Type":"application/json"});res.end(JSON.stringify({choices:[{message:{content:next.text}}]}));return;}
  res.writeHead(200,{"Content-Type":"text/event-stream"});
  const emit=(delta,finish=null)=>res.write(`data: ${JSON.stringify({id:"fixture",object:"chat.completion.chunk",created:1,model:"service/model",choices:[{index:0,delta,finish_reason:finish}]})}\n\n`);
  emit({role:"assistant"});
  if(next.tool){emit({tool_calls:[{index:0,id:"call-"+requests.length,type:"function",function:{name:next.tool,arguments:JSON.stringify(next.args)}}]});emit({},"tool_calls");}
  else{emit({content:next.text??"Done"});emit({},"stop");}
  res.end("data: [DONE]\n\n");
 });
 await new Promise(resolve=>server.listen(0,"127.0.0.1",resolve));t.after(()=>new Promise(resolve=>server.close(resolve)));
 const config={cwd,agentDir:path.join(root,"agent"),sessionDirectory:path.join(root,"sessions"),baseUrl:`http://127.0.0.1:${server.address().port}/v1`,key:"winone-fixture-session-token",model:{id:"service/model",name:"Fixture model"},language:"zh-CN",title:"Fixture",permission:"ask"};
 return {root,cwd,config,requests};
}

test("percentage compaction uses the effective model window and preserves legacy defaults",async(t)=>{
 const f=await fixture(t,[]);
 const config={...f.config,model:{...f.config.model,contextWindow:128000,compactionPercent:72.5,compactionThreshold:100000}};
 const agent=await createWinOneSession(config);t.after(()=>agent.close());
 assert.equal(agent.session.model.contextWindow,128000);
 assert.equal(agent.session.settingsManager.getCompactionSettings(agent.session.model).reserveTokens,35200);
 await agent.setModel({...f.config.model,contextWindow:101,compactionPercent:33.3});
 assert.equal(agent.session.settingsManager.getCompactionSettings(agent.session.model).reserveTokens,68,'percentage threshold is floored before deriving the reserve');
 await agent.setModel({...f.config.model,contextWindow:0,listedContextWindow:128000,compactionPercent:100});
 assert.equal(agent.session.model.contextWindow,128000);
 assert.equal(agent.session.settingsManager.getCompactionSettings(agent.session.model).reserveTokens,0);
 await agent.setModel({...f.config.model,contextWindow:128000,compactionPercent:0,compactionThreshold:100000});
 assert.equal(agent.session.settingsManager.getCompactionSettings(agent.session.model).reserveTokens,28000);
 await assert.rejects(agent.setModel({...f.config.model,compactionPercent:101}),/between 0 and 100/);
});
test('scheduled effort and priority reach the actual provider request',async(t)=>{
 const f=await fixture(t,[{text:'Configured result'}]);const agent=await createWinOneSession({...f.config,thinkingLevel:'high',fastMode:true,model:{...f.config.model,reasoning:true}});
 try{await agent.session.prompt('Use the selected options.');assert.equal(f.requests[0].reasoning_effort,'high');assert.equal(f.requests[0].service_tier,'priority');}
 finally{await agent.close();}
});
test('a model reporting its own reasoning levels is never sent a level it lacks',async(t)=>{
 // Our Free Model's MiMo V2.5 reports light/balanced/deep. Pi has no such
 // level, so none is sent and the plugin applies its own default.
 const f=await fixture(t,[{text:'Vendor default'},{text:'Highest'},{text:'Off'}]);
 const vendor=await createWinOneSession({...f.config,thinkingLevel:'high',model:{...f.config.model,levels:['light','balanced','deep'],efforts:['light','balanced','deep']}});
 try{await vendor.session.prompt('Use the vendor default.');assert.equal(f.requests[0].reasoning_effort,undefined);}
 finally{await vendor.close();}
 // Levels the model has are mapped by Pi itself, including max and off.
 const levels={...f.config.model,efforts:['none','low','medium','high','max']};
 const highest=await createWinOneSession({...f.config,thinkingLevel:'max',model:levels});
 try{await highest.session.prompt('Use the highest level.');assert.equal(f.requests[1].reasoning_effort,'max');}
 finally{await highest.close();}
 const off=await createWinOneSession({...f.config,thinkingLevel:'off',model:levels});
 try{await off.session.prompt('Do not reason.');assert.equal(f.requests[2].reasoning_effort,'none');}
 finally{await off.close();}
});

test("percentage triggers Pi's real summary, persists it and retains original history",async(t)=>{
 const f=await fixture(t,[{text:'First answer'},{text:'Second answer'},{text:'Saved checkpoint: first request completed.'},{text:'Continued from checkpoint'}]);
 const config={...f.config,model:{...f.config.model,contextWindow:60000,compactionPercent:50,compactionThreshold:59000}};
 const events=[];let agent=await createWinOneSession(config,{event:value=>events.push(value)});
 const first='FIRST_HISTORY_MARKER '+('earlier text '.repeat(7000)),second='SECOND_HISTORY_MARKER '+('recent text '.repeat(7000));
 await agent.session.prompt(first);await agent.session.prompt(second);
 assert.ok(events.some(event=>event.type==='compaction_start'&&event.reason==='threshold'),'percentage must cause an actual automatic compaction');
 const entries=agent.session.sessionManager.getEntries(),summary=entries.find(entry=>entry.type==='compaction');
 assert.ok(summary?.summary.includes('Saved checkpoint'),'Pi must persist the generated summary');
 const file=agent.state().file,raw=await fs.readFile(file,'utf8');
 assert.ok(raw.includes('FIRST_HISTORY_MARKER')&&raw.includes('SECOND_HISTORY_MARKER'),'compaction must retain original history on disk');
 assert.ok(f.requests.some(request=>JSON.stringify(request.messages).includes('<conversation>')),'summary must be requested from the model, not synthesized locally');
 await agent.close();agent=await createWinOneSession({...config,sessionFile:file});t.after(()=>agent.close());
 await agent.session.prompt('Continue using the checkpoint.');
 assert.ok(JSON.stringify(f.requests.at(-1).messages).includes('Saved checkpoint'),'reopened context must use the persisted checkpoint');
});

test("historical images are scaled only in the actual outgoing HTTP payload",async(t)=>{
 const f=await fixture(t,[{text:'Images and text received'}]);
 const config={...f.config,model:{...f.config.model,vision:true,contextWindow:1000000}};
 const image=Buffer.concat([await fs.readFile(new URL('../../internal/branding/chat-welcome.png',import.meta.url)),Buffer.alloc(1024*1024)]).toString('base64');
 let agent=await createWinOneSession(config);
 agent.session.sessionManager.appendMessage({role:'user',timestamp:Date.now(),content:[{type:'text',text:'RETAIN_ALL_HISTORICAL_TEXT'},...Array.from({length:13},()=>({type:'image',mimeType:'image/png',data:image}))]});
 const file=agent.state().file;await agent.close();const original=await fs.readFile(file,'utf8');
 agent=await createWinOneSession({...config,sessionFile:file});t.after(()=>agent.close());await agent.session.prompt('Inspect every historical image.');
 const sent=f.requests.at(-1);assert.ok(Buffer.byteLength(JSON.stringify(sent))<=16*1024*1024);assert.ok(JSON.stringify(sent).includes('RETAIN_ALL_HISTORICAL_TEXT'));
 const images=sent.messages.flatMap(message=>Array.isArray(message.content)?message.content.filter(part=>part.type==='image_url'):[]);assert.equal(images.length,13);
 assert.ok((await fs.readFile(file,'utf8')).startsWith(original),'wire resizing must not rewrite any persisted history');
});

test("Pi executes its real write, edit and PowerShell tools and reopens its persisted session",async(t)=>{
 const f=await fixture(t,[{tool:"write",args:{path:"hello.txt",content:"first"}},{tool:"edit",args:{path:"hello.txt",oldText:"first",newText:"second"}},{tool:"powershell",args:{command:"Get-Content -LiteralPath 'hello.txt'"}},{text:"已完成"},{text:"已恢复会话"}]);
 const approvals=[];let agent=await createWinOneSession(f.config,{authorize:async value=>{approvals.push(value.tool);return true;}});
 await agent.session.prompt("Create, edit and read hello.txt.");
 assert.equal(await fs.readFile(path.join(f.cwd,"hello.txt"),"utf8"),"second");
 assert.deepEqual(approvals,["write","edit","powershell"]);
 const state=agent.state();assert.ok(state.file?.endsWith(".jsonl"));assert.ok((await fs.readFile(state.file,"utf8")).includes('"type":"session"'));
 await agent.close();agent=await createWinOneSession({...f.config,sessionFile:state.file},{authorize:async()=>false});
 await agent.session.prompt("What did we do earlier?");
 assert.ok(f.requests.at(-1).messages.some(message=>JSON.stringify(message).includes("second")),"Pi must restore tool results through SessionManager");
 await agent.close();
});

test('PowerShell preserves Chinese files, native stdin and UTF-8 output through the actual model context',async(t)=>{
 const command=[
  "Get-Content -LiteralPath '中文文件.txt'",
  "Set-Content -LiteralPath '中文输出.txt' -Value '文件写入中文'",
  "'管道输入中文' | & '"+process.execPath.replaceAll("'","''")+"' -e \"process.stdin.setEncoding('utf8');process.stdin.on('data',value=>process.stdout.write(value))\"",
  "Write-Output '标准输出中文'",
  "[Console]::Error.WriteLine('标准错误中文')",
 ].join('\n');
 const f=await fixture(t,[{tool:'powershell',args:{command}},{text:'中文读取完成'}]);
 await fs.writeFile(path.join(f.cwd,'中文文件.txt'),'UTF-8 无 BOM 中文文件\n','utf8');
 const agent=await createWinOneSession(f.config,{authorize:async()=>true});t.after(()=>agent.close());
 await agent.session.prompt('读取中文文件并检查管道。');
 const result=agent.session.messages.find(message=>message.role==='toolResult');
 assert.equal(result?.isError,false);
 const output=result.content.map(part=>part.text??'').join('');
 for(const text of ['UTF-8 无 BOM 中文文件','管道输入中文','标准输出中文','标准错误中文'])assert.ok(output.includes(text),JSON.stringify(output));
 assert.ok(!output.includes('\uFFFD'));
 assert.ok(JSON.stringify(f.requests.at(-1).messages).includes('UTF-8 无 BOM 中文文件'));
 assert.equal((await fs.readFile(path.join(f.cwd,'中文输出.txt'),'utf8')).replace(/^\uFEFF/,'').trim(),'文件写入中文');
});

test("a denied tool cannot write even when the model requests it",async(t)=>{
 const f=await fixture(t,[{tool:"write",args:{path:"denied.txt",content:"must not be written"}},{text:"操作已拒绝"}]);
 const agent=await createWinOneSession(f.config,{authorize:async()=>false});
 await agent.session.prompt("Try the write.");
 await assert.rejects(fs.stat(path.join(f.cwd,"denied.txt")),{code:"ENOENT"});await agent.close();
});

test("Ask requires approval outside the project",async(t)=>{
 const f=await fixture(t,[{tool:"write",args:{path:"../outside.txt",content:"must not be written"}},{text:"操作已拒绝"}]);
 let outside=false;const agent=await createWinOneSession({...f.config,permission:"ask"},{authorize:async request=>{outside=request.outside;return false;}});
 await agent.session.prompt("Try an outside write.");assert.equal(outside,true);
 await assert.rejects(fs.stat(path.join(f.root,"outside.txt")),{code:"ENOENT"});await agent.close();
});

test("Auto reviews a command with the selected model before presenting its approval",async(t)=>{
 const f=await fixture(t,[{tool:"powershell",args:{command:"./unknown-tool.exe"}},{text:'{"verdict":"ask","reason":"Confirm this executable"}'},{text:"操作已拒绝"}]);
 let asked=0;const agent=await createWinOneSession({...f.config,permission:"auto"},{authorize:async request=>{asked++;assert.equal(request.reason,"Confirm this executable");return false;}});
 try{await agent.session.prompt("Inspect the workspace.");assert.equal(asked,1);assert.equal(f.requests[1].stream,false);assert.equal(f.requests[1].tools,undefined);assert.ok(f.requests[1].messages[1].content.includes("Inspect the workspace."));}
 finally{await agent.close();}
});

test('AGENTS.md and configured skills reach the actual model request',async(t)=>{
 const f=await fixture(t,[{text:'Resources received'}]);
 await fs.writeFile(path.join(f.cwd,'AGENTS.md'),'Apply PROJECT_INSTRUCTION_MARKER.');
 const skill=path.join(f.root,'custom-skills','sample');await fs.mkdir(skill,{recursive:true});
 await fs.writeFile(path.join(skill,'SKILL.md'),'---\nname: sample\ndescription: CUSTOM_SKILL_MARKER workflow\n---\nUse this workflow.');
 const agent=await createWinOneSession({...f.config,skillDirectories:[path.dirname(skill)]});
 try{await agent.session.prompt('Inspect context.');const body=JSON.stringify(f.requests[0]);assert.match(body,/PROJECT_INSTRUCTION_MARKER/);assert.match(body,/CUSTOM_SKILL_MARKER/);assert.equal(agent.state().resources.skills.find(skill=>skill.name==='sample')?.path,path.join(skill,'SKILL.md'));}
 finally{await agent.close();}
});

test('a model without tool support receives a real tool-free request',async(t)=>{
 const f=await fixture(t,[{text:'Text only answer'}]);const agent=await createWinOneSession({...f.config,model:{...f.config.model,tools:false}});
 try{await agent.session.prompt('Reply in plain text.');assert.deepEqual(agent.session.getActiveToolNames(),[]);assert.ok(!f.requests[0].tools?.length);}
 finally{await agent.close();}
});

test('an explicitly added plugin tool runs through the same permission review',async(t)=>{
 const f=await fixture(t,[{tool:'plugin_echo',args:{text:'plugin input'}},{text:'Plugin complete'}]);
 const plugin=path.join(f.root,'example-plugin');await fs.mkdir(plugin);
 await fs.writeFile(path.join(plugin,'index.mjs'),`export default function(pi){pi.registerTool({name:'plugin_echo',label:'Echo',description:'Echo input',parameters:{type:'object',properties:{text:{type:'string'}},required:['text']},execute:async(id,args)=>({content:[{type:'text',text:'PLUGIN_EXECUTED '+args.text}],details:{}})});}`);
 const asked=[];const agent=await createWinOneSession({...f.config,pluginDirectories:[plugin]},{authorize:async request=>{asked.push(request.tool);return true;}});
 try{await agent.session.prompt('Run the plugin.');assert.deepEqual(asked,['plugin_echo']);assert.match(JSON.stringify(f.requests.at(-1)),/PLUGIN_EXECUTED plugin input/);}
 finally{await agent.close();}
});

test('HTTP MCP tool discovery and calls use the shared permission pipeline',async(t)=>{
 let calls=0;const methods=[];
 const mcp=http.createServer(async(req,res)=>{
  if(req.method!=='POST'){res.writeHead(405);res.end();return;}
  const chunks=[];for await(const chunk of req)chunks.push(chunk);const message=JSON.parse(Buffer.concat(chunks));methods.push(message.method);
  if(message.id===undefined){res.writeHead(202);res.end();return;}
  let result;
  if(message.method==='initialize')result={protocolVersion:message.params.protocolVersion,capabilities:{tools:{}},serverInfo:{name:'fixture',version:'1.0'}};
  else if(message.method==='tools/list')result={tools:[{name:'echo',description:'Echo the supplied text',inputSchema:{type:'object',properties:{text:{type:'string'}},required:['text']}}]};
  else if(message.method==='tools/call'){calls++;result={content:[{type:'text',text:'MCP_RESULT '+message.params.arguments.text}]};}
  else result={};
  res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({jsonrpc:'2.0',id:message.id,result}));
 });
 await new Promise(resolve=>mcp.listen(0,'127.0.0.1',resolve));t.after(()=>new Promise(resolve=>mcp.close(resolve)));
 for(const allow of [false,true]){
  const f=await fixture(t,[{tool:'mcp__fixture__echo',args:{text:'hello'}},{text:'Finished'}]);let reviewed=0;
  const agent=await createWinOneSession({...f.config,mcpServers:[{name:'fixture',enabled:true,url:`http://127.0.0.1:${mcp.address().port}/mcp`}]},{authorize:async request=>{assert.equal(request.tool,'mcp__fixture__echo');reviewed++;return allow;}});
  try{await agent.session.prompt('Call the MCP echo tool.');assert.equal(reviewed,1,JSON.stringify({methods,tools:agent.session.getActiveToolNames(),diagnostics:agent.state().resources.diagnostics,messages:agent.session.messages.filter(message=>message.role==='toolResult')}));assert.equal(calls,allow?1:0);if(allow)assert.match(JSON.stringify(f.requests.at(-1)),/MCP_RESULT hello/);}
  finally{await agent.close();}
 }
});
