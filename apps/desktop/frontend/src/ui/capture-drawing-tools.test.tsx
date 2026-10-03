// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {Square,Circle} from "lucide-react";
import {CaptureDrawingTools} from "./capture-drawing-tools";

afterEach(cleanup);
const items=[{kind:"rectangle",icon:Square},{kind:"ellipse",icon:Circle}];
async function customize(){
 const user=userEvent.setup();
 await user.click(screen.getByRole("button",{name:"自定义工具栏"}));
 await waitFor(()=>expect(screen.getByRole("menuitemcheckbox",{name:"椭圆"}).getAttribute("aria-disabled")).not.toBe("true"));
 return user;
}
it("shows all tools by default and exposes customization directly",async()=>{
 render(<CaptureDrawingTools items={items} selected="rectangle" disabled={false} onSelect={()=>{}}/>);
 expect(screen.getByRole("button",{name:"矩形"})).toBeTruthy();
 expect(screen.getByRole("button",{name:"椭圆"})).toBeTruthy();
 expect(screen.queryByRole("button",{name:"更多工具"})).toBeNull();
 await customize();
 expect(screen.getByRole("menuitemcheckbox",{name:"椭圆"}).getAttribute("aria-checked")).toBe("true");
});
it("keeps saved hidden tools in customization",async()=>{
 const port={read:async()=>["rectangle"],save:vi.fn(async()=>{})};
 render(<CaptureDrawingTools items={items} selected="ellipse" disabled={false} onSelect={()=>{}} port={port}/>);
 await customize();
 expect(screen.queryByRole("button",{name:"椭圆"})).toBeNull();
 expect(screen.getByRole("menuitemcheckbox",{name:"椭圆"}).getAttribute("aria-checked")).toBe("false");
 expect(port.save).not.toHaveBeenCalled();
});
it("pins through the menu and restores the preference on reopening",async()=>{
 let stored=["rectangle"];
 const port={read:async()=>stored,save:vi.fn(async(tools:string[])=>{stored=tools;})};
 const props={items,selected:"rectangle",disabled:false,onSelect:vi.fn(),port};
 const view=render(<CaptureDrawingTools {...props}/>);
 const user=await customize();
 (await screen.findByRole("menuitemcheckbox",{name:"椭圆"})).focus();await user.keyboard("{Enter}");
 await waitFor(()=>expect(port.save).toHaveBeenCalledWith(["rectangle","ellipse"]));
 view.unmount();render(<CaptureDrawingTools {...props}/>);
 expect(await screen.findByRole("button",{name:"椭圆"})).toBeTruthy();
});
it("preserves saved preferences when pinning fails",async()=>{
 const port={read:async()=>["rectangle"],save:async()=>{throw Error("Save failed");}};
 render(<CaptureDrawingTools items={items} selected="rectangle" disabled={false} onSelect={()=>{}} port={port}/>);
 const user=await customize();
 (await screen.findByRole("menuitemcheckbox",{name:"椭圆"})).focus();await user.keyboard("{Enter}");
 expect(await screen.findByRole("alert")).toBeTruthy();
 expect(screen.queryByRole("button",{name:"椭圆"})).toBeNull();
});
