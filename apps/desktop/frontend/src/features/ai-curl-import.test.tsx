// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {AISettings} from "./ai-settings";
const call=vi.hoisted(()=>vi.fn());
vi.mock("../bridge",()=>({captureCall:call}));
afterEach(()=>{cleanup();call.mockReset();});
function fixture(preset=false){
 const provider={id:"one",name:"Saved service",authMode:"api-key",protocol:"chat",baseUrl:"https://saved.invalid/v1",models:["saved-model"],availableModels:["saved-model"],hasKey:true,...(preset?{preset:"fixture"}:{})};
 call.mockImplementation(async(method,value)=>{
  if(method==="ai.configuration.get")return {revision:1,providers:[provider],tasks:{}};
  if(method==="ai.accounts.catalog")return {agents:[],presets:preset?[{id:"fixture",name:"Fixture",endpoint:provider.baseUrl}]:[]};
  if(method==="ai.configuration.save")return {...value.configuration,revision:2};
  throw Error(method);
 });
 render(<AISettings/>);return userEvent.setup();
}
const command=(key="sk-fixture-private",endpoint="https://api.fixture.invalid/v1/messages")=>`curl '${endpoint}' -H 'x-api-key: ${key}' -d '{"model":"imported-model"}'`;
it("imports a curl command into the saved provider draft without changing enabled models or saving",async()=>{
 const user=fixture();const url=await screen.findByLabelText("API 基础地址");
 await user.click(url);await user.paste(command());
 expect((url as HTMLInputElement).value).toBe("https://api.fixture.invalid");
 expect((screen.getByLabelText("API 密钥") as HTMLInputElement).value).toBe("sk-fixture-private");
 expect(call.mock.calls.some(([method])=>method==="ai.configuration.save")).toBe(false);
 expect(screen.queryByText(/sk-fixture-private/)).toBeNull();
 await user.click(screen.getByRole("button",{name:"保存 AI 设置"}));
 await waitFor(()=>expect(call).toHaveBeenCalledWith("ai.configuration.save",expect.objectContaining({configuration:expect.objectContaining({providers:[expect.objectContaining({protocol:"anthropic",models:["saved-model"],availableModels:["saved-model"]})]}),keyUpdates:{one:{primary:"sk-fixture-private"}}})));
});
it("imports a new provider model but leaves placeholder keys empty",async()=>{
 const user=fixture();await user.click(await screen.findByRole("button",{name:"添加自定义供应商"}));
 await user.click(screen.getByLabelText("API 基础地址"));await user.paste(command("$ANTHROPIC_API_KEY"));
 expect((screen.getByLabelText("API 密钥") as HTMLInputElement).value).toBe("");
 expect(await screen.findByText("cURL 示例使用占位密钥，请补填真实密钥。")).toBeTruthy();
 expect(screen.getByRole("checkbox",{name:"imported-model"}).getAttribute("aria-checked")).toBe("true");
});
it("accepts curl pasted into a key slot and keeps preset connection settings",async()=>{
 const user=fixture(true);const key=await screen.findByLabelText("API 密钥");
 await user.click(key);await user.paste(command());
 expect((key as HTMLInputElement).value).toBe("sk-fixture-private");
 expect((screen.getByLabelText("API 基础地址") as HTMLInputElement).value).toBe("https://saved.invalid/v1");
});
it("retains existing credentials when curl contains a placeholder and warns for unknown endpoints",async()=>{
 const user=fixture();const key=await screen.findByLabelText("API 密钥");
 await user.click(key);await user.paste("keep-this-key");await user.paste(command("YOUR_API_KEY","https://unknown.invalid/generate"));
 expect((key as HTMLInputElement).value).toBe("keep-this-key");
 expect(screen.getByText("无法识别 cURL 的接口地址，请检查地址和协议。")).toBeTruthy();
 expect((screen.getByLabelText("API 基础地址") as HTMLInputElement).value).toBe("https://saved.invalid/v1");
});
