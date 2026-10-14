import {expect,it} from "vitest";
import {applyChatEvent,emptyChatState} from "./chat-state";
import {projectChatProcess} from "./chat-process";

it("retains one operation with its final duration and keeps it outside the answer",()=>{
 let view=emptyChatState();view.messages=[{id:"u",role:"user",content:"Run",turnId:"u"},{id:"a",role:"assistant",content:"Done",turnId:"u",timestamp:110}];
 view=applyChatEvent(view,{type:"operation",operation:{id:"retry",turnId:"u",kind:"retry",startedAt:100,status:"running",attempt:1,maxAttempts:2}});
 view=applyChatEvent(view,{type:"operation",operation:{id:"retry",turnId:"u",kind:"retry",startedAt:100,status:"completed",durationMs:20,attempt:1,maxAttempts:2}});
 expect(view.operations).toHaveLength(1);
 const items=projectChatProcess(view.messages,[],false,view.operations),group=items[1];
 if(group.kind!=="response")throw Error("missing response");
 expect(group.answer?.content).toBe("Done");expect(group.entries).toHaveLength(1);
 expect(group.entries[0].kind).toBe("operation");if(group.entries[0].kind!=="operation")throw Error("missing operation");expect(group.entries[0].operation.durationMs).toBe(20);
});

it("uses a persisted turn duration rather than inferring elapsed time from message timestamps",()=>{
 const items=projectChatProcess([{id:"u",role:"user",content:"Run",turnId:"u"},{role:"assistant",content:"Done",timestamp:9999,turnId:"u"}],[],false,[{id:"run",kind:"turn",turnId:"u",status:"completed",startedAt:200,durationMs:43}]);
 const group=items[1];if(group.kind!=="response")throw Error("missing response");expect(group.startedAt).toBe(200);expect(group.durationMs).toBe(43);expect(group.entries).toHaveLength(0);
});

it("a delta after resuming a stream cannot overwrite the previous turn",()=>{
 const before={...emptyChatState(),messages:[{role:"assistant",content:"Previous",timestamp:1,turnId:"old"},{role:"user",content:"Next",timestamp:2,turnId:"new"}]};
 const after=applyChatEvent(before,{type:"delta",kind:"text_delta",text:"New answer",turnId:"new",timestamp:3,index:0});
 expect(after.messages[0].content).toBe("Previous");expect(after.messages).toHaveLength(3);expect(after.messages[2].turnId).toBe("new");
});
