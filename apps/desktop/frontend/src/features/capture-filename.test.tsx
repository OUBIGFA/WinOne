// @vitest-environment jsdom
import {useState} from "react";
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {CaptureFileName} from "./capture-filename";

const call=vi.hoisted(()=>vi.fn());
vi.mock("../bridge",()=>({captureCall:call}));
afterEach(()=>{cleanup();call.mockReset();});
function Preview(){const [value,setValue]=useState("");return <CaptureFileName value={value} onChange={setValue}/>;}
it("previews a custom name without saving settings",async()=>{
 call.mockResolvedValue({name:"工作-20261001-01.png"});
 render(<Preview/>);expect(call).not.toHaveBeenCalled();
 const user=userEvent.setup();await user.click(screen.getByRole("textbox",{name:"文件名模板"}));
 await user.paste("工作-{date}-{sequence}");
 await screen.findByText("示例: 工作-20261001-01.png");
 expect(call).toHaveBeenLastCalledWith("filename.preview",{template:"工作-{date}-{sequence}"});
 expect(call.mock.calls.every(([method])=>method==="filename.preview")).toBe(true);
});
it("keeps an invalid template visible with an error for correction",async()=>{
 call.mockRejectedValue(Error("文件名模板无效"));render(<Preview/>);
 const input=screen.getByRole("textbox",{name:"文件名模板"});
 await userEvent.setup().type(input,"../outside");
 await waitFor(()=>expect(screen.getByRole("alert").textContent).toContain("文件名模板无效"));
 expect((input as HTMLInputElement).value).toBe("../outside");
 expect(input.getAttribute("aria-invalid")).toBe("true");
});

it.each([null,{}, {name:""}, {name:" "}, {name:42}, {name:{value:"invalid"}}])("reports a malformed filename response without crashing or losing the template: %j",async response=>{
 call.mockResolvedValue(response);render(<Preview/>);
 const input=screen.getByRole("textbox",{name:"文件名模板"});
 const user=userEvent.setup();await user.click(input);await user.paste("report-{date}");
 await waitFor(()=>expect(screen.getByRole("alert").textContent).toBe("文件名生成失败，请重试"));
 expect((input as HTMLInputElement).value).toBe("report-{date}");
 expect(screen.queryByText("截图加载失败")).toBeNull();
});

it("retries the same filename template without asking the user to retype it",async()=>{
 call.mockResolvedValue({});render(<Preview/>);
 const user=userEvent.setup(),input=screen.getByRole("textbox",{name:"文件名模板"});
 await user.click(input);await user.paste("report-{date}");await screen.findByRole("alert");
 const previousCalls=call.mock.calls.length;call.mockResolvedValue({name:"report-20261001.png"});
 await user.click(screen.getByRole("button",{name:"重试"}));
 await screen.findByText("示例: report-20261001.png");
 expect(call).toHaveBeenCalledTimes(previousCalls+1);
 expect(call).toHaveBeenLastCalledWith("filename.preview",{template:"report-{date}"});
 expect((input as HTMLInputElement).value).toBe("report-{date}");expect(document.activeElement).toBe(input);
});
