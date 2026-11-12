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


it("keeps exactly the five retained pages and never starts removed services",async()=>{
 render(<AISettings/>);await screen.findByLabelText("API 基础地址");
 expect(within(screen.getByRole("tablist",{name:"AI 服务"})).getAllByRole("tab").map(tab=>tab.textContent)).toEqual(["常用","自定义供应商","账号供应商","使用偏好","本地网关"]);
 expect(api.captureCall.mock.calls.some(([method])=>/ai\.(routing|usage|archive|magpie)\./.test(method))).toBe(false);
});

it("opens the selected provider models from Common by click and keyboard",async()=>{
 render(<AISettings/>);const user=userEvent.setup();await screen.findByLabelText("API 基础地址");
 await user.click(screen.getByRole("tab",{name:"常用"}));
 await user.click(within(screen.getByRole("region",{name:"已启用供应商"})).getByRole("button",{name:"另一服务（1）"}));
 expect(screen.getByRole("tab",{name:"自定义供应商"}).getAttribute("aria-selected")).toBe("true");
 expect(screen.getByRole("combobox",{name:"服务"}).textContent).toContain("另一服务");
 const models=screen.getByRole("region",{name:"模型设置"});
 expect(within(models).getByRole("checkbox",{name:"second-model"})).toBeTruthy();
 expect(document.activeElement).toBe(models);expect(scroll).toHaveBeenCalled();
 await user.click(screen.getByRole("tab",{name:"常用"}));
 within(screen.getByRole("region",{name:"已启用供应商"})).getByRole("button",{name:"工作账号（1）"}).focus();await user.keyboard("{Enter}");
 expect(screen.getByRole("tab",{name:"账号供应商"}).getAttribute("aria-selected")).toBe("true");
 expect(screen.getByRole("combobox",{name:"账号"}).textContent).toContain("工作账号");
 expect(within(screen.getByRole("region",{name:"模型设置"})).getByRole("checkbox",{name:"account-model"})).toBeTruthy();
});

it("preserves unsaved connection changes when Common opens another provider",async()=>{
 render(<AISettings/>);const user=userEvent.setup();
 fireEvent.change(await screen.findByLabelText("API 基础地址"),{target:{value:"https://draft.invalid/v1"}});
 for(const name of ["工作账号（1）","自定义服务（1）"]){
  await user.click(screen.getByRole("tab",{name:"常用"}));
  await user.click(within(screen.getByRole("region",{name:"已启用供应商"})).getByRole("button",{name}));
 }
 expect((screen.getByLabelText("API 基础地址") as HTMLInputElement).value).toBe("https://draft.invalid/v1");
 expect(screen.getByRole("button",{name:"保存 AI 设置"}).hasAttribute("disabled")).toBe(false);
 expect(api.captureCall.mock.calls.filter(([method])=>method==="ai.configuration.get")).toHaveLength(1);
 expect(api.captureCall.mock.calls.some(([method])=>method==="ai.configuration.save")).toBe(false);
});

it("blocks navigation during save and cannot open a removed draft provider",async()=>{
 const base=api.captureCall.getMockImplementation()!;
 let complete!:(value:unknown)=>void;let pending:any;
 api.captureCall.mockImplementation((method,value)=>method==="ai.configuration.save"?(pending=value,new Promise(resolve=>{complete=resolve;})):base(method,value));
 render(<AISettings/>);const user=userEvent.setup();
 fireEvent.change(await screen.findByLabelText("API 基础地址"),{target:{value:"https://draft.invalid/v1"}});
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));
 expect(within(screen.getByRole("tablist",{name:"AI 服务"})).getAllByRole("tab").every(tab=>tab.hasAttribute("disabled")||tab.getAttribute("aria-disabled")==="true")).toBe(true);
 await act(async()=>complete({...pending.configuration,revision:2}));
 await user.click(screen.getByRole("button",{name:"移除此服务"}));
 await user.click(screen.getByRole("tab",{name:"常用"}));
 expect(within(screen.getByRole("region",{name:"已启用供应商"})).getByRole("button",{name:"自定义服务（1）"}).hasAttribute("disabled")).toBe(true);
});
