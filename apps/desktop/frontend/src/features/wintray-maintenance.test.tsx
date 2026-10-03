// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {WinTrayMaintenance} from "./wintray-maintenance";

const call=vi.hoisted(()=>vi.fn());
vi.mock("../bridge",()=>({moduleCall:call}));
afterEach(()=>{cleanup();call.mockReset();});
const preview={configurationRevision:4,moduleRevision:7,programCount:3};

it("requires confirmation and submits the reviewed revisions once",async()=>{
  call.mockImplementation(async(method:string)=>method==="resetWinTray.preview"?preview:{state:"completed"});
  const changed=vi.fn();render(<WinTrayMaintenance onChanged={changed}/>);
  const user=userEvent.setup();
  await user.click(screen.getByRole("button",{name:"清空托管程序并还原"}));
  await screen.findByRole("dialog");
  expect(call).toHaveBeenCalledExactlyOnceWith("resetWinTray.preview");
  await user.click(screen.getByRole("button",{name:"取消"}));
  expect(call).toHaveBeenCalledTimes(1);
  await user.click(screen.getByRole("button",{name:"清空托管程序并还原"}));
  await user.click(await screen.findByRole("button",{name:"确认清空"}));
  expect(call).toHaveBeenLastCalledWith("resetWinTray",{configurationRevision:4,moduleRevision:7,confirmed:true});
  await screen.findByText("托管程序已清空，模块已关闭");
  expect(changed).toHaveBeenCalledOnce();
});

it("reports restore failure without claiming the program list was cleared",async()=>{
  call.mockImplementation(async(method:string)=>{if(method==="resetWinTray.preview")return preview;throw Error("窗口仍未恢复");});
  render(<WinTrayMaintenance onChanged={vi.fn()}/>);const user=userEvent.setup();
  await user.click(screen.getByRole("button",{name:"清空托管程序并还原"}));
  await user.click(await screen.findByRole("button",{name:"确认清空"}));
  await waitFor(()=>expect(screen.getByRole("alert").textContent).toContain("窗口仍未恢复"));
  expect(screen.queryByText("托管程序已清空，模块已关闭")).toBeNull();
});
