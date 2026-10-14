// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {act,cleanup,render,screen,waitFor,within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {ChatPage} from "./chat";
import type {ChatCatalog,Conversation} from "./chat-state";

const native=vi.hoisted(()=>({call:vi.fn(),pick:vi.fn(),ai:vi.fn()}));
vi.mock("../bridge",()=>({setFileDropTarget:vi.fn(async()=>{}),chatCall:native.call,pickDirectory:native.pick,pickChatFiles:vi.fn(),captureCall:native.ai}));
afterEach(()=>{cleanup();vi.resetAllMocks();localStorage.clear();});
const model={provider:"service",model:"model"};
const first:Conversation={id:"first",title:"First",directory:"C:\\first",sessionDirectory:"C:\\history",model,permission:"ask",updatedAt:"2026-10-10",createdAt:"2026-10-10"};
function backend(){
 let catalog:ChatCatalog={projects:[],conversations:[first]};
 native.ai.mockResolvedValue({providers:[],tasks:{chat:model}});native.pick.mockResolvedValue("D:\\Projects\\Existing");
 native.call.mockImplementation(async(method,params)=>{
  if(method==="catalog")return catalog;
  if(method==="open")return {messages:[{role:"assistant",content:`Answer ${params.id}`}],hasOlder:false};
  if(method==="draft.get")return {text:"Saved draft"};
  if(method==="poll")return {events:[],cursor:0,approvals:[],running:false,status:"idle"};
  if(method==="project.add"||method==="project.create"){
   const project={id:"opaque-project-id",name:params.name,path:method==="project.add"?params.path:params.parent+"\\"+params.name};
   catalog={...catalog,projects:[project]};return project;
  }
  if(method==="project.git")return {projectId:params.projectId,repository:true,branch:"feature/real"};
  return {};
 });
}
async function projectDialog(user:ReturnType<typeof userEvent.setup>){
 await user.click(within(screen.getByRole("complementary",{name:"项目与对话"})).getByRole("button",{name:"打开或新建项目"}));
 return screen.getByRole("dialog");
}

it("opens an existing folder, selects it, and preserves the new conversation draft",async()=>{
 backend();render(<ChatPage/>);const user=userEvent.setup();await screen.findByRole("button",{name:"First"});
 const composer=screen.getByRole("textbox",{name:"消息"});composer.focus();await user.type(composer,"Keep this draft",{skipClick:true});
 const dialog=await projectDialog(user);await user.click(within(dialog).getByRole("button",{name:"选择位置"}));
 await waitFor(()=>expect((within(dialog).getByRole("textbox",{name:"项目名称"}) as HTMLInputElement).value).toBe("Existing"));
 await user.click(within(dialog).getByRole("button",{name:"打开项目"}));
 await screen.findByRole("button",{name:"项目：Existing"});await screen.findByText("feature/real");
 expect((screen.getByRole("textbox",{name:"消息"}) as HTMLTextAreaElement).value).toBe("Keep this draft");
 expect(native.call).toHaveBeenCalledWith("project.add",{path:"D:\\Projects\\Existing",name:"Existing",permission:"ask"});
 expect(native.call.mock.calls.some(call=>call[0]==="project.create"||call[0]==="conversation.new")).toBe(false);
 expect(screen.queryByText("opaque-project-id")).toBeNull();
});

it("creates a child folder with the chosen name instead of opening its parent",async()=>{
 backend();render(<ChatPage/>);const user=userEvent.setup();await screen.findByRole("button",{name:"First"});
 const dialog=await projectDialog(user);await user.click(within(dialog).getByRole("checkbox",{name:"在所选位置新建项目文件夹"}));
 const name=within(dialog).getByRole("textbox",{name:"项目名称"});name.focus();await user.type(name,"Created",{skipClick:true});
 await user.click(within(dialog).getByRole("button",{name:"选择位置"}));
 expect((within(dialog).getByRole("textbox",{name:"项目名称"}) as HTMLInputElement).value).toBe("Created");
 await user.click(within(dialog).getByRole("button",{name:"创建"}));await screen.findByRole("button",{name:"项目：Created"});
 expect(native.call).toHaveBeenCalledWith("project.create",{parent:"D:\\Projects\\Existing",name:"Created",permission:"ask"});
 expect(native.call.mock.calls.some(call=>call[0]==="project.add")).toBe(false);
});

it("shows project creation errors in the open dialog",async()=>{
 backend();const fallback=native.call.getMockImplementation()!;native.call.mockImplementation((method,params)=>method==="project.add"?Promise.reject(new Error("Folder unavailable")):fallback(method,params));
 render(<ChatPage/>);const user=userEvent.setup();await screen.findByRole("button",{name:"First"});const dialog=await projectDialog(user);
 await user.click(within(dialog).getByRole("button",{name:"选择位置"}));await user.click(within(dialog).getByRole("button",{name:"打开项目"}));
 expect((await within(dialog).findByRole("alert")).textContent).toContain("Folder unavailable");
});

it("waits for the catalog and opens a requested existing conversation once",async()=>{
 backend();const fallback=native.call.getMockImplementation()!;let loaded!:(value:ChatCatalog)=>void;
 native.call.mockImplementation((method,params)=>method==="catalog"?new Promise(resolve=>{loaded=resolve;}):fallback(method,params));
 const handled=vi.fn();const view=render(<ChatPage requestedConversationId="first" onRequestedConversationHandled={handled}/>);
 expect(handled).not.toHaveBeenCalled();expect(native.call.mock.calls.some(call=>call[0]==="open")).toBe(false);
 await act(async()=>loaded({projects:[],conversations:[first]}));await screen.findByText("Answer first");await waitFor(()=>expect(handled).toHaveBeenCalledOnce());
 view.rerender(<ChatPage requestedConversationId="first" onRequestedConversationHandled={handled}/>);
 expect(screen.getByText("Answer first")).toBeTruthy();expect(native.call.mock.calls.filter(call=>call[0]==="open")).toHaveLength(1);
 expect(native.call.mock.calls.some(call=>call[0]==="conversation.new")).toBe(false);
});

it("reports a missing requested conversation without creating one",async()=>{
 backend();const handled=vi.fn();render(<ChatPage requestedConversationId="missing" onRequestedConversationHandled={handled}/>);
 await screen.findByText("该对话不存在或已被删除。");await waitFor(()=>expect(handled).toHaveBeenCalledOnce());
 expect(native.call.mock.calls.some(call=>call[0]==="open"||call[0]==="conversation.new")).toBe(false);
});

it("keeps the current transcript and draft when that conversation is requested again",async()=>{
 backend();const handled=vi.fn();const view=render(<ChatPage/>);const user=userEvent.setup();await user.click(await screen.findByRole("button",{name:"First"}));await screen.findByText("Answer first");
 await waitFor(()=>expect((screen.getByRole("textbox",{name:"消息"}) as HTMLTextAreaElement).value).toBe("Saved draft"));
 const composer=screen.getByRole("textbox",{name:"消息"});composer.focus();await user.type(composer," edited",{skipClick:true});
 view.rerender(<ChatPage requestedConversationId="first" onRequestedConversationHandled={handled}/>);await waitFor(()=>expect(handled).toHaveBeenCalledOnce());
 expect(screen.getByText("Answer first")).toBeTruthy();expect((screen.getByRole("textbox",{name:"消息"}) as HTMLTextAreaElement).value).toBe("Saved draft edited");
 expect(native.call.mock.calls.filter(call=>call[0]==="open")).toHaveLength(1);
});

it("lets the latest request win while a previous conversation draft is saving",async()=>{
 backend();const fallback=native.call.getMockImplementation()!;let saved!:()=>void;
 const second={...first,id:"second",title:"Second"},third={...first,id:"third",title:"Third",archived:true};
 native.call.mockImplementation((method,params)=>method==="catalog"?Promise.resolve({projects:[],conversations:[first,second,third]}):method==="draft.save"?new Promise<void>(resolve=>{saved=resolve;}):fallback(method,params));
 const handled=vi.fn();const view=render(<ChatPage/>);const user=userEvent.setup();await user.click(await screen.findByRole("button",{name:"First"}));await screen.findByText("Answer first");
 const composer=screen.getByRole("textbox",{name:"消息"});composer.focus();await user.type(composer,"X",{skipClick:true});
 view.rerender(<ChatPage requestedConversationId="second" onRequestedConversationHandled={handled}/>);
 view.rerender(<ChatPage requestedConversationId="third" onRequestedConversationHandled={handled}/>);
 await act(async()=>saved());await screen.findByText("Answer third");await waitFor(()=>expect(handled).toHaveBeenCalledOnce());
 expect(native.call.mock.calls.filter(call=>call[0]==="open").map(call=>call[1].id)).toEqual(["first","third"]);
 expect(screen.getByRole("button",{name:"Third"})).toBeTruthy();
 expect(native.call.mock.calls.some(call=>call[0]==="conversation.update")).toBe(false);
});
