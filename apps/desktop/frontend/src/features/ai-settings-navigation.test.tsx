// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from "vitest";
import {act,cleanup,fireEvent,render,screen,waitFor,within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {AISettings} from "./ai-settings";

const api=vi.hoisted(()=>({captureCall:vi.fn()}));
vi.mock("../bridge",()=>api);
const scroll=vi.fn();
const configuration={revision:1,engine:"google",ocrEngine:"local",target:"auto",prompt:"",tasks:{},providers:[
 {id:"first",name:"自定义服务",authMode:"api-key",protocol:"chat",baseUrl:"https://example.invalid/v1",models:["first-model"],hasKey:true},
 {id:"second",name:"另一服务",authMode:"api-key",protocol:"chat",baseUrl:"https://second.invalid/v1",models:["second-model"],hasKey:false},
 {id:"account",name:"工作账号",authMode:"account",agent:"codex",models:["account-model"],hasKey:false},
]};

beforeEach(()=>{
 // JSDOM does not lay out pages. Browser checks cover the actual scroll position.
 Object.defineProperty(Element.prototype,"scrollIntoView",{configurable:true,value:scroll});
 api.captureCall.mockImplementation(async(method,value)=>{
  if(method==="ai.configuration.get")return structuredClone(configuration);
  if(method==="ai.configuration.save")return {...value.configuration,revision:2};
  if(method==="ai.accounts.catalog")return {agents:[{id:"codex",name:"ChatGPT / Codex"}],presets:[]};
  if(method==="ai.accounts.list")return {accounts:[]};
  if(method==="ai.gateway.get")return {revision:0,enabled:false,port:3426,running:false,hasKey:false,baseUrl:"http://127.0.0.1:3426/v1"};
  throw Error(method);
 });
});
afterEach(()=>{cleanup();vi.clearAllMocks();Reflect.deleteProperty(Element.prototype,"scrollIntoView");});

it("places complete routing before usage and stores its switch in the shared draft",async()=>{
 const base=api.captureCall.getMockImplementation()!;api.captureCall.mockImplementation((method,value)=>method==="ai.routing.open"?Promise.resolve({url:"http://127.0.0.1:1234/?view=routing"}):method==="ai.routing.close"?Promise.resolve({}):base(method,value));
 render(<AISettings/>);const user=userEvent.setup();await screen.findByRole("region",{name:"已启用供应商"});
 const labels=within(screen.getByRole("tablist",{name:"AI 服务"})).getAllByRole("tab").map(tab=>tab.textContent);expect(labels.indexOf("路由")).toBe(labels.indexOf("用量统计")-1);
 await user.click(screen.getByRole("tab",{name:"路由"}));await user.click(await screen.findByRole("switch",{name:"启用模型路由"}));await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));
 await waitFor(()=>expect(api.captureCall).toHaveBeenCalledWith("ai.configuration.save",expect.objectContaining({configuration:expect.objectContaining({routingEnabled:true})})));
});

it("opens the exact provider model settings from another service page and by keyboard",async()=>{
 render(<AISettings/>);const user=userEvent.setup();
 const summary=await screen.findByRole("region",{name:"已启用供应商"});
 await user.click(screen.getByRole("tab",{name:"本地网关"}));
 await user.click(within(summary).getByRole("button",{name:"另一服务（1）"}));
 expect(screen.getByRole("tab",{name:"自定义供应商"}).getAttribute("aria-selected")).toBe("true");
 expect(screen.getByRole("combobox",{name:"服务"}).textContent).toContain("另一服务");
 const models=screen.getByRole("region",{name:"模型设置"});
 expect(within(models).getByRole("checkbox",{name:"second-model"})).toBeTruthy();
 expect(document.activeElement).toBe(models);expect(scroll).toHaveBeenCalled();
 within(summary).getByRole("button",{name:"工作账号（1）"}).focus();await user.keyboard("{Enter}");
 expect(screen.getByRole("tab",{name:"账号供应商"}).getAttribute("aria-selected")).toBe("true");
 expect(screen.getByRole("combobox",{name:"账号"}).textContent).toContain("工作账号");
 expect(within(screen.getByRole("region",{name:"模型设置"})).getByRole("checkbox",{name:"account-model"})).toBeTruthy();
});

it("keeps unsaved connection changes when the overview opens another provider",async()=>{
 render(<AISettings/>);const user=userEvent.setup();
 const address=await screen.findByLabelText("API 基础地址");
 fireEvent.change(address,{target:{value:"https://draft.invalid/v1"}});
 const summary=screen.getByRole("region",{name:"已启用供应商"});
 await user.click(within(summary).getByRole("button",{name:"工作账号（1）"}));
 await user.click(within(summary).getByRole("button",{name:"自定义服务（1）"}));
 expect((screen.getByLabelText("API 基础地址") as HTMLInputElement).value).toBe("https://draft.invalid/v1");
 expect(screen.getByRole("button",{name:"保存 AI 设置"}).hasAttribute("disabled")).toBe(false);
 expect(api.captureCall.mock.calls.filter(([method])=>method==="ai.configuration.get")).toHaveLength(1);
 expect(api.captureCall.mock.calls.some(([method])=>method==="ai.configuration.save")).toBe(false);
});

it("blocks navigation while saving and cannot route to a provider removed from the draft",async()=>{
 const base=api.captureCall.getMockImplementation()!;
 let complete!:(value:unknown)=>void;let pending:any;
 api.captureCall.mockImplementation((method,value)=>method==="ai.configuration.save"?(pending=value,new Promise(resolve=>{complete=resolve;})):base(method,value));
 render(<AISettings/>);const user=userEvent.setup();
 fireEvent.change(await screen.findByLabelText("API 基础地址"),{target:{value:"https://draft.invalid/v1"}});
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));
 const summary=screen.getByRole("region",{name:"已启用供应商"});
 expect(within(summary).getAllByRole("button").every(button=>button.hasAttribute("disabled"))).toBe(true);
 await act(async()=>complete({...pending.configuration,revision:2}));
 await waitFor(()=>expect(within(summary).getByRole("button",{name:"另一服务（1）"}).hasAttribute("disabled")).toBe(false));
 await user.click(screen.getByRole("button",{name:"移除此服务"}));
 expect(within(summary).getByRole("button",{name:"自定义服务（1）"}).hasAttribute("disabled")).toBe(true);
});
