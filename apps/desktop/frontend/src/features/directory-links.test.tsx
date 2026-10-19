// @vitest-environment jsdom
import {beforeEach,afterEach,expect,it,vi} from "vitest";
import {cleanup,fireEvent,render,screen,waitFor,within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {DirectoryLinksPage,type DirectoryLinkRecord} from "./directory-links";
import {i18n,setApplicationLanguage} from "../i18n";
import {directoryLinksMessages} from "../locales/directory-links";

const ports=vi.hoisted(()=>({call:vi.fn(),file:vi.fn(),folder:vi.fn(),files:vi.fn(),folders:vi.fn(),drop:vi.fn(async()=>{})}));
vi.mock("../bridge",()=>({directoryLinksCall:ports.call,pickFile:ports.file,pickDirectory:ports.folder,pickFiles:ports.files,pickDirectories:ports.folders,setFileDropTarget:ports.drop}));
const row:DirectoryLinkRecord={id:"r1",name:"Source alias",linkPath:"C:\\links\\source.txt",targetPath:"C:\\source.txt",kind:"hardlink",objectType:"file",targetText:"C:\\source.txt",relative:false,createdAt:"2026-10-10T10:00:00Z",checkedAt:"2026-10-10T10:05:00Z",status:"active",reason:"same-file",accessible:true,linkCount:2,entryIdentity:{file:"known"}};
const metadata=(method:string,value:unknown)=>method==="menu.get"?{enabled:false,partial:false}:method==="pending"?{requested:false,paths:[]}:method==="browse"?{directory:"C:\\links",parent:"C:\\",entries:[],total:0,offset:0,hasMore:false}:value;
beforeEach(async()=>{i18n.addResourceBundle("zh-CN","translation",{links:directoryLinksMessages.zh},true,true);i18n.addResourceBundle("en-US","translation",{links:directoryLinksMessages.en},true,true);await setApplicationLanguage("zh-CN");ports.call.mockImplementation(async(method:string)=>metadata(method,[]));ports.file.mockResolvedValue("C:\\source.txt");ports.folder.mockResolvedValue("C:\\links");ports.files.mockResolvedValue(["C:\\one.txt","C:\\two.txt"]);ports.folders.mockResolvedValue(["C:\\folder"]);});
afterEach(()=>{cleanup();Object.values(ports).forEach(port=>port.mockReset());});

it("shows link history only inside the find-existing tab",async()=>{
 ports.call.mockImplementation(async(method:string)=>metadata(method,[row]));
 render(<DirectoryLinksPage/>);await waitFor(()=>expect(ports.call).toHaveBeenCalledWith("list"));
 expect(screen.queryByRole("region",{name:"链接历史"})).toBeNull();
 const user=userEvent.setup();await user.click(screen.getByRole("tab",{name:"查找已有链接"}));
 const scan=screen.getByRole("tabpanel",{name:"查找已有链接"});
 expect(within(scan).getByRole("region",{name:"链接历史"})).toBeTruthy();expect(within(scan).getByText("Source alias")).toBeTruthy();
 expect(document.querySelector('.directory-links-history-group')?.hasAttribute('open')).toBe(false);
 await user.click(screen.getByRole("tab",{name:"新建链接"}));expect(screen.queryByRole("region",{name:"链接历史"})).toBeNull();
});

it("creates a single link directly from the merged create view",async()=>{
 ports.call.mockImplementation(async(method:string)=>metadata(method,method==="previewBatch"?{id:"batch-1",items:[{id:"p1",source:"C:\\one.txt",linkPath:"C:\\links\\one.txt",kind:"hardlink",objectType:"file"}]}:method==="createBatch"?{records:[row],completed:1,total:1}:[]));
 render(<DirectoryLinksPage/>);await waitFor(()=>expect(ports.call).toHaveBeenCalledWith("list"));
 fireEvent(window,new CustomEvent("winone:directory-links-drop",{detail:{paths:["C:\\one.txt"]}}));
 const batch=screen.getByRole("tabpanel",{name:"新建链接"});
 fireEvent.change(within(batch).getByLabelText("链接存放文件夹"),{target:{value:"C:\\links"}});
 const user=userEvent.setup();await user.click(within(batch).getByRole("button",{name:"创建链接"}));
 await screen.findByText("已核查生效 1 / 1 项。未完成项的原因保留在历史中。");
});

it("shows a readable lone hard-link entry without an active relationship badge",async()=>{
 const unlinked={...row,status:"unlinked",reason:"source-missing-no-links",linkCount:1};ports.call.mockImplementation(async(method:string)=>metadata(method,[unlinked]));
 render(<DirectoryLinksPage/>);await userEvent.setup().click(screen.getByRole("tab",{name:"查找已有链接"}));await screen.findByText("数据可用 · 原关联已消失");
 expect(screen.queryByText("生效",{exact:true})).toBeNull();expect(screen.getByText("只剩一个目录项，原硬链接关联已消失。")).toBeTruthy();expect(screen.getByText(/最后检查/)).toBeTruthy();
});

it("requires removal confirmation and sends only the chosen record identity",async()=>{
 ports.call.mockImplementation(async(method:string)=>metadata(method,method==="remove"?{...row,status:"removed",reason:"recycled"}:[row]));
 render(<DirectoryLinksPage/>);const user=userEvent.setup();await user.click(screen.getByRole("tab",{name:"查找已有链接"}));await screen.findByText("Source alias");
 await user.click(screen.getByText("source.txt",{selector:".directory-links-group-target > span:first-child"}));
 await waitFor(()=>expect((screen.getByRole("button",{name:"移除链接入口"}) as HTMLButtonElement).disabled).toBe(false));await user.click(screen.getByRole("button",{name:"移除链接入口"}));
 const dialog=await screen.findByRole("dialog");expect(ports.call.mock.calls.some(call=>call[0]==="remove")).toBe(false);
 expect(within(dialog).getByText(row.linkPath)).toBeTruthy();await user.click(within(dialog).getByRole("button",{name:"移到回收站"}));
 await waitFor(()=>expect(ports.call).toHaveBeenCalledWith("remove",{id:"r1",confirmed:true,checkedAt:row.checkedAt}));await screen.findByText("已回收",{exact:true});
});

it("preserves multiple native dropped paths and reports a partial batch result",async()=>{
 ports.call.mockImplementation(async(method:string)=>metadata(method,method==="previewBatch"?{id:"batch-1",items:[{id:"p1",source:"C:\\one.txt",linkPath:"C:\\links\\one.txt",kind:"hardlink",objectType:"file"},{id:"p2",source:"C:\\two.txt",linkPath:"C:\\links\\two.txt",kind:"hardlink",objectType:"file"}]}:method==="createBatch"?{records:[row,{...row,id:"r2",name:"two",status:"failed",reason:"not-attempted"}],completed:1,total:2}:[]));
 render(<DirectoryLinksPage/>);await waitFor(()=>expect(ports.call).toHaveBeenCalledWith("list"));
 fireEvent(window,new CustomEvent("winone:directory-links-drop",{detail:{paths:["C:\\one.txt","C:\\two.txt"]}}));
 const batch=screen.getByRole("tabpanel",{name:"新建链接"});
 const left=within(batch).getByRole("region",{name:"源文件夹"});await within(left).findByText("one.txt");await within(left).findByText("two.txt");
 fireEvent.change(within(batch).getByLabelText("链接存放文件夹"),{target:{value:"C:\\links"}});
 const user=userEvent.setup();await user.click(within(batch).getByRole("button",{name:"创建链接"}));
 await screen.findByText("已核查生效 1 / 2 项。未完成项的原因保留在历史中。");
});

it("queues new dropped sources until an active batch has cleared its completed input",async()=>{
 let finish!:(value:unknown)=>void;
 ports.call.mockImplementation(async(method:string)=>{
  if(method==="createBatch")return new Promise(resolve=>{finish=resolve;});
  return metadata(method,method==="previewBatch"?{id:"batch-1",items:[{id:"p1",source:"C:\\first.txt",linkPath:"C:\\links\\first.txt",kind:"hardlink",objectType:"file"}]}:[]);
 });
 render(<DirectoryLinksPage/>);await waitFor(()=>expect(ports.call).toHaveBeenCalledWith("list"));
 fireEvent(window,new CustomEvent("winone:directory-links-drop",{detail:{paths:["C:\\first.txt"]}}));
 const batch=await screen.findByRole("tabpanel",{name:"新建链接"});const user=userEvent.setup();
 fireEvent.change(within(batch).getByLabelText("链接存放文件夹"),{target:{value:"C:\\links"}});
 await user.click(within(batch).getByRole("button",{name:"创建链接"}));
 await waitFor(()=>expect(finish).toBeTypeOf("function"));
 fireEvent(window,new CustomEvent("winone:directory-links-drop",{detail:{paths:["C:\\next.txt"]}}));
 finish({records:[row],completed:1,total:1});
 const left=within(batch).getByRole("region",{name:"源文件夹"});
 await within(left).findByText("next.txt");
});
