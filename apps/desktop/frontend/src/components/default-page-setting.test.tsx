// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {Home,ScanLine,Settings} from "lucide-react";
import {DefaultPageSetting} from "./default-page-setting";

afterEach(cleanup);
const pages=[
 {id:"overview",title:"首页",icon:Home},
 {id:"capture",title:"截图助手",icon:ScanLine},
 {id:"settings",title:"设置",icon:Settings},
 {id:"extension",title:"新加入的模块"},
];

it("uses the complete navigation catalog and preserves a chosen page after a fresh render",async()=>{
 const change=vi.fn(),user=userEvent.setup();
 const view=render(<DefaultPageSetting value="settings" pages={pages} disabled={false} onChange={change}/>);
 const trigger=screen.getByRole("combobox",{name:"起始页面"});
 expect(trigger.textContent).toContain("设置");
 await user.click(screen.getByText("起始页面"));
 expect(screen.getAllByRole("option").map(option=>option.textContent)).toEqual(pages.map(page=>page.title));
 await user.click(screen.getByRole("option",{name:"新加入的模块"}));
 expect(change).toHaveBeenCalledExactlyOnceWith("extension");
 view.unmount();
 render(<DefaultPageSetting value="extension" pages={pages} disabled={false} onChange={change}/>);
 expect(screen.getByRole("combobox",{name:"起始页面"}).textContent).toContain("新加入的模块");
});

it("retains the full selected name for a narrow trigger",()=>{
 const title="名称很长的新模块".repeat(12);
 render(<DefaultPageSetting value="long" pages={[...pages,{id:"long",title}]} disabled={false} onChange={vi.fn()}/>);
 expect(screen.getByRole("combobox",{name:"起始页面"}).getAttribute("title")).toBe(title);
});

it("does not open or change the selection while settings are unavailable",async()=>{
 const change=vi.fn(),user=userEvent.setup();
 render(<DefaultPageSetting value="settings" pages={pages} disabled onChange={change}/>);
 await user.click(screen.getByText("起始页面"));
 expect(screen.queryByRole("listbox")).toBeNull();
 expect(change).not.toHaveBeenCalled();
});
