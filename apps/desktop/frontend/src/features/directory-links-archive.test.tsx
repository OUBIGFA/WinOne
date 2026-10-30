// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from "vitest";
import {cleanup,fireEvent,render,screen,waitFor} from "@testing-library/react";
import {DirectoryLinksArchive} from "./directory-links-archive";
import type {DirectoryLinkRecord} from "./directory-links";
const ports=vi.hoisted(()=>({call:vi.fn(),file:vi.fn(),directory:vi.fn()}));
vi.mock("../bridge",()=>({directoryLinksCall:ports.call,pickFile:ports.file,pickDirectory:ports.directory}));
const row:DirectoryLinkRecord={id:"local-observation",name:"文档",linkPath:"C:\\original",targetPath:"D:\\documents",kind:"junction",objectType:"directory",targetText:"D:\\documents",relative:false,createdAt:"",checkedAt:"2026-10-26T00:00:00Z",status:"active",reason:"target-accessible",accessible:true};
beforeEach(()=>{ports.file.mockResolvedValue("C:\\backup.json");ports.directory.mockResolvedValue("C:\\backup-folder");});
afterEach(()=>{cleanup();vi.resetAllMocks();});
it("exports only selected records after a destination is selected",async()=>{
 ports.call.mockResolvedValue({path:"C:\\backup-folder\\WinOne-links-123.json"});
 render(<DirectoryLinksArchive ids={["selected"]} onChanged={()=>{}}/>);
 fireEvent.click(screen.getByRole("button",{name:"导出选中记录"}));
 await screen.findByText(/历史备份已保存/);
 expect(ports.call).toHaveBeenCalledWith("history.export",{ids:["selected"],directory:"C:\\backup-folder"});
});
it("does not export all records when none are selected, and picker cancellation makes no request",async()=>{
 ports.file.mockResolvedValue("");
 render(<DirectoryLinksArchive ids={[]} onChanged={()=>{}}/>);
 expect((screen.getByRole("button",{name:"导出选中记录"}) as HTMLButtonElement).disabled).toBe(true);
 fireEvent.click(screen.getByRole("button",{name:"导入历史备份"}));
 await waitFor(()=>expect(ports.file).toHaveBeenCalled());
 expect(ports.call).not.toHaveBeenCalled();
});
it("previews verified and skipped records without importing until confirmation",async()=>{
 const changed=vi.fn(),activity=vi.fn();
 ports.call.mockImplementation(async(method:string)=>method==="history.preview"?{id:"single-use",records:[row],issues:[{path:"C:\\replaced",error:"入口不是链接"}]}:[row]);
 render(<DirectoryLinksArchive ids={[]} onChanged={changed} onActivity={activity}/>);
 fireEvent.click(screen.getByRole("button",{name:"导入历史备份"}));
 await screen.findByText("可导入 1 条，跳过 1 条。");
 expect(ports.call).toHaveBeenCalledTimes(1);expect(changed).not.toHaveBeenCalled();
 expect(activity.mock.calls.some(([dirty])=>dirty===true)).toBe(true);
 fireEvent.click(screen.getByRole("button",{name:"确认导入记录"}));
 await screen.findByText(/已导入 1 条记录/);
 expect(ports.call).toHaveBeenLastCalledWith("history.import",{token:"single-use",confirmed:true});
 expect(changed).toHaveBeenCalledWith([row]);
 await waitFor(()=>expect(screen.queryByRole("dialog")).toBeNull());
});
it("cancel and all-skipped previews never commit",async()=>{
 ports.call.mockResolvedValue({records:[],issues:[{path:"C:\\unknown",error:"无法读取"}]});
 render(<DirectoryLinksArchive ids={[]} onChanged={()=>{}}/>);
 fireEvent.click(screen.getByRole("button",{name:"导入历史备份"}));
 await screen.findByText("可导入 0 条，跳过 1 条。");
 expect((screen.getByRole("button",{name:"确认导入记录"}) as HTMLButtonElement).disabled).toBe(true);
 fireEvent.click(screen.getByRole("button",{name:"取消"}));
 expect(ports.call).toHaveBeenCalledTimes(1);
});
it("failed commit reports the error and requires a new preview rather than reusing the token",async()=>{
 const changed=vi.fn();
 ports.call.mockImplementation(async(method:string)=>{if(method==="history.preview")return {id:"single-use",records:[row],issues:[]};throw Error("链接在预览后发生变化，请重新预览");});
 render(<DirectoryLinksArchive ids={[]} onChanged={changed}/>);
 fireEvent.click(screen.getByRole("button",{name:"导入历史备份"}));
 await screen.findByText("可导入 1 条，跳过 0 条。");
 fireEvent.click(screen.getByRole("button",{name:"确认导入记录"}));
 await screen.findByText(/链接在预览后发生变化/);
 expect(changed).not.toHaveBeenCalled();await waitFor(()=>expect(screen.queryByRole("dialog")).toBeNull());
 expect((screen.getByRole("button",{name:"导入历史备份"}) as HTMLButtonElement).disabled).toBe(false);
});
