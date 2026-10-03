// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {WindowTitlebar} from "./window-titlebar";
import {TooltipProvider} from "./ui/tooltip";
afterEach(cleanup);
it("labels native window actions and updates maximize to restore",async()=>{
 let maximized=false;const command=vi.fn(async(action:string)=>{if(action==="toggle-maximize")maximized=!maximized;});
 const port={initialize:async()=>({maximized}),read:async()=>({maximized}),command};
 render(<TooltipProvider><WindowTitlebar logo="/logo.png" port={port}/></TooltipProvider>);const user=userEvent.setup();
 await user.click(await screen.findByRole("button",{name:"最大化窗口"}));await screen.findByRole("button",{name:"还原窗口"});
 await user.click(screen.getByRole("button",{name:"关闭窗口"}));expect(command).toHaveBeenCalledWith("close");
 await user.click(screen.getByRole("button",{name:"最小化窗口"}));expect(command).toHaveBeenCalledWith("minimize");
});
it("does not pretend that browser preview controls operate a native window",()=>{
 render(<TooltipProvider><WindowTitlebar logo="/logo.png"/></TooltipProvider>);
 expect((screen.getByRole("button",{name:"关闭窗口"}) as HTMLButtonElement).disabled).toBe(true);
});
