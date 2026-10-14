// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {act,cleanup,render,screen,waitFor,within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {ChatPage} from "./chat";
import {i18n} from "../i18n";
const native=vi.hoisted(()=>({call:vi.fn(),pick:vi.fn(),ai:vi.fn()}));
vi.mock("../bridge",()=>({setFileDropTarget:vi.fn(async()=>{}),chatCall:native.call,pickDirectory:native.pick,captureCall:native.ai}));
afterEach(()=>{cleanup();vi.resetAllMocks();});

it("uses the AI service default before the first provider and ignores a removed remembered model",async()=>{
 localStorage.setItem("winone.chat.last-model.v1",JSON.stringify({provider:"removed",model:"gone"}));
 native.ai.mockResolvedValue({providers:[{id:"first",name:"First",models:["first-model"]},{id:"default",name:"Default",models:["default-model"]}],tasks:{default:{provider:"default",model:"default-model"}}});native.call.mockResolvedValue({projects:[],conversations:[]});
 render(<ChatPage/>);await waitFor(()=>expect(screen.getByRole("button",{name:"模型"}).textContent).toContain("default-model"));localStorage.removeItem("winone.chat.last-model.v1");
});
const conversation={id:"conversation",title:"Project chat",directory:"C:\\project",sessionDirectory:"C:\\history",model:{provider:"provider",model:"model"},permission:"ask",updatedAt:"2026-10-09",createdAt:"2026-10-09"};
function backend(){native.ai.mockResolvedValue({providers:[{id:"provider",name:"Local service",models:["model"],modelNames:{model:"Model"}}],tasks:{}});native.call.mockImplementation(async(method,params)=>{
 if(method==="catalog")return {projects:[],conversations:[conversation]};
 if(method==="open"||method==="state")return {messages:[],hasOlder:false};
 if(method==="draft.get")return {text:"Draft text"};
 if(method==="poll")return {events:[],cursor:0,approvals:[],running:false,status:"idle"};
 return {};
});}
it("restores a draft and retains it when prompt submission fails",async()=>{
 backend();const original=native.call.getMockImplementation()!;native.call.mockImplementation((method,params)=>method==="prompt"?Promise.reject(Error("Connection failed")):original(method,params));
 render(<ChatPage/>);const user=userEvent.setup();await user.click(await screen.findByRole("button",{name:"Project chat"}));
 const input=await screen.findByRole("textbox",{name:"消息"});await waitFor(()=>expect((input as HTMLTextAreaElement).value).toBe("Draft text"));
 await user.click(screen.getByRole("button",{name:"发送"}));await screen.findByText("Connection failed");expect((input as HTMLTextAreaElement).value).toBe("Draft text");
});
it("shows the complete operation and uses the explicit session approval scope",async()=>{
 backend();native.call.mockImplementation(async method=>{
  if(method==="catalog")return {projects:[],conversations:[conversation]};
  if(method==="open")return {messages:[],hasOlder:false};
  if(method==="draft.get")return {text:"Retained draft"};
  if(method==="poll")return {events:[],cursor:1,running:true,status:"running",approvals:[{id:"permission1",tool:"powershell",args:{command:"Get-Content proof.txt"},scopeLabel:"Get-Content proof.txt"}]};
  return {};
 });
 render(<ChatPage/>);const user=userEvent.setup();await user.click(await screen.findByRole("button",{name:"Project chat"}));await screen.findAllByText("Get-Content proof.txt");
 await user.click(screen.getByRole("button",{name:"本会话允许此操作"}));
 await waitFor(()=>expect(native.call).toHaveBeenCalledWith("permission",{id:"conversation",approvalId:"permission1",allow:true,scope:"session"}));
});

it("keeps a new message if sending fails while the new conversation is still opening",async()=>{
 backend();let releaseDraft:(value:{text:string})=>void=()=>{};
 const draft=new Promise<{text:string}>(resolve=>{releaseDraft=resolve;});
 const original=native.call.getMockImplementation()!;
 native.call.mockImplementation((method,params)=>{
  if(method==="conversation.new")return Promise.resolve({...conversation,id:"new"});
  if(method==="draft.get")return draft;
  if(method==="prompt")return Promise.reject(Error("Send failed"));
  return original(method,params);
 });
 render(<ChatPage/>);const user=userEvent.setup();const input=await screen.findByRole("textbox",{name:"消息"});
 // jsdom has zero-sized rectangles, so the splitter's pointer hit-testing
 // cannot distinguish the composer. Focus directly for this keyboard test.
 input.focus();await user.type(input,"Keep this new message",{skipClick:true});expect((screen.getByRole("textbox",{name:"消息"}) as HTMLTextAreaElement).value).toBe("Keep this new message");await user.click(screen.getByRole("button",{name:"发送"}));
 await screen.findByText("Send failed");releaseDraft({text:""});
 await waitFor(()=>expect((screen.getByRole("textbox",{name:"消息"}) as HTMLTextAreaElement).value).toBe("Keep this new message"));
});

it("shows provider and model together and chooses chat storage independently of the workspace",async()=>{
 backend();render(<ChatPage/>);const user=userEvent.setup();
 const model=await screen.findByRole("button",{name:"模型"});await waitFor(()=>expect(model.textContent).toContain("Local service"));expect(model.textContent).toContain("Model");
 native.pick.mockResolvedValue("D:\\Chat data");await user.click(screen.getByRole("button",{name:"对话保存位置"}));
 const choices=screen.getAllByRole("button",{name:"选择位置"});await user.click(choices[1]);await user.click(screen.getByRole("button",{name:"应用"}));
 expect(screen.getByText("D:\\Chat data")).toBeTruthy();
 // The chosen folder is metadata for the next conversation, not a project add.
 expect(native.call.mock.calls.some(call=>call[0]==="project.add")).toBe(false);
});

it("changes a model without replacing the transcript, draft or selected model badge",async()=>{
 backend();const original=native.call.getMockImplementation()!;let finish!:(value:unknown)=>void;
 native.ai.mockResolvedValue({providers:[{id:"provider",name:"Local service",models:["model","free-model"],modelNames:{model:"Model","free-model":"Free model"},modelFree:{"free-model":true}}],tasks:{}});
 native.call.mockImplementation((method,params)=>{
  if(method==="open"){if(native.call.mock.calls.filter(call=>call[0]==="open").length>1)return new Promise(resolve=>{finish=resolve;});return Promise.resolve({messages:[{role:"assistant",content:"Visible answer"}],hasOlder:false});}
  if(method==="configure")return Promise.resolve({...conversation,model:params.model});
  return original(method,params);
 });
 render(<ChatPage/>);const user=userEvent.setup();await user.click(await screen.findByRole("button",{name:"Project chat"}));
 const answer=await screen.findByText("Visible answer"),input=screen.getByRole("textbox",{name:"消息"});await waitFor(()=>expect((input as HTMLTextAreaElement).value).toBe("Draft text"));
 await user.click(screen.getByRole("button",{name:"模型"}));await user.click(await screen.findByRole("option",{name:/Free model/}));
 await waitFor(()=>expect(screen.getByRole("button",{name:"模型"}).textContent).toContain("Free model"));
 expect(screen.getByText("Visible answer")).toBe(answer);expect(screen.getByRole("textbox",{name:"消息"})).toBe(input);expect((input as HTMLTextAreaElement).value).toBe("Draft text");
 expect(within(screen.getByRole("button",{name:"模型"})).getByText("免费")).toBeTruthy();
 if(finish)await act(async()=>finish({messages:[{role:"assistant",content:"Visible answer"}],hasOlder:false}));
 expect(screen.getByText("Visible answer")).toBe(answer);
});

it("leaves a reconnecting chat for a usable new conversation and ignores the late reopen",async()=>{
 backend();const original=native.call.getMockImplementation()!;let opens=0,finish!:(value:unknown)=>void;
 native.ai.mockResolvedValue({providers:[{id:"provider",name:"Local service",models:["model","second"],modelNames:{model:"Model",second:"Second model"}}],tasks:{}});
 native.call.mockImplementation((method,params)=>{
  if(method==="open"&&++opens>1)return new Promise(resolve=>{finish=resolve;});
  if(method==="configure")return Promise.resolve({...conversation,model:params.model});
  return original(method,params);
 });
 render(<ChatPage/>);const user=userEvent.setup();await user.click(await screen.findByRole("button",{name:"Project chat"}));
 await waitFor(()=>expect((screen.getByRole("textbox",{name:"消息"}) as HTMLTextAreaElement).value).toBe("Draft text"));
 await user.click(screen.getByRole("button",{name:"模型"}));await user.click(await screen.findByRole("option",{name:/Second model/}));
 await waitFor(()=>expect(opens).toBe(2));
 await user.click(screen.getByRole("button",{name:i18n.t("chat.newChat")}));
 const input=screen.getByRole("textbox",{name:"消息"}) as HTMLTextAreaElement;
 expect(input.disabled).toBe(false);input.focus();await user.type(input,"New draft",{skipClick:true});
 await act(async()=>finish({messages:[{role:"assistant",content:"Late old answer"}],hasOlder:false}));
 expect(screen.queryByText("Late old answer")).toBeNull();expect(input.value).toBe("New draft");
});

it("keeps tool calls and working notes inside the process disclosure",async()=>{
 backend();const original=native.call.getMockImplementation()!;
 native.call.mockImplementation((method,params)=>method==="open"?Promise.resolve({hasOlder:false,messages:[
  {id:"u",role:"user",content:"Create a file"},
  {id:"work",role:"assistant",content:[{type:"thinking",thinking:"Check the request"},{type:"text",text:"Preparing the file"},{type:"toolCall",id:"write",name:"write",arguments:{path:"a.txt",content:"a"}}]},
  {id:"tool-result",role:"toolResult",toolName:"write",toolCallId:"write",content:[{type:"text",text:"Saved output"}]},
  {id:"final",role:"assistant",content:[{type:"text",text:"Final answer"}]},
 ]}):original(method,params));
 render(<ChatPage/>);const user=userEvent.setup();await user.click(await screen.findByRole("button",{name:"Project chat"}));
 const final=await screen.findByText("Final answer");expect(final.closest(".chat-work-process")).toBeNull();expect(screen.queryByText("Preparing the file")).toBeNull();
 await user.click(screen.getByRole("button",{name:/工作过程/}));expect((await screen.findByText("Preparing the file")).closest(".chat-work-process")).not.toBeNull();expect(screen.queryByText("Saved output")).toBeNull();
 const groups=document.querySelectorAll('[data-slot="cindy-work"]');await user.click(groups[groups.length-1]);
 const tool=await screen.findByRole("button",{name:"写入 a.txt"});expect(tool.closest(".chat-work-process")).not.toBeNull();await user.click(tool);
 expect(within(await screen.findByRole("dialog")).getByText("Saved output")).toBeTruthy();
});

it("retains the transcript and draft when the current chat is selected again",async()=>{
 backend();const original=native.call.getMockImplementation()!;
 native.call.mockImplementation((method,params)=>method==="open"?Promise.resolve({hasOlder:false,messages:[{role:"assistant",content:"Retained answer"}]}):original(method,params));
 render(<ChatPage/>);const user=userEvent.setup();const row=await screen.findByRole("button",{name:"Project chat"});await user.click(row);
 await screen.findByText("Retained answer");await user.click(row);
 expect(screen.getByText("Retained answer")).toBeTruthy();expect((screen.getByRole("textbox",{name:"消息"}) as HTMLTextAreaElement).value).toBe("Draft text");
 expect(native.call.mock.calls.filter(call=>call[0]==="open")).toHaveLength(1);
});

it("renders opened messages while the independent draft request is still pending",async()=>{
 backend();const original=native.call.getMockImplementation()!;let finish!:(value:{text:string})=>void;
 const draft=new Promise(resolve=>{finish=resolve;});
 native.call.mockImplementation((method,params)=>method==="draft.get"?draft:method==="open"?Promise.resolve({hasOlder:false,messages:[{role:"assistant",content:"Available now"}]}):original(method,params));
 render(<ChatPage/>);await userEvent.setup().click(await screen.findByRole("button",{name:"Project chat"}));
 await screen.findByText("Available now");await act(async()=>{finish({text:"Loaded later"});});
 await waitFor(()=>expect((screen.getByRole("textbox",{name:"消息"}) as HTMLTextAreaElement).value).toBe("Loaded later"));
});

it("keeps the latest selection when an earlier open finishes late",async()=>{
 backend();const original=native.call.getMockImplementation()!;let finish!:(value:unknown)=>void;
 const first=new Promise(resolve=>{finish=resolve;});
 native.call.mockImplementation((method,params)=>method==="catalog"?Promise.resolve({projects:[],conversations:[conversation,{...conversation,id:"second",title:"Second chat"}]}):method==="open"?(params.id==="conversation"?first:Promise.resolve({hasOlder:false,messages:[{role:"assistant",content:"Second answer"}]})):original(method,params));
 render(<ChatPage/>);const user=userEvent.setup();await user.click(await screen.findByRole("button",{name:"Project chat"}));await user.click(screen.getByRole("button",{name:"Second chat"}));
 await screen.findByText("Second answer");await act(async()=>{finish({hasOlder:false,messages:[{role:"assistant",content:"Stale answer"}]});});
 expect(screen.queryByText("Stale answer")).toBeNull();expect(screen.getByText("Second answer")).toBeTruthy();
});
