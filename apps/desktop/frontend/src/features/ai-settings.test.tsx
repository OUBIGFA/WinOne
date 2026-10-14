// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {act,cleanup,render,screen,waitFor,within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {AISettings} from "./ai-settings";
const api=vi.hoisted(()=>({captureCall:vi.fn()}));
vi.mock("../bridge",()=>api);
afterEach(()=>{cleanup();vi.clearAllMocks();});
const original={revision:2,engine:"google",ocrEngine:"local",target:"auto",prompt:"",providers:[{id:"one",name:"个人服务",authMode:"api-key",protocol:"chat",baseUrl:"https://example.com/v1",models:["test-model"],hasKey:true}],tasks:{default:{provider:"one",model:"test-model"}}};
function fixture(){let saved:any;api.captureCall.mockImplementation(async(method,value)=>{
 if(method==="ai.configuration.get")return structuredClone(original);
 if(method==="ai.configuration.save"){saved=value;return {...value.configuration,revision:3};}
 if(method==="ai.accounts.catalog")return {agents:[{id:"codex",name:"ChatGPT / Codex",icon:"openai"},{id:"claude",name:"Claude Code",icon:"claude-color"}]};
 if(method==="ai.accounts.list")return {accounts:[]};
 throw Error(method);
});return ()=>saved;}

it("reopens the visible routing editor after saving its configuration",async()=>{
 fixture();const base=api.captureCall.getMockImplementation()!;let opened=0;
 api.captureCall.mockImplementation(async(method,value)=>method==="ai.routing.open"?{url:`http://127.0.0.1:${4300+(++opened)}/?view=routing`}:method==="ai.routing.close"?{}:base(method,value));
 render(<AISettings/>);const user=userEvent.setup();await screen.findByRole("tab",{name:"路由"});await user.click(screen.getByRole("tab",{name:"路由"}));
 await waitFor(()=>expect(screen.getByTitle("路由").getAttribute("src")).toContain(":4301/"));
 await user.click(screen.getByRole("switch",{name:"启用模型路由"}));await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));
 await screen.findByText("已保存");await waitFor(()=>expect(screen.getByTitle("路由").getAttribute("src")).toContain(":4302/"));
});

it("keeps connection and advanced edits while switching the provider settings tabs",async()=>{
 // This checks preservation across tabs, so paste complete values instead of
 // measuring a full page render for every keystroke under concurrent tests.
 const saved=fixture();render(<AISettings/>);const user=userEvent.setup();await screen.findByRole("tab",{name:"连接设置"});await user.clear(screen.getByLabelText("API 基础地址"));await user.paste("https://changed.example/v1");
 await user.click(within(screen.getByRole("tablist",{name:"供应商设置"})).getByRole("tab",{name:"高级设置"}));await user.click(screen.getByRole("button",{name:"添加请求头"}));await user.click(screen.getByRole("textbox",{name:"请求头名称 1"}));await user.paste("X-Config");await user.click(screen.getByRole("textbox",{name:"请求头值 1"}));await user.paste('{"team":"test"}');await user.click(screen.getByRole("button",{name:"以 JSON 编辑"}));
 const field=screen.getByRole("textbox",{name:"请求头值 1"});const formatted=(field as HTMLTextAreaElement).value;
 await user.click(screen.getByRole("tab",{name:"连接设置"}));expect((screen.getByLabelText("API 基础地址") as HTMLInputElement).value).toBe("https://changed.example/v1");await user.click(within(screen.getByRole("tablist",{name:"供应商设置"})).getByRole("tab",{name:"高级设置"}));expect(screen.getByRole("textbox",{name:"请求头值 1"})).toBe(field);expect((field as HTMLTextAreaElement).value).toBe(formatted);
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByText("已保存");expect(saved().configuration.providers[0]).toMatchObject({baseUrl:"https://changed.example/v1",headers:{"X-Config":'{"team":"test"}'}});
});

it("saves and reopens a selected custom picture without restoring the provider brand",async()=>{
 const saved=fixture(),base=api.captureCall.getMockImplementation()!;const icon="file:abcdef0123456710.png";
 api.captureCall.mockImplementation(async(method,value)=>method==="ai.icons.store"?{icon}:method==="ai.icons.read"?{data:btoa("picture"),size:7,mimeType:"image/png"}:method==="ai.configuration.get"&&saved()?saved().configuration:base(method,value));
 const view=render(<AISettings/>);const user=userEvent.setup();const providerTabs=await screen.findByRole("tablist",{name:"供应商设置"});await user.click(within(providerTabs).getByRole("tab",{name:"高级设置"}));const file=new File(["picture"],"picture.png",{type:"image/png"});Object.defineProperty(file,"arrayBuffer",{value:async()=>new TextEncoder().encode("picture").buffer});
 await user.upload(screen.getByLabelText("选择图片"),file);await waitFor(()=>expect(view.container.querySelector(`[data-icon="${icon}"] img`)).toBeTruthy());await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByText("已保存");expect(saved().configuration.providers[0].avatar).toBe(icon);
 view.unmount();const reopened=render(<AISettings/>);await waitFor(()=>expect(reopened.container.querySelector(`[data-icon="${icon}"] img`)).toBeTruthy());
});

it("saves custom headers and uses their current form values for model discovery",async()=>{
 const saved=fixture();const base=api.captureCall.getMockImplementation()!;
 api.captureCall.mockImplementation(async(method,value)=>method==="ai.models.fetch"?{models:[{id:"test-model"}]}:base(method,value));
 render(<AISettings/>);const user=userEvent.setup();const providerTabs=await screen.findByRole("tablist",{name:"供应商设置"});await user.click(within(providerTabs).getByRole("tab",{name:"高级设置"}));await user.click(screen.getByRole("button",{name:"添加请求头"}));
 await user.type(screen.getByRole("textbox",{name:"请求头名称 1"}),"X-Team");await user.type(screen.getByRole("textbox",{name:"请求头值 1"}),"development");
 await user.click(screen.getByRole("button",{name:"获取模型列表"}));
 await waitFor(()=>expect(api.captureCall).toHaveBeenCalledWith("ai.models.fetch",expect.objectContaining({connection:expect.objectContaining({headers:{"X-Team":"development"}})})));
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await waitFor(()=>expect(saved()?.configuration.providers[0].headers).toEqual({"X-Team":"development"}));
});

it("keeps the saved provider summary identical across every service tab",async()=>{
 fixture();const base=api.captureCall.getMockImplementation()!;
 api.captureCall.mockImplementation(async(method,value)=>method==="ai.gateway.get"?{revision:0,enabled:false,port:3426,running:false,hasKey:false,baseUrl:"http://127.0.0.1:3426/v1"}:base(method,value));
 render(<AISettings/>);const user=userEvent.setup();
 const summary=await screen.findByRole("region",{name:"已启用供应商"});
 expect(within(summary).getByRole("button",{name:"个人服务（1）"})).toBeTruthy();
 for(const tab of ["账号供应商","默认 AI 服务","本地网关","自定义供应商"]){
  await user.click(screen.getByRole("tab",{name:tab}));
  expect(screen.getByRole("region",{name:"已启用供应商"})).toBe(summary);
  expect(within(summary).getByRole("button",{name:"个人服务（1）"})).toBeTruthy();
 }
});

it("updates the provider summary from unsaved model selections",async()=>{
 fixture();render(<AISettings/>);const user=userEvent.setup();
 const summary=await screen.findByRole("region",{name:"已启用供应商"});
 expect(within(summary).getByRole("button",{name:"个人服务（1）"})).toBeTruthy();
 await user.click(screen.getByRole("button",{name:"全不选"}));
 await waitFor(()=>expect(within(summary).getByText("勾选并保存模型后，供应商会显示在这里。")).toBeTruthy());
});

it("saves refreshed catalogs without obsolete task references and retains unaffected selections",async()=>{
 const initial={...structuredClone(original),providers:[
  {...original.providers[0],models:["test-model","retained"],availableModels:["test-model","retained"]},
  {...original.providers[0],id:"other",name:"Other provider"},
 ],tasks:{default:{provider:"one",model:"test-model"},translate:{provider:"one",model:"retained"},chat:{provider:"other",model:"test-model"}}};
 let saved:typeof initial|undefined;
 api.captureCall.mockImplementation(async(method,value)=>{
  if(method==="ai.configuration.get")return structuredClone(saved??initial);
  if(method==="ai.models.fetch")return {models:[{id:"retained",name:"Current model"}]};
  if(method==="ai.configuration.save"){
   // The Host rejects references absent from the provider's complete catalog.
   for(const selection of Object.values(value.configuration.tasks) as {provider:string;model:string}[]){
    if(!selection.provider&&!selection.model)continue;
    const provider=value.configuration.providers.find((provider:{id:string})=>provider.id===selection.provider);
    if(!provider?.availableModels?.includes(selection.model)&&!provider?.models.includes(selection.model))throw Error("Obsolete task selection");
   }
   saved=value.configuration;return {...saved,revision:3};
  }
  throw Error(method);
 });
 const view=render(<AISettings/>),user=userEvent.setup();
 await user.click(await screen.findByRole("button",{name:"获取模型列表"}));await screen.findByRole("checkbox",{name:"Current model"});
 expect(saved).toBeUndefined();
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByText("已保存");
 expect(saved!.tasks).toEqual({default:{provider:"",model:""},translate:{provider:"one",model:"retained"},chat:{provider:"other",model:"test-model"}});
 expect(saved!.providers[0]).toMatchObject({models:["retained"],availableModels:["retained"],modelNames:{retained:"Current model"}});
 view.unmount();render(<AISettings/>);await screen.findByRole("checkbox",{name:"Current model"});
 expect(screen.queryByRole("checkbox",{name:"test-model"})).toBeNull();
});

it("keeps tabs and page navigation available while the account catalog is pending",async()=>{
 fixture();const base=api.captureCall.getMockImplementation()!;
 let finish!:(value:unknown)=>void;const pending=new Promise(resolve=>{finish=resolve;});
 api.captureCall.mockImplementation((method,value)=>method==="ai.accounts.catalog"?pending:base(method,value));
 const activity=vi.fn();render(<AISettings onActivity={activity}/>);const user=userEvent.setup();
 await screen.findByRole("combobox",{name:"服务"});await user.click(screen.getByRole("tab",{name:"账号供应商"}));
 await user.click(screen.getByRole("tab",{name:"自定义供应商"}));
 expect(screen.getByRole("tab",{name:"自定义供应商"}).getAttribute("aria-selected")).toBe("true");
 expect(activity.mock.calls.at(-1)?.slice(0,2)).toEqual([false,false]);
 await user.click(screen.getByRole("tab",{name:"账号供应商"}));
 expect(api.captureCall.mock.calls.filter(([method])=>method==="ai.accounts.catalog")).toHaveLength(1);
 await act(async()=>finish({agents:[]}));
});

it("reuses pending and completed account reads across tab switches and refreshes on demand",async()=>{
 fixture();const base=api.captureCall.getMockImplementation()!;
 let finish!:(value:unknown)=>void;const pending=new Promise(resolve=>{finish=resolve;});let reads=0;
 api.captureCall.mockImplementation((method,value)=>{
  if(method==="ai.configuration.get")return Promise.resolve({...structuredClone(original),providers:[{id:"work",name:"工作账号",authMode:"account",agent:"codex",models:[]}]});
  if(method==="ai.accounts.list"){reads++;return reads===1?pending:Promise.resolve({accounts:[{agent:"codex",user:"updated@example.invalid"}]});}
  return base(method,value);
 });
 const activity=vi.fn();render(<AISettings onActivity={activity}/>);const user=userEvent.setup();
 await user.click(await screen.findByRole("tab",{name:"账号供应商"}));await screen.findByText("正在读取账号");
 expect(screen.getByRole("button",{name:"获取模型列表"}).hasAttribute("disabled")).toBe(true);
 await user.click(screen.getByRole("tab",{name:"默认 AI 服务"}));
 expect(screen.getByRole("tab",{name:"默认 AI 服务"}).getAttribute("aria-selected")).toBe("true");
 expect(activity.mock.calls.at(-1)?.slice(0,2)).toEqual([false,false]);
 await user.click(screen.getByRole("tab",{name:"账号供应商"}));expect(reads).toBe(1);
 await act(async()=>finish({accounts:[{agent:"codex",user:"original@example.invalid"}]}));await screen.findByText("original@example.invalid");
 await user.click(screen.getByRole("tab",{name:"自定义供应商"}));await user.click(screen.getByRole("tab",{name:"账号供应商"}));
 await screen.findByText("original@example.invalid");expect(reads).toBe(1);
 await user.click(screen.getByRole("button",{name:"刷新状态"}));await screen.findByText("updated@example.invalid");expect(reads).toBe(2);
});

it("keeps the shared introduction and tabs while switching to the gateway",async()=>{
 fixture();const base=api.captureCall.getMockImplementation()!;
 api.captureCall.mockImplementation(async(method,value)=>method==="ai.gateway.get"?{revision:0,enabled:false,port:3426,running:false,hasKey:false,baseUrl:"http://127.0.0.1:3426/v1"}:base(method,value));
 render(<AISettings/>);const user=userEvent.setup();
 const intro=await screen.findByText("连接供应商并勾选模型，再到各功能中选择使用。");
 const tabs=screen.getByRole("tablist",{name:"AI 服务"});
 await user.click(screen.getByRole("tab",{name:"本地网关"}));await screen.findByLabelText("端口");
 expect(screen.getByText(intro.textContent!)).toBe(intro);
 expect(screen.getByRole("tablist",{name:"AI 服务"})).toBe(tabs);
});

it("renames in the service selector, cancels with Escape and saves the same connection",async()=>{
 const saved=fixture();render(<AISettings/>);const user=userEvent.setup();
 await screen.findByRole("combobox",{name:"服务"});
 expect(screen.queryByRole("textbox",{name:"服务名称"})).toBeNull();
 await user.click(screen.getByRole("button",{name:"重命名服务"}));
 const name=screen.getByRole("textbox",{name:"服务名称"});
 expect((name as HTMLInputElement).selectionEnd).toBe("个人服务".length);
 await user.clear(name);await user.type(name,"临时名称");await user.keyboard("{Escape}");
 expect(screen.getByRole("combobox",{name:"服务"}).textContent).toContain("个人服务");
 expect(screen.getByRole("button",{name:"保存 AI 设置"}).hasAttribute("disabled")).toBe(true);
 await user.click(screen.getByRole("button",{name:"重命名服务"}));
 await user.clear(screen.getByRole("textbox",{name:"服务名称"}));await user.type(screen.getByRole("textbox",{name:"服务名称"}),"工作服务{Enter}");
 expect(screen.queryByRole("textbox",{name:"服务名称"})).toBeNull();
 expect(saved()).toBeUndefined();
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByText("已保存");
 expect(saved().configuration.providers).toEqual([{...original.providers[0],name:"工作服务"}]);
 expect(saved().configuration.tasks).toEqual(original.tasks);
});

it("chooses the account type once and keeps connection identity out of the login form",async()=>{
 fixture();render(<AISettings/>);const user=userEvent.setup();
 await user.click(await screen.findByRole("tab",{name:"账号供应商"}));
 const add=screen.getByRole("button",{name:/添加账号/});
 await waitFor(()=>expect(add.hasAttribute("disabled")).toBe(false));await user.click(add);
 await user.click(within(screen.getByRole("dialog")).getByRole("button",{name:"Claude Code"}));
 const account=await screen.findByRole("region",{name:"账号登录"});
 expect(within(account).queryByRole("combobox")).toBeNull();
 expect(within(account).getByRole("heading",{name:"Claude Code"})).toBeTruthy();
 expect(api.captureCall.mock.calls.some(([method,payload])=>method==="ai.accounts.list"&&payload.provider&&payload.agent==="claude"&&payload.draft?.agent==="claude")).toBe(true);
 expect(api.captureCall.mock.calls.some(([method])=>method==="ai.accounts.signin.start"||method==="ai.configuration.save")).toBe(false);
});

it("chooses an agent before creating an account and canceling leaves no draft",async()=>{
 const saved=fixture();render(<AISettings/>);const user=userEvent.setup();
 await user.click(await screen.findByRole("tab",{name:"账号供应商"}));
 const add=await screen.findByRole("button",{name:"添加账号"});
 await waitFor(()=>expect(add.hasAttribute("disabled")).toBe(false));
 await user.click(add);
 const picker=screen.getByRole("dialog",{name:"选择账号类型"});
 expect(api.captureCall.mock.calls.some(([method])=>method==="ai.accounts.list")).toBe(false);
 await user.click(within(picker).getByRole("button",{name:"关闭"}));
 expect(screen.getByRole("button",{name:"保存 AI 设置"}).hasAttribute("disabled")).toBe(true);
 await user.click(add);await user.click(within(screen.getByRole("dialog")).getByRole("button",{name:"Claude Code"}));
 expect(screen.getByRole("combobox",{name:"账号"}).textContent).toContain("Claude Code");
 await waitFor(()=>expect(api.captureCall.mock.calls.some(([method,payload])=>method==="ai.accounts.list"&&payload.draft?.agent==="claude")).toBe(true));
 expect(saved()).toBeUndefined();
});

it("fetches models using a new custom connection and key before saving",async()=>{
 const saved=fixture();const base=api.captureCall.getMockImplementation()!;
 api.captureCall.mockImplementation(async(method,value)=>method==="ai.models.fetch"?{models:[{id:"draft-model",name:"Draft model"}]}:base(method,value));
 render(<AISettings/>);const user=userEvent.setup();await screen.findByRole("button",{name:"重命名服务"});
 await user.click(screen.getByRole("button",{name:"添加自定义供应商"}));
 await user.type(screen.getByLabelText("API 基础地址"),"https://draft.example/v1");await user.type(screen.getByLabelText("API 密钥"),"unsaved-secret");
 await user.click(screen.getByRole("button",{name:"获取模型列表"}));await screen.findByRole("checkbox",{name:"Draft model"});
 const [,payload]=api.captureCall.mock.calls.find(([method])=>method==="ai.models.fetch")!;
 expect(payload.connection).toMatchObject({protocol:"chat",baseUrl:"https://draft.example/v1",keys:[{id:"primary",enabled:true}]});
 expect(payload.keyUpdates).toEqual({primary:"unsaved-secret"});expect(saved()).toBeUndefined();
 expect((screen.getByLabelText("API 密钥") as HTMLInputElement).value).toBe("unsaved-secret");
});

it("fetches a local preset before saving without requiring an API key",async()=>{
 const saved=fixture();const base=api.captureCall.getMockImplementation()!;
 api.captureCall.mockImplementation(async(method,value)=>method==="ai.accounts.catalog"?{agents:[],presets:[{id:"lmstudio",name:"LM Studio",chat:"http://localhost:1234/v1"}]}:method==="ai.models.fetch"?{models:[{id:"local-model"}]}:base(method,value));
 render(<AISettings/>);const user=userEvent.setup();await screen.findByRole("button",{name:"重命名服务"});await user.click(screen.getByRole("button",{name:"从服务预设添加"}));await user.click(await screen.findByRole("button",{name:"LM Studio"}));
 await user.click(screen.getByRole("button",{name:"获取模型列表"}));await screen.findByRole("checkbox",{name:"local-model"});
 const [,payload]=api.captureCall.mock.calls.find(([method])=>method==="ai.models.fetch")!;
 expect(payload.connection).toMatchObject({preset:"lmstudio",baseUrl:"http://localhost:1234/v1"});expect(saved()).toBeUndefined();
});

it("places global defaults after account providers and preserves drafts across tabs",async()=>{
 fixture();render(<AISettings/>);const user=userEvent.setup();await screen.findByRole("button",{name:"重命名服务"});
 await user.type(screen.getByLabelText("API 密钥"),"draft-key");
 expect(within(screen.getByRole("tablist",{name:"AI 服务"})).getAllByRole("tab").map(tab=>tab.textContent)).toEqual(["自定义供应商","账号供应商","默认 AI 服务","本地网关","路由","用量统计","高级设置"]);
 expect(screen.queryByRole("combobox",{name:"默认 AI 服务 · 供应商"})).toBeNull();
 await user.click(screen.getByRole("tab",{name:"默认 AI 服务"}));expect(screen.getByRole("combobox",{name:"默认 AI 服务 · 供应商"})).toBeTruthy();
 await waitFor(()=>expect(screen.queryByLabelText("API 基础地址")).toBeNull());
 await user.click(screen.getByRole("tab",{name:"自定义供应商"}));expect((screen.getByLabelText("API 密钥") as HTMLInputElement).value).toBe("draft-key");
});

it("fills a new blank custom service from a local preset without leaving an invalid service",async()=>{
 const saved=fixture();const base=api.captureCall.getMockImplementation()!;
 api.captureCall.mockImplementation(async(method,value)=>method==="ai.accounts.catalog"?{agents:[],presets:[{id:"lmstudio",name:"LM Studio",chat:"http://localhost:1234/v1"}]}:base(method,value));
 render(<AISettings/>);const user=userEvent.setup();await screen.findByRole("button",{name:"重命名服务"});
 await user.click(screen.getByRole("button",{name:"添加自定义供应商"}));
 await user.click(screen.getByRole("button",{name:"从服务预设添加"}));await user.click(await screen.findByRole("button",{name:"LM Studio"}));
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByText("已保存");
 expect(saved().configuration.providers).toHaveLength(2);
 expect(saved().configuration.providers.every((p:{name:string})=>p.name.trim())).toBe(true);
 expect(saved().configuration.providers[1].baseUrl).toBe("http://localhost:1234/v1");
});

it("sets the global default without replacing feature overrides",async()=>{
 let saved:any;const second={id:"two",name:"第二服务",authMode:"api-key",protocol:"chat",baseUrl:"https://second.example/v1",models:["second-model"],hasKey:false};
 api.captureCall.mockImplementation(async(method,value)=>{if(method==="ai.configuration.get")return {...structuredClone(original),providers:[...original.providers,second],tasks:{...original.tasks,translate:{provider:"one",model:"test-model"}}};if(method==="ai.configuration.save"){saved=value;return {...value.configuration,revision:3}};throw Error(method)});
 render(<AISettings/>);const user=userEvent.setup();await user.click(await screen.findByRole("tab",{name:"默认 AI 服务"}));await user.click(screen.getByRole("combobox",{name:"默认 AI 服务 · 供应商"}));await user.click(await screen.findByRole("option",{name:"第二服务"}));
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByText("已保存");expect(saved.configuration.tasks.default).toEqual({provider:"two",model:"second-model"});expect(saved.configuration.tasks.translate).toEqual({provider:"one",model:"test-model"});
});

it("shows discovered model names but saves the protocol ID",async()=>{
 const saved=fixture();const base=api.captureCall.getMockImplementation()!;api.captureCall.mockImplementation(async(method,value)=>method==="ai.models.fetch"?{models:[{id:"qmodel_38max",name:"Qwen3.8-Max"}]}:base(method,value));
 render(<AISettings/>);const user=userEvent.setup();await user.click(await screen.findByRole("button",{name:"获取模型列表"}));await user.click(await screen.findByRole("checkbox",{name:"Qwen3.8-Max"}));await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByText("已保存");
 expect(saved().configuration.providers[0].models).toContain("qmodel_38max");expect(saved().configuration.providers[0].models).not.toContain("Qwen3.8-Max");expect(saved().configuration.providers[0].modelNames.qmodel_38max).toBe("Qwen3.8-Max");
});
it("opens an existing account service without racing its shared catalog request",async()=>{
 let catalogActive=false;let finishCatalog!:(value:unknown)=>void;
 api.captureCall.mockImplementation(async(method)=>{
  if(method==="ai.configuration.get")return {...structuredClone(original),providers:[...original.providers,{id:"account",name:"工作账号",authMode:"account",agent:"codex",models:["test-model"]}]};
  // The Host serializes catalog reads with account operations. Hold the first
  // response so a duplicate mount request reproduces the real busy response.
  if(method==="ai.accounts.catalog"){
   if(catalogActive)throw Error("账号正在登录或使用，请稍后再试");
   catalogActive=true;return await new Promise(resolve=>{finishCatalog=resolve;});
  }
  if(method==="ai.accounts.list")return {accounts:[{agent:"codex",user:"test@example.invalid"}]};
  throw Error(method);
 });
 render(<AISettings/>);const user=userEvent.setup();await screen.findByRole("button",{name:"重命名服务"});
 await user.click(screen.getByRole("tab",{name:"账号供应商"}));
 await waitFor(()=>expect(finishCatalog).toBeTypeOf("function"));
 await act(async()=>finishCatalog({agents:[{id:"codex",name:"ChatGPT / Codex",icon:"openai"}]}));
 await screen.findByText("test@example.invalid");
 expect(screen.queryByRole("alert")).toBeNull();
 expect(screen.getByRole("button",{name:"添加账号"}).hasAttribute("disabled")).toBe(false);
 expect(screen.getByRole("heading",{name:"ChatGPT / Codex"})).toBeTruthy();
});
it("adds separate keys and removes only the chosen key while retaining the other draft",async()=>{
 const saved=fixture();render(<AISettings/>);const user=userEvent.setup();
 await user.click(await screen.findByRole("button",{name:"添加密钥"}));
 await user.type(screen.getByLabelText("API 密钥 2"),"second-secret");
 await user.click(screen.getByRole("button",{name:"删除密钥 1"}));
 expect((screen.getByLabelText("API 密钥") as HTMLInputElement).value).toBe("second-secret");
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByText("已保存");
 const payload=saved();const slots=payload.configuration.providers[0].keys;
 expect(slots).toHaveLength(1);expect(slots[0].id).not.toBe("primary");
 expect(payload.keyUpdates.one).toEqual({[slots[0].id]:"second-secret"});
 expect(payload.keys).toBeUndefined();
});
it("saves rotation controls and key availability without resubmitting saved secrets",async()=>{
 const saved=fixture();render(<AISettings/>);const user=userEvent.setup();
 await user.click(await screen.findByRole("button",{name:"添加密钥"}));
 await user.click(screen.getByRole("combobox",{name:"密钥使用策略"}));
 await user.click(await screen.findByRole("option",{name:"按顺序使用"}));
 await user.clear(screen.getByLabelText("最多尝试密钥数"));await user.type(screen.getByLabelText("最多尝试密钥数"),"2");
 await user.clear(screen.getByLabelText("失败冷却时间（秒）"));await user.type(screen.getByLabelText("失败冷却时间（秒）"),"120");
 await user.click(screen.getByRole("switch",{name:"启用密钥 1"}));
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByText("已保存");
 expect(saved().configuration.providers[0].keyPolicy).toEqual({strategy:"ordered",failover:true,maxAttempts:2,cooldownSeconds:120});
 expect(saved().configuration.providers[0].keys[0].enabled).toBe(false);
 expect(saved().keyUpdates).toBeUndefined();
});
it("defaults reasoning display on for old settings and saves an explicit off choice",async()=>{
 const saved=fixture();render(<AISettings/>);const user=userEvent.setup();
 await user.click(await screen.findByRole("tab",{name:"默认 AI 服务"}));const toggle=await screen.findByRole("switch",{name:"显示思考过程"});
 expect(toggle.getAttribute("aria-checked")).toBe("true");
 await user.click(toggle);await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByText("已保存");
 expect(saved().configuration.showReasoning).toBe(false);expect(saved().keys).toBeUndefined();
 expect(screen.getByRole("switch",{name:"显示思考过程"}).getAttribute("aria-checked")).toBe("false");
});
it("keeps each provider key unchanged when editing its models",async()=>{
 const saved=fixture();render(<AISettings/>);const user=userEvent.setup();
 const models=await screen.findByLabelText("搜索或填写模型 ID");await user.type(models,"new-model");await user.click(screen.getByRole("button",{name:"添加模型"}));
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByText("已保存");
 expect(saved().configuration.providers[0].models).toEqual(["test-model","new-model"]);expect(saved().keys).toBeUndefined();
 expect((screen.getByLabelText("API 密钥") as HTMLInputElement).value).toBe("");
});
it("saves a second supplier without overwriting the first supplier or task",async()=>{
 const saved=fixture();render(<AISettings/>);const user=userEvent.setup();await screen.findByRole("button",{name:"重命名服务"});
 await user.click(screen.getByRole("button",{name:"添加自定义供应商"}));
 await user.type(screen.getByLabelText(/^(服务名称|账号名称)$/),"工作服务");await user.type(screen.getByLabelText("API 基础地址"),"https://work.example/v1");await user.type(screen.getByLabelText("搜索或填写模型 ID"),"work-model");await user.click(screen.getByRole("button",{name:"添加模型"}));await user.type(screen.getByLabelText("API 密钥"),"work-secret");
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByText("已保存");
 const value=saved();expect(value.configuration.providers).toHaveLength(2);expect(value.configuration.providers[0]).toEqual(original.providers[0]);expect(value.configuration.tasks).toEqual(original.tasks);expect(Object.values(value.keyUpdates[value.configuration.providers[1].id])).toEqual(["work-secret"]);expect(value.keyUpdates.one).toBeUndefined();
});
it("shows scoped sign-in before saving the account profile",async()=>{
 fixture();render(<AISettings/>);const user=userEvent.setup();await screen.findByRole("button",{name:"重命名服务"});await user.click(screen.getByRole("tab",{name:"账号供应商"}));await user.click(screen.getByRole("button",{name:"添加账号"}));
 await user.click(within(screen.getByRole("dialog")).getByRole("button",{name:"ChatGPT / Codex"}));
 expect(await screen.findByRole("button",{name:"登录"})).toBeTruthy();
 expect(api.captureCall.mock.calls.some(([method,payload])=>method==="ai.accounts.list"&&payload.provider&&payload.draft?.agent==="codex")).toBe(true);
 expect(api.captureCall.mock.calls.some(([method])=>method==="ai.configuration.save")).toBe(false);
});

it("names account services from the agent, avoids duplicates and preserves custom names",async()=>{
 const saved=fixture();render(<AISettings/>);const user=userEvent.setup();await screen.findByRole("button",{name:"重命名服务"});
 await user.click(screen.getByRole("tab",{name:"账号供应商"}));
 await user.click(await screen.findByRole("button",{name:"添加账号"}));
 await user.click(within(screen.getByRole("dialog")).getByRole("button",{name:"ChatGPT / Codex"}));
 expect(screen.getByRole("combobox",{name:"账号"}).textContent).toContain("ChatGPT / Codex");
 await user.click(screen.getByRole("button",{name:"添加账号"}));
 await user.click(within(screen.getByRole("dialog")).getByRole("button",{name:"ChatGPT / Codex"}));
 expect(screen.getByRole("combobox",{name:"账号"}).textContent).toContain("ChatGPT / Codex 2");
 await user.click(screen.getByRole("button",{name:"重命名账号"}));await user.clear(screen.getByLabelText("账号名称"));await user.type(screen.getByLabelText("账号名称"),"工作账号");
 await user.click(screen.getByRole("button",{name:"添加账号"}));
 await user.click(within(screen.getByRole("dialog")).getByRole("button",{name:"Claude Code"}));
 expect(screen.getByRole("combobox",{name:"账号"}).textContent).toContain("Claude Code");
 await user.click(screen.getByRole("combobox",{name:"账号"}));await user.click(await screen.findByRole("option",{name:"工作账号"}));
 expect(screen.getByRole("combobox",{name:"账号"}).textContent).toContain("工作账号");
 expect(screen.getByRole("heading",{name:"ChatGPT / Codex"})).toBeTruthy();
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByText("已保存");
 const accounts=saved().configuration.providers.filter((p:{authMode:string})=>p.authMode==="account");
 expect(accounts.map((p:{name:string;agent:string})=>[p.name,p.agent])).toEqual([["ChatGPT / Codex","codex"],["工作账号","codex"],["Claude Code","claude"]]);
 expect(new Set(accounts.map((p:{id:string})=>p.id)).size).toBe(3);
 expect(saved().configuration.providers[0]).toEqual(original.providers[0]);
 expect(saved().configuration.tasks).toEqual(original.tasks);
});
it("saves the selected plugin backend without changing existing built-in accounts",async()=>{
 const saved=fixture();const base=api.captureCall.getMockImplementation()!;
 api.captureCall.mockImplementation(async(method,value)=>method==="ai.accounts.catalog"?{agents:[{id:"codex",name:"ChatGPT / Codex"},{id:"cline",name:"Cline",backend:"plugin"}]}:base(method,value));
 render(<AISettings/>);const user=userEvent.setup();await screen.findByRole("button",{name:"重命名服务"});
 await user.click(screen.getByRole("tab",{name:"账号供应商"}));await user.click(await screen.findByRole("button",{name:"添加账号"}));
 await user.click(within(screen.getByRole("dialog")).getByRole("button",{name:"Cline"}));
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByText("已保存");
 expect(saved().configuration.providers[1].agentBackend).toBe("plugin");
 expect(saved().configuration.providers[1].name).toBe("Cline");
 expect(saved().configuration.providers[0]).toEqual(original.providers[0]);
});
it("adds an OpenCode service preset with its native endpoint and retained preset identity",async()=>{
 const saved=fixture();const base=api.captureCall.getMockImplementation()!;
 api.captureCall.mockImplementation(async(method,value)=>method==="ai.accounts.catalog"?{agents:[],presets:[{id:"opencode-go",name:"OpenCode Go",chat:"https://opencode.ai/zen/go/v1",responses:"https://opencode.ai/zen/go/v1",anthropic:"https://opencode.ai/zen/go"}]}:base(method,value));
 render(<AISettings/>);const user=userEvent.setup();await screen.findByRole("button",{name:"重命名服务"});
 await user.click(screen.getByRole("button",{name:"从服务预设添加"}));
 await user.type(await screen.findByRole("textbox",{name:"搜索供应商"}),"OpenCode");
 await user.click(await screen.findByRole("button",{name:"OpenCode Go"}));
 expect((screen.getByLabelText("API 基础地址") as HTMLInputElement).value).toBe("https://opencode.ai/zen/go/v1");
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByText("已保存");
 expect(saved().configuration.providers[1].preset).toBe("opencode-go");
 expect(saved().configuration.providers[1].name).toBe("OpenCode Go");
});
it("preserves a replacement credential if saving fails",async()=>{
 fixture();const originalCall=api.captureCall.getMockImplementation()!;api.captureCall.mockImplementation(async(method,value)=>{if(method==="ai.configuration.save")throw Error("save failed");return originalCall(method,value);});
 render(<AISettings/>);const user=userEvent.setup();await user.type(await screen.findByLabelText("API 密钥"),"replacement");await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByRole("alert");expect((screen.getByLabelText("API 密钥") as HTMLInputElement).value).toBe("replacement");
});

it("retains manual entry when discovery fails and keeps unchecked models out of the enabled set",async()=>{
 const saved=fixture();const base=api.captureCall.getMockImplementation()!;
 api.captureCall.mockImplementation(async(method,value)=>{if(method==="ai.models.fetch")throw Error("discovery unsupported");return base(method,value);});
 render(<AISettings/>);const user=userEvent.setup();await user.click(await screen.findByRole("button",{name:"获取模型列表"}));
 await screen.findByText("Error: discovery unsupported");
 await user.type(screen.getByLabelText("搜索或填写模型 ID"),"manual-model");await user.click(screen.getByRole("button",{name:"添加模型"}));
 await user.click(screen.getByRole("checkbox",{name:"test-model"}));
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByText("已保存");
 expect(saved().configuration.providers[0].models).toEqual(["manual-model"]);
 expect(saved().configuration.providers[0].availableModels).toEqual(["test-model","manual-model"]);
 expect(saved().keys).toBeUndefined();
 expect(within(screen.getByRole("tablist",{name:"AI 服务"})).getAllByRole("tab").map(tab=>tab.textContent)).toEqual(["自定义供应商","账号供应商","默认 AI 服务","本地网关","路由","用量统计","高级设置"]);
});
