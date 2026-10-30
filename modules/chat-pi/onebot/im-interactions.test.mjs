import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createOneBotInteractions} from './im-interactions.mjs';
import {createPermissionGate} from '../permissions.mjs';
import {channelForceConfirmToolCall,channelForceConfirmMutatingToolCall} from '../cindy/bots/im-policy/channelToolPolicy.ts';

function fixture(cards=false){
 const messages=[],handlers=new Set(),updates=[];
 const item={record:{id:'connection'},abort:new AbortController(),im:{
  async sendText(to,text){messages.push({to,text});return {messageId:'text'};},
  ...(cards?{onCardAction(fn){handlers.add(fn);return()=>handlers.delete(fn);},async sendInteractiveCard(to,card){messages.push({to,card});return {messageId:'card'};},async updateInteractiveCard(id,card){updates.push({id,card});}}:{})
 }};
 const message={senderId:'g/room',speaker:{id:'owner',isOwner:true},messageId:'source',text:'task',attachments:[]};
 const reply=(text,patch={})=>({...message,...patch,messageId:'reply',text});
 return {item,message,reply,messages,updates,action:value=>{for(const fn of handlers)fn(value);}};
}
const permission={id:'operation',tool:'powershell',args:{command:'Remove-Item ./fixture'}};
const owner=message=>message.speaker?.isOwner===true;
const tick=()=>new Promise(resolve=>setTimeout(resolve,0));

test('text confirmation bypasses an awaiting turn but never accepts another group speaker or original input',async()=>{
 const ui=createOneBotInteractions(),f=fixture();
 const result=ui.request(f.item,f.message,permission,owner);
 assert.equal(ui.consume(f.item,f.reply('允许',{speaker:{id:'other',isOwner:false}})),false);
 assert.equal(ui.consume(f.item,{...f.message,text:'允许'}),false);
 assert.equal(ui.consume(f.item,f.reply('what?')),true);
 assert.equal(ui.consume(f.item,f.reply('允许')),true);
 assert.deepEqual(await result,{allow:true});
 assert.equal(ui.consume(f.item,f.reply('允许')),false);
 assert.ok(f.messages[0].text.includes('Remove-Item'));
});

test('all questions are answered in order with free text; a reply is not a permission grant',async()=>{
 const ui=createOneBotInteractions(),f=fixture();
 const result=ui.request(f.item,f.message,{kind:'question',questions:[{question:'选目录',options:['项目目录','另选目录']},{question:'名称'}]},owner);
 ui.consume(f.item,f.reply('2'));await tick();
 ui.consume(f.item,f.reply('允许'));
 assert.deepEqual(await result,{answers:['另选目录','允许']});
});

test('cards use stored Cindy choices, refuse forged callbacks, and remove session-wide permissions',async()=>{
 const ui=createOneBotInteractions(),f=fixture(true);
 const result=ui.request(f.item,f.message,permission,owner);await tick();
 const card=f.messages[0].card;
 assert.deepEqual(card.buttons.map(button=>button.id),['permission:allow-once','permission:deny']);
 const allow=card.buttons[0];
 f.action({senderId:'other',buttonId:allow.id,payload:allow.payload});
 f.action({senderId:f.message.senderId,buttonId:'forged',payload:allow.payload});
 f.action({senderId:f.message.senderId,buttonId:allow.id,payload:allow.payload});
 assert.deepEqual(await result,{allow:true});await tick();
 assert.deepEqual(f.updates[0].card.buttons,[]);
});

test('disconnect and expiration release pending questions and deny permissions',async()=>{
 const ui=createOneBotInteractions({timeoutMs:10}),f=fixture();
 const result=ui.request(f.item,f.message,permission,owner);
 await new Promise(resolve=>setTimeout(resolve,15));
 assert.deepEqual(await result,{allow:false});
 const again=ui.request(f.item,f.message,{kind:'question',questions:[{question:'保留？'}]},owner);
 const stopped=assert.rejects(again,/确认已取消/);f.item.abort.abort();await stopped;
});

test('IM force-confirm cannot be bypassed by full access, auto review or a previous session approval',async()=>{
 for(const mode of ['ask','auto','bypassPermissions']){
  const approvals=[];
  const gate=createPermissionGate({mode,cwd:process.cwd(),review:async()=>({verdict:'allow'}),authorize:async request=>{approvals.push(request);return {allow:true,scope:'session'};}});
  const request={id:'one',tool:'powershell',args:{command:'Remove-Item ./fixture'},forceConfirm:true};
  await gate.check(request);await gate.check({...request,id:'two'});
  assert.equal(approvals.length,2,mode);assert.equal(approvals[0].scopeLabel,undefined);
 }
 assert.equal(channelForceConfirmToolCall('mcp__plugin__ghost_call',{tool:'call_tool',args:{name:'powershell',args:{command:'Remove-Item ./fixture'}}}),true);
 assert.equal(channelForceConfirmMutatingToolCall('write',{path:'safe.txt'}),true);
 assert.equal(channelForceConfirmToolCall('read',{path:'safe.txt'}),false);
});
