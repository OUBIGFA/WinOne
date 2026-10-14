// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AlwaysOnTopPage } from "./always-on-top";
import { setApplicationLanguage } from "../i18n";

const call = vi.hoisted(() => vi.fn());
const logs=vi.hoisted(()=>vi.fn(async()=>({folderAvailable:false,entries:[]})));
vi.mock("../bridge", () => ({ alwaysOnTopCall: call,readModuleLogs:logs,openModuleLogFolder:vi.fn() }));
afterEach(async () => { cleanup(); call.mockReset(); logs.mockClear(); await setApplicationLanguage("zh-CN"); });
const settings = {
  schemaVersion: 1, revision: 0, enabled: true, frame: true, frameThickness: 3,
  frameOpacity: 100, frameColor: "#808080", useAccent: true, roundCorners: true,
  blockInGameMode: true, sound: false, excludedApps: [],
};
it("lets a custom color replace the accent color without discarding other edits",async()=>{
 call.mockImplementation(async(method:string,payload?:unknown)=>method==="settings.save"?{...(payload as object),revision:1}:settings);
 render(<AlwaysOnTopPage disabled/>);
 const input=await screen.findByLabelText("自定义边框颜色") as HTMLInputElement;
 expect(input.disabled).toBe(false);
 const user=userEvent.setup();await user.click(screen.getByRole("switch",{name:"播放提示音"}));
 fireEvent.change(input,{target:{value:"#123456"}});
 expect(screen.getByRole("switch",{name:"使用 Windows 强调色"}).getAttribute("aria-checked")).toBe("false");
 await user.click(screen.getByRole("button",{name:"保存设置"}));
 await screen.findByText("已保存");
 expect(input.value).toBe("#123456");
 expect(screen.getByRole("switch",{name:"播放提示音"}).getAttribute("aria-checked")).toBe("true");
 expect(call).toHaveBeenCalledWith("settings.save",expect.objectContaining({frameColor:"#123456",useAccent:false,sound:true}));
});
it("keeps pinning failure feedback without an independent log tab",async()=>{
 call.mockRejectedValue(Error("pinning unavailable"));
 render(<AlwaysOnTopPage/>);await screen.findByRole("alert");
 expect(screen.queryByRole("tab",{name:"诊断日志"})).toBeNull();
 expect(logs).not.toHaveBeenCalled();
 expect(screen.queryByRole("switch",{name:"显示置顶边框"})).toBeNull();
});
it("offers common and advanced tabs while retaining the draft",async()=>{
 call.mockResolvedValue(settings);render(<AlwaysOnTopPage disabled/>);
 const user=userEvent.setup();await screen.findByRole("switch",{name:"播放提示音"});
 expect(screen.getAllByRole("tab").map(tab=>tab.textContent)).toEqual(["常规设置","高级设置"]);
 await user.click(screen.getByRole("tab",{name:"高级设置"}));
 const text=screen.getByRole("textbox");await user.type(text,"example.exe");
 await user.click(screen.getByRole("tab",{name:"常规设置"}));
 expect(logs).not.toHaveBeenCalled();
 await user.click(screen.getByRole("tab",{name:"高级设置"}));
 expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe("example.exe");
});
it("retries failed loading without leaving the settings page",async()=>{
 call.mockRejectedValueOnce(Error("暂时无法连接")).mockResolvedValue(settings);
 render(<AlwaysOnTopPage disabled/>);const user=userEvent.setup();
 await screen.findByRole("alert");await user.click(screen.getByRole("button",{name:"重试"}));
 await screen.findByRole("switch",{name:"播放提示音"});
 expect(screen.queryByRole("alert")).toBeNull();expect(call).toHaveBeenCalledTimes(2);
});
it("clears unsaved status when changes are restored to their saved values",async()=>{
 call.mockResolvedValue(settings);const activity=vi.fn();render(<AlwaysOnTopPage disabled onActivity={activity}/>);const user=userEvent.setup();
 const sound=await screen.findByRole("switch",{name:"播放提示音"});
 await user.click(sound);await user.click(sound);
 expect(screen.getByRole("button",{name:"保存设置"}).hasAttribute("disabled")).toBe(true);
 expect(activity).toHaveBeenLastCalledWith(false,false,"page");
});
it("uses the last successful save as the baseline for further edits",async()=>{
 call.mockImplementation(async(method:string,payload?:unknown)=>method==="settings.get"?settings:{...payload as object,revision:1});
 render(<AlwaysOnTopPage disabled/>);const user=userEvent.setup();
 const sound=await screen.findByRole("switch",{name:"播放提示音"});await user.click(sound);
 await user.click(screen.getByRole("button",{name:"保存设置"}));await screen.findByText("已保存");
 await user.click(sound);expect(screen.getByRole("button",{name:"保存设置"}).hasAttribute("disabled")).toBe(false);
 await user.click(sound);expect(screen.getByRole("button",{name:"保存设置"}).hasAttribute("disabled")).toBe(true);
 await user.click(screen.getByRole("tab",{name:"高级设置"}));
 const exclusions=screen.getByRole("textbox",{name:"排除的程序（每行一个名称或路径）"});
 await user.type(exclusions,"keep.exe");expect(screen.getByRole("button",{name:"保存设置"}).hasAttribute("disabled")).toBe(false);
 await user.clear(exclusions);expect(screen.getByRole("button",{name:"保存设置"}).hasAttribute("disabled")).toBe(true);
});
it("keeps an empty number editable and blocks invalid settings from being saved",async()=>{
 call.mockResolvedValue(settings);render(<AlwaysOnTopPage disabled/>);const user=userEvent.setup();
 const input=await screen.findByRole("spinbutton",{name:"边框粗细（1–15 像素）"});
  await user.clear(input);
  expect((input as HTMLInputElement).value).toBe("");
  expect(input.getAttribute("aria-invalid")).toBe("true");
  expect(document.getElementById(input.getAttribute("aria-describedby")!)?.textContent).toContain("1–15");
 expect((screen.getByRole("button",{name:"保存设置"}) as HTMLButtonElement).disabled).toBe(true);
 await user.type(input,"4");
 expect((screen.getByRole("button",{name:"保存设置"}) as HTMLButtonElement).disabled).toBe(false);
 expect(call).toHaveBeenCalledExactlyOnceWith("settings.get");
});
it("keeps advanced drafts while switching tabs",async()=>{
 call.mockResolvedValue(settings);render(<AlwaysOnTopPage disabled/>);const user=userEvent.setup();
 await user.click(await screen.findByRole("tab",{name:"高级设置"}));
 await user.type(screen.getByRole("textbox",{name:"排除的程序（每行一个名称或路径）"}),"keep.exe");
 await user.click(screen.getByRole("tab",{name:"常规设置"}));
 // Base UI marks the outgoing panel inert, then unmounts it after its exit.
 await waitFor(()=>expect(screen.queryByRole("textbox",{name:"排除的程序（每行一个名称或路径）"})).toBeNull());
 await user.click(screen.getByRole("tab",{name:"高级设置"}));
 expect((screen.getByRole("textbox",{name:"排除的程序（每行一个名称或路径）"}) as HTMLTextAreaElement).value).toBe("keep.exe");
});
it("edits disabled-module preferences without requesting window actions", async () => {
  call.mockImplementation(async (method: string, payload?: unknown) => {
    if (method === "settings.get") return settings;
    if (method === "settings.save") return { ...(payload as object), revision: 1 };
    throw new Error(`unexpected activation: ${method}`);
  });
  render(<AlwaysOnTopPage disabled />);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("tab",{name:"高级设置"}));
  const input = await screen.findByRole("textbox", { name: "排除的程序（每行一个名称或路径）" });
  await user.type(input, "game.exe");
  await user.click(screen.getByRole("button", { name: "保存设置" }));
  expect(await screen.findByText("已保存")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "启用管理员置顶" })).toBeNull();
  expect(screen.queryByRole("button", { name: "刷新窗口" })).toBeNull();
  expect(call.mock.calls.every(([method]) => ["settings.get", "settings.save"].includes(method))).toBe(true);
});
it("keeps permission controls in global settings",async()=>{
 call.mockResolvedValue({settings,administrator:true});render(<AlwaysOnTopPage/>);
 await userEvent.setup().click(await screen.findByRole("tab",{name:"高级设置"}));
 expect(screen.queryByRole("button",{name:/管理员|普通模式/})).toBeNull();
 expect(screen.getByRole("textbox",{name:"排除的程序（每行一个名称或路径）"})).toBeTruthy();
});
it("does not list or offer per-window pin actions",async()=>{
 call.mockResolvedValue({settings,windows:[{title:"private window",pinned:false}],administrator:false});
 render(<AlwaysOnTopPage/>);
 await screen.findByRole("button",{name:"保存设置"});
 expect(screen.queryByText("private window")).toBeNull();expect(screen.queryByRole("button",{name:"刷新窗口"})).toBeNull();expect(screen.queryByRole("textbox",{name:"查找窗口"})).toBeNull();
});
it("keeps unsaved settings when refreshing windows after a save conflict", async () => {
  call.mockImplementation(async (method: string) => {
    if (method === "snapshot") return { settings, windows: [] };
    throw new Error("置顶设置已变更，请刷新后重试");
  });
  render(<AlwaysOnTopPage />);
  const user = userEvent.setup();

  await user.click(await screen.findByRole("tab",{name:"高级设置"}));
  const input = await screen.findByRole("textbox", { name: "排除的程序（每行一个名称或路径）" });
  await user.type(input, "keep.exe");
  await user.click(screen.getByRole("button", { name: "保存设置" }));
  await screen.findByRole("alert");

  expect((input as HTMLTextAreaElement).value).toBe("keep.exe");
});

it("saves disabled-module settings through the English controls",async()=>{
  await setApplicationLanguage("en-US");
  call.mockImplementation(async(method:string,payload?:unknown)=>{
    if(method==="settings.get")return settings;
    if(method==="settings.save")return {...payload as object,revision:1};
    throw new Error(method);
  });
  render(<AlwaysOnTopPage disabled/>);
  const user=userEvent.setup();
  await user.click(await screen.findByRole("switch",{name:"Play a sound"}));
  await user.click(screen.getByRole("button",{name:"Save settings"}));
  expect(await screen.findByText("Saved")).toBeTruthy();
});
