// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {Updates} from "./updates";
const native=vi.hoisted(()=>({info:vi.fn(),check:vi.fn(),cancel:vi.fn(),open:vi.fn()}));
vi.mock("../bridge",()=>({updateInfo:native.info,checkUpdates:native.check,cancelUpdateCheck:native.cancel,openUpdateRelease:native.open}));
afterEach(()=>{cleanup();vi.resetAllMocks();});
it("shows an unconfigured source without checking the network",async()=>{
 native.info.mockResolvedValue({version:"1.0.0",configured:false});render(<Updates/>);
 await screen.findByText("当前版本：1.0.0");expect((screen.getByRole("button",{name:"检查更新"}) as HTMLButtonElement).disabled).toBe(true);
 expect(native.check).not.toHaveBeenCalled();expect(screen.queryByText("已是最新版")).toBeNull();
});
it("checks once and opens only the returned release action",async()=>{
 native.open.mockResolvedValue(undefined);
 native.info.mockResolvedValue({version:"1.0.0",configured:true});native.check.mockResolvedValue({state:"available",latestVersion:"1.1.0"});
 render(<Updates/>);await screen.findByText("当前版本：1.0.0");const user=userEvent.setup();await user.click(screen.getByRole("button",{name:"检查更新"}));
 await screen.findByText("发现新版本：1.1.0");await user.click(screen.getByRole("button",{name:"查看发行说明"}));expect(native.open).toHaveBeenCalledOnce();expect(native.check).toHaveBeenCalledOnce();
});
it("keeps errors distinct from successful checks",async()=>{
 native.info.mockResolvedValue({version:"1.0.0",configured:true});native.check.mockResolvedValue({state:"failed",errorCode:"network"});
 render(<Updates/>);await screen.findByText("当前版本：1.0.0");await userEvent.setup().click(screen.getByRole("button",{name:"检查更新"}));
 await screen.findByRole("alert");expect(screen.queryByText("已是最新版")).toBeNull();
});
it("cancels only its pending request when leaving",async()=>{
 native.info.mockResolvedValue({version:"1.0.0",configured:true});native.check.mockImplementation(()=>new Promise(()=>{}));native.cancel.mockResolvedValue(undefined);
 const view=render(<Updates/>);await screen.findByText("当前版本：1.0.0");await userEvent.setup().click(screen.getByRole("button",{name:"检查更新"}));
 await waitFor(()=>expect(native.check).toHaveBeenCalledOnce());view.unmount();expect(native.cancel).toHaveBeenCalledWith(native.check.mock.calls[0][0]);
});

it("can cancel a pending check and retry without claiming success",async()=>{
 let finish!:(value:unknown)=>void;
 native.info.mockResolvedValue({version:"1.0.0",configured:true});
 native.check.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve;})).mockResolvedValue({state:"up_to_date"});
 native.cancel.mockImplementation(async()=>finish({state:"failed",errorCode:"cancelled"}));
 render(<Updates/>);await screen.findByText("当前版本：1.0.0");const user=userEvent.setup();
 await user.click(screen.getByRole("button",{name:"检查更新"}));await user.click(screen.getByRole("button",{name:"取消检查"}));
 await screen.findByText("已取消检查");expect(screen.queryByText("已是最新版")).toBeNull();
 await user.click(screen.getByRole("button",{name:"检查更新"}));await screen.findByText("已是最新版");expect(native.check).toHaveBeenCalledTimes(2);
});
