import {expect,it} from "vitest";
import {projectChatProcess} from "./chat-process";

it("groups thinking, working text and tool results while keeping the final answer outside",()=>{
 const items=projectChatProcess([
  {id:"user",role:"user",content:"Create a file",timestamp:1},
  {id:"work",role:"assistant",timestamp:10,content:[{type:"thinking",thinking:"Check the request"},{type:"text",text:"I will create it"},{type:"toolCall",id:"write",name:"write",arguments:{path:"file.txt",content:"test"}}]},
  {id:"result",role:"toolResult",toolCallId:"write",toolName:"write",timestamp:20,content:[{type:"text",text:"File saved"}]},
  {id:"answer",role:"assistant",timestamp:30,durationMs:5,content:[{type:"text",text:"Created file.txt"}]},
 ],[],false);
 expect(items).toHaveLength(2);
 const response=items[1];if(response.kind!=="response")throw Error("response group missing");
 expect(response.answer?.id).toBe("answer");expect(response.entries.map(entry=>entry.kind)).toEqual(["thinking","commentary","tool"]);
 const tool=response.entries[2];if(tool.kind!=="tool")throw Error("tool missing");expect(tool.tool.result).toBe("File saved");expect(tool.tool.running).toBe(false);expect(response.durationMs).toBe(25);
});
it("deduplicates live tool events and separates consecutive user turns",()=>{
 const items=projectChatProcess([{id:"u1",role:"user",content:"One"},{role:"assistant",content:[{type:"toolCall",id:"read",name:"read",arguments:{path:"a"}}]},{id:"u2",role:"user",content:"Two"},{role:"assistant",content:[{type:"text",text:"Second"}]}],[{id:"read",name:"read",args:{path:"a"},running:false,result:"Read"}],false);
 expect(items.filter(item=>item.kind==="response")).toHaveLength(2);
 const tools=items.flatMap(item=>item.kind==="response"?item.entries.filter(entry=>entry.kind==="tool"):[]);expect(tools).toHaveLength(1);
});
it("does not attach new tool activity to the previous reply before the next assistant message",()=>{
 const items=projectChatProcess([{id:"u1",role:"user",content:"One"},{id:"a1",role:"assistant",content:"Done"},{id:"u2",role:"user",content:"Two"}],[{id:"new-tool",name:"read",args:{},running:true}],true);
 const first=items.find(item=>item.kind==="response"&&item.key==="u1");
 if(!first||first.kind!=="response")throw Error("first reply missing");
 expect(first.running).toBe(false);expect(first.entries).toHaveLength(0);
 const last=items.at(-1);if(last?.kind!=="response")throw Error("active process missing");expect(last.key).toBe("u2");expect(last.entries[0]?.kind).toBe("tool");
});
