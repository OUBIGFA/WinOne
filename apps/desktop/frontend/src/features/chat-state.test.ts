import {expect,it} from "vitest";
import {applyChatEvent,emptyChatState} from "./chat-state";

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
