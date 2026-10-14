// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ModulesPage } from "./modules";
import { setApplicationLanguage } from "../i18n";

it("shows a registered optional tool without leaking a translation key",async()=>{
 const state=initial();state.modules[0].manifest.id="example.tool";state.modules[0].manifest.name={"zh-CN":"扩展示例","en-US":"Example tool"};
 call.mockResolvedValue(state);render(<ModulesPage onChanged={()=>{}}/>);
 expect(await screen.findByRole("switch",{name:"扩展示例"})).toBeTruthy();
 expect(screen.queryByText("moduleDescription.example.tool")).toBeNull();
});

it.each([true,false])('keeps a faulted package disabled in the UI and retries through its recovery action (pending stop %s)',async pendingStop=>{
 const state:any=initial();state.modules[0]={manifest:{id:'example.tool',name:{'zh-CN':'测试扩展','en-US':'Test extension'},distribution:'optional'},enabled:true,pendingStop,blocked:true,recoveryId:'change-1',runtime:{id:'example.tool',name:'测试扩展',state:'unavailable',detail:'signature check failed'}};
 if(pendingStop)state.configurationError='模块配置需要恢复';
 call.mockImplementation(async(method:string)=>{if(method==='snapshot')return structuredClone(state);if(method==='recoverPackage'){delete state.configurationError;state.modules[0].blocked=false;state.modules[0].pendingStop=false;state.modules[0].runtime.state='ready';return {state:'completed'};}throw Error(method)});
 const user=userEvent.setup();render(<ModulesPage onChanged={()=>{}}/>);
 const toggle=await screen.findByRole('switch',{name:'测试扩展'});expect(toggle.getAttribute('data-disabled')).not.toBeNull();
 expect(screen.queryByRole('button',{name:'继续关闭'})).toBeNull();
 await user.click(screen.getByRole('button',{name:'恢复旧版本'}));
 await screen.findByText('测试扩展已重新加载');
 expect(call).toHaveBeenCalledWith('recoverPackage',{id:'example.tool',revision:0,recoveryId:'change-1'});
 expect(screen.getByRole('switch',{name:'测试扩展'}).getAttribute('data-disabled')).toBeNull();
});

it('retains a fault and retry action when package recovery fails',async()=>{
 const state:any=initial();state.modules[0]={manifest:{id:'example.tool',name:{'zh-CN':'测试扩展','en-US':'Test extension'},distribution:'optional'},enabled:false,pendingStop:false,blocked:true,runtime:{id:'example.tool',name:'测试扩展',state:'unavailable',detail:'signature check failed'}};
 call.mockImplementation(async(method:string)=>{if(method==='snapshot')return state;throw Error('签名仍不匹配');});
 render(<ModulesPage onChanged={()=>{}}/>);await userEvent.setup().click(await screen.findByRole('button',{name:'重试加载'}));
 await screen.findByText('签名仍不匹配');expect(screen.getByRole('switch',{name:'测试扩展'}).getAttribute('data-disabled')).not.toBeNull();
 await waitFor(()=>expect(screen.getByRole('button',{name:'重试加载'}).hasAttribute('disabled')).toBe(false));
});

const call = vi.hoisted(() => vi.fn());
const shortcuts = vi.hoisted(() => vi.fn(async()=>({bindings:[{id:"capture",shortcut:"Ctrl+Alt+S",registered:true}],lastError:""})));
vi.mock("../bridge", () => ({ moduleCall: call, getShortcuts:shortcuts }));
afterEach(async () => { cleanup(); call.mockReset(); shortcuts.mockReset(); await setApplicationLanguage("zh-CN"); });

it.each(["zh-CN","en-US"])("keeps Home focused on tools without shortcut or startup summaries in %s",async language=>{
 await setApplicationLanguage(language);call.mockResolvedValue(initial());
 const ready=vi.fn();render(<ModulesPage onChanged={()=>{}} onReady={ready}/>);
 await waitFor(()=>expect(ready).toHaveBeenCalledOnce());
 expect(screen.getByRole("switch",{name:language==="zh-CN"?"截图":"Capture"})).toBeTruthy();
 expect(shortcuts).not.toHaveBeenCalled();
 expect(screen.queryByText(language==="zh-CN"?"当前快捷键":"Current shortcuts")).toBeNull();
 expect(screen.queryByText(language==="zh-CN"?"登录 Windows 时启动":"Start at Windows sign-in")).toBeNull();
});

it("does not report failed module loading ready",async()=>{
 call.mockRejectedValue(Error("modules unavailable"));
 const ready=vi.fn();render(<ModulesPage onChanged={()=>{}} onReady={ready}/>);
 await screen.findByRole("alert");expect(ready).not.toHaveBeenCalled();
});

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

it("keeps mouse gestures inside Convenience while retaining its dedicated enable control",async()=>{
 const state=initial();state.modules.push({manifest:{id:"stroke",name:{"zh-CN":"鼠标手势","en-US":"Mouse gestures"},distribution:"bundled"},enabled:true,pendingStop:false,runtime:{id:"stroke",name:"鼠标手势",state:"ready",detail:""}});
 call.mockResolvedValue(state);
 const view=render(<ModulesPage onChanged={()=>{}}/>);
 await screen.findByRole("switch",{name:"截图"});
 expect(screen.queryByRole("heading",{name:"鼠标手势"})).toBeNull();
 expect(screen.queryByRole("switch",{name:"鼠标手势"})).toBeNull();
 view.rerender(<ModulesPage only="stroke" onChanged={()=>{}}/>);
 expect(await screen.findByRole("switch",{name:"启用鼠标手势"})).toBeTruthy();
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

it('keeps pending file cleanup visible after failure and clears it after retry',async()=>{
 const state:any=initial();state.cleanup={pending:true};let occupied=true;
 call.mockImplementation(async(method:string)=>{
  if(method==='snapshot')return structuredClone(state);
  if(method==='cleanupPackages'){if(occupied)throw Error('旧文件仍被占用');state.cleanup.pending=false;return {state:'completed'};}
  throw Error(method);
 });
 const user=userEvent.setup();render(<ModulesPage onChanged={()=>{}}/>);
 await user.click(await screen.findByRole('button',{name:'重试清理'}));await screen.findByText('旧文件仍被占用');
 await waitFor(()=>expect(screen.getByRole('button',{name:'重试清理'}).hasAttribute('disabled')).toBe(false));
 occupied=false;await user.click(screen.getByRole('button',{name:'重试清理'}));await screen.findByText('安装遗留文件已处理');
 expect(screen.queryByRole('button',{name:'重试清理'})).toBeNull();
});
