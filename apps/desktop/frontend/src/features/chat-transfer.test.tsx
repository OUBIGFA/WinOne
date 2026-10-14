// @vitest-environment jsdom
import {act,cleanup,render,screen,waitFor,within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {afterEach,expect,it,vi} from "vitest";
import {ChatPage} from "./chat";
const native=vi.hoisted(()=>({call:vi.fn(),ai:vi.fn()}));
vi.mock("../bridge",()=>({chatCall:native.call,captureCall:native.ai,setFileDropTarget:vi.fn(async()=>{})}));
afterEach(()=>{cleanup();vi.resetAllMocks();});
// JSDOM has no splitter geometry; exercise the menu's real keyboard path.
async function openMenu(user:ReturnType<typeof userEvent.setup>,name:string){const trigger=await screen.findByRole("button",{name});trigger.focus();await user.keyboard("{ArrowDown}");}
function backend(){
 const first={id:"first",title:"First",projectId:"alpha",directory:"C:\\Alpha",sessionDirectory:"C:\\history\\first",model:{provider:"service",model:"model"},permission:"ask",createdAt:"2026-10-11",updatedAt:"2026-10-11"};
 let conversations=[first,{...first,id:"second",title:"Second",projectId:"beta",directory:"C:\\Beta"}];
 const projects=[{id:"alpha",name:"Alpha",path:"C:\\Alpha"},{id:"beta",name:"Beta",path:"C:\\Beta"}];
 native.ai.mockResolvedValue({providers:[{id:"service",name:"Service",models:["model"]}],tasks:{}});
 native.call.mockImplementation(async(method,params)=>{
  if(method==="catalog")return {projects,conversations};
  if(method==="open"||method==="state")return {messages:[{role:"assistant",content:"Keep this answer"}],hasOlder:false};
  if(method==="draft.get")return {text:"Keep this draft"};
  if(method==="poll")return {events:[],cursor:0,running:false,status:"idle",approvals:[]};
  if(method==="conversation.move"){conversations=conversations.map(c=>c.id===params.id?{...c,projectId:params.projectId,directory:projects.find(p=>p.id===params.projectId)?.path??c.directory}:c);return conversations.find(c=>c.id===params.id);}
  if(method==="export")return {status:"ok",fidelity:"full",filePath:"C:\\exports\\task.cshare",mediaMissing:0,missingTranscripts:[]};
  return {};
 });
}

it("moves a non-active chat to another project and preserves the open answer and draft",async()=>{
 backend();render(<ChatPage/>);const user=userEvent.setup();await user.click(await screen.findByRole("button",{name:"First"}));
 const answer=await screen.findByText("Keep this answer"),input=screen.getByRole("textbox",{name:"消息"});await waitFor(()=>expect((input as HTMLTextAreaElement).value).toBe("Keep this draft"));
 await openMenu(user,"更多操作 · Second");await user.click(await screen.findByRole("menuitem",{name:"移到项目"}));
 const dialog=await screen.findByRole("dialog");await user.click(within(dialog).getByRole("button",{name:/Alpha/}));await user.click(within(dialog).getByRole("button",{name:"移动"}));
 await waitFor(()=>expect(native.call).toHaveBeenCalledWith("conversation.move",{id:"second",projectId:"alpha"}));
 expect(screen.getByText("Keep this answer")).toBe(answer);expect(screen.getByRole("textbox",{name:"消息"})).toBe(input);expect((input as HTMLTextAreaElement).value).toBe("Keep this draft");
});

it("suppresses the retiring runtime's poll error while moving the active chat",async()=>{
 backend();const original=native.call.getMockImplementation()!;let rejectPoll!:(reason:unknown)=>void,finishMove!:()=>void,polled=false;
 const pendingPoll=new Promise((_,reject)=>{rejectPoll=reject;});
 native.call.mockImplementation((method,params)=>{
  if(method==="poll"&&!polled){polled=true;return pendingPoll;}
  if(method==="conversation.move")return new Promise(resolve=>{finishMove=()=>resolve(original(method,params));});
  return original(method,params);
 });
 render(<ChatPage/>);const user=userEvent.setup();await user.click(await screen.findByRole("button",{name:"First"}));
 const answer=await screen.findByText("Keep this answer"),input=screen.getByRole("textbox",{name:"消息"}) as HTMLTextAreaElement;
 await waitFor(()=>expect(input.value).toBe("Keep this draft"));
 await openMenu(user,"更多操作 · First");await user.click(await screen.findByRole("menuitem",{name:"移到项目"}));
 const dialog=await screen.findByRole("dialog");await user.click(within(dialog).getByRole("button",{name:/Beta/}));await user.click(within(dialog).getByRole("button",{name:"移动"}));
 await waitFor(()=>expect(native.call).toHaveBeenCalledWith("conversation.move",{id:"first",projectId:"beta"}));
 await act(async()=>rejectPoll(Error("请先打开对话")));
 expect(screen.queryAllByText("请先打开对话")).toHaveLength(0);
 await act(async()=>finishMove());await waitFor(()=>expect(screen.queryByRole("dialog")).toBeNull());
 expect(screen.getByText("Keep this answer")).toBe(answer);expect(input.value).toBe("Keep this draft");expect(input.disabled).toBe(false);
});

it("clears the moving state after a failed move without changing the current answer or draft",async()=>{
 backend();const original=native.call.getMockImplementation()!;
 native.call.mockImplementation((method,params)=>method==="conversation.move"?Promise.reject(Error("Move failed")):original(method,params));
 render(<ChatPage/>);const user=userEvent.setup();await user.click(await screen.findByRole("button",{name:"First"}));
 const answer=await screen.findByText("Keep this answer"),input=screen.getByRole("textbox",{name:"消息"}) as HTMLTextAreaElement;
 await waitFor(()=>expect(input.value).toBe("Keep this draft"));
 await openMenu(user,"更多操作 · First");await user.click(await screen.findByRole("menuitem",{name:"移到项目"}));
 const dialog=await screen.findByRole("dialog");await user.click(within(dialog).getByRole("button",{name:/Beta/}));await user.click(within(dialog).getByRole("button",{name:"移动"}));
 await within(dialog).findByText("Move failed");await user.click(within(dialog).getByRole("button",{name:"取消"}));
 expect(screen.getByText("Keep this answer")).toBe(answer);expect(input.value).toBe("Keep this draft");expect(input.disabled).toBe(false);
});

it("validates export passwords before calling the native complete-archive operation",async()=>{
 backend();render(<ChatPage/>);const user=userEvent.setup();await openMenu(user,"更多操作 · Second");await user.click(await screen.findByRole("menuitem",{name:"导出任务"}));
 const dialog=await screen.findByRole("dialog");await user.click(within(dialog).getByRole("checkbox",{name:"使用密码保护"}));
 await user.type(within(dialog).getByLabelText("密码"),"test-pass");await user.type(within(dialog).getByLabelText("再次输入密码"),"different");
 expect((within(dialog).getByRole("button",{name:"导出"}) as HTMLButtonElement).disabled).toBe(true);
 await user.clear(within(dialog).getByLabelText("再次输入密码"));await user.type(within(dialog).getByLabelText("再次输入密码"),"test-pass");await user.click(within(dialog).getByRole("button",{name:"导出"}));
 await waitFor(()=>expect(native.call).toHaveBeenCalledWith("export",{id:"second",password:"test-pass"}));
 expect(await screen.findByText(/已完整导出/)).toBeTruthy();
});
