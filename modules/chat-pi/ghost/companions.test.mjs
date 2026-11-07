import test from 'node:test';
import assert from 'node:assert/strict';
import {createGhostCompanions} from './platform-companions.mjs';

function fixture(request){
 const row={id:'partner',name:'Partner',description:'Local teammate',avatar:'🐼',canonical:null,activities:[{phase:'needs-interaction'}],
  pending:[{sessionId:'owned-session',fingerprint:'original',request}]};
 const sent=[],confirmations=[];
 let confirm=async value=>{confirmations.push(value);return {confirmed:true};};
 const adapter=createGhostCompanions({canAnswer:()=>true,ui:(kind,value)=>confirm(value),host:async(method,input)=>{
  assert.equal(method,'task');
  if(input.op==='companion-snapshot')return [row];
  if(input.op==='companion-answer'){sent.push(input);return {resolved:true};}
  if(input.op==='companion-open'){sent.push(input);return true;}
  throw Error('Unexpected Host call '+input.op);
 }});
 return {adapter,row,sent,confirmations,setConfirm:handler=>{confirm=handler;},guard(){}};
}

test('companion roster exposes only request summaries, never tool arguments',async()=>{
 const f=fixture({id:'permission-1',requestId:'input-1',tool:'powershell',args:{command:'echo PRIVATE_ARGUMENT'}});
 f.row.pending[0].workingDir='C:/owned-workspace';
 const rows=await f.adapter.list(f.guard);
 assert.equal(rows[0].status,'awaiting');
 assert.equal(rows[0].interactions[0].kind,'permission');
 assert.doesNotMatch(JSON.stringify(rows),/PRIVATE_ARGUMENT|permission-1|owned-session/);
 const detail=await f.adapter.interaction('partner',rows[0].interactions[0].ref,f.guard);
 assert.match(detail.preview,/PRIVATE_ARGUMENT/);
 assert.match(detail.preview,/# cwd: C:\/owned-workspace/);
 assert.deepEqual(await f.adapter.avatar('partner',f.guard),{src:null,fallback:'🐼'});
});

test('a valid question answer retains ordering and rejects fabricated choices',async()=>{
 const f=fixture({id:'question-1',requestId:'input-1',kind:'question',questions:[{question:'Format?',options:['Text','Table']},{question:'Why?'}]});
 const ref=(await f.adapter.list(f.guard))[0].interactions[0].ref;
 await assert.rejects(f.adapter.answer('partner',ref,{kind:'question',answers:[{choices:['Not offered']},{choices:[],text:'Reason'}]},{ghostId:'fixture',guard:f.guard}),/INVALID_PARAMS/);
 assert.equal(f.sent.length,0);
 assert.deepEqual(await f.adapter.answer('partner',ref,{kind:'question',answers:[{choices:['Table']},{choices:[],text:'Reason'}]},{ghostId:'fixture',guard:f.guard}),{answered:true});
 assert.deepEqual(f.sent[0].answers,['Table','Reason']);
 assert.equal(f.sent[0].sessionId,'owned-session');
 assert.equal(f.sent[0].requestId,'input-1');
 assert.equal(f.confirmations.length,0);
});

test('file writes are allowed only in WinOne, but can be declined in the plugin',async()=>{
 const f=fixture({id:'write-1',requestId:'input-1',tool:'write',args:{path:'file.txt',content:'not shown in the permission preview'}});
 const ref=(await f.adapter.list(f.guard))[0].interactions[0].ref;
 const detail=await f.adapter.interaction('partner',ref,f.guard);assert.equal(detail.allowHere,false);
 await assert.rejects(f.adapter.answer('partner',ref,{kind:'permission',decision:'allow'},{ghostId:'fixture',guard:f.guard}),/ALLOW_IN_CINDY/);
 assert.equal(f.sent.length,0);
 await f.adapter.answer('partner',ref,{kind:'permission',decision:'deny'},{ghostId:'fixture',guard:f.guard});
 assert.equal(f.sent[0].allow,false);
});

test('permission confirmation rechecks the exact request before sending an answer',async()=>{
 const f=fixture({id:'permission-1',requestId:'input-1',tool:'powershell',args:{command:'echo original'}});
 const ref=(await f.adapter.list(f.guard))[0].interactions[0].ref;
 f.setConfirm(async()=>{f.row.pending[0].fingerprint='changed';f.row.pending[0].request.args.command='echo changed';return {confirmed:true};});
 await assert.rejects(f.adapter.answer('partner',ref,{kind:'permission',decision:'allow'},{ghostId:'fixture',guard:f.guard}),/TARGET_UNAVAILABLE/);
 assert.equal(f.sent.length,0);
});
