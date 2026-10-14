// @vitest-environment jsdom
import {act,cleanup,render,screen,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {afterEach,expect,it,vi} from "vitest";
import {ChatAutomations} from "./chat-automations";
import type {ScheduleRun,WinOneSchedule} from "./chat-automations-types";
const native=vi.hoisted(()=>({call:vi.fn(),pick:vi.fn()}));
vi.mock("../bridge",()=>({chatCall:native.call,pickDirectory:native.pick}));
afterEach(()=>{cleanup();vi.resetAllMocks();});
const model={provider:"provider",model:"model"},providers=[{id:"provider",name:"Service",authMode:"api-key" as const,hasKey:true,models:["model"],modelNames:{model:"Model"}}];
const schedule:WinOneSchedule={id:"task",name:"Daily review",prompt:"Check the project",kind:"cron",cronExpr:"0 9 * * *",timezone:"Asia/Shanghai",recurring:true,manual:false,agentKind:"pi",modelAgentKind:"pi",providerId:"provider",model:"model",workspaceKind:"dialogue",useWorktree:false,notify:{desktop:true,feishu:false},status:"active",createdAt:1,updatedAt:1,unreadFailureCount:1};
const failedRun:ScheduleRun={id:"run1",scheduleId:"task",sessionId:"conversation",firedAt:Date.UTC(2026,9,10),finishedAt:Date.UTC(2026,9,10)+1500,status:"failed",errorMsg:"Connection lost"};
const runtime={schedulerInstanceId:"instance",inFlight:0,slotsInUse:0,maxConcurrentRuns:4,inFlightRuns:[],waitingSchedules:[]};
function backend(){let current=[{...schedule}],runs=[{...failedRun}];native.call.mockImplementation(async(method,params)=>{
 if(method==="automations.list")return current;
 if(method==="automations.runtime")return runtime;
 if(method==="automations.get")return current.find(item=>item.id===params.id);
 if(method==="automations.runs")return runs;
 if(method==="automations.events")return new Promise(()=>{});
 if(method==="automations.preview")return {nextFireAt:Date.UTC(2026,9,11,1),description:"Daily"};
 if(method==="automations.pause"||method==="automations.resume"){current=current.map(item=>item.id===params.id?{...item,status:method==="automations.pause"?"paused":"active"}:item);return;}
 if(method==="automations.delete"){current=current.filter(item=>item.id!==params.id);return;}
 if(method==="automations.markRead"){runs=runs.map(run=>run.id===params.runId?{...run,readAt:Date.now()}:run);current=current.map(item=>({...item,unreadFailureCount:0}));return;}
 if(method==="automations.run")return {runId:"run2"};
 if(method==="automations.create"){const created={...schedule,...params.input,id:"created"};current=[...current,created];return created;}
 if(method==="automations.update"){const updated={...current.find(item=>item.id===params.id),...params.patch};current=current.map(item=>item.id===params.id?updated:item);return updated;}
 return {};
});}
const renderPage=(onConversation=vi.fn())=>render(<ChatAutomations providers={providers} defaultModel={model} conversations={[]} onBack={()=>{}} onConversation={onConversation}/>);
async function editText(user:ReturnType<typeof userEvent.setup>,name:string,value:string){const input=await screen.findByRole("textbox",{name});input.focus();await user.clear(input);await user.type(input,value,{skipClick:true});}

it("shows persisted failures and marks the run read before opening its conversation",async()=>{
 backend();const open=vi.fn();renderPage(open);const user=userEvent.setup();await screen.findByText("Connection lost");expect(screen.getByText("1 次未读失败")).toBeTruthy();await user.click(screen.getByRole("button",{name:"打开执行对话"}));await waitFor(()=>expect(open).toHaveBeenCalledWith("conversation"));expect(native.call).toHaveBeenCalledWith("automations.markRead",{runId:"run1"});
});
it("pauses, resumes and dispatches a task without waiting for completion",async()=>{
 backend();renderPage();const user=userEvent.setup();await user.click(await screen.findByRole("button",{name:"暂停"}));await user.click(await screen.findByRole("button",{name:"恢复"}));await user.click(await screen.findByRole("button",{name:"立即运行"}));await waitFor(()=>expect(native.call).toHaveBeenCalledWith("automations.run",{id:"task"}));expect((await screen.findByRole("button",{name:"立即运行"})).hasAttribute("disabled")).toBe(false);
});
it("deletes only after the task confirmation",async()=>{
 backend();renderPage();const user=userEvent.setup();await user.click(await screen.findByRole("button",{name:"删除"}));await screen.findByRole("dialog");expect(native.call.mock.calls.some(call=>call[0]==="automations.delete")).toBe(false);await user.click(screen.getByRole("button",{name:"删除"}));await waitFor(()=>expect(native.call).toHaveBeenCalledWith("automations.delete",{id:"task"}));await screen.findByText("暂无自动化");
});
it("creates a manual task through the per-task API",async()=>{
 backend();renderPage();const user=userEvent.setup();await user.click(screen.getByRole("button",{name:"新建自动化"}));await editText(user,"任务名称","Manual review");await editText(user,"提示词","Review recent changes");await user.click(screen.getByRole("tab",{name:"执行时间"}));await user.selectOptions(screen.getByRole("combobox",{name:"时间方式"}),"manual");await user.click(screen.getByRole("button",{name:"保存"}));
 await waitFor(()=>expect(native.call).toHaveBeenCalledWith("automations.create",{input:expect.objectContaining({name:"Manual review",prompt:"Review recent changes",manual:true,agentKind:"pi",modelAgentKind:"pi",permissionMode:"ask",providerId:"provider",model:"model"})}));expect(native.call.mock.calls.some(call=>call[0]==="settings.save")).toBe(false);
});
it("keeps an edit after the host rejects saving",async()=>{
 backend();const original=native.call.getMockImplementation()!;native.call.mockImplementation((method,params)=>method==="automations.update"?Promise.reject(Error("Database unavailable")):original(method,params));renderPage();const user=userEvent.setup();await user.click(await screen.findByRole("button",{name:"编辑自动化"}));await editText(user,"任务名称","Retained edit");await waitFor(()=>expect(screen.getByRole("button",{name:"保存"}).hasAttribute("disabled")).toBe(false));await user.click(screen.getByRole("button",{name:"保存"}));await screen.findByText("Database unavailable");expect((screen.getByRole("textbox",{name:"任务名称"}) as HTMLInputElement).value).toBe("Retained edit");
});
it("refreshes after a scheduler event and does not poll the catalog while idle",async()=>{
 backend();const original=native.call.getMockImplementation()!;let emit!:(value:unknown)=>void;let changed=false;
 native.call.mockImplementation((method,params)=>method==="automations.events"?new Promise(resolve=>{emit=resolve;}):method==="automations.list"?Promise.resolve([{...schedule,name:changed?"Event update":schedule.name}]):original(method,params));renderPage();await screen.findByRole("heading",{name:"Daily review"});const before=native.call.mock.calls.filter(call=>call[0]==="automations.list").length;expect(before).toBe(1);
 changed=true;await act(async()=>emit({cursor:1,events:[{type:"changed",scheduleId:"task"}]}));await screen.findByRole("heading",{name:"Event update"});expect(native.call.mock.calls.filter(call=>call[0]==="automations.list")).toHaveLength(2);
});
it("edits a script's command and hook through the real update API",async()=>{
 backend();const fallback=native.call.getMockImplementation()!;const script={...schedule,executionMode:"script",prompt:"",workspaceKind:"project",workingDir:"C:\\project",scriptConfig:{command:"node old.mjs",capabilities:[]}};
 native.call.mockImplementation((method,params)=>method==="automations.get"?Promise.resolve(script):fallback(method,params));renderPage();const user=userEvent.setup();await user.click(await screen.findByRole("button",{name:"编辑自动化"}));await user.click(screen.getByRole("tab",{name:"执行与检查"}));
 await editText(user,"脚本命令","node changed.mjs");await user.click(screen.getByRole("checkbox",{name:"jira.read"}));await user.click(screen.getByRole("checkbox",{name:"运行前执行检查"}));await editText(user,"前置检查命令","node hook.mjs");
 await waitFor(()=>expect(screen.getByRole("button",{name:"保存"}).hasAttribute("disabled")).toBe(false));await user.click(screen.getByRole("button",{name:"保存"}));
 await waitFor(()=>expect(native.call).toHaveBeenCalledWith("automations.update",{id:"task",patch:expect.objectContaining({executionMode:"script",scriptConfig:expect.objectContaining({command:"node changed.mjs",capabilities:["jira.read"]}),preRunHook:{command:"node hook.mjs",timeoutMs:undefined}})}));
});
it("restores a deleted task through its recovery entry",async()=>{
 backend();const fallback=native.call.getMockImplementation()!;native.call.mockImplementation((method,params)=>method==="automations.deleted"?Promise.resolve([{...schedule,name:"Deleted review",deletedAt:1}]):fallback(method,params));renderPage();const user=userEvent.setup();await user.click(screen.getByRole("button",{name:"已删除任务"}));await user.click(await screen.findByRole("button",{name:"恢复任务"}));await waitFor(()=>expect(native.call).toHaveBeenCalledWith("automations.restore",{id:"task"}));
});
it("applies a template into an editable draft without creating a task first",async()=>{
 backend();const fallback=native.call.getMockImplementation()!;const template={id:"template",name:"Template task",prompt:"Check files",description:"Template description",source:"builtin",category:"dev",useWorktree:true,cronExpr:"0 2 * * *"};
 native.call.mockImplementation((method,params)=>method==="automations.templates"?Promise.resolve({categories:[{id:"dev",name:"Development",order:1}],templates:[template]}):method==="automations.template.apply"?Promise.resolve(template):fallback(method,params));renderPage();const user=userEvent.setup();await user.click(screen.getByRole("button",{name:"模板"}));await user.click(await screen.findByRole("button",{name:"Template task"}));await user.click(screen.getByRole("button",{name:"使用模板"}));await screen.findByRole("textbox",{name:"任务名称"});expect((screen.getByRole("textbox",{name:"任务名称"}) as HTMLInputElement).value).toBe("Template task");expect(native.call.mock.calls.some(call=>call[0]==="automations.create")).toBe(false);
});
it("reads and saves project configuration through the project operations",async()=>{
 backend();const fallback=native.call.getMockImplementation()!;native.pick.mockResolvedValue("C:\\project");native.call.mockImplementation((method,params)=>method==="automations.project.read"?Promise.resolve({config:{version:1,schedules:[]}}):fallback(method,params));renderPage();const user=userEvent.setup();await user.click(screen.getByRole("button",{name:"项目配置"}));await user.click(screen.getByRole("button",{name:"选择文件夹"}));await waitFor(()=>expect(native.call).toHaveBeenCalledWith("automations.project.read",{workingDir:"C:\\project"}));await user.click(screen.getByRole("button",{name:"保存"}));await waitFor(()=>expect(native.call).toHaveBeenCalledWith("automations.project.write",{workingDir:"C:\\project",config:{version:1,schedules:[]}}));
});
