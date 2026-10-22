// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen,waitFor,within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {ChatPage} from "./chat";
import {ChatSettings} from "./chat-settings";
import {ChatInspector} from "./chat-inspector";
import {ChatMessageView} from "./chat-messages";
import {emptyChatState,type Conversation} from "./chat-state";

const native=vi.hoisted(()=>({call:vi.fn(),pick:vi.fn(),files:vi.fn(),ai:vi.fn()}));
vi.mock("../bridge",()=>({inspectDroppedPaths:async(paths:string[])=>paths.map(path=>({path,kind:'file'})),setFileDropTarget:vi.fn(async()=>{}),chatCall:native.call,pickDirectory:native.pick,pickChatFiles:native.files,captureCall:native.ai}));
afterEach(()=>{cleanup();vi.resetAllMocks();localStorage.clear();});
const model={provider:"service",model:"model"};
const providers=[{id:"service",name:"Current service",authMode:"api-key" as const,hasKey:false,models:["model"],modelNames:{model:"Current model"}}];
const settings={workspaceDirectory:"C:\\work",skillDirectories:[],pluginDirectories:[],mcpServers:[],automations:[]};

it('saves the MCP exposure choice without changing the server address',async()=>{
 native.call.mockImplementation(async(method,params)=>method==='settings.get'?{...settings,mcpServers:[{name:'fixture',enabled:true,url:'http://127.0.0.1:12345'}]}:params.settings);
 const user=userEvent.setup();render(<ChatSettings open onOpenChange={()=>{}} providers={providers} defaultModel={model} onSaved={()=>{}} onConversation={()=>{}}/>);
 await user.click(await screen.findByRole('tab',{name:'MCP'}));
 await user.selectOptions(screen.getByRole('combobox',{name:'工具调用方式'}),'codemode');
 await user.click(screen.getByRole('button',{name:'保存设置'}));
 await waitFor(()=>expect(native.call).toHaveBeenCalledWith('settings.save',expect.objectContaining({settings:expect.objectContaining({mcpServers:[expect.objectContaining({url:'http://127.0.0.1:12345',exposure:'codemode'})]})})));
});
const first:Conversation={id:"first",title:"First",directory:"C:\\first",sessionDirectory:"C:\\history1",model,permission:"ask",updatedAt:"2026-10-10",createdAt:"2026-10-10"};
const second:Conversation={...first,id:"second",title:"Second",directory:"C:\\second"};
function backend(){
 let catalog={projects:[],conversations:[first,second]};native.ai.mockResolvedValue({providers,tasks:{chat:model}});native.files.mockResolvedValue(["C:\\notes.txt"]);
 native.call.mockImplementation(async(method,params)=>{
  if(method==="catalog")return catalog;
  if(method==="open")return {messages:[{role:"assistant",content:`Answer ${params.id}`}],hasOlder:false};
  if(method==="draft.get")return {text:"Draft"};
  if(method==="poll")return {events:[],cursor:0,approvals:[],running:false,status:"idle"};
  if(method==="conversation.update"){const updated={...catalog.conversations.find(value=>value.id===params.id),...params};catalog={...catalog,conversations:catalog.conversations.map(value=>value.id===params.id?updated:value)};return updated;}
  if(method==="conversation.delete"){catalog={...catalog,conversations:catalog.conversations.filter(value=>value.id!==params.id)};return catalog;}
  if(method==="settings.get")return settings;
  if(method==="settings.save")return params.settings;
  if(method==="attachments.add")return {attachments:[{path:"C:\\history1\\attachments\\notes.txt",name:"notes.txt",size:42,mimeType:"text/plain"}]};
  return {};
 });
}
const menu=async(user:ReturnType<typeof userEvent.setup>,name:string)=>{const trigger=await screen.findByRole("button",{name});trigger.focus();await user.keyboard("{ArrowDown}");};
const type=async(user:ReturnType<typeof userEvent.setup>,input:HTMLElement,text:string)=>{input.focus();await user.clear(input);await user.type(input,text,{skipClick:true});};

it('shows the persisted conversation-text title after AI generation fails',async()=>{
 backend();const original=native.call.getMockImplementation()!;let failed=false;
 native.call.mockImplementation(async(method,params)=>{
  if(method==='title.regenerate'){failed=true;throw Error('Service unavailable');}
  const value=await original(method,params);
  return method==='catalog'&&failed?{...value,conversations:value.conversations.map((row:Conversation)=>row.id===second.id?{...row,title:'Conversation opening fallback'}:row)}:value;
 });
 render(<ChatPage/>);const user=userEvent.setup();await menu(user,'更多操作 · Second');await user.click(await screen.findByRole('menuitem',{name:'AI 重命名'}));
 expect(await screen.findByRole('button',{name:'Conversation opening fallback'})).toBeTruthy();
 expect(screen.getByText('Service unavailable')).toBeTruthy();
 expect(native.call.mock.calls.filter(([method])=>method==='title.regenerate')).toHaveLength(1);
});

it('renames a closed conversation without requiring a live Pi session',async()=>{
 backend();const fallback=native.call.getMockImplementation()!;
 native.call.mockImplementation((method,params)=>method==='state'?Promise.reject(Error('请先打开对话')):method==='title.regenerate'?Promise.resolve({...second,title:'Renamed from saved history'}):fallback(method,params));
 render(<ChatPage/>);const user=userEvent.setup();await menu(user,'更多操作 · Second');await user.click(await screen.findByRole('menuitem',{name:'AI 重命名'}));
 expect(await screen.findByRole('button',{name:'Renamed from saved history'})).toBeTruthy();
 expect(native.call.mock.calls.some(([method])=>method==='state'||method==='open')).toBe(false);
});

it('keeps other conversations and the editor available while AI renames one row',async()=>{
 backend();let rejectRename!:(reason:Error)=>void;
 const fallback=native.call.getMockImplementation()!;
 native.call.mockImplementation((method,params)=>method==='title.regenerate'?new Promise((_,reject)=>{rejectRename=reject;}):method==='state'?Promise.resolve({messages:[]}):fallback(method,params));
 render(<ChatPage/>);const user=userEvent.setup();await user.click(await screen.findByRole('button',{name:'First'}));await screen.findByText('Answer first');
 await menu(user,'更多操作 · Second');await user.click(await screen.findByRole('menuitem',{name:'AI 重命名'}));
 await screen.findByRole('status',{name:'正在生成标题 · Second'});
 expect((screen.getByRole('textbox',{name:'消息'}) as HTMLTextAreaElement).disabled).toBe(false);
 await user.click(screen.getByRole('button',{name:'Second'}));await screen.findByText('Answer second');
 rejectRename(new Error('Service unavailable'));
 expect(await screen.findByRole('alert',{name:'标题生成失败 · Second'})).toBeTruthy();
 expect(screen.getByText('Service unavailable')).toBeTruthy();
 expect((screen.getByRole('button',{name:'First'}) as HTMLButtonElement).disabled).toBe(false);
});

it("renames a non-active row without replacing the active transcript",async()=>{
 backend();render(<ChatPage/>);const user=userEvent.setup();await user.click(await screen.findByRole("button",{name:"First"}));await screen.findByText("Answer first");
 await menu(user,"更多操作 · Second");await user.click(await screen.findByRole("menuitem",{name:"重命名"}));
 await type(user,screen.getByRole("textbox",{name:"名称"}),"Renamed");await user.click(screen.getByRole("button",{name:"重命名"}));
 await screen.findByRole("button",{name:"Renamed"});expect(screen.getByText("Answer first")).toBeTruthy();expect(native.call).toHaveBeenCalledWith("conversation.update",{id:"second",title:"Renamed"});
});

it('keeps a manual title when an older AI response arrives after the manual save',async()=>{
 backend();let finish!:(value:Conversation)=>void;const fallback=native.call.getMockImplementation()!;
 native.call.mockImplementation((method,params)=>method==='title.regenerate'?new Promise(resolve=>{finish=resolve;}):method==='state'?Promise.resolve({messages:[]}):fallback(method,params));
 render(<ChatPage/>);const user=userEvent.setup();await menu(user,'更多操作 · Second');await user.click(await screen.findByRole('menuitem',{name:'AI 重命名'}));await screen.findByRole('status');
 await menu(user,'更多操作 · Second');await user.click(await screen.findByRole('menuitem',{name:/^重命名$/}));await type(user,screen.getByRole('textbox',{name:'名称'}),'Manual title');await user.click(screen.getByRole('button',{name:/^重命名$/}));await screen.findByRole('button',{name:'Manual title'});
 finish({...second,title:'Older AI title'});await waitFor(()=>expect(screen.queryByRole('status',{name:/正在生成标题/})).toBeNull());
 expect(screen.getByRole('button',{name:'Manual title'})).toBeTruthy();expect(screen.queryByRole('button',{name:'Older AI title'})).toBeNull();
});

it("deletes only the selected row after its confirmation and retains the active chat",async()=>{
 backend();render(<ChatPage/>);const user=userEvent.setup();await user.click(await screen.findByRole("button",{name:"First"}));await screen.findByText("Answer first");
 await menu(user,"更多操作 · Second");await user.click(await screen.findByRole("menuitem",{name:"删除对话"}));
 expect(native.call.mock.calls.some(call=>call[0]==="conversation.delete")).toBe(false);await user.click(screen.getByRole("button",{name:"删除对话"}));
 await waitFor(()=>expect(screen.queryByRole("button",{name:"Second"})).toBeNull());expect(screen.getByText("Answer first")).toBeTruthy();expect(native.call).toHaveBeenCalledWith("conversation.delete",{id:"second"});
});

it("archives and restores a row through the archive list",async()=>{
 backend();render(<ChatPage/>);const user=userEvent.setup();await menu(user,"更多操作 · Second");await user.click(await screen.findByRole("menuitem",{name:"归档"}));
 await waitFor(()=>expect(screen.queryByRole("button",{name:"Second"})).toBeNull());await user.click(screen.getByRole("button",{name:"查看归档"}));
 await menu(user,"更多操作 · Second");await user.click(await screen.findByRole("menuitem",{name:"恢复"}));
 await waitFor(()=>expect(native.call).toHaveBeenCalledWith("conversation.update",{id:"second",archived:false}));
});

it("uses the selected folder name as the initial project name",async()=>{
 backend();native.pick.mockResolvedValue("D:\\Projects\\Selected folder\\");render(<ChatPage/>);const user=userEvent.setup();await user.click(within(screen.getByRole("complementary",{name:"项目与对话"})).getByRole("button",{name:"打开或新建项目"}));await user.click(screen.getByRole("button",{name:/选择位置/}));
 await waitFor(()=>expect((screen.getByRole("textbox",{name:"项目名称"}) as HTMLInputElement).value).toBe("Selected folder"));
});

it('collapses project chats while retaining every project and the selected transcript',async()=>{
 backend();const fallback=native.call.getMockImplementation()!;
 native.call.mockImplementation((method,params)=>method==='catalog'?Promise.resolve({projects:[{id:'project',name:'Visible project',path:'C:\\project'}],conversations:[{...first,projectId:'project'}]}):fallback(method,params));
 render(<ChatPage/>);const user=userEvent.setup();await user.click(await screen.findByRole('button',{name:'First'}));await screen.findByText('Answer first');
 await user.click(screen.getByRole('button',{name:'收起所有项目的对话'}));
 expect(screen.getByRole('button',{name:'Visible project'})).toBeTruthy();expect(screen.queryByRole('button',{name:'First'})).toBeNull();expect(screen.getByText('Answer first')).toBeTruthy();
 await user.click(screen.getByRole('button',{name:'展开所有项目的对话'}));expect(screen.getByRole('button',{name:'First'})).toBeTruthy();
});

it("imports an attachment and saves it with the existing draft",async()=>{
 backend();render(<ChatPage/>);const user=userEvent.setup();await user.click(await screen.findByRole("button",{name:"First"}));await screen.findByText("Answer first");
 await menu(user,"附件与工具");await user.click(await screen.findByRole("menuitem",{name:"添加附件"}));await screen.findByText("notes.txt");
 expect(native.call).toHaveBeenCalledWith("attachments.add",{id:"first",paths:["C:\\notes.txt"]});await waitFor(()=>expect(native.call).toHaveBeenCalledWith("draft.save",{id:"first",draft:{text:"Draft",attachments:[{path:"C:\\history1\\attachments\\notes.txt",name:"notes.txt",size:42,mimeType:"text/plain"}]}}));
});

it("sends a file-only prompt through the Cindy composer",async()=>{
 backend();render(<ChatPage/>);const user=userEvent.setup();await user.click(await screen.findByRole("button",{name:"First"}));await screen.findByText("Answer first");
 await menu(user,"附件与工具");await user.click(await screen.findByRole("menuitem",{name:"添加附件"}));await screen.findByText("notes.txt");
 const input=screen.getByRole("textbox",{name:"消息"}) as HTMLTextAreaElement;await user.clear(input);await user.click(screen.getByRole("button",{name:"发送"}));
 await waitFor(()=>expect(native.call.mock.calls.some(([method,params])=>method==="prompt"&&params.id==="first"&&params.text===""&&params.attachments?.[0]==="C:\\history1\\attachments\\notes.txt")).toBe(true));
});

it("removes a draft attachment and persists the empty draft",async()=>{
 backend();render(<ChatPage/>);const user=userEvent.setup();await user.click(await screen.findByRole("button",{name:"First"}));await screen.findByText("Answer first");
 await menu(user,"附件与工具");await user.click(await screen.findByRole("menuitem",{name:"添加附件"}));await screen.findByText("notes.txt");
 await user.clear(screen.getByRole("textbox",{name:"消息"}));
 await user.click(screen.getByRole("button",{name:"移除附件 · notes.txt"}));
 expect(screen.queryByText("notes.txt")).toBeNull();
 await waitFor(()=>expect(native.call).toHaveBeenCalledWith("draft.save",{id:"first",draft:{text:"",attachments:[]}}));
});

it("saves default workspace settings without moving a conversation",async()=>{
 backend();const saved=vi.fn();render(<ChatSettings open onOpenChange={()=>{}} providers={providers} defaultModel={model} onSaved={saved} onConversation={()=>{}}/>);const user=userEvent.setup();
 await type(user,await screen.findByRole("textbox",{name:"默认工作目录"}),"D:\\new-work");await user.click(screen.getByRole("button",{name:"保存设置"}));await waitFor(()=>expect(native.call).toHaveBeenCalledWith("settings.save",expect.anything()));expect(screen.queryByText("设置已保存")).toBeNull();
 expect(native.call).toHaveBeenCalledWith("settings.save",{settings:{...settings,workspaceDirectory:"D:\\new-work"}});expect(native.call.mock.calls.some(call=>call[0]==="location")).toBe(false);expect(saved).toHaveBeenCalledOnce();
});

it("renders actual files and resources returned by the host",async()=>{
 backend();const original=native.call.getMockImplementation()!;native.call.mockImplementation((method,params)=>method==="files"?Promise.resolve({directory:"C:\\first",entries:[{name:"proof.txt",path:"C:\\first\\proof.txt",size:15,directory:false}]}):method==="resources"?Promise.resolve({agents:[{path:"C:\\first\\AGENTS.md"}],skills:[{name:"Review skill",description:"Review code",path:"C:\\skills\\review"}],plugins:[],mcpServers:[],diagnostics:[]}):original(method,params));
 const reference=vi.fn(),preview=vi.fn();render(<ChatInspector id="first" onClose={()=>{}} onReference={reference} onOpenFile={preview}/>);const user=userEvent.setup();await user.click(await screen.findByRole("button",{name:/^proof.txt/}));expect(preview).toHaveBeenCalledWith("C:\\first\\proof.txt");expect(reference).not.toHaveBeenCalled();
 await user.click(screen.getByRole("tab",{name:"资源"}));await screen.findByText("Review skill");expect(screen.getByText("C:\\first\\AGENTS.md")).toBeTruthy();
});

it("shows the historical provider and model as plain text",()=>{
 render(<ChatMessageView message={{role:"assistant",content:"Answer",provider:"service",providerName:"Historical service",model:"model",modelName:"Historical model"}} providers={providers} onCopy={()=>{}} onFork={()=>{}} canFork={false}/>);
 const source=screen.getByText("Historical service · Historical model");expect(within(source).queryByRole("img")).toBeNull();expect(screen.queryByText("Current service")).toBeNull();
});

it("keeps file-only user messages visible with their attachment names",()=>{
 render(<ChatMessageView message={{role:"user",content:"",attachments:[{name:"report.pdf",path:"C:\\history\\attachments\\report.pdf",size:1024,mimeType:"application/pdf"}]}} conversationId="first" onCopy={()=>{}} onFork={()=>{}} canFork={false}/>);
 expect(screen.getByRole("button",{name:/report.pdf/})).toBeTruthy();
});

it("saves session runtime defaults with chat settings",async()=>{
 backend();render(<ChatSettings open onOpenChange={()=>{}} providers={providers} defaultModel={model} onSaved={()=>{}} onConversation={()=>{}}/>);const user=userEvent.setup();
 await user.selectOptions(await screen.findByRole("combobox",{name:"默认思考强度"}),"high");
 await user.click(screen.getByRole("checkbox",{name:"自动重试"}));
 await user.click(screen.getByRole('checkbox',{name:'后台压缩'}));
 await user.click(screen.getByRole('checkbox',{name:'Code Mode'}));
 await user.click(screen.getByRole('checkbox',{name:'计划工具'}));
 await user.selectOptions(screen.getByRole('combobox',{name:'工具预设'}),'read-only');
 const limit=screen.getByRole('spinbutton',{name:'同时运行的子代理上限'});await user.clear(limit);await user.type(limit,'2');
 await user.click(screen.getByRole("button",{name:"保存设置"}));
 await waitFor(()=>expect(native.call).toHaveBeenCalledWith("settings.save",{settings:{...settings,defaultThinkingLevel:"high",autoRetry:false,backgroundCompaction:true,codeMode:false,planning:false,toolPreset:'read-only',maxSubagents:2}}));
});
