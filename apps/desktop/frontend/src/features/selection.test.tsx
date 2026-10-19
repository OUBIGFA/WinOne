// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,fireEvent,render,screen} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {SelectionPage} from "./selection";
import {setApplicationLanguage} from "../i18n";
const call=vi.hoisted(()=>vi.fn());
const logs=vi.hoisted(()=>vi.fn(async()=>({folderAvailable:false,entries:[]})));
vi.mock("../bridge",()=>({selectionCall:call,captureCall:vi.fn(async()=>({providers:[{id:"selected-provider",name:"工作服务",models:["fast","precise"],availableModels:["disabled","fast","precise"]}],tasks:{}})),readModuleLogs:logs,openModuleLogFolder:vi.fn()}));
afterEach(async()=>{cleanup();call.mockReset();logs.mockClear();await setApplicationLanguage("zh-CN");});
const preferences={schemaVersion:1,revision:3,automatic:true,keyboard:true,excludedApps:[]};
it("saves exclusions and activation preferences through the shared module contract",async()=>{
  call.mockImplementation(async(method:string,payload?:object)=>method==="settings.save"?{...payload,revision:4}:preferences);
  render(<SelectionPage/>);await userEvent.setup().click(screen.getByRole("tab",{name:"常规"}));const input=await screen.findByRole("textbox",{name:"排除应用"});
  fireEvent.change(input,{target:{value:"notepad.exe\n Code.exe "}});
  await userEvent.setup().click(screen.getByRole("switch",{name:"划词后显示工具栏"}));
  await userEvent.setup().click(screen.getByRole("button",{name:"保存设置"}));
  await screen.findByText("已保存");
  expect(call).toHaveBeenCalledWith("settings.save",{...preferences,automatic:false,excludedApps:["notepad.exe","Code.exe"]});
});
it("keeps failure feedback without an independent log tab",async()=>{
  call.mockRejectedValue(Error("settings unavailable"));render(<SelectionPage/>);await screen.findByRole("alert");
  expect(screen.queryByRole("tab",{name:"诊断日志"})).toBeNull();
  expect(logs).not.toHaveBeenCalled();
});
it("adds a custom AI instruction and preserves it when switching settings tabs",async()=>{
  call.mockImplementation(async(method:string,payload?:object)=>method==="settings.save"?{...payload,revision:4}:{...preferences,actions:[{id:"copy",name:"复制",icon:"copy",kind:"copy",enabled:true}]});
  render(<SelectionPage/>);await screen.findByRole("button",{name:"添加自定义功能"});const user=userEvent.setup();
  await user.click(screen.getByRole("tab",{name:"工具栏"}));await user.click(screen.getByRole("button",{name:"添加自定义功能"}));
  const name=screen.getByLabelText("功能名称");expect(document.activeElement).toBe(name);fireEvent.change(name,{target:{value:"邮件回复"}});
  fireEvent.change(screen.getByLabelText("AI 指令"),{target:{value:"根据选中文字写一封简短的邮件回复。"}});
  await user.click(screen.getByRole("tab",{name:"常规"}));await user.click(screen.getByRole("tab",{name:"工具栏"}));
  expect((screen.getByLabelText("功能名称") as HTMLInputElement).value).toBe("邮件回复");
  await user.click(screen.getByRole("combobox",{name:"邮件回复 · 供应商"}));await user.click(await screen.findByRole("option",{name:"工作服务"}));
  await user.click(screen.getByRole("combobox",{name:"邮件回复 · 模型名称"}));expect(screen.queryByRole("option",{name:"disabled"})).toBeNull();await user.click(await screen.findByRole("option",{name:"precise"}));
  await user.click(screen.getByRole("button",{name:"保存设置"}));await screen.findByText("已保存");
  expect(call).toHaveBeenCalledWith("settings.save",expect.objectContaining({actions:expect.arrayContaining([expect.objectContaining({name:"邮件回复",kind:"ai",selection:{provider:"selected-provider",model:"precise"},prompt:"根据选中文字写一封简短的邮件回复。"})])}));
});
it("shows distinct action rows and preserves edits when another action is opened",async()=>{
  call.mockResolvedValue({...preferences,actions:[{id:"copy",name:"复制",icon:"copy",kind:"copy",enabled:true},{id:"explain",name:"解释",icon:"explain",kind:"ai",prompt:"解释内容",enabled:true}]});
  const user=userEvent.setup();render(<SelectionPage/>);await screen.findByRole("button",{name:"添加自定义功能"});
  await user.click(screen.getByRole("tab",{name:"工具栏"}));
  expect(screen.queryByLabelText("功能名称")).toBeNull();
  await user.click(screen.getByRole("button",{name:"编辑 解释"}));
  fireEvent.change(screen.getByLabelText("AI 指令"),{target:{value:"保留这条指令"}});
  await user.click(screen.getByRole("button",{name:"编辑 复制"}));
  expect(screen.queryByLabelText("AI 指令")).toBeNull();
  await user.click(screen.getByRole("button",{name:"编辑 解释"}));
  expect((screen.getByLabelText("AI 指令") as HTMLTextAreaElement).value).toBe("保留这条指令");
  expect(screen.getByRole("button",{name:"编辑 解释"}).getAttribute("aria-expanded")).toBe("true");
});
