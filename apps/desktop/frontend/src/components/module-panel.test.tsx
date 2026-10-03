// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from "vitest";
import {cleanup,render,screen} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {useState} from "react";
import {ModulePanel} from "./module-panel";
import {SettingsPanel} from "./settings-panel";
const api=vi.hoisted(()=>({updateInfo:vi.fn(),readModuleLogs:vi.fn(),openModuleLogFolder:vi.fn(),getSnapshot:vi.fn(),getShortcuts:vi.fn(),saveSettings:vi.fn(),shortcutRecordingPort:{begin:vi.fn(async()=>"token"),end:vi.fn(async()=>{})}}));
vi.mock("../bridge",()=>api);
beforeEach(()=>{api.updateInfo.mockResolvedValue({version:"1.0.0",configured:false});api.getSnapshot.mockResolvedValue({settings:{schemaVersion:1,revision:0,theme:"system",pinShortcut:"Ctrl+Alt+T",captureShortcut:"Ctrl+Alt+C"}});api.getShortcuts.mockResolvedValue({bindings:[],lastError:""});});
afterEach(()=>{cleanup();vi.resetAllMocks();});
function Draft(){const [value,setValue]=useState("");return <input aria-label="draft" value={value} onChange={event=>setValue(event.target.value)}/>;}
it("keeps shortcut settings outside the module-owned panel",async()=>{
 render(<ModulePanel moduleId="always-on-top" onSaved={()=>{}} onActivity={()=>{}}><Draft/></ModulePanel>);
 await userEvent.setup().type(screen.getByLabelText("draft"),"keep");
 expect(await screen.findByRole("textbox",{name:"切换窗口置顶"})).toBeTruthy();
 expect(screen.queryByRole("tab",{name:"置顶设置"})).toBeNull();
 expect(screen.queryByRole("tab",{name:"快捷键"})).toBeNull();
 expect((screen.getByLabelText("draft") as HTMLInputElement).value).toBe("keep");
});
it("does not add shortcut controls to WinTray",()=>{
 render(<ModulePanel moduleId="wintray" onSaved={()=>{}} onActivity={()=>{}}><Draft/></ModulePanel>);
 expect(screen.queryByRole("tab",{name:"快捷键"})).toBeNull();
});

it("shows pinning shortcuts above a separate settings and logs group",async()=>{
 api.getSnapshot.mockResolvedValue({settings:{schemaVersion:1,revision:0,theme:"system",pinShortcut:"Ctrl+Alt+T"}});api.getShortcuts.mockResolvedValue({bindings:[],lastError:""});
 render(<ModulePanel moduleId="always-on-top" onSaved={()=>{}} onActivity={()=>{}}><Draft/></ModulePanel>);
 await screen.findByRole("textbox",{name:"切换窗口置顶"});expect(screen.getByLabelText("draft")).toBeTruthy();expect(api.readModuleLogs).not.toHaveBeenCalled();
 expect(screen.queryByRole("tab",{name:"快捷键"})).toBeNull();
});
it("global settings exposes only WinOne logs",async()=>{
 api.readModuleLogs.mockResolvedValue({folderAvailable:false,entries:[]});render(<SettingsPanel><Draft/></SettingsPanel>);
 expect(api.readModuleLogs).not.toHaveBeenCalled();await userEvent.setup().click(screen.getByRole("tab",{name:"诊断日志"}));
 expect(api.readModuleLogs).toHaveBeenCalledExactlyOnceWith("host");expect(screen.queryByRole("combobox")).toBeNull();
});
