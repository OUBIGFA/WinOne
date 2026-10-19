// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,fireEvent,render,screen,waitFor} from "@testing-library/react";
import {DirectoryLinksPage,groupLinkHistory,type DirectoryLinkRecord} from "./directory-links";
const ports=vi.hoisted(()=>({call:vi.fn()}));
vi.mock("../bridge",()=>({directoryLinksCall:ports.call,setFileDropTarget:vi.fn(),pickDirectory:vi.fn()}));
afterEach(()=>{cleanup();vi.resetAllMocks();});
const record=(id:string,targetPath:string,status="active",objectType:"file"|"directory"="file"):DirectoryLinkRecord=>({id,name:id,linkPath:`C:\\links\\${id}`,targetPath,kind:"hardlink",objectType,targetText:targetPath,relative:false,createdAt:"2026-10-16T10:00:00Z",checkedAt:"2026-10-16T11:00:00Z",status,reason:"same-file",accessible:true});
it("groups normalized targets with folders first and numeric path order",()=>{
 const rows=[record("a","C:/docs/part10.txt"),record("b","c:\\docs\\part2.txt"),record("c","C:\\DOCS\\part2.txt"),record("d","C:\\folder", "active","directory")];
 const groups=groupLinkHistory(rows);
 expect(groups.map(group=>group.records.map(row=>row.id))).toEqual([["d"],["b","c"],["a"]]);
});
it("collapses each target independently and clears only history using the freshly checked host result",async()=>{
 const active=record("valid","C:\\valid.txt"),broken=record("invalid","C:\\invalid.txt","broken"),offline=record("offline","C:\\offline.txt","unavailable");
 ports.call.mockImplementation(async(method:string)=>method==="pending"?{requested:false}:method==="menu.get"?{enabled:false}:method==="browse"?{directory:"C:\\",entries:[]}:method==="forgetInvalid"?{records:[active,offline],forgotten:1}:[active,broken,offline]);
 render(<DirectoryLinksPage/>);
 fireEvent.click(screen.getByRole("tab",{name:"查找已有链接"}));
 await waitFor(()=>expect(document.querySelectorAll(".directory-links-history-group").length).toBe(3));
 expect(document.querySelectorAll(".directory-links-history-group[open]").length).toBe(0);
 fireEvent.click(screen.getByText("invalid.txt",{selector:".directory-links-group-target > span:first-child"}));
 expect(document.querySelectorAll(".directory-links-history-group[open]").length).toBe(1);
 expect(screen.getByText("invalid",{selector:".directory-links-record-name"}).closest("article")?.getAttribute("data-invalid")).toBe("true");
 fireEvent.click(screen.getByRole("button",{name:"一键删除失效历史"}));
 await screen.findByText("已删除 1 条失效历史，真实文件和链接未改变。");
 expect(ports.call).toHaveBeenCalledWith("forgetInvalid",{confirmed:true});
 expect(screen.queryByText("invalid",{selector:".directory-links-record-name"})).toBeNull();
 expect(screen.getByText("offline",{selector:".directory-links-record-name"})).toBeTruthy();
 expect(ports.call.mock.calls.some(([method])=>method==="remove"||method==="forget")).toBe(false);
});
