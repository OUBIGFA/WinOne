// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Profiles } from "./profiles";
import { useState } from "react";
import { usePageNavigation } from "../ui/page-navigation";

const exportFile=vi.hoisted(()=>vi.fn());
vi.mock("../bridge",()=>({exportProfile:exportFile,pickProfile:vi.fn(),pickProfileLocation:vi.fn(),profileCall:vi.fn()}));
afterEach(()=>{cleanup();exportFile.mockReset();});
it("keeps the export page until writing finishes",async()=>{
 let finish!:(path:string)=>void;exportFile.mockImplementation(()=>new Promise<string>(resolve=>{finish=resolve;}));
 function Navigation(){const [page,setPage]=useState("settings");const navigation=usePageNavigation(page,setPage);return <><button onClick={()=>navigation.requestPage("home")}>首页</button>{page==="settings"?<Profiles onActivity={navigation.onActivity}/>:<p>工具首页</p>}{navigation.dialog}</>;}
 render(<Navigation/>);const user=userEvent.setup();await user.click(screen.getByRole("button",{name:"导出配置"}));await user.click(screen.getByRole("button",{name:"首页"}));
 expect((await screen.findByRole("dialog")).textContent).toContain("操作尚未完成");
 expect(screen.queryByRole("button",{name:"放弃并离开"})).toBeNull();
 finish("C:\\Backup\\WinOne.winone");await screen.findByText("工具首页");
});
it("reports export only after a file is written",async()=>{
 let finish!:(path:string)=>void;
 exportFile.mockImplementation(()=>new Promise<string>(resolve=>{finish=resolve;}));
 render(<Profiles/>);
 await userEvent.setup().click(screen.getByRole("button",{name:"导出配置"}));
 expect(screen.queryByText(/已导出/)).toBeNull();
 finish("C:\\Backup\\WinOne.winone");
 expect(await screen.findByText(/已导出/)).toBeTruthy();
});
it("does not claim success after cancelling the save dialog",async()=>{
 exportFile.mockResolvedValue("");
 render(<Profiles/>);
 await userEvent.setup().click(screen.getByRole("button",{name:"导出配置"}));
 expect(screen.queryByText(/已导出/)).toBeNull();
});
