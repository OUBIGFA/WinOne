// The original 21-tool Orca MCP server through the same Pi adapter used by
// production. HTTP supplies only the controlled native boundary for this test.
import test from 'node:test';
import assert from 'node:assert/strict';
import http from 'node:http';
import {orcaAgentTools} from './orca-agent-tools.mjs';

test('Orca exposes all local controls, preserves model providers and refreshes caller role',async t=>{
 let role='',started=0;const asks=[];
 const server=http.createServer(async(req,res)=>{
  const parts=[];for await(const part of req)parts.push(part);
  assert.equal(req.headers.authorization,'Bearer fixture-orca-key');
  const {method,params}=JSON.parse(Buffer.concat(parts));
  res.setHeader('Content-Type','application/json');
  if(method==='context')return res.end(JSON.stringify({id:'real-host-task',directory:process.cwd(),orcaRole:role,orcaTeamId:'team'}));
  if(method==='startTeam'){assert.equal(params.leadSessionId,'real-host-task');started++;return res.end(JSON.stringify({ok:true,teamId:'team',workerPermissionMode:'auto'}));}
  if(method==='listAvailableModels')return res.end(JSON.stringify({ok:true,agentDeviceId:null,locations:[],pi:[{id:'model',label:'Model',providers:[{id:'vendor',name:'Vendor'}],defaultProviderId:'vendor'}],codex:[],claude_code:[]}));
  res.statusCode=400;res.end(JSON.stringify({error:'unexpected '+method}));
 });
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 const adapter=await orcaAgentTools({orca:{agentKind:'pi',workingDir:process.cwd(),sessionId:'old-captured-task'},ghostBridge:{baseUrl:`http://127.0.0.1:${server.address().port}`,key:'fixture-orca-key'},model:{tools:true},runtimeSettings:{subagents:true}},async input=>asks.push(input));
 t.after(async()=>{await adapter.close();await new Promise(resolve=>server.close(resolve));});
 assert.equal(adapter.tools.length,21);
 const list=await adapter.tools.find(tool=>tool.name==='list_available_models').execute('list',{agent:'pi'});
 const catalog=JSON.parse(list.content[0].text);assert.equal(catalog.pi[0].providers[0].provider_id,'vendor');assert.equal(catalog.pi[0].default_provider_id,'vendor');assert.equal(asks.length,0);
 const start=adapter.tools.find(tool=>tool.name==='start_team');
 assert.equal((await start.execute('start',{})).isError,false);assert.equal(started,1);assert.equal(asks.length,1);
 role='worker';
 assert.equal((await start.execute('wrong-root',{})).isError,true);assert.equal(started,1,'Worker role was refreshed before a mutation');
});
