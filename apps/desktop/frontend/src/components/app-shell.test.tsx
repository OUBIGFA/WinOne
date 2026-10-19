// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from "vitest";
import {cleanup,render,screen,within,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {Home,Settings} from "lucide-react";
import {AppShell} from "./app-shell";
import {SettingsSaveBar} from "./settings-save-bar";

beforeEach(()=>vi.stubGlobal("matchMedia",()=>({matches:false,addEventListener:()=>{},removeEventListener:()=>{}})));
afterEach(cleanup); // tests/setup.ts owns the lifetime of browser polyfills.
it("keeps short-page save actions outside the scroll area and retains their callbacks",async()=>{
 const save=vi.fn(),props={pages:[],logo:"/logo.png",onNavigate:()=>{},page:"capture"};
 const view=render(<AppShell {...props}><input aria-label="Draft" defaultValue="original"/><SettingsSaveBar><button onClick={save}>保存设置</button></SettingsSaveBar></AppShell>);
 const footer=view.container.querySelector('[data-slot="settings-footer"]')!;
 expect(within(footer as HTMLElement).getByRole("button",{name:"保存设置"})).toBeTruthy();
 expect(view.container.querySelector('[data-slot="page-scroll"] .settings-save-bar')).toBeNull();
 await userEvent.setup().click(screen.getByRole("button",{name:"保存设置"}));expect(save).toHaveBeenCalledOnce();
});
it("mirrors kept-mounted tabs and parent fieldset locks into the footer",async()=>{
 const props={pages:[],logo:"/logo.png",onNavigate:()=>{},page:"capture"};
 const content=(hidden:boolean,disabled:boolean)=><AppShell {...props}><div hidden={hidden}><fieldset disabled={disabled}><SettingsSaveBar><button>保存设置</button></SettingsSaveBar></fieldset></div></AppShell>;
 const view=render(content(false,true));
 const button=screen.getByRole<HTMLButtonElement>("button",{name:"保存设置"});expect(button.matches(':disabled')).toBe(true);
 view.rerender(content(true,false));await waitFor(()=>expect(screen.queryByRole("button",{name:"保存设置"})).toBeNull());
 view.rerender(content(false,false));await waitFor(()=>expect(screen.getByRole("button",{name:"保存设置"}).matches(':disabled')).toBe(false));
 view.rerender(<AppShell {...props} page="chat"><p>Chat</p></AppShell>);
 expect(view.container.querySelector('[data-slot="settings-footer"]')).toBeNull();
});
it("keeps settings actions in the fixed titlebar and leaves Chat unchanged",async()=>{
 const refresh=vi.fn(),props={pages:[],logo:"/logo.png",onNavigate:()=>{},toolbar:<button onClick={refresh}>刷新状态</button>};
 const view=render(<AppShell {...props} page="capture"><p>截图设置</p></AppShell>);
 const titlebar=view.container.querySelector<HTMLElement>('[data-slot="window-titlebar"]')!;
 await userEvent.setup().click(within(titlebar).getByRole("button",{name:"刷新状态"}));
 expect(refresh).toHaveBeenCalledOnce();
 view.rerender(<AppShell {...props} page="chat"><p>Chat</p></AppShell>);
 expect(screen.queryByRole("button",{name:"刷新状态"})).toBeNull();
});
it("shows one navigation entry per page and preserves the active location",async()=>{
 const navigate=vi.fn();
 render(<AppShell page="overview" pages={[{id:"overview",title:"首页",icon:Home},{id:"settings",title:"设置",icon:Settings}]} logo="/logo.png" onNavigate={navigate}><h1>首页内容</h1></AppShell>);
 expect(screen.getAllByRole("button",{name:"首页"}).length).toBe(1);
 expect(screen.getByRole("button",{name:"首页"}).getAttribute("aria-current")).toBe("page");
 await userEvent.setup().click(screen.getByRole("button",{name:"设置"}));
 expect(navigate).toHaveBeenCalledWith("settings");expect(screen.getByRole("heading",{name:"首页内容"})).toBeTruthy();
});

it("limits settings styling to non-Chat pages across navigation",()=>{
 const props={pages:[],logo:"/logo.png",onNavigate:()=>{}};
 const view=render(<AppShell {...props} page="settings"><p>Settings</p></AppShell>);
 const content=()=>view.container.querySelector('[data-slot="page-content"]')!;
 expect(content().classList.contains("settings-surface")).toBe(true);
 expect(content().getAttribute("data-settings-page")).toBe("settings");
 expect(document.documentElement.dataset.settingsUi).toBe("true");
 view.rerender(<AppShell {...props} page="chat"><p>Chat</p></AppShell>);
 expect(content().classList.contains("settings-surface")).toBe(false);
 expect(content().getAttribute("data-settings-page")).toBeNull();
 expect(document.documentElement.hasAttribute("data-settings-ui")).toBe(false);
 view.rerender(<AppShell {...props} page="capture"><p>Capture</p></AppShell>);
 expect(content().classList.contains("settings-surface")).toBe(true);
 expect(document.documentElement.dataset.settingsUi).toBe("true");
 view.unmount();
 expect(document.documentElement.hasAttribute("data-settings-ui")).toBe(false);
});
