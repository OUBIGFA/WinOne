// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {StartupSettings} from "./startup-settings";
afterEach(cleanup);
it("changes login and silent startup independently",async()=>{
 const change=vi.fn();render(<StartupSettings value={{runAtLogon:false,startMinimized:true}} disabled={false} onChange={change}/>);const user=userEvent.setup();
 await user.click(screen.getByRole("switch",{name:"登录 Windows 时启动"}));expect(change).toHaveBeenLastCalledWith({runAtLogon:true,startMinimized:true});
 await user.click(screen.getByRole("switch",{name:"静默启动到托盘"}));expect(change).toHaveBeenLastCalledWith({runAtLogon:false,startMinimized:false});
});
it("defaults background retention on and can explicitly disable it",async()=>{
 const change=vi.fn();render(<StartupSettings value={{runAtLogon:false,startMinimized:true}} disabled={false} onChange={change}/>);
 const toggle=screen.getByRole("switch",{name:"保持界面后台运行"});
 expect(toggle.getAttribute("aria-checked")).toBe("true");
 await userEvent.setup().click(toggle);
 expect(change).toHaveBeenLastCalledWith({runAtLogon:false,startMinimized:true,keepUIRunning:false});
});
