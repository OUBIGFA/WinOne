// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen,within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {AISettings} from "./ai-settings";
const api=vi.hoisted(()=>({captureCall:vi.fn()}));
vi.mock("../bridge",()=>api);
afterEach(()=>{cleanup();vi.clearAllMocks();});
function fixture(){let saved:any;const original={revision:1,engine:"google",ocrEngine:"local",target:"auto",prompt:"",tasks:{},providers:[{id:"one",name:"示例服务",authMode:"api-key",baseUrl:"https://example.invalid",models:["vision"],modelMetadata:{vision:{vision:true}},hasKey:true}],magpieSettings:{noStats:true}};api.captureCall.mockImplementation(async(method,value)=>{
 if(method==="ai.configuration.get")return structuredClone(saved?.configuration??original);
 if(method==="ai.configuration.save"){saved=value;return {...value.configuration,revision:2};}
 // Mirror the Host's object-only RPC contract; null params must fail in tests.
 if(method==="ai.magpie.catalog"){expect(value).toEqual({});return {searchVendors:[{id:"tavily",name:"Tavily",keysURL:"https://tavily.com"},{id:"searxng",name:"SearXNG",needURL:true}],visionModels:[{id:"one/vision",name:"Vision",provider:"one",providerName:"示例服务"}],imageGenModels:[],searchChoices:[],codexUsers:["first@example.invalid","second@example.invalid"],pluginCheckins:[{id:"plugin-one",name:"Plugin One"}]};}
 throw Error(method);
 });return ()=>saved;}

it("keeps quota and privacy drafts across tabs, saves them and restores them on reopen",async()=>{
 const saved=fixture(),user=userEvent.setup(),view=render(<AISettings/>);
 const serviceTabs=await screen.findByRole("tablist",{name:"AI 服务"});await user.click(within(serviceTabs).getByRole("tab",{name:"高级设置"}));
 await user.click(await screen.findByRole("tab",{name:"额度设置"}));
 await user.click(screen.getByRole("button",{name:"剩余额度"}));
 await user.click(screen.getByRole("button",{name:"仅主动读取"}));
 await user.click(screen.getByRole("switch",{name:"额度使用提醒"}));
 await user.clear(screen.getByLabelText("提醒时已用额度（%）"));await user.type(screen.getByLabelText("提醒时已用额度（%）"),"85");
 await user.click(screen.getByRole("tab",{name:"隐私设置"}));
 await user.click(screen.getByRole("switch",{name:"脱敏个人信息"}));
 await user.click(screen.getByRole("switch",{name:"公网 IP 地址"}));
 await user.type(screen.getByLabelText("自定义脱敏词"),"Project Alpha,客户名称");
 await user.click(screen.getByRole("tab",{name:"额度设置"}));
 expect((screen.getByLabelText("提醒时已用额度（%）") as HTMLInputElement).value).toBe("85");
 expect(screen.queryByRole("button",{name:"移除此服务"})).toBeNull();
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByText("已保存");
 expect(saved().configuration.magpieSettings).toMatchObject({quotaLeft:true,quotaReads:"asked",usageAlert:85,redactPersonal:true,redactKinds:{ip:true},redactWords:["Project Alpha","客户名称"],hideAccounts:true});
 view.unmount();render(<AISettings/>);const reopen=await screen.findByRole("tablist",{name:"AI 服务"});await user.click(within(reopen).getByRole("tab",{name:"高级设置"}));await user.click(await screen.findByRole("tab",{name:"隐私设置"}));expect(screen.getByRole("switch",{name:"公网 IP 地址"}).getAttribute("aria-checked")).toBe("true");
});

it("saves an image helper, ordered search APIs and custom masking rules through the shared save flow",async()=>{
 const saved=fixture(),user=userEvent.setup();render(<AISettings/>);
 const serviceTabs=await screen.findByRole("tablist",{name:"AI 服务"});await user.click(within(serviceTabs).getByRole("tab",{name:"高级设置"}));
 await user.click(await screen.findByRole("tab",{name:"模型设置"}));
 await user.click(await screen.findByRole("combobox",{name:"识图模型"}));await user.click(screen.getByRole("option",{name:"Vision · 示例服务"}));
 await user.type(screen.getByLabelText("搜索 API 密钥"),"fixture-only-key");await user.click(screen.getByRole("button",{name:"添加搜索 API"}));
 await user.click(screen.getByRole("tab",{name:"隐私设置"}));await user.type(screen.getByLabelText("规则占位名称"),"INTERNAL_TOKEN");await user.type(screen.getByLabelText("匹配内容"),"oc_sk_");await user.click(screen.getByRole("button",{name:"添加脱敏规则"}));
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByText("已保存");
 expect(saved().configuration.magpieSettings).toMatchObject({vision:"one/vision",searchAPIs:[{vendor:"tavily",key:"fixture-only-key"}],redactRules:[{kind:"INTERNAL_TOKEN",prefix:"oc_sk_"}]});
});

it("retains every daily warm-up time and separate per-account choices",async()=>{
 const saved=fixture(),user=userEvent.setup();render(<AISettings/>);const serviceTabs=await screen.findByRole("tablist",{name:"AI 服务"});await user.click(within(serviceTabs).getByRole("tab",{name:"高级设置"}));await user.click(await screen.findByRole("tab",{name:"额度设置"}));
 const codex=await screen.findByRole("region",{name:"Codex 额度预热"});await user.click(within(codex).getByRole("switch",{name:"每日定时预热"}));await user.click(within(codex).getByRole("button",{name:"添加时间"}));
 expect(within(codex).getAllByLabelText(/预热时间 \d/)).toHaveLength(2);
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));await screen.findByText("已保存");expect(saved().configuration.magpieSettings.codexWarmAts).toEqual(["06:00","11:05"]);
});
