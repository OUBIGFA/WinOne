import {test} from 'node:test';
import assert from 'node:assert/strict';
import http from 'node:http';
import {createOneBotIMDispatcher,oneBotIMTarget} from './im-dispatch.mjs';

test('owner DM routes canonical while owner/guest group turns retain the same isolated lane',()=>{
 const connection={id:'fixture12345',channel:'telegram',botContextId:'bot'};
 const ownerDM=oneBotIMTarget(connection,{senderId:'owner'},'owner');
 assert.equal(ownerDM.routeKey,'canonical');assert.equal(ownerDM.ownerAuthorized,true);
 const group={senderId:'g/-10001',contextId:'bot',speaker:{id:'owner',isOwner:true}};
 const first=oneBotIMTarget(connection,group,'owner');
 const guest=oneBotIMTarget(connection,{...group,speaker:{id:'guest',isOwner:false}},'owner');
 assert.equal(first.group,true);assert.equal(first.ownerAuthorized,true);
 assert.equal(guest.routeKey,first.routeKey);assert.equal(guest.ownerAuthorized,false);
 assert.notEqual(guest.routeKey,'canonical');
});

test('private IM dispatcher sends original prompt, real attachments, streaming text and question/permission decisions',async()=>{
 const requests=[],stream=[],decisions=[];let sendApprovalResult;
 const server=http.createServer(async(req,res)=>{
  try{
   let raw='';for await(const chunk of req)raw+=chunk;const body=JSON.parse(raw);requests.push({url:req.url,body});
   assert.equal(req.headers.authorization,'Bearer fixture-private-im-key');
   res.setHeader('Content-Type','application/json');
   if(req.url==='/v1/im/route'){res.end(JSON.stringify({sessionId:'session',attachments:['staged/file'],allowedFileRoots:['workspace']}));return;}
   if(req.url==='/v1/prepare'){res.end(JSON.stringify({sessionId:'session'}));return;}
   if(req.url==='/v1/im/interaction'){
    decisions.push(body);res.end('{}');sendApprovalResult?.();return;
   }
   if(req.url==='/v1/execute'){
    res.setHeader('Content-Type','application/x-ndjson');
    res.write(JSON.stringify({events:[{data:{type:'delta',kind:'text_delta',text:'正在处理'}}]})+'\n');
    for(const approval of [{id:'p',tool:'write',args:{path:'fixture'}},{id:'q',kind:'question',questions:[{question:'输出格式',options:['文本','表格']}]}]){
     await new Promise(resolve=>{sendApprovalResult=resolve;res.write(JSON.stringify({approvals:[{...approval,requestId:'another-turn'},{...approval,requestId:body.runId}]})+'\n');});
    }
    res.end(JSON.stringify({terminal:{status:'completed',state:{messages:[{role:'assistant',content:[{type:'text',text:'已核对'}]}]}}})+'\n');return;
   }
   throw Error('Unexpected route '+req.url);
  }catch(error){res.statusCode=500;res.end(error.message);}
 });
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 try{
  const dispatch=createOneBotIMDispatcher({baseUrl:`http://127.0.0.1:${server.address().port}/v1`,key:'fixture-private-im-key'});
  const result=await dispatch({connection:{id:'fixture12345',botId:'alpha',channel:'telegram'},message:{senderId:'owner',messageId:'input',text:'检查附件',attachments:[{absPath:'received/file'}]},ownerUserId:'owner',signal:AbortSignal.timeout(5000),onText:text=>stream.push(text),requestInteraction:async request=>request.kind==='question'?{answers:['文本']}:{allow:true}});
  assert.equal(result.text,'已核对');assert.deepEqual(stream,['正在处理']);
  assert.deepEqual(decisions.map(value=>value.allow),[true,false]);assert.deepEqual(decisions[1].answers,['文本']);
  const execute=requests.find(value=>value.url==='/v1/execute').body;
  assert.equal(execute.prompt,'检查附件');assert.deepEqual(execute.attachments,['staged/file']);
  assert.equal(requests[0].body.ownerAuthorized,true);
  assert.equal(requests[0].body.channel,'telegram');
 }finally{await new Promise(resolve=>server.close(resolve));}
});
