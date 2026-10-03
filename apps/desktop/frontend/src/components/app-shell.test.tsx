// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from "vitest";
import {cleanup,render,screen} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {Home,Settings} from "lucide-react";
import {AppShell} from "./app-shell";

beforeEach(()=>vi.stubGlobal("matchMedia",()=>({matches:false,addEventListener:()=>{},removeEventListener:()=>{}})));
afterEach(()=>{cleanup();vi.unstubAllGlobals();});
it("shows one navigation entry per page and preserves the active location",async()=>{
 const navigate=vi.fn();
 render(<AppShell page="overview" pages={[{id:"overview",title:"首页",icon:Home},{id:"settings",title:"设置",icon:Settings}]} logo="/logo.png" onNavigate={navigate}><h1>首页内容</h1></AppShell>);
 expect(screen.getAllByRole("button",{name:"首页"}).length).toBe(1);
 expect(screen.getByRole("button",{name:"首页"}).getAttribute("aria-current")).toBe("page");
 await userEvent.setup().click(screen.getByRole("button",{name:"设置"}));
 expect(navigate).toHaveBeenCalledWith("settings");expect(screen.getByRole("heading",{name:"首页内容"})).toBeTruthy();
});
