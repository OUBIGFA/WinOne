// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from "vitest";
import {cleanup,render,screen,within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import defaults from "../../../../../modules/stroke/defaults.json";
import {MouseGesturesSettings} from "./mouse-gestures";
import {i18n} from "../i18n";
import {mouseGesturesMessages} from "../locales/mouse-gestures";
i18n.addResourceBundle("zh-CN","translation",{stroke:mouseGesturesMessages.zh},true,true);
const bridge=vi.hoisted(()=>({strokeCall:vi.fn()}));
vi.mock("../bridge",()=>bridge);
// Canvas painting needs a browser; these tests cover scene selection and the
// settings payload, while mouse-gestures-check.mjs checks actual rendered trails.
beforeEach(()=>{vi.spyOn(HTMLCanvasElement.prototype,"getContext").mockReturnValue(null);});
afterEach(()=>{cleanup();vi.restoreAllMocks();vi.clearAllMocks();});

it("defaults the legacy pen to enabled and saves an explicit off without changing its color or width",async()=>{
 const preferences=structuredClone({...defaults,revision:4}) as {pen:{enabled?:boolean;color:string;opacity:number;thickness:number}};delete preferences.pen.enabled;
 bridge.strokeCall.mockImplementation(async(method:string,value:unknown)=>method==="settings.save"?value:preferences);render(<MouseGesturesSettings/>);const user=userEvent.setup();await user.click(await screen.findByRole("tab",{name:"常规"}));
 const toggle=screen.getByRole("switch",{name:"显示手势画笔"});expect(toggle.getAttribute("aria-checked")).toBe("true");await user.click(toggle);await user.click(screen.getByRole("button",{name:"保存鼠标手势设置"}));
 expect(bridge.strokeCall).toHaveBeenCalledWith("settings.save",expect.objectContaining({pen:{...preferences.pen,enabled:false}}));
});

it("opens the Explorer scene first without rewriting the user's scope order",async()=>{
 const preferences={...structuredClone(defaults),revision:7,scopes:[...defaults.scopes].reverse()};
 bridge.strokeCall.mockResolvedValue(preferences);render(<MouseGesturesSettings/>);
 const scenes=await screen.findByRole("navigation",{name:"使用场景"});
 const first=within(scenes).getAllByRole("button")[0];
 expect(first.getAttribute("aria-label")).toBe("资源管理器文件夹");
 expect(first.getAttribute("aria-pressed")).toBe("true");
 expect(screen.getByRole("group",{name:"场景动作：新建文件夹"})).toBeTruthy();
 expect(screen.getByRole("group",{name:"场景动作：复制"})).toBeTruthy();
 expect(preferences.scopes[0].id).toBe("keyboard");
 expect(screen.getByRole("button",{name:"保存鼠标手势设置"}).hasAttribute("disabled")).toBe(true);
});

it("assigns a gesture to one scene while preserving the shared actions used elsewhere",async()=>{
 bridge.strokeCall.mockImplementation(async(method:string,params:unknown)=>method==="settings.get"?{...structuredClone(defaults),revision:7}:method==="settings.save"?{...(params as object),revision:8}:{});
 const user=userEvent.setup();render(<MouseGesturesSettings/>);
 const copy=await screen.findByRole("group",{name:"场景动作：复制"});
 await user.click(within(copy).getByRole("combobox",{name:"复制的手势"}));
 await user.click(await screen.findByRole("option",{name:"按住手势键 + 中键"}));
 await user.click(screen.getByRole("button",{name:"保存鼠标手势设置"}));
 const saved=bridge.strokeCall.mock.calls.find(call=>call[0]==="settings.save")?.[1];
 const explorer=saved.scopes.find((scope:{id:string})=>scope.id==="explorer-folders");
 const original=saved.actionPackages.find((pack:{id:string})=>pack.id==="global");
 const assigned=saved.actionPackages.find((pack:{id:string;actions:{id:string}[]})=>explorer.packages.includes(pack.id)&&pack.actions.some(action=>action.id==="copy"));
 expect(original.actions.find((action:{id:string})=>action.id==="copy").gesture).toBe("up-then-left");
 expect(assigned.actions.find((action:{id:string})=>action.id==="copy").gesture).toBe("#0");
 expect(assigned.actions.find((action:{id:string})=>action.id==="copy").code).toBe(defaults.actionPackages[0].actions.find((action:{id:string})=>action.id==="copy")!.code);
 expect(saved.scopes.find((scope:{id:string})=>scope.id==="global").packages).toEqual(["global"]);
 expect(saved.scopes.filter((scope:{enabled:boolean})=>scope.enabled)).toHaveLength(1);
 for(const original of defaults.actionPackages)expect(saved.actionPackages.find((pack:{id:string})=>pack.id===original.id)).toEqual(original);
});

it("shows the winning action when multiple assignments share a gesture",async()=>{
 const preferences={...structuredClone(defaults),revision:1};
 preferences.actionPackages.find(pack=>pack.id==="explorer")!.actions[0].gesture="up-then-left";
 preferences.actionPackages.find(pack=>pack.id==="explorer")!.actions[0].enabled=true;
 preferences.actionPackages.find(pack=>pack.id==="global")!.actions.find(action=>action.id==="copy")!.enabled=true;
 bridge.strokeCall.mockResolvedValue(preferences);render(<MouseGesturesSettings/>);
 const copy=await screen.findByRole("group",{name:"场景动作：复制"});
 expect(within(copy).getByText("此手势优先执行：刷新")).toBeTruthy();
 const refresh=screen.getByRole("group",{name:"场景动作：刷新"});
 expect(within(refresh).queryByText(/此手势优先执行/)).toBeNull();
});

it("edits existing scopes and saves without enabling other applications",async()=>{
 bridge.strokeCall.mockImplementation(async(method:string,params:unknown)=>method==="settings.get"?{...defaults,revision:7}:method==="settings.save"?{...(params as object),revision:8}:{});
 render(<MouseGesturesSettings/>);
 await screen.findByRole("navigation",{name:"使用场景"});
 expect(screen.getByRole("switch",{name:"启用场景：资源管理器文件夹"}).getAttribute("aria-checked")).toBe("true");
 await userEvent.click(screen.getByRole("tab",{name:"常规"}));
 const width=screen.getByLabelText("轨迹宽度（像素）");await userEvent.clear(width);await userEvent.type(width,"3");
 await userEvent.click(screen.getByRole("button",{name:"保存鼠标手势设置"}));
 const saved=bridge.strokeCall.mock.calls.find(call=>call[0]==="settings.save")?.[1];
 expect(saved.pen.thickness).toBe(3);expect(saved.scopes.filter((scope:{enabled:boolean})=>scope.enabled)).toHaveLength(1);
 expect(saved.actionPackages.reduce((n:number,p:{actions:unknown[]})=>n+p.actions.length,0)).toBe(31);
});

it("keeps the full upstream compiler and execution settings editable",async()=>{
 bridge.strokeCall.mockResolvedValue({...defaults,revision:1});render(<MouseGesturesSettings/>);
 await userEvent.click(await screen.findByRole("tab",{name:"高级"}));await userEvent.click(screen.getByRole("tab",{name:"编译"}));
 expect((screen.getByLabelText("命名空间（每行一个）") as HTMLTextAreaElement).value.split("\n")).toHaveLength(37);
 expect((screen.getByLabelText("程序集（每行一个）") as HTMLTextAreaElement).value).toContain("System.Net.Http.dll");
 const namespaces=screen.getByLabelText("命名空间（每行一个）");await userEvent.type(namespaces,"\nSystem.Globalization");expect((namespaces as HTMLTextAreaElement).value).toContain("\nSystem.Globalization");
 await userEvent.click(screen.getByRole("tab",{name:"执行线程"}));
 expect((screen.getByLabelText("最大执行线程数") as HTMLInputElement).value).toBe("8");
 expect((screen.getByLabelText("动作超时（秒，0 不限制）") as HTMLInputElement).value).toBe("0");
});
