// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Shortcuts } from "./shortcuts";
const api=vi.hoisted(()=>({getSnapshot:vi.fn(),getShortcuts:vi.fn(),saveSettings:vi.fn(),shortcutRecordingPort:{begin:vi.fn(async()=>"token"),end:vi.fn(async()=>{})}}));
vi.mock("../bridge",()=>api);
afterEach(()=>{cleanup();vi.clearAllMocks();});
it("edits and clears the global open shortcut while displaying its conflict",async()=>{
 api.getSnapshot.mockResolvedValue({settings:{schemaVersion:1,revision:0,theme:"system",openShortcut:"Alt+A"}});
 api.getShortcuts.mockResolvedValue({bindings:[{id:"open-winone",registered:false,shortcut:"Alt+A",error:"快捷键被占用"}],lastError:""});
 api.saveSettings.mockImplementation(async value=>({...value,revision:1}));
 render(<Shortcuts moduleId="application" onSaved={()=>{}}/>);const user=userEvent.setup();
 const input=await screen.findByRole("textbox",{name:"打开 WinOne"});
 expect((input as HTMLInputElement).value).toBe("Alt+A");await screen.findByText("快捷键被占用");
 await user.click(input);await screen.findByText("请按下组合键，Esc 取消，Delete 清除。");await user.keyboard('{Delete}');
 await user.click(screen.getByRole("button",{name:"保存快捷键"}));
 await screen.findByText("已保存");
 expect(api.saveSettings).toHaveBeenCalledWith(expect.objectContaining({openShortcut:""}));
});
it("does not mark recording the saved shortcut as an unsaved change",async()=>{
 api.getSnapshot.mockResolvedValue({settings:{schemaVersion:1,revision:0,theme:"system",captureShortcut:"Ctrl+Alt+S"}});api.getShortcuts.mockResolvedValue({bindings:[]});
 render(<Shortcuts moduleId="capture" onSaved={()=>{}}/>);const user=userEvent.setup();
 await user.click(await screen.findByRole("textbox",{name:"开始截图"}));await screen.findByText("请按下组合键，Esc 取消，Delete 清除。");
 await user.keyboard('{Control>}{Alt>}s{/Alt}{/Control}');
 expect(screen.getByRole("button",{name:"保存快捷键"}).hasAttribute("disabled")).toBe(true);
 expect(screen.queryByRole("button",{name:"撤销修改"})).toBeNull();
});
it("reports persisted settings even if refreshing registration status fails",async()=>{
 api.getSnapshot.mockResolvedValue({settings:{schemaVersion:1,revision:0,theme:"system",captureShortcut:"Ctrl+Alt+S"}});api.getShortcuts.mockResolvedValue({bindings:[]});
 api.saveSettings.mockImplementation(async value=>{api.getShortcuts.mockRejectedValue(Error("无法读取快捷键状态"));return {...value,revision:1};});
 const saved=vi.fn();render(<Shortcuts moduleId="capture" onSaved={saved}/>);const user=userEvent.setup();
 await user.click(await screen.findByRole("textbox",{name:"开始截图"}));await screen.findByText("请按下组合键，Esc 取消，Delete 清除。");
 await user.keyboard('{Control>}{Shift>}{F9}{/Shift}{/Control}');
 await user.click(screen.getByRole("button",{name:"保存快捷键"}));await screen.findByRole("alert");
 expect(saved).toHaveBeenCalledOnce();expect(screen.getByText("已保存")).toBeTruthy();
 expect(screen.getAllByText("暂时无法确认是否生效")).toHaveLength(3);
 expect(screen.getByRole("button",{name:"保存快捷键"}).hasAttribute("disabled")).toBe(true);
});
it("shows registration conflicts and keeps the draft on persistence failure",async()=>{
 api.getSnapshot.mockResolvedValue({settings:{schemaVersion:1,revision:0,theme:"system",captureShortcut:"Ctrl+Alt+S",pinShortcut:"Ctrl+Alt+T"}});
 api.getShortcuts.mockResolvedValue({bindings:[{id:"capture",registered:false,shortcut:"Ctrl+Alt+S",error:"快捷键被占用"}],lastError:""});
 api.saveSettings.mockRejectedValue(new Error("设置已变更"));
 render(<Shortcuts moduleId="capture" onSaved={()=>{}}/>);
 await screen.findByText("快捷键被占用");
 const user=userEvent.setup();const input=screen.getByRole("textbox",{name:"开始截图"});
 await user.click(input);await screen.findByText("请按下组合键，Esc 取消，Delete 清除。");await user.keyboard('{Control>}{Shift>}{F9}{/Shift}{/Control}');
 await user.click(screen.getByRole("button",{name:"保存快捷键"}));
 await screen.findByRole("alert");
 expect((input as HTMLInputElement).value).toBe("Ctrl+Shift+F9");
});

it("shows only the selected module's binding and action failure",async()=>{
 api.getSnapshot.mockResolvedValue({settings:{schemaVersion:1,revision:0,theme:"system",captureShortcut:"Ctrl+Alt+S",pinShortcut:"Ctrl+Alt+T"}});
 api.getShortcuts.mockResolvedValue({bindings:[],lastError:"capture error",actionErrors:{capture:"capture error",pin:"pin error"}});
 render(<Shortcuts moduleId="always-on-top" onSaved={()=>{}}/>);
 await screen.findByRole("textbox",{name:"切换窗口置顶"});
 expect(screen.queryByRole("textbox",{name:"开始截图"})).toBeNull();
 expect((await screen.findByRole("alert")).textContent).toContain("pin error");expect(screen.queryByText(/capture error/)).toBeNull();
});

it("does not show technical maintenance actions in the normal state",async()=>{
 api.getSnapshot.mockResolvedValue({settings:{schemaVersion:1,revision:0,theme:"system",pinShortcut:"Ctrl+Alt+T"}});api.getShortcuts.mockResolvedValue({bindings:[{id:"pin",registered:true,error:""}],lastError:""});
 render(<Shortcuts moduleId="always-on-top" onSaved={()=>{}}/>);
 const save=await screen.findByRole("button",{name:"保存快捷键"});expect((save as HTMLButtonElement).disabled).toBe(true);
 expect(screen.queryByRole("button",{name:"重新注册快捷键"})).toBeNull();expect(screen.queryByRole("button",{name:"刷新快捷键状态"})).toBeNull();expect(screen.queryByRole("button",{name:"重载设置"})).toBeNull();
});
