// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {UsageAliases} from "./ai-usage-aliases";

const call=vi.hoisted(()=>vi.fn());
vi.mock("../bridge",()=>({captureCall:call}));
afterEach(()=>{cleanup();call.mockReset();});

it("validates the full alias draft and leaves persistence to the shared Save action",async()=>{
 const change=vi.fn(),user=userEvent.setup();
 call.mockResolvedValue({usageAliases:{old:"new","new-model":"combined-model"}});
 render(<UsageAliases aliases={{old:"new"}} onChange={change}/>);
 await user.type(screen.getByRole("textbox",{name:"原模型标识"})," new-model");
 await user.type(screen.getByRole("textbox",{name:"归并名称"}),"combined-model");
 await user.click(screen.getByRole("button",{name:"应用归并"}));
 expect(call).toHaveBeenCalledWith("ai.magpie.usage-aliases",{aliases:{old:"new"},edit:{from:" new-model",to:"combined-model"}});
 expect(change).toHaveBeenCalledWith({old:"new","new-model":"combined-model"});
 expect(call).toHaveBeenCalledTimes(1);
 expect((screen.getByRole("textbox",{name:"原模型标识"}) as HTMLInputElement).value).toBe("");
 expect(screen.getByText(/点击.*保存.*生效/)).toBeTruthy();
});

it("retains the form and existing aliases when upstream validation fails",async()=>{
 const change=vi.fn(),user=userEvent.setup();call.mockRejectedValue(Error("归并关系形成循环"));
 render(<UsageAliases aliases={{old:"new"}} onChange={change}/>);
 await user.type(screen.getByRole("textbox",{name:"原模型标识"}),"new");
 await user.type(screen.getByRole("textbox",{name:"归并名称"}),"old");
 await user.click(screen.getByRole("button",{name:"应用归并"}));
 expect((await screen.findByRole("alert")).textContent).toContain("归并关系形成循环");
 expect(change).not.toHaveBeenCalled();
 expect((screen.getByRole("textbox",{name:"原模型标识"}) as HTMLInputElement).value).toBe("new");
 expect(screen.getByRole("button",{name:"移除 old 的归并"})).toBeTruthy();
});

it("removes via the same draft validation and respects the page lock",async()=>{
 const change=vi.fn(),user=userEvent.setup();call.mockResolvedValue({usageAliases:{}});
 const view=render(<UsageAliases aliases={{old:"new"}} onChange={change} disabled/>);
 await user.click(screen.getByRole("button",{name:"移除 old 的归并"}));expect(call).not.toHaveBeenCalled();
 view.rerender(<UsageAliases aliases={{old:"new"}} onChange={change}/>);
 await user.click(screen.getByRole("button",{name:"移除 old 的归并"}));
 expect(call).toHaveBeenCalledWith("ai.magpie.usage-aliases",{aliases:{old:"new"},edit:{from:"old",to:""}});
 expect(change).toHaveBeenCalledWith({});
});
