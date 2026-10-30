import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import {createDurableSession} from '../durable-session.mjs';

const base=process.env.WINONE_ONEBOT_TEST_OUTPUT;
if(!base||!path.isAbsolute(base))throw Error('An isolated OneBot test output is required');
for(const rejectRuntime of [false,true])test(rejectRuntime?'a failed runtime update never sends the queued follow-up with old settings':'a queued follow-up adopts new runtime choices after the executing tool and current run finish',async()=>{
 const directory=await fs.mkdtemp(path.join(base,'runtime-refresh-')),cwd=path.join(directory,'workspace'),plugin=path.join(directory,'plugin');
 await fs.mkdir(cwd);await fs.mkdir(plugin);
 const shutdown=path.join(directory,'old-runtime-closed'),finished=path.join(directory,'old-tool-finished');
 let toolStarted,releaseTool;
 const started=new Promise(resolve=>{toolStarted=resolve;}),release=new Promise(resolve=>{releaseTool=resolve;});
 const requests=[];let desired,recipeReads=0,applied=0;
 const server=http.createServer(async(req,res)=>{
  try{
   if(req.url==='/hold'){
    toolStarted();await release;res.setHeader('Content-Type','application/json');res.end('{}');return;
   }
   let text='';for await(const chunk of req)text+=chunk;
   if(req.url==='/onebot/runtime'){
    assert.equal(req.headers.authorization,'Bearer fixture-runtime-token');recipeReads++;
    if(rejectRuntime){res.writeHead(503);res.end('Runtime configuration unavailable');return;}
    res.setHeader('Content-Type','application/json');res.end(JSON.stringify(desired));return;
   }
   if(req.url==='/onebot/runtime.applied'){
    assert.equal(req.headers.authorization,'Bearer fixture-runtime-token');assert.equal(JSON.parse(text).revision,'new-runtime');applied++;
    res.setHeader('Content-Type','application/json');res.end('{}');return;
   }
   const body=JSON.parse(text);requests.push(body);
   res.writeHead(200,{'Content-Type':'text/event-stream'});
   const emit=(delta,finish=null)=>res.write(`data: ${JSON.stringify({id:'fixture',object:'chat.completion.chunk',model:body.model,choices:[{index:0,delta,finish_reason:finish}]})}\n\n`);
   emit({role:'assistant'});
   if(requests.length===1){emit({tool_calls:[{index:0,id:'hold-call',type:'function',function:{name:'hold',arguments:'{}'}}]});emit({},'tool_calls');}
   else{emit({content:requests.length===2?'First run finished':'Follow-up finished'});emit({},'stop');}
   res.end('data: [DONE]\n\n');
  }catch(error){res.writeHead(500);res.end(error.message);}
 });
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 const address=`http://127.0.0.1:${server.address().port}`;
 await fs.writeFile(path.join(plugin,'package.json'),JSON.stringify({name:'held-tools',pi:{extensions:['./extension.mjs']}}));
 await fs.writeFile(path.join(plugin,'extension.mjs'),`import fs from 'node:fs/promises';export default pi=>{
  pi.on('session_shutdown',async()=>{await fs.writeFile(${JSON.stringify(shutdown)},'closed');});
  pi.registerTool({name:'hold',label:'Hold',description:'A real asynchronous fixture tool',parameters:{type:'object',properties:{}},execute:async(_id,_args,signal)=>{
   await fetch(${JSON.stringify(address+'/hold')},{signal});if(signal.aborted)throw Error('old tool was interrupted');
   await fs.writeFile(${JSON.stringify(finished)},'finished');return {content:[{type:'text',text:'Tool finished'}]};
  }});
 };`);
 const skill=path.join(directory,'skills','new-skill');await fs.mkdir(skill,{recursive:true});
 await fs.writeFile(path.join(skill,'SKILL.md'),'---\nname: new-skill\ndescription: Newly selected skill\n---\nUse it on demand.');
 const firstModel={id:'fixture/first',name:'First',reasoning:true,efforts:['low','high']},secondModel={id:'fixture/second',name:'Second',reasoning:true,efforts:['low','high']};
 const config={cwd,agentDir:path.join(directory,'agent'),sessionDirectory:path.join(directory,'sessions'),baseUrl:address+'/v1',key:'fixture-model-token',model:firstModel,models:[firstModel,secondModel],thinkingLevel:'low',permission:'bypassPermissions',
  pluginDirectories:[plugin],skillDirectories:[path.dirname(skill)],runtimeSettings:{codeMode:false,subagents:false,planning:false,autoRetry:false},
  onebot:{botId:'fixture',homeDir:directory,baseUrl:address+'/onebot',key:'fixture-runtime-token',memoryEnabled:false,skillDirectories:[],capabilities:{skillMode:'allowlist',skills:[],toolsetMode:'inherit',toolsets:[]}}};
 desired={revision:'new-runtime',config:{model:secondModel,models:[firstModel,secondModel],thinkingLevel:'high',fastMode:true,skillDirectories:config.skillDirectories,pluginDirectories:config.pluginDirectories,mcpServers:[],
  onebot:{...config.onebot,capabilities:{...config.onebot.capabilities,skills:['new-skill'],toolsetMode:'allowlist',toolsets:[]}}}};
 let agent,first;
 try{
  agent=await createDurableSession(config);
  first=agent.session.prompt('First run',{requestId:'first',inputReceipt:{id:'first'}});first.catch(()=>{});
  await started;
  await agent.requestRuntimeRefresh();
  await agent.session.prompt('Queued follow-up',{requestId:'second',streamingBehavior:'followUp',inputReceipt:{id:'second'}});
  assert.equal(requests.length,1);await assert.rejects(fs.access(shutdown),{code:'ENOENT'});
  releaseTool();await first;await agent.session.waitForIdle();
  assert.equal(await fs.readFile(finished,'utf8'),'finished');
  if(rejectRuntime){
   assert.equal(requests.length,2);
   assert.equal(await agent.session.inputStatus('second'),'unanswered');
   assert.match(agent.state().runtimeError,/Runtime configuration unavailable/);
   return;
  }
  assert.deepEqual(requests.map(request=>request.model),['fixture/first','fixture/first','fixture/second']);
  assert.equal(requests[2].reasoning_effort,'high');assert.equal(requests[2].service_tier,'priority');
  assert.ok(!requests[2].tools.some(tool=>tool.function.name==='hold'));
  assert.deepEqual(agent.state().resources.skills.map(item=>item.name),['new-skill']);
  assert.equal(recipeReads,1);
  assert.equal(applied,1);
  assert.deepEqual(agent.session.sessionManager.getBranch().filter(entry=>entry.type==='message'&&entry.message.role==='assistant').map(entry=>entry.message.modelName),['First','First','Second']);
  const readsBefore=recipeReads;
  agent.requestRuntimeRefresh();agent.requestRuntimeRefresh();
  await agent.session.prompt('After two idle invalidations',{requestId:'third',inputReceipt:{id:'third'}});
  assert.equal(recipeReads,readsBefore+1);
 }finally{releaseTool();await first?.catch(()=>{});await agent?.close();await new Promise(resolve=>server.close(resolve));}
});
