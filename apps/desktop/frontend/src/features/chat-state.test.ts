import {expect,it} from "vitest";
import {applyChatEvent,emptyChatState} from "./chat-state";
import {projectChatProcess} from './chat-process';

it('keeps a retried user before the first reply and retires the old branch immediately',()=>{
 const state={...emptyChatState(),messages:[{id:'old-user',role:'user',content:'Retry me'},{id:'old-answer',role:'assistant',content:'Retired answer'}]};
 const replacement={id:'retry',requestId:'retry',turnId:'retry',role:'user',content:'Retry me',pending:true};
 let next=applyChatEvent(state,{type:'branch_reset',state:{messages:[replacement],hasOlder:false,streaming:true}});
 next=applyChatEvent(next,{type:'delta',kind:'text_delta',turnId:'retry',timestamp:10,text:'New answer'});
 expect(next.messages.some(message=>message.content==='Retired answer')).toBe(false);
 const items=projectChatProcess(next.messages,next.tools,next.running,next.operations);
 expect(items.map(item=>item.kind)).toEqual(['user','response']);expect(items[0].key).toBe('retry');
});
it('uses receipt identity when a steer overtakes a queued optimistic message',()=>{
 let state:ReturnType<typeof emptyChatState>={...emptyChatState(),messages:[{role:'user',requestId:'queue',content:'Next task',pending:true},{role:'user',requestId:'steer',content:'Correction',pending:true}]};
 state=applyChatEvent(state,{type:'pi_event',eventType:'message_start',message:{id:'saved-steer',requestId:'steer',turnId:'steer',role:'user',content:'Correction',timestamp:10}});
 state=applyChatEvent(state,{type:'pi_event',eventType:'message_end',message:{id:'answer',turnId:'steer',role:'assistant',content:'Corrected',timestamp:20}});
 expect(state.messages.map(message=>message.content)).toEqual(['Correction','Corrected','Next task']);
 expect(state.messages[0].pending).toBeUndefined();
});
it('anchors a live response after its user even when the partial arrives first',()=>{
 const items=projectChatProcess([{role:'assistant',turnId:'retry',timestamp:10,content:'Streaming'},{role:'user',turnId:'retry',content:'Retry me'}],[],true,[]);
 expect(items.map(item=>item.kind)).toEqual(['user','response']);expect(items[0].key).toBe('retry');
});
it('reconciles a snapshot ID with the same live assistant instead of duplicating it',()=>{
 let state:ReturnType<typeof emptyChatState>={...emptyChatState(),messages:[{id:'saved-answer',role:'assistant',turnId:'turn',timestamp:10,content:'Partial'}]};
 state=applyChatEvent(state,{type:'pi_event',eventType:'message_end',message:{role:'assistant',turnId:'turn',timestamp:10,content:'Full'}});
 expect(state.messages).toHaveLength(1);expect(state.messages[0].content).toBe('Full');expect(state.messages[0].id).toBe('saved-answer');
});

it('keeps paused task ownership and restores the running snapshot on explicit resume',()=>{
 const taskGraph={scheduling:'paused',total:1,tasks:[{id:3,kind:'fixture',conversationId:1,background:false,state:'ready'}]};
 let state=applyChatEvent(emptyChatState(),{type:'durable_state',paused:true,taskGraph});
 expect(state.paused).toBe(true);expect(state.taskGraph).toEqual(taskGraph);
 state=applyChatEvent(state,{type:'resumed',state:{messages:[{role:'user',content:'Keep history'}],hasOlder:false,paused:false,streaming:true,taskGraph:{...taskGraph,scheduling:'running'}}});
 expect(state.paused).toBe(false);expect(state.running).toBe(true);expect(state.messages[0].content).toBe('Keep history');
});

it('keeps committed workflow updates and completed plans in the conversation snapshot',()=>{
 const workflow={items:[{step:'Read configuration',status:'in_progress' as const}],explanation:'Check before changing'};
 let state=applyChatEvent(emptyChatState(),{type:'durable_state',workflow});
 expect(state.workflow).toEqual(workflow);
 state=applyChatEvent(state,{type:'durable_state',workflow:{items:[{step:'Read configuration',status:'completed'}]}});
 expect(state.workflow?.items[0].status).toBe('completed');
});

it("keeps tool activity and answer deltas distinct, then replaces live output with saved Pi messages",()=>{
 let state=emptyChatState();
 state=applyChatEvent(state,{type:"pi_event",eventType:"message_start",message:{role:"assistant",timestamp:1,content:[]}});
 state=applyChatEvent(state,{type:"delta",kind:"thinking_delta",index:0,text:"Consider"});
 state=applyChatEvent(state,{type:"delta",kind:"text_delta",index:1,text:"Answer"});
 expect(state.messages[0].content).toEqual([{type:"thinking",thinking:"Consider"},{type:"text",text:"Answer"}]);
 state=applyChatEvent(state,{type:"pi_event",eventType:"tool_execution_start",toolCallId:"write1",toolName:"write",args:{path:"proof.txt"}});
 expect(state.tools[0].running).toBe(true);
 state=applyChatEvent(state,{type:"settled",status:"completed",state:{messages:[{id:"saved1",role:"assistant",content:[{type:"text",text:"Saved answer"}]}],hasOlder:false}});
 expect(state.messages).toHaveLength(1);expect(state.messages[0].id).toBe("saved1");expect(state.running).toBe(false);
});
it("retains a pending permission until the host confirms resolution",()=>{
 let state=applyChatEvent(emptyChatState(),{type:"approval",request:{id:"approve1",tool:"powershell",args:{command:"build"}}});
 expect(state.approvals[0].id).toBe("approve1");
 state=applyChatEvent(state,{type:"approval_cancelled",id:"approve1"});expect(state.approvals).toEqual([]);
});

it('reconciles the optimistic user row with live and persisted receipt identities',()=>{
 let state=emptyChatState();state.messages=[{id:'request-1',requestId:'request-1',role:'user',content:'Same text',timestamp:10}];
 state=applyChatEvent(state,{type:'pi_event',eventType:'message_start',message:{requestId:'request-1',turnId:'request-1',role:'user',content:'Same text',timestamp:20}});
 expect(state.messages).toHaveLength(1);
 state=applyChatEvent(state,{type:'pi_event',eventType:'message_end',message:{id:'saved-1',requestId:'request-1',role:'user',content:'Same text',timestamp:20}});
 expect(state.messages).toHaveLength(1);expect(state.messages[0].id).toBe('saved-1');
 state=applyChatEvent(state,{type:'pi_event',eventType:'message_start',message:{requestId:'request-2',role:'user',content:'Same text',timestamp:30}});
 expect(state.messages).toHaveLength(2);
});

it('tracks durable usage and collaboration updates without touching messages',()=>{
 let state=emptyChatState();
 state=applyChatEvent(state,{type:'usage',usage:{input:10,output:5,cacheRead:0,cacheWrite:0,totalTokens:15,cost:0,costKnown:false},contextUsage:{tokens:15,contextWindow:1000,percent:1.5}});
 expect(state.contextUsage?.percent).toBe(1.5);expect(state.usage?.totalTokens).toBe(15);
 state=applyChatEvent(state,{type:'durable_state',subagents:[{name:'scout',status:'running'}],backgroundRunning:true});
 expect(state.subagents?.[0].name).toBe('scout');expect(state.backgroundRunning).toBe(true);
 expect(state.messages).toEqual([]);
});
