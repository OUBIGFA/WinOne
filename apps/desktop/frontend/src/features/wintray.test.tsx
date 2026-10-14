// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, screen, within,waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { WinTrayPage, type Program } from "./wintray";
import { setApplicationLanguage } from "../i18n";

const native = vi.hoisted(() => ({ info: vi.fn(async(path:string)=>({name:path.split(/[\\/]/).pop()!.replace(/\.[^.]+$/,""),icon:""})), call: vi.fn(), pick: vi.fn(),pickMany:vi.fn(),residency:vi.fn().mockResolvedValue({requested:false,reasons:[]}) }));
const logs=vi.hoisted(()=>vi.fn(async()=>({folderAvailable:false,entries:[]})));
it("sorts programs directly to either end without polling tray actions",async()=>{
 const managedApps=["甲","乙","丙"].map((name,index):Program=>({id:String(index),name,exePath:`C:\\Apps\\${index}.exe`,args:"",runOnStartup:true,collectTrayIcon:true,launchViaLogonTask:false,launchHiddenInBackground:false,trayBehavior:{autoMinimizeAndHideOnLaunch:false,closeDelaySeconds:0},schedule:{enabled:false,startDelayMinutes:0,autoExitMinutes:0,frequencyEnabled:false,frequencyDays:1,frequencyRuns:1}}));
 native.info.mockResolvedValue({name:"",icon:""});
 native.call.mockImplementation(async(method:string)=>method==="icons"?[]:{revision:"1",settings:{schemaVersion:3,language:"zh-CN",managedApps,startupIntervalSeconds:3,closeWindowRetrySeconds:10}});
 render(<WinTrayPage/>);const user=userEvent.setup();
 (await screen.findByRole("combobox",{name:"丙的排序序号"})).focus();await user.keyboard("{ArrowDown}");await user.click(await screen.findByRole("option",{name:"1"}));
 expect(screen.getAllByRole("listitem").map(row=>row.getAttribute("aria-label"))).toEqual(["丙","甲","乙"]);
 screen.getByRole("combobox",{name:"丙的排序序号"}).focus();await user.keyboard("{ArrowDown}");await user.click(await screen.findByRole("option",{name:"3"}));
 expect(screen.getAllByRole("listitem").map(row=>row.getAttribute("aria-label"))).toEqual(["甲","乙","丙"]);
 expect(native.call.mock.calls.some(([method])=>method==="icons")).toBe(false);
 expect(screen.queryByText("原程序菜单")).toBeNull();
});
vi.mock("../bridge", () => ({
  wintrayCall: native.call,
  programInfo: native.info,
  pickProgram: native.pick,
  pickPrograms: native.pickMany,
  pickWinTrayConfig: native.pick,
  getResidency:native.residency,
  readModuleLogs:logs,
  openModuleLogFolder:vi.fn(),
}));
afterEach(async () => {
  cleanup();
  native.call.mockReset();
  native.info.mockReset();
  native.pick.mockReset();
  native.pickMany.mockReset();
  native.residency.mockClear();
  logs.mockClear();
  await setApplicationLanguage("zh-CN");
});
it("keeps program failure feedback without an independent log tab",async()=>{
 native.call.mockRejectedValue(Error("program settings unavailable"));
 render(<WinTrayPage/>);await screen.findByRole("alert");
 expect(screen.queryByRole("tab",{name:"诊断日志"})).toBeNull();
 expect(logs).not.toHaveBeenCalled();
 expect(screen.queryByRole("button",{name:"添加程序"})).toBeNull();
});

it("preserves an edited name matching the executable stem after saving",async()=>{
 let settings={schemaVersion:3,language:"zh-CN",runAtLogon:false,startMinimizedToTray:true,exitAfterManagedAppsCompleted:false,closeWindowRetrySeconds:10,startupIntervalSeconds:3,managedApps:[{id:"custom",name:"原名称",exePath:"C:\\Apps\\custom.exe",args:"",runOnStartup:false,collectTrayIcon:false,launchViaLogonTask:false,launchHiddenInBackground:false,trayBehavior:{autoMinimizeAndHideOnLaunch:false,closeDelaySeconds:0},schedule:{enabled:false,startDelayMinutes:0,autoExitMinutes:30,frequencyEnabled:false,frequencyDays:1,frequencyRuns:1}}]};
 native.info.mockResolvedValue({name:"自动识别名称",icon:""});
 native.call.mockImplementation(async(method:string,payload?:{settings:typeof settings})=>{if(method==="settings.get")return {revision:"1",settings};if(method==="settings.save"){settings=payload!.settings;return {state:"completed"};}throw Error(method);});
 render(<WinTrayPage disabled/>);const user=userEvent.setup();await user.click(await screen.findByRole("tab",{name:"更多设置"}));
 const input=screen.getByRole("textbox",{name:"名称"}) as HTMLInputElement;
 await user.clear(input);input.focus();await user.type(input,"custom",{skipClick:true});expect(input.value).toBe("custom");
 await user.click(screen.getByRole("button",{name:"保存修改"}));await screen.findByText("已保存");
 expect(settings.managedApps[0].name).toBe("custom");expect(input.value).toBe("custom");
});

it("keeps an empty program name editable and blocks saving across tabs",async()=>{
 const program:Program={id:"name",name:"程序",exePath:"C:\\Apps\\program.exe",args:"",runOnStartup:false,collectTrayIcon:false,launchViaLogonTask:false,launchHiddenInBackground:false,trayBehavior:{autoMinimizeAndHideOnLaunch:false,closeDelaySeconds:0},schedule:{enabled:false,startDelayMinutes:0,autoExitMinutes:30,frequencyEnabled:false,frequencyDays:1,frequencyRuns:1}};
 native.info.mockResolvedValue({name:"识别出的程序",icon:""});native.call.mockResolvedValue({revision:"1",settings:{schemaVersion:3,language:"zh-CN",runAtLogon:false,startMinimizedToTray:true,exitAfterManagedAppsCompleted:false,closeWindowRetrySeconds:10,startupIntervalSeconds:3,managedApps:[program]}});
 render(<WinTrayPage disabled/>);const user=userEvent.setup();await user.click(await screen.findByRole("tab",{name:"更多设置"}));
 const input=screen.getByRole("textbox",{name:"名称"}) as HTMLInputElement;await user.clear(input);
 expect(input.value).toBe("");expect(input.getAttribute("aria-invalid")).toBe("true");
 expect(document.getElementById(input.getAttribute("aria-describedby")!)?.textContent).toContain("更多设置");
 await user.click(screen.getByRole("tab",{name:"常用设置"}));await user.click(screen.getByRole("button",{name:"保存修改"}));
 expect(native.call.mock.calls.some(([method])=>method==="settings.save")).toBe(false);
 await user.click(screen.getByRole("tab",{name:"更多设置"}));expect((screen.getByRole("textbox",{name:"名称"}) as HTMLInputElement).value).toBe("");
});

it.each(["","121","-1","1.5"])("keeps invalid timing input '%s' editable and blocks saving until corrected",async value=>{
 let settings={schemaVersion:3,language:"zh-CN",runAtLogon:false,startMinimizedToTray:true,exitAfterManagedAppsCompleted:false,closeWindowRetrySeconds:10,startupIntervalSeconds:3,managedApps:[]};
 native.call.mockImplementation(async(method:string,payload?:{settings:typeof settings})=>{
  if(method==="settings.get")return {revision:"1",settings};
  if(method==="settings.save"){settings=payload!.settings;return {state:"completed"};}
  throw Error(method);
 });
 render(<WinTrayPage disabled/>);const user=userEvent.setup();
 await user.click(await screen.findByRole("tab",{name:"启动设置"}));
 const input=screen.getByRole("spinbutton",{name:"程序之间的启动间隔（秒）"}) as HTMLInputElement;
 await user.clear(input);if(value)await user.type(input,value);
 expect(input.value).toBe(value);
 expect(input.getAttribute("aria-invalid")).toBe("true");
 expect(document.getElementById(input.getAttribute("aria-describedby")!)?.textContent).toContain("0–120");
 await user.click(screen.getByRole("button",{name:"保存修改"}));
 expect(native.call.mock.calls.some(([method])=>method==="settings.save")).toBe(false);
 await user.click(screen.getByRole("tab",{name:"程序列表"}));
 expect(screen.getByText(/请检查数值：启动设置/)).toBeTruthy();
 expect((screen.getByRole("button",{name:"保存修改"}) as HTMLButtonElement).disabled).toBe(true);
 await user.click(screen.getByRole("tab",{name:"启动设置"}));
 const restored=screen.getByRole("spinbutton",{name:"程序之间的启动间隔（秒）"});
 await user.clear(restored);await user.type(restored,"8");
 await user.click(screen.getByRole("button",{name:"保存修改"}));await screen.findByText("已保存");
 expect(settings.startupIntervalSeconds).toBe(8);
});

it("saves and reloads all custom timing without enabling ordinary startup",async()=>{
 const program:Program={id:"independent",name:"定时程序",exePath:"C:\\Apps\\scheduled.exe",args:"",runOnStartup:false,collectTrayIcon:false,launchViaLogonTask:false,launchHiddenInBackground:false,trayBehavior:{autoMinimizeAndHideOnLaunch:false,closeDelaySeconds:0},schedule:{enabled:false,startDelayMinutes:0,autoExitMinutes:30,frequencyEnabled:false,frequencyDays:1,frequencyRuns:1}};
 let settings={schemaVersion:3,language:"zh-CN",runAtLogon:false,startMinimizedToTray:true,exitAfterManagedAppsCompleted:false,closeWindowRetrySeconds:10,startupIntervalSeconds:3,managedApps:[program]};
 native.call.mockImplementation(async(method:string,payload?:{settings:typeof settings})=>{
  if(method==="settings.get")return {revision:"1",settings};
  if(method==="settings.save"){settings=payload!.settings;return {state:"completed"};}
  throw Error(method);
 });
 const view=render(<WinTrayPage disabled/>);const user=userEvent.setup();
 await user.click(await screen.findByRole("switch",{name:"自定义时间"}));
 const change=async(name:string,value:string)=>{const input=screen.getByRole("spinbutton",{name});await user.clear(input);input.focus();await user.type(input,value,{skipClick:true});};
 await change("延后启动（分钟）","7");
 await user.click(screen.getByRole("tab",{name:"更多设置"}));
 await user.click(screen.getByRole("checkbox",{name:"限制自动运行次数"}));
 await change("最近天数","2");await change("最多运行次数","3");
 await user.click(screen.getByRole("button",{name:"保存修改"}));await screen.findByText("已保存");
 expect(settings.managedApps[0].runOnStartup).toBe(false);
 expect(settings.managedApps[0].schedule).toEqual({enabled:true,startDelayMinutes:7,autoExitMinutes:30,frequencyEnabled:true,frequencyDays:2,frequencyRuns:3});
 view.unmount();render(<WinTrayPage disabled/>);
 expect((await screen.findByRole("spinbutton",{name:"延后启动（分钟）"}) as HTMLInputElement).value).toBe("7");
 expect(screen.getByRole("switch",{name:"开机启动此程序"}).getAttribute("aria-checked")).toBe("false");
 await user.click(screen.getByRole("tab",{name:"更多设置"}));
 expect((screen.getByRole("spinbutton",{name:"最近天数"}) as HTMLInputElement).value).toBe("2");
 expect((screen.getByRole("spinbutton",{name:"最多运行次数"}) as HTMLInputElement).value).toBe("3");
});

it("identifies a program with unfinished timing after its controls are hidden",async()=>{
 const program:Program={id:"timing",name:"音乐播放器",exePath:"C:\\Apps\\music.exe",args:"",runOnStartup:true,collectTrayIcon:false,launchViaLogonTask:false,launchHiddenInBackground:false,trayBehavior:{autoMinimizeAndHideOnLaunch:true,closeDelaySeconds:3},schedule:{enabled:true,startDelayMinutes:5,autoExitMinutes:30,frequencyEnabled:false,frequencyDays:1,frequencyRuns:1}};
 native.call.mockResolvedValue({revision:"1",settings:{schemaVersion:3,language:"zh-CN",runAtLogon:false,startMinimizedToTray:true,exitAfterManagedAppsCompleted:false,closeWindowRetrySeconds:10,startupIntervalSeconds:3,managedApps:[program]}});
 render(<WinTrayPage disabled/>);const user=userEvent.setup();
 const input=await screen.findByRole("spinbutton",{name:"延后启动（分钟）"});
 await user.clear(input);
 await user.click(screen.getByRole("switch",{name:"自定义时间"}));
 expect(screen.queryByRole("spinbutton",{name:"延后启动（分钟）"})).toBeNull();
 expect(screen.getByText("请检查数值：音乐播放器")).toBeTruthy();
 expect((screen.getByRole("button",{name:"保存修改"}) as HTMLButtonElement).disabled).toBe(true);
 await user.click(screen.getByRole("switch",{name:"自定义时间"}));
 const restored=screen.getByRole("spinbutton",{name:"延后启动（分钟）"}) as HTMLInputElement;
 // jsdom has no panel geometry; focus directly so the resize separator cannot intercept the synthetic click.
 expect(restored.value).toBe("");restored.focus();await user.type(restored,"8",{skipClick:true});expect(restored.value).toBe("8");
 expect(screen.queryByText("请检查数值：音乐播放器")).toBeNull();
 expect((screen.getByRole("button",{name:"保存修改"}) as HTMLButtonElement).disabled).toBe(false);
});

it.each([true,false])("keeps timing edits when a running job cannot be identified (missing ID: %s)",async missing=>{
 const settings={schemaVersion:3,language:"zh-CN",runAtLogon:false,startMinimizedToTray:true,exitAfterManagedAppsCompleted:false,closeWindowRetrySeconds:10,startupIntervalSeconds:3,managedApps:[]};
 native.call.mockImplementation(async(method:string)=>{
  if(method==="settings.get")return {revision:"1",settings};
  if(method==="settings.save")return missing?{state:"running"}:{id:"expected",state:"running"};
  if(method==="jobs.get")return {id:"another",state:"completed"};
  throw Error(method);
 });
 render(<WinTrayPage disabled/>);const user=userEvent.setup();
 await user.click(await screen.findByRole("tab",{name:"启动设置"}));
 const input=screen.getByRole("spinbutton",{name:"程序之间的启动间隔（秒）"}) as HTMLInputElement;
 await user.clear(input);await user.type(input,"8");await user.click(screen.getByRole("button",{name:"保存修改"}));
 await screen.findByRole("alert");expect(input.value).toBe("8");expect(screen.queryByText("已保存")).toBeNull();
 expect(native.call.mock.calls.filter(([method])=>method==="jobs.get")).toHaveLength(missing?0:1);
});

it("selects the whole program row and keeps common actions before both settings tabs",async()=>{
 const programs=["first","second"].map(name=>({id:name,name,exePath:`C:\\Apps\\${name}.exe`,args:"",runOnStartup:true,collectTrayIcon:false,launchViaLogonTask:false,launchHiddenInBackground:false,trayBehavior:{autoMinimizeAndHideOnLaunch:false,closeDelaySeconds:0},schedule:{enabled:false,startDelayMinutes:0,autoExitMinutes:30,frequencyEnabled:false,frequencyDays:1,frequencyRuns:1}}));
 native.call.mockResolvedValue({revision:"1",settings:{schemaVersion:3,language:"zh-CN",runAtLogon:false,startMinimizedToTray:true,exitAfterManagedAppsCompleted:false,closeWindowRetrySeconds:10,startupIntervalSeconds:3,managedApps:programs}});
 native.info.mockImplementation(async path=>({name:path.includes("second")?"识别出的程序":"first",icon:"aWNvbg=="}));
 render(<WinTrayPage/>);const user=userEvent.setup();
 const row=await screen.findByRole("listitem",{name:"second"});
 await user.click(within(row).getByText("second"));
 expect(row.getAttribute("aria-current")).toBe("true");
 expect(within(row).queryByRole("button",{name:"second"})).toBeNull();
 await waitFor(()=>expect(row.querySelector("img")?.getAttribute("src")).toBe("data:image/png;base64,aWNvbg=="));
 const actions=screen.getByRole("group",{name:"程序操作"});
 expect(within(actions).getByRole("button",{name:"立即启动"})).toBeTruthy();
 const tabs=screen.getByRole("tab",{name:"常用设置"}).closest('[data-slot="tabs"]')!;
 expect(actions.compareDocumentPosition(tabs)&Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
 await user.click(screen.getByRole("tab",{name:"更多设置"}));
 expect(screen.getByRole("group",{name:"程序操作"})).toBe(actions);
 const first=screen.getByRole("listitem",{name:"first"});first.focus();await user.keyboard("{Enter}");
 expect(first.getAttribute("aria-current")).toBe("true");
});

it("adds several programs in one draft and preserves executable/script defaults",async()=>{
  let settings={schemaVersion:3,language:"zh-CN",runAtLogon:false,startMinimizedToTray:true,exitAfterManagedAppsCompleted:false,closeWindowRetrySeconds:10,startupIntervalSeconds:3,managedApps:[] as Program[]};
  native.call.mockImplementation(async(method:string,payload?:{settings:typeof settings})=>{
    if(method==="settings.get")return {revision:"1",settings};
    if(method==="settings.save"){settings=payload!.settings;return {state:"completed"};}
    throw Error(method);
  });
  native.pickMany.mockResolvedValue(["C:\\Apps\\alpha.exe","C:\\Jobs\\backup.ps1"]);
  native.info.mockImplementation(async path=>({name:path.endsWith("alpha.exe")?"Alpha 应用":"备份脚本",icon:""}));
  render(<WinTrayPage disabled/>);
  const user=userEvent.setup();
  await user.click(await screen.findByRole("button",{name:"添加程序"}));
  expect(await screen.findByRole("listitem",{name:"Alpha 应用"})).toBeTruthy();
  expect(screen.getByRole("listitem",{name:"备份脚本"})).toBeTruthy();
  expect(native.call.mock.calls.some(([method])=>method==="settings.save")).toBe(false);
  await user.click(screen.getByRole("button",{name:"保存修改"}));
  await screen.findByText("已保存");
  expect(settings.managedApps).toHaveLength(2);
  expect(settings.managedApps.map(program=>program.name)).toEqual(["Alpha 应用","备份脚本"]);
  expect(new Set(settings.managedApps.map(app=>app.id)).size).toBe(2);
  expect(settings.managedApps[0].trayBehavior.autoMinimizeAndHideOnLaunch).toBe(true);
  expect(settings.managedApps[1].launchHiddenInBackground).toBe(true);
  expect(settings.managedApps[1].trayBehavior.autoMinimizeAndHideOnLaunch).toBe(false);
});

it("replaces an executable with a script without losing its identity or schedule",async()=>{
  const schedule={enabled:true,startDelayMinutes:5,autoExitMinutes:45,frequencyEnabled:true,frequencyDays:3,frequencyRuns:2};
  let settings={schemaVersion:3,language:"zh-CN",runAtLogon:false,startMinimizedToTray:true,exitAfterManagedAppsCompleted:false,closeWindowRetrySeconds:10,startupIntervalSeconds:3,managedApps:[{id:"existing",name:"示例",exePath:"C:\\Apps\\old.exe",args:"--daily",runOnStartup:true,collectTrayIcon:true,launchViaLogonTask:true,launchHiddenInBackground:false,trayBehavior:{autoMinimizeAndHideOnLaunch:false,closeDelaySeconds:3},schedule}]};
  native.call.mockImplementation(async(method:string,payload?:{settings:typeof settings})=>{
    if(method==="settings.get")return {revision:"1",settings};
    if(method==="settings.save"){settings=payload!.settings;return {state:"completed"};}
    throw Error(method);
  });
  native.pick.mockResolvedValue("D:\\Jobs\\replacement.ps1");
  render(<WinTrayPage disabled/>);
  const user=userEvent.setup();
  await user.click(await screen.findByRole("listitem",{name:/^示例/}));
  await user.click(screen.getByRole("tab",{name:"更多设置"}));
  await user.click(screen.getByRole("button",{name:/^更换 .* 的程序$/}));
  await screen.findByText("D:\\Jobs\\replacement.ps1");
  expect(native.call.mock.calls.some(([method])=>method==="settings.save")).toBe(false);
  await user.click(screen.getByRole("button",{name:"保存修改"}));
  await screen.findByText("已保存");
  expect(settings.managedApps[0]).toMatchObject({id:"existing",name:"replacement",exePath:"D:\\Jobs\\replacement.ps1",args:"--daily",runOnStartup:true,collectTrayIcon:false,launchViaLogonTask:false,launchHiddenInBackground:true,schedule});
});

it("keeps custom settings when replacement is cancelled or the same path is selected",async()=>{
  const program:Program={id:"keep",name:"自定义名称",exePath:"C:\\Apps\\Keep.exe",args:"",runOnStartup:true,collectTrayIcon:true,launchViaLogonTask:false,launchHiddenInBackground:false,trayBehavior:{autoMinimizeAndHideOnLaunch:true,closeDelaySeconds:2},schedule:{enabled:false,startDelayMinutes:0,autoExitMinutes:30,frequencyEnabled:false,frequencyDays:1,frequencyRuns:1}};
  const settings={schemaVersion:3,language:"zh-CN",runAtLogon:false,startMinimizedToTray:true,exitAfterManagedAppsCompleted:false,closeWindowRetrySeconds:10,startupIntervalSeconds:3,managedApps:[program]};
  native.call.mockResolvedValue({revision:"1",settings});
  native.pick.mockResolvedValueOnce("").mockResolvedValueOnce("c:\\apps\\KEEP.EXE");
  render(<WinTrayPage disabled/>);
  const user=userEvent.setup();
  await user.click(await screen.findByRole("listitem",{name:/^自定义名称/}));
  await user.click(screen.getByRole("tab",{name:"更多设置"}));
  for(let attempt=0;attempt<2;attempt++){
    await user.click(screen.getByRole("button",{name:/^更换 .* 的程序$/}));
    expect((screen.getByRole("button",{name:"保存修改"}) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByLabelText("名称") as HTMLInputElement).value).toBe("自定义名称");
    expect(screen.getByRole("switch",{name:"托盘收纳"}).getAttribute("aria-checked")).toBe("true");
  }
  expect(native.call).toHaveBeenCalledTimes(1);
});

it("does not create entries after cancelling the multiple-file picker",async()=>{
  native.call.mockResolvedValue({revision:"1",settings:{schemaVersion:3,language:"zh-CN",runAtLogon:false,startMinimizedToTray:true,exitAfterManagedAppsCompleted:false,closeWindowRetrySeconds:10,startupIntervalSeconds:3,managedApps:[]}});
  native.pickMany.mockResolvedValue([]);
  render(<WinTrayPage disabled/>);
  await userEvent.setup().click(await screen.findByRole("button",{name:"添加程序"}));
  expect(screen.getByText("还没有程序，添加后可设置启动方式")).toBeTruthy();
  expect((screen.getByRole("button",{name:"保存修改"}) as HTMLButtonElement).disabled).toBe(true);
});

it("does not show or poll background status in startup settings",async()=>{
  const settings={schemaVersion:3,language:"zh-CN",runAtLogon:false,startMinimizedToTray:true,exitAfterManagedAppsCompleted:false,closeWindowRetrySeconds:10,startupIntervalSeconds:3,managedApps:[]};
  native.call.mockResolvedValue({revision:"1",settings});
  render(<WinTrayPage/>);
  const user=userEvent.setup();
  const advanced=await screen.findByText("启动设置");
  expect(native.residency).not.toHaveBeenCalled();
  await user.click(advanced);
  expect(screen.queryByText("后台运行状态")).toBeNull();
  expect(native.residency).not.toHaveBeenCalled();
  await user.click(screen.getByRole("tab",{name:"程序列表"}));
  expect(native.residency).not.toHaveBeenCalled();
});

it("edits disabled-module preferences without activating WinTray", async () => {
  let settings={schemaVersion:3,language:"zh-CN",runAtLogon:false,startMinimizedToTray:true,exitAfterManagedAppsCompleted:false,closeWindowRetrySeconds:10,startupIntervalSeconds:3,managedApps:[]};
  native.call.mockImplementation(async(method:string,payload?:{settings:typeof settings})=>{
    if(method==="settings.get")return {revision:"1",settings};
    if(method==="settings.save") { settings=payload!.settings; return {state:"completed"}; }
    throw new Error(`unexpected activation: ${method}`);
  });
  render(<WinTrayPage disabled />);
  const user=userEvent.setup();
  await user.click(await screen.findByText("启动设置"));
  const input=await screen.findByRole("spinbutton",{name:"程序之间的启动间隔（秒）"});
  await user.clear(input);await user.type(input,"8");
  await user.click(screen.getByRole("button",{name:"保存修改"}));
  expect(await screen.findByText("已保存")).toBeTruthy();
  expect(screen.queryByRole("button",{name:"导入 WinTray 配置"})).toBeNull();
  expect(native.call.mock.calls.every(([method])=>["settings.get","settings.save"].includes(method))).toBe(true);
});

it("saves startup timing through the English controls",async()=>{
  await setApplicationLanguage("en-US");
  let settings={schemaVersion:3,language:"en-US",runAtLogon:false,startMinimizedToTray:true,exitAfterManagedAppsCompleted:false,closeWindowRetrySeconds:10,startupIntervalSeconds:3,managedApps:[]};
  native.call.mockImplementation(async(method:string,payload?:{settings:typeof settings})=>{
    if(method==="settings.get")return {revision:"1",settings};
    if(method==="settings.save"){settings=payload!.settings;return {state:"completed"};}
    throw new Error(method);
  });
  render(<WinTrayPage disabled/>);
  const user=userEvent.setup();
  await user.click(await screen.findByText("Startup settings"));
  const interval=screen.getByRole("spinbutton",{name:"Delay between programs (seconds)"});
  await user.clear(interval);await user.type(interval,"6");
  await user.click(screen.getByRole("button",{name:"Save changes"}));
  expect(await screen.findByText("Saved")).toBeTruthy();
  expect(settings.startupIntervalSeconds).toBe(6);
});

it("does not expose legacy import or product login switches",async()=>{
 const settings={schemaVersion:3,language:"zh-CN",runAtLogon:false,startMinimizedToTray:true,exitAfterManagedAppsCompleted:false,closeWindowRetrySeconds:10,startupIntervalSeconds:3,managedApps:[]};
 native.call.mockResolvedValue({revision:"1",settings});
 render(<WinTrayPage disabled/>);
 await userEvent.setup().click(await screen.findByRole("tab",{name:"启动设置"}));
 expect(screen.queryByRole("button",{name:"导入 WinTray 配置"})).toBeNull();
 expect(screen.queryByRole("switch",{name:"登录时仅在托盘运行"})).toBeNull();
 expect(screen.queryByRole("switch",{name:"登录时运行 WinOne 并启动已启用的程序"})).toBeNull();
});

it("keeps the user's draft and exposes recovery after a failed save", async () => {
  let failed = false;
  const settings = {
    schemaVersion: 3,
    language: "zh-CN",
    runAtLogon: false,
    startMinimizedToTray: true,
    exitAfterManagedAppsCompleted: false,
    closeWindowRetrySeconds: 10,
    startupIntervalSeconds: 3,
    managedApps: [
      {
        id: "a",
        name: "示例",
        exePath: "C:\\Apps\\example.exe",
        args: "",
        runOnStartup: true,
        collectTrayIcon: false,
        launchViaLogonTask: false,
        launchHiddenInBackground: false,
        trayBehavior: {
          autoMinimizeAndHideOnLaunch: false,
          closeDelaySeconds: 0,
        },
        schedule: {
          enabled: false,
          startDelayMinutes: 0,
          autoExitMinutes: 30,
          frequencyEnabled: false,
          frequencyDays: 1,
          frequencyRuns: 1,
        },
      },
    ],
  };
  native.call.mockImplementation(async (method: string) => {
    if (method === "snapshot")
      return {
        revision: "first",
        settings: structuredClone(settings),
        recoveryError: failed ? "需要恢复" : "",
      };
    if (method === "settings.save") {
      failed = true;
      return { id: "job", state: "failed", error: "保存失败" };
    }
    throw new Error(method);
  });
  render(<WinTrayPage />);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("listitem", { name: /^示例/ }));
  await user.click(screen.getByRole("tab",{name:"更多设置"}));
  const name = screen.getByLabelText("名称") as HTMLInputElement;
  await user.clear(name);
  expect(name.isConnected).toBe(true);
  // JSDOM gives every element a zero-sized rect; a synthetic pointer click
  // would hit the resizer. clear() already focused the real input.
  await user.type(name, "修改名称", {skipClick:true});
  expect(name.value).toBe("修改名称");
  await user.click(screen.getByRole("button", { name: "保存修改" }));
  await screen.findByRole("button", { name: "恢复上次设置" });
  expect(name.value).toBe("修改名称");
  expect(
    (screen.getByRole("button", { name: "保存修改" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
});
