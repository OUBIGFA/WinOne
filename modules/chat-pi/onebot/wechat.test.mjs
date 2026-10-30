import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {createOneBotTaskStore} from './task-store.mjs';
import {WechatTaskStore} from '../cindy/wechat/main/im/wechat/taskStore.ts';
import {createOneBotWechat} from './wechat.mjs';
import {createOneBotRuntime} from './runtime.mjs';
import {createIMSecrets} from './im-secrets.mjs';
import {createOneBotPaths} from './content-service.mjs';
import {withIMScope} from './im-scope.mjs';
const base=process.env.WINONE_ONEBOT_TEST_OUTPUT;
if(!base||!path.isAbsolute(base))throw Error('Run through build/test-onebot.mjs');
test('the original WeChat inbox encrypts context tokens and reopens its cursor and pending task',async()=>{
 const directory=await fs.mkdtemp(path.join(base,'wechat-inbox-'));let db=createOneBotTaskStore(directory);
 const key=Buffer.alloc(32,9);let store=new WechatTaskStore(db,key);
 try{
  const now=Date.now();
  db.syncSession({id:'wechat-session',title:'Wechat',agentKind:'pi',workingDir:directory,workspaceKind:'dialogue',source:'wechat',status:'active',createdAt:now,updatedAt:now});
  await store.activateBindingEpoch({bindingEpoch:'binding-one',expectedActiveEpoch:null,initialCursor:'',now});
  const result=await store.commitPollBatch({bindingEpoch:'binding-one',expectedCursor:'',nextCursor:'cursor-one',now,messages:[{id:'incoming-one',platformMessageId:'platform-one',platformSeq:1,peerId:'owner',receivedAt:now,platformCreatedAt:now,sessionId:'wechat-session',conversationEpoch:0,payloadJson:JSON.stringify({text:'Keep this pending'}),contextToken:'private-wechat-context'}]});
  assert.ok(result);
  assert.ok(!JSON.stringify(db.query('SELECT * FROM wechat_inbox')).includes('private-wechat-context'));
  store.destroy();db.close();db=createOneBotTaskStore(directory);store=new WechatTaskStore(db,key);
  assert.equal((await store.getActiveBinding()).cursor,'cursor-one');
  assert.equal(db.queryOne('SELECT COUNT(*) AS count FROM wechat_inbox WHERE status=?',['pending']).count,1);
 }finally{store.destroy();db.close();}
});
test('personal WeChat authorization and durable inbox dispatch reach the native boundary and deliver once',async()=>{
 const directory=await fs.mkdtemp(path.join(base,'wechat-flow-'));
 const runtime=createOneBotRuntime({directory});await runtime.call('profiles.create',{input:{id:'wechat-owner',name:'Wechat owner'}});
 const db=createOneBotTaskStore(directory),paths=createOneBotPaths(directory),sent=[],calls=[],events=[];
 const profile=await runtime.call('profiles.get',{botId:'wechat-owner'});
 db.syncProfile(profile,[]);const now=Date.now();db.syncSession({id:'wechat-canonical',title:'Wechat',agentKind:'pi',workingDir:directory,workspaceKind:'dialogue',source:'wechat',status:'active',createdAt:now,updatedAt:now});
 const scope={key:'wechat-test',paths,closed:false,taskStore:db,profiles:{get:()=>profile},event:event=>events.push(event),host:{openWechatAuthorization:async url=>calls.push(url)},refreshTaskSession:async()=>{}};
 scope.importSecrets=createIMSecrets(paths,Buffer.alloc(32,2).toString('base64'),{warn:()=>{}});
 let first=true;
 const transport={beginAuthorization:async()=>({id:'auth-test',qrCodeUrl:'https://weixin.qq.com/test',createdAt:Date.now()}),waitAuthorization:async()=>({token:'test-token',botId:'test-bot',userId:'owner-peer',baseUrl:'https://ilinkai.weixin.qq.com'}),notifyStart:async()=>{},notifyStop:async()=>{},
  poll:async(_cursor,signal)=>{if(first){first=false;await new Promise(resolve=>setTimeout(resolve,15));return {cursor:'cursor-after-one',messages:[{senderId:'owner-peer',messageId:'platform-one',contextToken:'private-context',text:'Hello from WeChat',media:[],createdAt:Date.now()}]};}return new Promise((resolve,reject)=>{signal.addEventListener('abort',()=>reject(Error('ABORTED')),{once:true});});},
  sendMessage:async input=>{sent.push(input);return {messageId:input.clientId};},getTypingTicket:async()=>'',setTyping:async()=>{},downloadMedia:async()=>new Uint8Array(),uploadMedia:async()=>{throw Error('not needed');},sendMedia:async()=>({})};
 const dispatcher=async input=>{calls.push(input.message.text);return {text:'Native answer',sessionId:'wechat-canonical'};};
 dispatcher.resolve=async()=>({sessionId:'wechat-canonical'});dispatcher.requestId=()=> 'test-native-request';dispatcher.stop=async()=>{};
 const service=withIMScope(scope,()=>createOneBotWechat(scope,{dispatcher,transportFactory:()=>transport}));
 try{
  await withIMScope(scope,()=>service.call('setCompanion',{botId:'wechat-owner'}));await withIMScope(scope,()=>service.call('authorize'));
  const deadline=Date.now()+5000;while(!sent.length&&Date.now()<deadline)await new Promise(resolve=>setTimeout(resolve,20));
  assert.equal(sent[0]?.text,'Native answer',JSON.stringify({sent,calls,events}));assert.equal(calls.filter(value=>value==='Hello from WeChat').length,1);
  assert.equal(db.queryOne('SELECT sync_cursor AS cursor FROM wechat_sync_state WHERE is_active=1').cursor,'cursor-after-one');
 }finally{await withIMScope(scope,()=>service.close());scope.importSecrets.close();db.close();await runtime.close();}
});
