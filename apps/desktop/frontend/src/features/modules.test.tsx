// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ModulesPage } from "./modules";
import { setApplicationLanguage } from "../i18n";

it("shows a registered optional tool without leaking a translation key",async()=>{
 const state=initial();state.modules[0].manifest.id="example.tool";state.modules[0].manifest.name={"zh-CN":"扩展示例","en-US":"Example tool"};
 call.mockResolvedValue(state);render(<ModulesPage onChanged={()=>{}}/>);
 expect(await screen.findByRole("switch",{name:"扩展示例"})).toBeTruthy();
 expect(screen.queryByText("moduleDescription.example.tool")).toBeNull();
});

const call = vi.hoisted(() => vi.fn());
vi.mock("../bridge", () => ({ moduleCall: call, getShortcuts:async()=>({bindings:[{id:"capture",shortcut:"Ctrl+Alt+S",registered:true}],lastError:""}) }));
afterEach(async () => { cleanup(); call.mockReset(); await setApplicationLanguage("zh-CN"); });

it("manages enabled tools without duplicating sidebar navigation",async()=>{
 const state=initial();state.modules[0].enabled=false;state.modules[0].runtime.state="disabled";
 call.mockResolvedValue(state);
 render(<ModulesPage onChanged={()=>{}}/>);
 await screen.findByRole("switch",{name:"截图"});
 expect(screen.queryByRole("button",{name:"打开"})).toBeNull();
 expect(screen.queryByRole("link")).toBeNull();expect(call.mock.calls.every(([method])=>method==="snapshot")).toBe(true);
});

function initial() {
  return { revision: 0, modules: [{
    manifest: { id: "capture", name: { "zh-CN": "截图", "en-US": "Capture" }, distribution: "bundled" },
    enabled: true, pendingStop: false,
    runtime: { id: "capture", name: "截图", state: "ready", detail: "已连接" },
  }] };
}

it.each([[false,"普通权限"],[true,"管理员"]] as const)("shows observed Host privilege without implying elevation is enabled",async(administrator,label)=>{
 call.mockResolvedValue(initial());render(<ModulesPage onChanged={()=>{}} administrator={administrator}/>);
 expect(await screen.findByText(label)).toBeTruthy();
 expect(screen.queryByRole("switch",{name:"运行权限"})).toBeNull();
});

it("clears a startup error after the user successfully retries loading",async()=>{
 call.mockRejectedValueOnce(Error("offline")).mockResolvedValue(initial());
 render(<ModulesPage onChanged={()=>{}}/>);
 await screen.findByRole("alert");await userEvent.setup().click(screen.getByRole("button",{name:"重试"}));
 await screen.findByRole("switch",{name:"截图"});expect(screen.queryByRole("alert")).toBeNull();
});

it.each([true,false])("accepts a completed configuration response when enabling is %s",async enabled=>{
 const state=initial();state.modules[0].enabled=!enabled;
 call.mockImplementation(async(method:string)=>{
  if(method==="snapshot")return structuredClone(state);
  if(method==="setEnabled"){state.modules[0].enabled=enabled;state.revision++;return {id:"",state:"completed"};}
  throw Error(method);
 });
 render(<ModulesPage onChanged={()=>{}}/>);
 await userEvent.setup().click(await screen.findByRole("switch",{name:"截图"}));
 await screen.findByText(enabled?"截图已开启":"截图已关闭");
 expect(screen.queryByRole("alert")).toBeNull();
 expect(screen.getByRole("switch",{name:"截图"}).getAttribute("aria-checked")).toBe(String(enabled));
 expect(call.mock.calls.some(([method])=>method==="jobs.get")).toBe(false);
});

it("shows a recovery retry failure and keeps recovery available",async()=>{
 call.mockImplementation(async(method:string)=>{
  if(method==="snapshot")return {...initial(),configurationError:"上次恢复未完成"};
  if(method==="recoverConfiguration")throw Error("请先完成当前截图");
  throw Error(method);
 });
 render(<ModulesPage onChanged={()=>{}}/>);
 await userEvent.setup().click(await screen.findByRole("button",{name:"重试恢复配置"}));
 await screen.findByText("请先完成当前截图");
 await waitFor(()=>expect(screen.getByRole("button",{name:"重试恢复配置"}).hasAttribute("disabled")).toBe(false));
 expect(screen.getByRole("switch",{name:"截图"}).getAttribute("data-disabled")).not.toBeNull();
});

it.each([true,false])("rejects an unidentifiable module operation instead of confirming success (missing id: %s)",async missing=>{
 call.mockImplementation(async(method:string)=>{
  if(method==="snapshot")return initial();
  if(method==="setEnabled")return missing?{state:"running"}:{id:"expected",state:"running"};
  if(method==="jobs.get")return {id:"unrelated",state:"completed"};
  throw Error(method);
 });
 render(<ModulesPage onChanged={()=>{}}/>);
 await userEvent.setup().click(await screen.findByRole("switch",{name:"截图"}));
 await screen.findByRole("alert");
 expect(screen.queryByText("截图已关闭")).toBeNull();
 expect(screen.getByRole("switch",{name:"截图"}).getAttribute("aria-checked")).toBe("true");
 expect(call.mock.calls.filter(([method])=>method==="jobs.get")).toHaveLength(missing?0:1);
});

it("renders the module controls from the English resource bundle",async()=>{
  await setApplicationLanguage("en-US");
  call.mockResolvedValue(initial());
  render(<ModulesPage onChanged={()=>{}}/>);
  await screen.findByRole("switch",{name:"Capture"});
  expect(screen.getByText("Enabled")).toBeTruthy();
  expect(screen.queryByRole("button",{name:"Open"})).toBeNull();
});

it("offers configuration recovery while preventing new module changes",async()=>{
  let recoveryError="截图操作尚未完成";
  let finish:()=>void=()=>{};
  const pending=new Promise<void>(resolve=>{finish=resolve;});
  call.mockImplementation(async(method:string)=>{
    if(method==="snapshot")return {...initial(),configurationError:recoveryError};
    if(method==="recoverConfiguration"){await pending;recoveryError="";return {state:"completed"};}
    throw new Error(method);
  });
  render(<ModulesPage onChanged={()=>{}}/>);
  const retry=await screen.findByRole("button",{name:"重试恢复配置"});
  expect(screen.getByRole("switch",{name:"截图"}).getAttribute("data-disabled")).not.toBeNull();
  await userEvent.setup().click(retry);
  expect(retry.hasAttribute("disabled")).toBe(true);
  expect(screen.queryByText("配置已恢复")).toBeNull();
  await userEvent.setup().click(retry);
  expect(call.mock.calls.filter(([method])=>method==="recoverConfiguration")).toHaveLength(1);
  finish();
  await screen.findByText("配置已恢复");
  expect(screen.queryByRole("button",{name:"重试恢复配置"})).toBeNull();
});

it("waits for the backend job before reporting a module disabled", async () => {
  const state = initial();
  let complete: (value: unknown) => void = () => {};
  const pending = new Promise(resolve => { complete = resolve; });
  const changed = vi.fn();
  call.mockImplementation(async (method: string) => {
    if (method === "snapshot") return structuredClone(state);
    if (method === "setEnabled") return { id: "disable", state: "running" };
    if (method === "jobs.get") return pending;
    throw new Error(method);
  });
  render(<ModulesPage onChanged={changed} />);
  await userEvent.setup().click(await screen.findByRole("switch", { name: "截图" }));
  await screen.findByText("正在更新状态");
  expect(changed).not.toHaveBeenCalled();
  expect(screen.getByRole("switch", { name: "截图" }).getAttribute("aria-checked")).toBe("true");
  state.revision = 2; state.modules[0].enabled = false; state.modules[0].runtime.state = "disabled";
  complete({ id: "disable", state: "completed" });
  await waitFor(() => expect(changed).toHaveBeenCalledOnce());
  expect(screen.getByRole("switch", { name: "截图" }).getAttribute("aria-checked")).toBe("false");
});

it("keeps an incomplete close visible and retries with the latest revision", async () => {
  const state = initial();
  let attempts = 0;
  call.mockImplementation(async (method: string) => {
    if (method === "snapshot") return structuredClone(state);
    if (method === "setEnabled") {
      attempts++; state.revision++; state.modules[0].enabled = false;
      state.modules[0].pendingStop = attempts === 1;
      state.modules[0].runtime.state = attempts === 1 ? "faulted" : "disabled";
      return attempts === 1 ? { id: "a", state: "failed", error: "请先完成截图操作" } : { id: "b", state: "completed" };
    }
    throw new Error(method);
  });
  render(<ModulesPage onChanged={() => {}} />);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("switch", { name: "截图" }));
  await screen.findByRole("alert");
  const retry = await screen.findByRole("button", { name: "继续关闭" });
  expect(screen.getByText("关闭未完成")).toBeTruthy();
  await user.click(retry);
  await waitFor(() => expect(screen.queryByRole("button", { name: "继续关闭" })).toBeNull());
  expect(call).toHaveBeenLastCalledWith("snapshot");
  expect(call).toHaveBeenCalledWith("setEnabled", { id: "capture", enabled: false, revision: 1 });
});
