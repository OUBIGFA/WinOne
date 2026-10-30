import {test} from 'node:test';
import assert from 'node:assert/strict';
import http from 'node:http';
import {createOneBotTaskClient} from './task-client.mjs';

test('native receipt recovery retries reads and failed settlement, never task dispatch',async()=>{
 const calls=[],diagnostics=[],execution={instanceId:'accepted-pi',generation:1};
 let settled=0,complete;
 const completed=new Promise(resolve=>{complete=resolve;});
 const server=http.createServer(async(req,res)=>{
  assert.equal(req.headers.authorization,'Bearer fixture-only-key');
  let raw='';for await(const bytes of req)raw+=bytes;
  const input=JSON.parse(raw);calls.push(input);
  res.setHeader('Content-Type','application/json');
  if(calls.length===1){res.statusCode=503;res.end(JSON.stringify({error:'Temporary receipt outage'}));return;}
  res.end(JSON.stringify({value:{cursor:10,terminal:{execution,status:'completed',resultText:'Original result',error:''}}}));
 });
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 const client=createOneBotTaskClient({baseUrl:`http://127.0.0.1:${server.address().port}/v1`,key:'fixture-only-key',event:value=>diagnostics.push(value),settle:async value=>{
  assert.equal(value.resultText,'Original result');assert.deepEqual(value.execution,execution);
  if(++settled===1)throw Error('Temporary persistence failure');
  complete();
 }});
 try{
  const receipt={sessionId:'task',requestId:'original-input',execution};
  client.watchAccepted(receipt);client.watchAccepted(receipt);
  await Promise.race([completed,new Promise((_,reject)=>{const timer=setTimeout(()=>reject(Error('Receipt recovery timed out')),10000);timer.unref();})]);
  assert.equal(calls.length,3);assert.equal(settled,2);
  assert.ok(calls.every(call=>call.method==='wait'&&call.params.requestId==='original-input'));
  assert.equal(diagnostics.length,1);
 }finally{await client.close();server.closeAllConnections();await new Promise(resolve=>server.close(resolve));}
});

test('closing native receipt observation aborts its pending request without resubmitting work',async()=>{
 let called,closed;const entered=new Promise(resolve=>{called=resolve;}),disconnected=new Promise(resolve=>{closed=resolve;});
 const server=http.createServer((req,res)=>{req.resume();res.on('close',closed);called();});
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 const client=createOneBotTaskClient({baseUrl:`http://127.0.0.1:${server.address().port}/v1`,key:'fixture-only-key',event:()=>assert.fail('shutdown is not a task error'),settle:()=>assert.fail('unfinished task was settled')});
 try{
  client.watchAccepted({sessionId:'task',requestId:'original-input',execution:{instanceId:'accepted-pi',generation:1}});
  await entered;await client.close();await disconnected;
 }finally{await client.close();server.closeAllConnections();await new Promise(resolve=>server.close(resolve));}
});
