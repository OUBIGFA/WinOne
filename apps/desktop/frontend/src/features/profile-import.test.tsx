// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { usePageNavigation } from "../ui/page-navigation";
import userEvent from "@testing-library/user-event";
import { ProfileImport } from "./profile-import";
import { setApplicationLanguage } from "../i18n";
const native=vi.hoisted(()=>({pick:vi.fn(),location:vi.fn(),call:vi.fn()}));
vi.mock("../bridge",()=>({pickProfile:native.pick,pickProfileLocation:native.location,profileCall:native.call}));
afterEach(async()=>{cleanup();vi.resetAllMocks();await setApplicationLanguage("zh-CN");});
function preview(mapped=false){return {name:"家用配置",sourceHash:"reviewed-hash",configurationRevision:4,moduleRevision:2,
  preferences:{theme:"dark",language:"zh-CN",runAtLogon:false,startMinimized:true,enabled:{capture:false},shortcuts:{"capture.region":"Ctrl+Alt+S"}},
  currentShortcuts:{"capture.region":"Ctrl+Alt+S","always-on-top.toggle":"Ctrl+Alt+T"},
  plan:{modules:[{id:"capture",state:mapped?"ready":"needs_mapping",requiredPaths:["capture.output"],missingPaths:mapped?[]:["capture.output"],argumentApps:[],settings:{openEditor:true,scrollDelay:300}}],shortcutIssues:[]},
  paths:[{id:"capture.output",label:"截图保存目录",kind:"directory",state:mapped?"ready":"needs_mapping",value:mapped?"D:\\Images":""}]};}

function ImportNavigation(){
 const [page,setPage]=useState("settings");const navigation=usePageNavigation(page,setPage);
 return <><button onClick={()=>navigation.requestPage("home")}>首页</button>{page==="settings"?<ProfileImport onApplied={()=>{}} onActivity={navigation.onActivity}/>:<p>工具首页</p>}{navigation.dialog}</>;
}
it("shows the enabled-after-import state only for selected modules",async()=>{
 native.pick.mockResolvedValue("C:\\Home.winone");native.call.mockResolvedValue(preview(true));
 render(<ProfileImport onApplied={()=>{}}/>);const user=userEvent.setup();
 await user.click(screen.getByRole("button",{name:"导入配置"}));await screen.findByText(/导入后关闭/);
 await user.click(screen.getByRole("checkbox",{name:"截图"}));
 expect(screen.queryByText(/导入后关闭/)).toBeNull();
 await user.click(screen.getByRole("checkbox",{name:"截图"}));
 expect(screen.getByText(/导入后关闭/)).toBeTruthy();
});
it("keeps the current review and mappings when a replacement archive cannot be read",async()=>{
 native.pick.mockResolvedValueOnce("C:\\Home.winone").mockResolvedValueOnce("C:\\Broken.winone");
 native.location.mockResolvedValue("D:\\Images");
 native.call.mockImplementation(async(method:string,args:{path:string;bindings:Record<string,string>})=>{
  if(args.path==="C:\\Broken.winone")throw Error("Unreadable archive");
  if(method==="import.preview")return preview(!!args.bindings["capture.output"]);
  return {revision:5};
 });
 render(<ProfileImport onApplied={()=>{}}/>);const user=userEvent.setup();
 await user.click(screen.getByRole("button",{name:"导入配置"}));
 await user.click(await screen.findByRole("button",{name:"设置本机路径"}));
 await user.click(screen.getByRole("button",{name:"选择截图保存目录"}));
 await screen.findByText("D:\\Images");
 await user.click(screen.getByRole("tab",{name:"导入内容"}));
 await user.click(screen.getByRole("checkbox",{name:"主题、语言和登录设置"}));
 await user.click(screen.getByRole("button",{name:"导入配置"}));
 await screen.findByText(/Unreadable archive/);
 expect(screen.getByText("家用配置")).toBeTruthy();
 expect(screen.getByText("C:\\Home.winone")).toBeTruthy();
 expect(screen.getByRole("checkbox",{name:"主题、语言和登录设置"}).getAttribute("aria-checked")).toBe("false");
 expect((screen.getByRole("button",{name:"确认导入"}) as HTMLButtonElement).disabled).toBe(true);
 await user.click(screen.getByRole("button",{name:"重新预览"}));
 await waitFor(()=>expect((screen.getByRole("button",{name:"确认导入"}) as HTMLButtonElement).disabled).toBe(false));
 expect(native.call.mock.lastCall?.[1]).toEqual({path:"C:\\Home.winone",bindings:{"capture.output":"D:\\Images"}});
});
it("preserves reviewed choices when the user cancels leaving",async()=>{
 native.pick.mockResolvedValue("C:\\Home.winone");native.call.mockResolvedValue(preview(true));
 render(<ImportNavigation/>);const user=userEvent.setup();
 await user.click(screen.getByRole("button",{name:"导入配置"}));await screen.findByText("家用配置");
 await user.click(screen.getByRole("checkbox",{name:"截图"}));
 await user.click(screen.getByRole("button",{name:"首页"}));
 await user.click(await screen.findByRole("button",{name:"继续编辑"}));
 expect(screen.getByRole("checkbox",{name:"截图"}).getAttribute("aria-checked")).toBe("false");
 await user.click(screen.getByRole("button",{name:"首页"}));
 await user.click(await screen.findByRole("button",{name:"放弃并离开"}));
 expect(await screen.findByText("工具首页")).toBeTruthy();
 expect(native.call.mock.calls.every(([method])=>method==="import.preview")).toBe(true);
});
it("waits for import to complete before finishing a requested navigation",async()=>{
 native.pick.mockResolvedValue("C:\\Home.winone");let finish!:(value:unknown)=>void;
 native.call.mockImplementation(async(method:string)=>method==="import.preview"?preview(true):new Promise(resolve=>{finish=resolve;}));
 render(<ImportNavigation/>);const user=userEvent.setup();
 await user.click(screen.getByRole("button",{name:"导入配置"}));await screen.findByText("家用配置");
 await user.click(screen.getByRole("button",{name:"确认导入"}));await user.click(screen.getByRole("button",{name:"首页"}));
 expect((await screen.findByRole("dialog")).textContent).toContain("操作尚未完成");
 expect(screen.queryByRole("button",{name:"放弃并离开"})).toBeNull();
 finish({revision:5});await screen.findByText("工具首页");
 await waitFor(()=>expect(screen.queryByRole("dialog")).toBeNull());
});
it("keeps the review after a pending import fails",async()=>{
 native.pick.mockResolvedValue("C:\\Home.winone");let fail!:(reason:Error)=>void;
 native.call.mockImplementation(async(method:string)=>method==="import.preview"?preview(true):new Promise((_,reject)=>{fail=reject;}));
 render(<ImportNavigation/>);const user=userEvent.setup();
 await user.click(screen.getByRole("button",{name:"导入配置"}));await screen.findByText("家用配置");
 await user.click(screen.getByRole("button",{name:"确认导入"}));await user.click(screen.getByRole("button",{name:"首页"}));
 await screen.findByRole("dialog");fail(Error("配置已变化，请重新核对"));
 await user.click(await screen.findByRole("button",{name:"继续编辑"}));
 expect(screen.getByText("家用配置")).toBeTruthy();
 expect(screen.getByRole("alert").textContent).toContain("配置已变化，请重新核对");
 expect(screen.getByRole("button",{name:"确认导入"}).hasAttribute("disabled")).toBe(true);
 expect(screen.queryByText("工具首页")).toBeNull();
});
it("leaves freely after the import review is cancelled",async()=>{
 native.pick.mockResolvedValue("C:\\Home.winone");native.call.mockResolvedValue(preview(true));
 render(<ImportNavigation/>);const user=userEvent.setup();
 await user.click(screen.getByRole("button",{name:"导入配置"}));await screen.findByText("家用配置");
 await user.click(screen.getByRole("button",{name:"取消"}));await user.click(screen.getByRole("button",{name:"首页"}));
 expect(screen.queryByRole("dialog")).toBeNull();expect(await screen.findByText("工具首页")).toBeTruthy();
});
it("requires a mapped location and explicit confirmation before applying",async()=>{
  native.pick.mockResolvedValue("C:\\Backup\\Home.winone");native.location.mockResolvedValue("D:\\Images");
  let finish!:(value:unknown)=>void;
  native.call.mockImplementation(async(method:string,args:{bindings:Record<string,string>})=>{
    if(method==="import.preview")return preview(!!args.bindings["capture.output"]);
    return new Promise(resolve=>{finish=resolve;});
  });
  const changed=vi.fn();render(<ProfileImport onApplied={changed}/>);
  const user=userEvent.setup();await user.click(screen.getByRole("button",{name:"导入配置"}));
  await screen.findByText("家用配置");
  expect((screen.getByRole("button",{name:"确认导入"}) as HTMLButtonElement).disabled).toBe(true);
  await user.click(screen.getByRole("button",{name:"设置本机路径"}));
  await user.click(screen.getByRole("button",{name:"选择截图保存目录"}));
  await screen.findByText("D:\\Images");
  expect(native.call.mock.calls.some(([method])=>method==="import.apply")).toBe(false);
  await user.click(screen.getByRole("button",{name:"确认导入"}));
  expect(changed).not.toHaveBeenCalled();finish({revision:5});
  await screen.findByText("配置已导入");
  expect(changed).toHaveBeenCalledOnce();
  expect(native.call).toHaveBeenCalledWith("import.apply",expect.objectContaining({sourceHash:"reviewed-hash",configurationRevision:4,moduleRevision:2,modules:["capture"],bindings:{"capture.output":"D:\\Images"}}));
});

it("keeps module selection while switching between review tabs",async()=>{
 native.pick.mockResolvedValue("C:\\Home.winone");native.call.mockResolvedValue(preview(true));
 render(<ProfileImport onApplied={()=>{}}/>);const user=userEvent.setup();
 await user.click(screen.getByRole("button",{name:"导入配置"}));await screen.findByText("家用配置");
 await user.click(screen.getByRole("tab",{name:"详细设置"}));
 expect(screen.getByRole("combobox",{name:"模块"})).toBeTruthy();expect(screen.getByText("300")).toBeTruthy();
 await user.click(screen.getByRole("tab",{name:"导入内容"}));
 await user.click(screen.getByRole("checkbox",{name:"截图"}));
 await user.click(screen.getByRole("tab",{name:"详细设置"}));
 expect(screen.getByText("所选内容没有可预览的模块设置。")).toBeTruthy();
 await user.click(screen.getByRole("tab",{name:"本机路径"}));
 expect(screen.getByText("所选内容无需指定本机路径。")).toBeTruthy();
 expect(native.call.mock.calls.every(([method])=>method==="import.preview")).toBe(true);
});
it("requires a fresh review after a failed apply",async()=>{
  native.pick.mockResolvedValue("C:\\Home.winone");
  native.call.mockImplementation(async(method:string)=>{if(method==="import.preview")return preview(true);throw new Error("文件已变化，请重新预览");});
  render(<ProfileImport onApplied={()=>{}}/>);
  const user=userEvent.setup();await user.click(screen.getByRole("button",{name:"导入配置"}));
  await screen.findByText("家用配置");await user.click(screen.getByRole("button",{name:"确认导入"}));
  await screen.findByRole("alert");
  expect((screen.getByRole("button",{name:"确认导入"}) as HTMLButtonElement).disabled).toBe(true);
  expect(screen.queryByText("配置已导入")).toBeNull();
});
it("checks imported shortcuts against settings kept on this computer",async()=>{
  native.pick.mockResolvedValue("C:\\Home.winone");
  const value=preview(true);value.preferences.shortcuts["capture.region"]="alt+ctrl+t";
  native.call.mockResolvedValue(value);
  render(<ProfileImport onApplied={()=>{}}/>);
  const user=userEvent.setup();await user.click(screen.getByRole("button",{name:"导入配置"}));
  await screen.findByText("截图与窗口置顶使用了相同快捷键");
  expect((screen.getByRole("button",{name:"确认导入"}) as HTMLButtonElement).disabled).toBe(true);
  await user.click(screen.getByRole("checkbox",{name:"截图"}));
  expect(screen.queryByText("截图与窗口置顶使用了相同快捷键")).toBeNull();
});

it("confirms the reviewed profile using English labels",async()=>{
  await setApplicationLanguage("en-US");
  native.pick.mockResolvedValue("C:\\Home.winone");
  native.call.mockImplementation(async(method:string)=>method==="import.preview"?preview(true):{revision:5});
  render(<ProfileImport onApplied={()=>{}}/>);
  const user=userEvent.setup();
  await user.click(screen.getByRole("button",{name:"Import configuration"}));
  await screen.findByRole("checkbox",{name:"Capture"});
  await user.click(screen.getByRole("button",{name:"Confirm import"}));
  expect(await screen.findByText("Configuration imported")).toBeTruthy();
  expect(native.call).toHaveBeenCalledWith("import.apply",expect.objectContaining({sourceHash:"reviewed-hash",configurationRevision:4}));
});
