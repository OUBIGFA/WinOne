// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen,within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {AISettings} from "./ai-settings";

it("shows the image and vision MCP addresses beside their model choices and copies the selected endpoint",async()=>{
 fixture();const user=userEvent.setup();render(<AISettings/>);
 await user.click(within(await screen.findByRole('tablist',{name:'AI 服务'})).getByRole('tab',{name:'高级设置'}));
 await user.click(await screen.findByRole('tab',{name:'模型设置'}));
 await screen.findByText('http://127.0.0.1:3426/mcp/magpie/image');
 expect(screen.getByText('http://127.0.0.1:3426/mcp/magpie/vision')).toBeTruthy();
 await user.click(screen.getByRole('button',{name:'复制图像生成 MCP 地址'}));
 expect(await navigator.clipboard.readText()).toBe('http://127.0.0.1:3426/mcp/magpie/image');
});

it("keeps local request archiving off until saved and preserves its limits on reopen",{timeout:30000},async()=>{
 const saved=fixture(),user=userEvent.setup(),view=render(<AISettings/>);
 const open=async()=>{await user.click(within(await screen.findByRole('tablist',{name:'AI 服务'})).getByRole('tab',{name:'高级设置'}));await user.click(await screen.findByRole('tab',{name:'隐私设置'}));};
 await open();
 expect(screen.getByRole('switch',{name:'在本机保存请求正文'}).getAttribute('aria-checked')).toBe('false');
 await user.click(screen.getByRole('switch',{name:'在本机保存请求正文'}));
 await user.clear(screen.getByRole('spinbutton',{name:'归档总容量（MiB）'}));await user.type(screen.getByRole('spinbutton',{name:'归档总容量（MiB）'}),'256');
 await user.clear(screen.getByRole('spinbutton',{name:'每份正文上限（MiB）'}));await user.type(screen.getByRole('spinbutton',{name:'每份正文上限（MiB）'}),'8');
 expect(saved()).toBeUndefined();
 await user.click(screen.getByRole('button',{name:'保存 AI 设置'}));await screen.findByText('已保存');
 expect(saved().configuration.magpieSettings).toMatchObject({requestArchiveLocal:true,requestArchiveLocalMB:256,requestArchiveMaxMB:8});
 view.unmount();render(<AISettings/>);await open();
 expect(screen.getByRole('switch',{name:'在本机保存请求正文'}).getAttribute('aria-checked')).toBe('true');
 expect((screen.getByRole('spinbutton',{name:'归档总容量（MiB）'}) as HTMLInputElement).value).toBe('256');
 await user.click(screen.getByRole('switch',{name:'在本机保存请求正文'}));
 await user.click(screen.getByRole('button',{name:'撤销修改'}));
 expect(screen.getByRole('switch',{name:'在本机保存请求正文'}).getAttribute('aria-checked')).toBe('true');
});

it("saves private download hosts and Claude's lean system prompt",{timeout:30000},async()=>{
 const saved=fixture(),user=userEvent.setup();render(<AISettings/>);
 const tabs=await screen.findByRole('tablist',{name:'AI 服务'});
 await user.click(within(tabs).getByRole('tab',{name:'高级设置'}));
 await user.click(await screen.findByRole('tab',{name:'模型设置'}));
 await user.click(screen.getByRole('switch',{name:'Claude 订阅使用精简系统提示'}));
 await user.click(screen.getByRole('tab',{name:'隐私设置'}));
 await user.type(screen.getByRole('textbox',{name:'允许下载的内网地址'}),'nas.lan:8080,192.168.1.0/24');
 await user.click(screen.getByRole('button',{name:'保存 AI 设置'}));await screen.findByText('已保存');
 expect(saved().configuration.magpieSettings).toMatchObject({claudeLeanPrompt:true,fetchPrivate:['nas.lan:8080','192.168.1.0/24']});
});
const api=vi.hoisted(()=>({captureCall:vi.fn()}));
vi.mock("../bridge",()=>api);
afterEach(()=>{cleanup();vi.clearAllMocks();});
function fixture(){let saved:any;const original={revision:1,engine:"google",ocrEngine:"local",target:"auto",prompt:"",tasks:{},providers:[{id:"one",name:"示例服务",authMode:"api-key",baseUrl:"https://example.invalid",models:["vision"],modelMetadata:{vision:{vision:true}},hasKey:true}],magpieSettings:{noStats:true}};api.captureCall.mockImplementation(async(method,value)=>{
 if(method==="ai.configuration.get")return structuredClone(saved?.configuration??original);
 if(method==="ai.configuration.save"){saved=value;return {...value.configuration,revision:2};}
 // The settings fixture has no third-party surfaces. Model the real empty
 // extension lease instead of reporting an unrelated plugin failure.
 if(method==="ai.routing.extensions.open")return {url:"http://127.0.0.1:4310/session/fixture/plugins.html",pages:[],panels:[]};
 if(method==="ai.routing.extensions.close")return {};
 if(method==="ai.magpie.privacy.open"||method==="ai.magpie.privacy.state"||method==="ai.magpie.privacy.action")return {on:false,supported:false,installed:false,size:1};
 if(method==="ai.magpie.privacy.close")return {};
 // Mirror the Host's object-only RPC contract; null params must fail in tests.
 if(method==="ai.magpie.catalog"){expect(value).toEqual({});return {gateway:"http://127.0.0.1:3426",searchVendors:[{id:"tavily",name:"Tavily",keysURL:"https://tavily.com"},{id:"searxng",name:"SearXNG",needURL:true,keyOptional:true},{id:"deepseek",name:"DeepSeek"},{id:"you",name:"You.com",keyOptional:true}],visionModels:[{id:"one/vision",name:"Vision",provider:"one",providerName:"示例服务"}],imageGenModels:[],searchChoices:[],codexUsers:["first@example.invalid","second@example.invalid"],pluginCheckins:[{id:"plugin-one",name:"Plugin One"}]};}
 throw Error(method);
 });return ()=>saved;}

it("keeps quota and privacy drafts across tabs, saves them and restores them on reopen",{timeout:60000},async()=>{
 const saved=fixture(),user=userEvent.setup(),view=render(<AISettings/>);
 const serviceTabs=await screen.findByRole("tablist",{name:"AI 服务"});await user.click(within(serviceTabs).getByRole("tab",{name:"高级设置"}));
 await user.click(await screen.findByRole("tab",{name:"额度设置"}));
 await user.click(screen.getByRole("radio",{name:"剩余额度"}));
 await user.click(screen.getByRole("radio",{name:"仅主动读取"}));
 await user.click(screen.getByRole("switch",{name:"额度使用提醒"}));
 await user.clear(screen.getByLabelText("提醒时已用额度（%）"));await user.type(screen.getByLabelText("提醒时已用额度（%）"),"85");
 await user.click(screen.getByRole("tab",{name:"隐私设置"}));
 await user.click(screen.getByRole("switch",{name:"保留密钥格式"}));
 await user.click(screen.getByRole("switch",{name:"脱敏个人信息"}));
 await user.click(screen.getByRole("switch",{name:"公网 IP 地址"}));
 await user.type(screen.getByLabelText("自定义脱敏词"),"Project Alpha,客户名称");
 await user.click(screen.getByRole("switch",{name:"脱敏词忽略大小写"}));
 await user.click(screen.getByRole("tab",{name:"额度设置"}));
 expect((screen.getByLabelText("提醒时已用额度（%）") as HTMLInputElement).value).toBe("85");
 expect(screen.queryByRole("button",{name:"移除此服务"})).toBeNull();
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByText("已保存");
 expect(saved().configuration.magpieSettings).toMatchObject({quotaLeft:true,quotaReads:"asked",usageAlert:85,redactPersonal:true,redactShape:true,redactKinds:{ip:true},redactWords:["Project Alpha","客户名称"],redactWordsAnyCase:true,hideAccounts:true});
 view.unmount();render(<AISettings/>);const reopen=await screen.findByRole("tablist",{name:"AI 服务"});await user.click(within(reopen).getByRole("tab",{name:"高级设置"}));await user.click(await screen.findByRole("tab",{name:"隐私设置"}));expect(screen.getByRole("switch",{name:"保留密钥格式"}).getAttribute("aria-checked")).toBe("true");expect(screen.getByRole("switch",{name:"公网 IP 地址"}).getAttribute("aria-checked")).toBe("true");
});

it("saves an image helper, ordered search APIs and custom masking rules through the shared save flow",{timeout:60000},async()=>{
 const saved=fixture(),user=userEvent.setup();render(<AISettings/>);
 const serviceTabs=await screen.findByRole("tablist",{name:"AI 服务"});await user.click(within(serviceTabs).getByRole("tab",{name:"高级设置"}));
 await user.click(await screen.findByRole("tab",{name:"模型设置"}));
 await user.click(await screen.findByRole("combobox",{name:"识图模型"}));await user.click(await screen.findByRole("option",{name:"Vision · 示例服务"}));
 await user.type(screen.getByLabelText("搜索 API 密钥"),"fixture-only-key");await user.click(screen.getByRole("button",{name:"添加搜索 API"}));
 await user.click(screen.getByRole("tab",{name:"隐私设置"}));await user.type(screen.getByLabelText("规则占位名称"),"INTERNAL_TOKEN");await user.type(screen.getByLabelText("匹配内容"),"oc_sk_");await user.click(screen.getByRole("button",{name:"添加脱敏规则"}));
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByText("已保存");
 expect(saved().configuration.magpieSettings).toMatchObject({vision:"one/vision",searchAPIs:[{vendor:"tavily",key:"fixture-only-key"}],redactRules:[{kind:"INTERNAL_TOKEN",prefix:"oc_sk_"}]});
});

it("preserves DeepSeek keys, API ordering, disablement and free You.com search",async()=>{
 const saved=fixture(),user=userEvent.setup();render(<AISettings/>);
 await user.click(within(await screen.findByRole("tablist",{name:"AI 服务"})).getByRole("tab",{name:"高级设置"}));
 await user.click(await screen.findByRole("tab",{name:"模型设置"}));
 await user.click(screen.getByRole("combobox",{name:"搜索 API"}));await user.click(await screen.findByRole("option",{name:"DeepSeek"}));
 await user.type(screen.getByLabelText("搜索 API 密钥"),"fixture-one,fixture-two");
 await user.type(screen.getByLabelText("搜索 API 地址"),"http://127.0.0.1:9091");
 await user.click(screen.getByRole("button",{name:"添加搜索 API"}));
 await user.click(screen.getByRole("switch",{name:"启用 DeepSeek 搜索"}));
 await user.click(screen.getByRole("radio",{name:"按顺序尝试"}));
 await user.click(screen.getByRole("combobox",{name:"搜索 API"}));await user.click(await screen.findByRole("option",{name:"You.com"}));
 await user.click(screen.getByRole("button",{name:"添加搜索 API"}));
 await user.click(screen.getByRole("button",{name:"上移 You.com"}));
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByText("已保存");
 expect(saved().configuration.magpieSettings.searchAPIs).toMatchObject([{vendor:"you"},{vendor:"deepseek",key:"fixture-one,fixture-two",url:"http://127.0.0.1:9091",off:true,pick:"order"}]);
});

it("retains every daily warm-up time and separate per-account choices",{timeout:60000},async()=>{
 const saved=fixture(),user=userEvent.setup();render(<AISettings/>);const serviceTabs=await screen.findByRole("tablist",{name:"AI 服务"});await user.click(within(serviceTabs).getByRole("tab",{name:"高级设置"}));await user.click(await screen.findByRole("tab",{name:"额度设置"}));
 const codexSummary=await screen.findByText("自动预热与签到",{selector:"summary"});await user.click(codexSummary);
 const codex=await screen.findByRole("region",{name:"Codex 额度预热"});await user.click(within(codex).getByRole("switch",{name:"每日定时预热"}));await user.click(within(codex).getByRole("button",{name:"添加时间"}));
 expect(within(codex).getAllByLabelText(/预热时间 \d/)).toHaveLength(2);
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByText("已保存");expect(saved().configuration.magpieSettings.codexWarmAts).toEqual(["06:00","11:05"]);
});

it("persists the classifier timeout and restores it when reopening",async()=>{
 const saved=fixture(),user=userEvent.setup();const view=render(<AISettings/>);
 await user.click(within(await screen.findByRole("tablist",{name:"AI 服务"})).getByRole("tab",{name:"高级设置"}));
 await user.click(await screen.findByRole("tab",{name:"模型设置"}));
 const field=screen.getByLabelText("路由判断超时（秒）");await user.clear(field);await user.type(field,"23");
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByText("已保存");
 expect(saved().configuration.magpieSettings.classifyTimeout).toBe(23);
 view.unmount();render(<AISettings/>);await user.click(within(await screen.findByRole("tablist",{name:"AI 服务"})).getByRole("tab",{name:"高级设置"}));await user.click(await screen.findByRole("tab",{name:"模型设置"}));expect((screen.getByLabelText("路由判断超时（秒）") as HTMLInputElement).value).toBe("23");
});
