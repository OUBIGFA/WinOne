// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,fireEvent,render,screen} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {AIGateway} from "./ai-gateway";
import type {AIProvider} from "./ai-configuration";
const call=vi.hoisted(()=>vi.fn());vi.mock("../bridge",()=>({captureCall:call}));
const original={revision:0,enabled:false,port:3426,running:false,hasKey:false,baseUrl:"http://127.0.0.1:3426/v1"};
afterEach(()=>{cleanup();call.mockReset()});

it("repeatedly shows the gateway access key and hides it without an extra read",async()=>{
 call.mockImplementation(async method=>method==="ai.gateway.get"?{...original,hasKey:true}:{key:"local-test-key"});
 render(<AIGateway active providers={[]}/>);const user=userEvent.setup();
 for(let cycle=1;cycle<=3;cycle++){
  await user.click(await screen.findByRole("button",{name:"显示访问密钥"}));
  expect((screen.getByLabelText("访问密钥") as HTMLInputElement).value).toBe("local-test-key");
  await user.click(screen.getByRole("button",{name:"隐藏访问密钥"}));
  expect((screen.getByLabelText("访问密钥") as HTMLInputElement).value).toBe("");
  expect(call.mock.calls.filter(([method])=>method==="ai.gateway.key")).toHaveLength(cycle);
 }
});

it("shows provider and model names with icons and saves a separate gateway selection",async()=>{
 const providers:AIProvider[]=[{id:"opaque-supplier",name:"工作服务",authMode:"api-key",hasKey:false,icon:"openai",models:["opaque-model-a","claude-sonnet-4"],modelNames:{"opaque-model-a":"GPT 5"}},{id:"another-supplier",name:"私人账号",authMode:"account",hasKey:false,icon:"openai",models:["opaque-model-a"],modelNames:{"opaque-model-a":"GPT 5"}}];
 call.mockImplementation(async(method,value)=>method==="ai.gateway.get"?original:{...original,...value.settings,revision:1});
 const activity=vi.fn();render(<AIGateway active providers={providers} onActivity={activity}/>);const user=userEvent.setup();
 const model=await screen.findByRole("checkbox",{name:"工作服务 GPT 5"});
 expect(model.getAttribute("aria-checked")).toBe("true");
 expect(screen.queryByText(/opaque-supplier|opaque-model-a/)).toBeNull();
 expect(document.querySelectorAll('[data-icon="openai"]').length).toBeGreaterThan(0);
 expect(document.querySelector('[data-icon="claude-color"]')).not.toBeNull();
 await user.click(model);expect(activity).toHaveBeenLastCalledWith(true,false,"ai-gateway",expect.any(Function));
 await user.click(screen.getByRole("button",{name:"保存网关设置"}));await screen.findByText("已保存");
 const payload=call.mock.calls.find(([method])=>method==="ai.gateway.save")?.[1];
 expect(payload.settings.models).toEqual([{provider:"opaque-supplier",model:"claude-sonnet-4"},{provider:"another-supplier",model:"opaque-model-a"}]);
 expect(providers[0].models).toEqual(["opaque-model-a","claude-sonnet-4"]);
});
it("saves a local gateway without modifying providers and protects an unsaved port",async()=>{
 const activity=vi.fn();call.mockImplementation(async(method,value)=>method==="ai.gateway.get"?original:{...original,...value.settings,revision:1,running:true,hasKey:true});
 render(<AIGateway active providers={[]} onActivity={activity}/>);const user=userEvent.setup();
 await user.click(await screen.findByRole("switch",{name:"启用本地网关"}));fireEvent.change(screen.getByLabelText("端口"),{target:{value:"3456"}});
 expect(activity).toHaveBeenLastCalledWith(true,false,"ai-gateway",expect.any(Function));
 await user.click(screen.getByRole("button",{name:"保存网关设置"}));await screen.findByText("已保存");
 expect(call).toHaveBeenCalledWith("ai.gateway.save",{settings:{revision:0,enabled:true,port:3456,models:null},resetKey:false});
 expect(call.mock.calls.every(([method])=>method.startsWith("ai.gateway."))).toBe(true);
});
it("retains the form and shows the error when a port cannot be bound",async()=>{
 call.mockImplementation(async method=>{if(method==="ai.gateway.get")return original;throw Error("端口不可用")});render(<AIGateway active providers={[]}/>);const user=userEvent.setup();
 await user.click(await screen.findByRole("switch",{name:"启用本地网关"}));await user.click(screen.getByRole("button",{name:"保存网关设置"}));await screen.findByRole("alert");
 expect(screen.getByRole("switch").getAttribute("aria-checked")).toBe("true");expect((screen.getByLabelText("端口") as HTMLInputElement).value).toBe("3426");
});
