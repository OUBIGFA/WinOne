// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { act,cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CapturePage } from "./capture";
import { setApplicationLanguage } from "../i18n";
import {useState} from "react";
import {usePageNavigation} from "../ui/page-navigation";

function CaptureNavigation(){
 const [page,setPage]=useState("capture"),navigation=usePageNavigation(page,setPage);
 return <><button onClick={()=>navigation.requestPage("home")}>首页</button>{page==="capture"?<CapturePage disabled onActivity={navigation.onActivity}/>:<p>工具首页</p>}{navigation.dialog}</>;
}

it("waits for directory selection and protects the newly selected unsaved path",async()=>{
 const settings={schemaVersion:1,revision:0,outputDirectory:"C:\\Images",copyToClipboard:false,askWhereToSave:false,openEditor:false,pinAfterCapture:false,theme:"system",scrollDelay:300,scrollFromTop:false};
 native.call.mockResolvedValue(settings);let finish!:(value:string)=>void;native.pick.mockImplementation(()=>new Promise(resolve=>{finish=resolve;}));
 render(<CaptureNavigation/>);const user=userEvent.setup();
 const choose=await screen.findByRole("button",{name:"选择目录"});await user.click(choose);
 expect(choose.matches(":disabled")).toBe(true);
 await user.click(screen.getByRole("button",{name:"首页"}));
 expect((await screen.findByRole("dialog")).textContent).toContain("操作尚未完成");
 await act(async()=>finish("D:\\Captures"));
 await screen.findByText("有未保存的更改");await user.click(screen.getByRole("button",{name:"继续编辑"}));
 expect(screen.getByText("D:\\Captures")).toBeTruthy();expect(screen.queryByText("工具首页")).toBeNull();
 expect(native.pick).toHaveBeenCalledOnce();expect(native.call.mock.calls.some(([method])=>method==="settings.save")).toBe(false);
});

it.each(["cancel","failure"])("restores directory selection after %s without changing preferences",async outcome=>{
 const settings={schemaVersion:1,revision:0,outputDirectory:"C:\\Images",copyToClipboard:false,askWhereToSave:false,openEditor:false,pinAfterCapture:false,theme:"system",scrollDelay:300,scrollFromTop:false};
 native.call.mockResolvedValue(settings);let finish!:(value:string)=>void,fail!:(error:Error)=>void;
 native.pick.mockImplementation(()=>new Promise((resolve,reject)=>{finish=resolve;fail=reject;}));const activity=vi.fn();
 render(<CapturePage disabled onActivity={activity}/>);const user=userEvent.setup();
 await user.click(await screen.findByRole("button",{name:"选择目录"}));
 expect(activity).toHaveBeenLastCalledWith(false,true,"page");
 await act(async()=>{if(outcome==="cancel")finish("");else fail(Error("Folder picker failed"));});
 expect(activity).toHaveBeenLastCalledWith(false,false,"page");
 expect(screen.getByText("C:\\Images")).toBeTruthy();
 expect(screen.getByRole("button",{name:"保存设置"}).hasAttribute("disabled")).toBe(true);
 expect(screen.getByRole("button",{name:"选择目录"}).matches(":disabled")).toBe(false);
 if(outcome==="failure")expect(screen.getByRole("alert").textContent).toContain("Folder picker failed");
});

it.each(["","99","2001","300.5"])("preserves invalid scroll interval '%s' and refuses to save it",async value=>{
 let settings={schemaVersion:1,revision:0,outputDirectory:"C:\\Images",copyToClipboard:false,askWhereToSave:false,openEditor:false,pinAfterCapture:false,theme:"system",scrollDelay:300,scrollFromTop:false};
 native.call.mockImplementation(async(method:string,payload?:typeof settings)=>{
  if(method==="settings.get")return settings;
  if(method==="settings.save"){settings={...payload!,revision:1};return settings;}
  throw Error(method);
 });
 render(<CapturePage disabled/>);const user=userEvent.setup();
 await user.click(await screen.findByRole("tab",{name:"滚动截图设置"}));
 const input=screen.getByRole("spinbutton") as HTMLInputElement;
 await user.clear(input);if(value)await user.type(input,value);
 expect(input.value).toBe(value);expect(input.getAttribute("aria-invalid")).toBe("true");
 expect(document.getElementById(input.getAttribute("aria-describedby")!)?.textContent).toContain("100–2000");
 await user.click(screen.getByRole("button",{name:"保存设置"}));
 expect(native.call.mock.calls.some(([method])=>method==="settings.save")).toBe(false);
 await user.click(screen.getByRole("tab",{name:"截图后"}));
 expect((screen.getByRole("button",{name:"保存设置"}) as HTMLButtonElement).disabled).toBe(true);
 await user.click(screen.getByRole("button",{name:"检查滚动间隔"}));
 const restored=screen.getByRole("spinbutton") as HTMLInputElement;
 expect(restored.value).toBe(value);
 await user.clear(restored);await user.type(restored,"500");
 await user.click(screen.getByRole("button",{name:"保存设置"}));await screen.findByText("已保存");
 expect(settings.scrollDelay).toBe(500);
});

const native = vi.hoisted(() => ({
  call: vi.fn(),
  pick: vi.fn(),
  reveal: vi.fn().mockResolvedValue(undefined),
  logs: vi.fn().mockResolvedValue({folderAvailable:false,entries:[]}),
}));
vi.mock("../bridge", () => ({
  captureCall: native.call,
  pickDirectory: native.pick,
  reveal: native.reveal,
  readModuleLogs: native.logs,
  openModuleLogFolder: vi.fn(),
}));
afterEach(async () => {
  cleanup();
  native.call.mockReset();
  native.pick.mockReset();
  native.logs.mockClear();
  await setApplicationLanguage("zh-CN");
});

it("groups settings and diagnostics before the separate capture actions",async()=>{
 const settings={schemaVersion:1,revision:0,outputDirectory:"C:\\Images",copyToClipboard:false,askWhereToSave:false,openEditor:false,pinAfterCapture:false,theme:"system",scrollDelay:300,scrollFromTop:true};
 native.call.mockResolvedValue({settings,busy:false,pins:0});
 render(<CapturePage/>);
 const tabs=await screen.findByRole("tablist");
 const capture=screen.getByRole("button",{name:"窗口截图"});
 expect(tabs.compareDocumentPosition(capture)&Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
 await userEvent.setup().click(screen.getByRole("tab",{name:"诊断日志"}));
 await waitFor(()=>expect(native.logs).toHaveBeenCalledWith("capture"));
 expect(screen.getByRole("button",{name:"窗口截图"})).toBeTruthy();
});

it("keeps diagnostics available when capture settings fail to load",async()=>{
 native.call.mockRejectedValue(Error("capture unavailable"));
 render(<CapturePage/>);await screen.findByRole("alert");
 await userEvent.setup().click(screen.getByRole("tab",{name:"诊断日志"}));
 await waitFor(()=>expect(native.logs).toHaveBeenCalledWith("capture"));
 expect(screen.queryByRole("button",{name:"窗口截图"})).toBeNull();
});

it("keeps the diagnostic tab selected when a delayed settings read finishes",async()=>{
 let finish!:(value:unknown)=>void;
 native.call.mockImplementation(()=>new Promise(resolve=>{finish=resolve;}));
 render(<CapturePage/>);
 await userEvent.setup().click(await screen.findByRole("tab",{name:"诊断日志"}));
 await waitFor(()=>expect(native.logs).toHaveBeenCalledWith("capture"));
 await act(async()=>finish({settings:{schemaVersion:1,revision:0,outputDirectory:"C:\\Images",copyToClipboard:false,askWhereToSave:false,openEditor:false,pinAfterCapture:false,theme:"system",scrollDelay:300,scrollFromTop:true},busy:false,pins:0}));
 expect(screen.getByRole("tab",{name:"诊断日志"}).getAttribute("aria-selected")).toBe("true");
 expect(screen.queryByRole("radio",{name:"截完保存"})).toBeNull();
});

it("does not submit settings twice while diagnostics stay accessible",async()=>{
 const settings={schemaVersion:1,revision:0,outputDirectory:"C:\\Images",copyToClipboard:false,askWhereToSave:false,openEditor:false,pinAfterCapture:false,theme:"system",scrollDelay:300,scrollFromTop:true};
 let finish!:(value:typeof settings)=>void;
 native.call.mockImplementation((method:string)=>method==="settings.get"?Promise.resolve(settings):new Promise(resolve=>{finish=resolve;}));
 render(<CapturePage disabled/>);const user=userEvent.setup();
 await user.click(await screen.findByRole("checkbox",{name:"复制到剪贴板"}));
 await user.click(screen.getByRole("button",{name:"保存设置"}));
 expect((screen.getByRole("button",{name:"保存设置"}) as HTMLButtonElement).disabled).toBe(true);
 await user.click(screen.getByRole("tab",{name:"诊断日志"}));
 expect(native.logs).toHaveBeenCalledWith("capture");
 await act(async()=>finish({...settings,copyToClipboard:true,revision:1}));
 expect(native.call.mock.calls.filter(([method])=>method==="settings.save")).toHaveLength(1);
});

it("stops only the active scrolling job and opens its retained draft",async()=>{
 const settings={schemaVersion:1,revision:0,outputDirectory:"C:\\Images",copyToClipboard:false,askWhereToSave:false,openEditor:false,pinAfterCapture:false,theme:"system",scrollDelay:300,scrollFromTop:false};
 let stopped=false;
 native.call.mockImplementation(async(method:string,params:any)=>{
  if(method==="snapshot")return {settings,busy:false,pins:0};
  if(method==="capture.unified")return {id:"scroll-one",state:"running"};
  if(method==="scroll.stop"){expect(params).toEqual({id:"scroll-one"});stopped=true;return {stopping:true};}
  if(method==="jobs.get")return stopped?{id:"scroll-one",state:"completed",result:{draftId:"a".repeat(32)}}:{id:"scroll-one",state:"running"};
  throw Error(method);
 });
 const opened=vi.fn(async()=>{});
 render(<CapturePage onOpenDraft={opened}/>);
 const user=userEvent.setup();
 await user.click(await screen.findByRole("button",{name:"滚动截图"}));
 await user.click(await screen.findByRole("button",{name:"停止"}));
 await waitFor(()=>expect(opened).toHaveBeenCalledExactlyOnceWith("a".repeat(32)));
 expect(screen.queryByRole("button",{name:"停止"})).toBeNull();
});

it("reports a window restoration failure after capture",async()=>{
 const settings={schemaVersion:1,revision:0,outputDirectory:"C:\\Images",copyToClipboard:false,askWhereToSave:false,openEditor:false,pinAfterCapture:false,theme:"system",scrollDelay:300,scrollFromTop:false};
 native.call.mockImplementation(async(method:string)=>method==="snapshot"?{settings,busy:false,pins:0}:{state:"completed"});
 native.reveal.mockRejectedValueOnce(Error("Window restore failed"));
 render(<CapturePage/>);
 await userEvent.setup().click(await screen.findByRole("button",{name:"窗口截图"}));
 expect((await screen.findByRole("alert")).textContent).toContain("Window restore failed");
 expect(screen.queryByText("操作完成")).toBeNull();
});

it("quick-save mode clears the save-location prompt and persists through the shared settings path",async()=>{
 let settings={schemaVersion:1,revision:0,outputDirectory:"C:\\Images",copyToClipboard:true,askWhereToSave:true,openEditor:true,pinAfterCapture:false,theme:"system",scrollDelay:300,scrollFromTop:false};
 native.call.mockImplementation(async(method:string,payload?:typeof settings)=>{
  if(method==="settings.get")return settings;
  if(method==="settings.save"){settings={...payload!,revision:1};return settings;}
  throw Error(method);
 });
 render(<CapturePage disabled/>);const user=userEvent.setup();
 await user.click(await screen.findByRole("radio",{name:"截完保存"}));
 expect(screen.queryByRole("checkbox",{name:"每次选择保存位置"})).toBeNull();
 await user.click(screen.getByRole("button",{name:"保存设置"}));
 await screen.findByText("已保存");
 expect(settings.openEditor).toBe(false);expect(settings.askWhereToSave).toBe(false);
});

it("hands a unified capture draft to the shared workspace",async()=>{
  const id="a".repeat(32),open=vi.fn(async()=>{});
  const settings={schemaVersion:1,revision:0,outputDirectory:"C:\\Images",copyToClipboard:false,askWhereToSave:false,openEditor:true,pinAfterCapture:false,theme:"system",scrollDelay:300,scrollFromTop:false};
  native.call.mockImplementation(async(method:string)=>{
    if(method==="snapshot")return {settings,busy:false,pins:0};
    if(method==="capture.unified")return {state:"completed",result:{draftId:id}};
    throw Error(method);
  });
  render(<CapturePage onOpenDraft={open}/>);
  await userEvent.setup().click(await screen.findByRole("button",{name:"区域截图"}));
  await screen.findByText("操作完成");
  expect(open).toHaveBeenCalledWith(id);
  expect(native.call).toHaveBeenCalledWith("capture.unified",{mode:"region"});
});

it("saves capture preferences through the English controls",async()=>{
  await setApplicationLanguage("en-US");
  const settings={schemaVersion:1,revision:0,outputDirectory:"C:\\Images",copyToClipboard:true,askWhereToSave:false,openEditor:false,pinAfterCapture:false,theme:"system",scrollDelay:300,scrollFromTop:false};
  native.call.mockImplementation(async(method:string,payload?:unknown)=>{
    if(method==="settings.get")return settings;
    if(method==="settings.save")return {...payload as object,revision:1};
    throw new Error(method);
  });
  render(<CapturePage disabled/>);
  const user=userEvent.setup();
  await user.click(await screen.findByRole("radio",{name:"Edit before saving"}));
  await user.click(screen.getByRole("button",{name:"Save settings"}));
  expect(await screen.findByText("Saved")).toBeTruthy();
});

it("does not reload the worker after cancelling Save as",async()=>{
  const settings={schemaVersion:1,revision:0,outputDirectory:"C:\\Images",copyToClipboard:false,askWhereToSave:false,openEditor:false,pinAfterCapture:false,theme:"system",scrollDelay:300,scrollFromTop:false};
  native.call.mockImplementation(async(method:string)=>{
    if(method==="snapshot")return {settings,lastCapture:{id:"image-1",path:"C:\\Images\\one.png",width:10,height:10,copied:false,pinned:false},busy:false,pins:0};
    if(method==="save.last")return {state:"cancelled"};
    throw new Error(method);
  });
  render(<CapturePage/>);
  await userEvent.setup().click(await screen.findByRole("button",{name:"另存为"}));
  await screen.findByText("已取消");
  expect(native.call.mock.calls.filter(([method])=>method==="snapshot")).toHaveLength(1);
  expect(native.call).toHaveBeenCalledWith("save.last",{expectedId:"image-1"});
});

it("saves disabled-module preferences without starting capture", async () => {
  const settings = { schemaVersion: 1, revision: 0, outputDirectory: "C:\\Images", copyToClipboard: true,
    askWhereToSave: false, openEditor: false, pinAfterCapture: false, theme: "system", scrollDelay: 300, scrollFromTop: false };
  native.call.mockImplementation(async (method: string, payload?: unknown) => {
    if (method === "settings.get") return settings;
    if (method === "settings.save") return { ...(payload as object), revision: 1 };
    throw new Error(`unexpected activation: ${method}`);
  });
  render(<CapturePage disabled />);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("radio", { name: "截图后编辑" }));
  await user.click(screen.getByRole("button", { name: "保存设置" }));
  expect(await screen.findByText("已保存")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "区域截图" })).toBeNull();
  expect(native.call.mock.calls.every(([method]) => ["settings.get", "settings.save"].includes(method))).toBe(true);
});

it("shows the saved image when clipboard output fails", async () => {
  let failed = false;
  native.call.mockImplementation(async (method: string) => {
    if (method === "snapshot")
      return {
        settings: {
          schemaVersion: 1,
          revision: 0,
          outputDirectory: "C:\\Images",
          copyToClipboard: true,
          askWhereToSave: false,
          openEditor: false,
          pinAfterCapture: false,
          theme: "system",
          scrollDelay: 300,
          scrollFromTop: false,
        },
        lastCapture: failed
          ? {
              path: "C:\\Images\\saved.png",
              width: 32,
              height: 24,
              copied: false,
              pinned: false,
            }
          : null,
        busy: false,
        pins: 0,
      };
    if (method === "capture.start") {
      failed = true;
      return { id: "job", state: "failed", error: "截图已保存，复制失败" };
    }
    throw new Error(method);
  });
  render(<CapturePage />);
  await userEvent
    .setup()
    .click(await screen.findByRole("button", { name: "区域截图" }));
  await screen.findByText("C:\\Images\\saved.png");
  expect(screen.getByRole("button", { name: "复制" })).toBeTruthy();
});
