// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from "vitest";
import {cleanup,fireEvent,render,screen,waitFor,within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {DirectoryLinksBatch} from "./directory-links-batch";
import {DirectoryLinksPage} from "./directory-links";
import {i18n,setApplicationLanguage} from "../i18n";
import {directoryLinksMessages} from "../locales/directory-links";

const ports=vi.hoisted(()=>({call:vi.fn(),folder:vi.fn(),files:vi.fn(),folders:vi.fn(),drop:vi.fn()}));
vi.mock("../bridge",()=>({directoryLinksCall:ports.call,pickDirectory:ports.folder,pickFile:vi.fn(),pickFiles:ports.files,pickDirectories:ports.folders,setFileDropTarget:ports.drop}));
const source="C:\\source",destination="C:\\links";
const identity=(file:string)=>({volume:1,file,created:1});
const file={name:"说明.txt",path:source+"\\说明.txt",objectType:"file",identity:identity("file")};
const folder={name:"项目",path:source+"\\项目",objectType:"directory",identity:identity("folder")};
const listing=(directory:string,entries:unknown[])=>({directory,identity:identity(directory),parent:directory==="C:\\"?"":"C:\\",entries,total:entries.length,offset:0,hasMore:false});
beforeEach(async()=>{
 i18n.addResourceBundle("zh-CN","translation",{links:directoryLinksMessages.zh},true,true);
 await setApplicationLanguage("zh-CN");
 ports.folder.mockResolvedValue(source);ports.files.mockResolvedValue([]);ports.folders.mockResolvedValue([]);ports.drop.mockResolvedValue(undefined);
 ports.call.mockImplementation(async(method:string,args:{directory?:string;sources?:string[]})=>{
  if(method==="browse")return listing(args.directory!,args.directory===source?[folder,file]:[]);
  if(method==="previewBatch")return {id:"plan-1",items:args.sources!.map((path,index)=>({id:String(index),source:path,linkPath:destination+"\\"+path.split("\\").at(-1),objectType:path===folder.path?"directory":"file",kind:path===folder.path?"junction":"hardlink",targetText:path}))};
  return [];
 });
});

it("routes native drops by pane, preserves mixed source paths, and keeps the destination on invalid drops",async()=>{
 const defaultCall=ports.call.getMockImplementation()!;
 ports.call.mockImplementation(async(method:string,args:any)=>{
  if(method==="browse"&&args.directory===file.path)throw new Error("请选择需要浏览的文件夹");
  return defaultCall(method,args);
 });
 render(<DirectoryLinksPage/>);await waitFor(()=>expect(ports.call).toHaveBeenCalledWith("list"));
 const target=screen.getByRole("region",{name:"目标文件夹"});
 const targetAddress=within(target).getByRole("textbox");
 fireEvent.change(targetAddress,{target:{value:destination}});fireEvent.keyDown(targetAddress,{key:"Enter"});
 await waitFor(()=>expect(ports.call).toHaveBeenCalledWith("browse",{directory:destination,offset:0}));
 vi.spyOn(target,"getBoundingClientRect").mockReturnValue({x:500,y:0,left:500,right:1000,top:0,bottom:500,width:500,height:500,toJSON(){return {};}});
 const drop=(paths:string[],position?:{x:number;y:number})=>fireEvent(window,new CustomEvent("winone:directory-links-drop",{detail:{paths,position}}));
 drop([file.path,folder.path]);
 const left=within(screen.getByRole("region",{name:"源文件夹"}));
 expect(left.getByRole("checkbox",{name:"选择来源：说明.txt"}).getAttribute("aria-checked")).toBe("true");
 expect(left.getByRole("checkbox",{name:"选择来源：项目"}).getAttribute("aria-checked")).toBe("true");
 drop(["C:\\new-target"],{x:750,y:180});
 await waitFor(()=>expect((within(target).getByLabelText("链接存放文件夹") as HTMLInputElement).value).toBe("C:\\new-target"));
 drop([file.path],{x:750,y:180});
 await screen.findByText(/右侧只接收文件夹/);
 expect((within(target).getByLabelText("链接存放文件夹") as HTMLInputElement).value).toBe("C:\\new-target");
 drop([source,destination],{x:750,y:180});
 await screen.findByText(/右侧每次只接收一个目标文件夹/);
 expect((within(target).getByLabelText("链接存放文件夹") as HTMLInputElement).value).toBe("C:\\new-target");
 expect(ports.call.mock.calls.some(([method])=>String(method).startsWith("create"))).toBe(false);
});
afterEach(()=>{cleanup();Object.values(ports).forEach(port=>port.mockReset());});

async function openBoth(){
 const user=userEvent.setup();
 await user.click(screen.getByRole("button",{name:"浏览源文件夹"}));
 await screen.findByRole("checkbox",{name:"选择来源：说明.txt"});
 ports.folder.mockResolvedValue(destination);
 await user.click(screen.getByRole("button",{name:"选择链接位置"}));
 await waitFor(()=>expect(ports.call).toHaveBeenCalledWith("browse",{directory:destination,offset:0}));
 return user;
}

it("browses folders, selects files and folders, and shows their destination before creation",async()=>{
 render(<DirectoryLinksBatch onChanged={()=>{}}/>);
 const user=await openBoth();
 await user.click(screen.getByRole("checkbox",{name:"选择来源：说明.txt"}));
 await user.click(screen.getByRole("checkbox",{name:"选择来源：项目"}));
 const target=screen.getByRole("region",{name:"目标文件夹"});
 expect(within(target).getByText("说明.txt")).toBeTruthy();
 expect(within(target).getByText("项目")).toBeTruthy();
 expect(within(target).getByText(file.path)).toBeTruthy();
 expect(within(target).getAllByText("待检查")).toHaveLength(2);
 expect(screen.getByRole("button",{name:"创建链接"}).hasAttribute("disabled")).toBe(false);
 expect(ports.call.mock.calls.some(([method])=>String(method).startsWith("create"))).toBe(false);
 await user.click(screen.getByRole("button",{name:"打开文件夹：项目"}));
 await waitFor(()=>expect(ports.call).toHaveBeenCalledWith("browse",{directory:folder.path,offset:0}));
 expect(within(screen.getByRole("region",{name:"源文件夹"})).getByRole("checkbox",{name:"选择来源：说明.txt"}).getAttribute("aria-checked")).toBe("true");
});

it("keeps existing target entries visible and marks name collisions before execution",async()=>{
 const defaultCall=ports.call.getMockImplementation()!;
 ports.call.mockImplementation(async(method:string,args:any)=>method==="browse"&&args.directory===destination?listing(destination,[{...file,path:destination+"\\说明.txt"}]):defaultCall(method,args));
 render(<DirectoryLinksBatch onChanged={()=>{}}/>);const user=await openBoth();
 await user.click(screen.getByRole("checkbox",{name:"选择来源：说明.txt"}));
 const target=screen.getByRole("region",{name:"目标文件夹"});
 expect(within(target).getAllByText("说明.txt").length).toBeGreaterThan(0);
 expect(within(target).getByText("同名冲突")).toBeTruthy();
 expect(screen.getByRole("button",{name:"创建链接"})).toBeTruthy();
});

it("shows the reviewed result when Host normalizes a manually entered source path",async()=>{
 const defaultCall=ports.call.getMockImplementation()!;
 ports.call.mockImplementation(async(method:string,args:any)=>method==="previewBatch"?{id:"normalized-plan",items:[{id:"one",source:file.path,linkPath:destination+"\\说明.txt",objectType:"file",kind:"hardlink",targetText:file.path}]}:defaultCall(method,args));
 render(<DirectoryLinksBatch onChanged={()=>{}} incoming={[source+"\\.\\说明.txt"]}/>);
 const target=screen.getByRole("region",{name:"目标文件夹"});
 fireEvent.change(within(target).getByRole("textbox"),{target:{value:destination}});
 const user=userEvent.setup();await user.click(screen.getByRole("button",{name:"更多选项"}));await user.click(await screen.findByRole("button",{name:"预览创建计划"}));
 await within(target).findByText("待创建");
 expect(within(target).getByText("文件硬链接")).toBeTruthy();
});

it("creates directly and refreshes actual target contents after creation",async()=>{
 let created=false;const defaultCall=ports.call.getMockImplementation()!;const changed=vi.fn();
 ports.call.mockImplementation(async(method:string,args:any)=>{
  if(method==="createBatch"){created=true;return {records:[],completed:1,total:1};}
  if(method==="browse"&&args.directory===destination&&created)return listing(destination,[{...file,path:destination+"\\说明.txt",kind:"hardlink"}]);
  return defaultCall(method,args);
 });
 render(<DirectoryLinksBatch onChanged={changed}/>);const user=await openBoth();
 await user.click(screen.getByRole("checkbox",{name:"选择来源：说明.txt"}));
 await user.click(screen.getByRole("button",{name:"创建链接"}));
 await waitFor(()=>expect(changed).toHaveBeenCalled());
 const target=screen.getByRole("region",{name:"目标文件夹"});
 await within(target).findByText("说明.txt");
 expect((screen.getByLabelText("链接存放文件夹") as HTMLInputElement).value).toBe(destination);
});

it("imports multiple native files through one source menu without navigating the source browser",async()=>{
 ports.files.mockResolvedValue([file.path,"D:\\other\\notes.txt"]);
 render(<DirectoryLinksBatch onChanged={()=>{}}/>);const user=userEvent.setup();
 screen.getByRole("button",{name:"添加来源"}).focus();await user.keyboard("{ArrowDown}");
 await user.click(await screen.findByRole("menuitem",{name:"选择文件…"}));
 const left=within(screen.getByRole("region",{name:"源文件夹"}));
 await left.findByRole("checkbox",{name:"选择来源：说明.txt"});
 expect(left.getByRole("checkbox",{name:"选择来源：notes.txt"}).getAttribute("aria-checked")).toBe("true");
 expect((left.getByRole("textbox") as HTMLInputElement).value).toBe("");
});

it("imports quoted clipboard paths by pasting into the source list and removes duplicates",async()=>{
 render(<DirectoryLinksBatch onChanged={()=>{}}/>);
 const sourcePane=screen.getByRole("region",{name:"源文件夹"});
 fireEvent.paste(sourcePane,{clipboardData:{getData:()=>`"${file.path}"\r\n${folder.path}\r\n${file.path}`}});
 expect(within(sourcePane).getByRole("checkbox",{name:"选择来源：说明.txt"}).getAttribute("aria-checked")).toBe("true");
 expect(within(sourcePane).getByRole("checkbox",{name:"选择来源：项目"}).getAttribute("aria-checked")).toBe("true");
 expect(screen.getByText("已选择 2 项")).toBeTruthy();
});

it("keeps a custom single link name in the direct create safety preview",async()=>{
 const defaultCall=ports.call.getMockImplementation()!;
 ports.call.mockImplementation(async(method:string,args:any)=>method==="previewBatch"?{id:"named-plan",items:[{id:"one",source:file.path,linkPath:destination+"\\别名.txt",objectType:"file",kind:"hardlink",targetText:file.path}]}:method==="createBatch"?{records:[],completed:1,total:1}:defaultCall(method,args));
 render(<DirectoryLinksBatch onChanged={()=>{}} incoming={[file.path]}/>);const user=userEvent.setup();
 fireEvent.change(screen.getByLabelText("链接存放文件夹"),{target:{value:destination}});
 await user.clear(screen.getByLabelText("链接名称"));await user.type(screen.getByLabelText("链接名称"),"别名.txt");
 await user.click(screen.getByRole("button",{name:"创建链接"}));
 await waitFor(()=>expect(ports.call).toHaveBeenCalledWith("previewBatch",{sources:[file.path],directory:destination,kind:"auto",relative:false,linkName:"别名.txt"}));
 const methods=ports.call.mock.calls.map(([method])=>method);
 expect(methods.indexOf("previewBatch")).toBeLessThan(methods.indexOf("createBatch"));
});

it("does not create when the safety preview refuses a destination conflict",async()=>{
 const defaultCall=ports.call.getMockImplementation()!;
 ports.call.mockImplementation(async(method:string,args:any)=>{if(method==="previewBatch")throw Error("目标路径已被占用");return defaultCall(method,args);});
 render(<DirectoryLinksBatch onChanged={()=>{}} incoming={[file.path]}/>);
 fireEvent.change(screen.getByLabelText("链接存放文件夹"),{target:{value:destination}});
 await userEvent.setup().click(screen.getByRole("button",{name:"创建链接"}));
 await screen.findByText("Error: 目标路径已被占用");
 expect(ports.call.mock.calls.some(([method])=>String(method).startsWith("create"))).toBe(false);
});

it("marks an inspected destination link without labeling ordinary files as links",async()=>{
 const defaultCall=ports.call.getMockImplementation()!;
 ports.call.mockImplementation(async(method:string,args:any)=>method==="browse"&&args.directory===destination?listing(destination,[{...file,path:destination+"\\说明.txt",kind:"hardlink"},{name:"ordinary.txt",path:destination+"\\ordinary.txt",objectType:"file"}]):defaultCall(method,args));
 render(<DirectoryLinksBatch onChanged={()=>{}}/>);await openBoth();
 const right=within(screen.getByRole("region",{name:"目标文件夹"}));
 expect(right.getAllByText("已链接")).toHaveLength(1);
 expect(right.queryByText("已存在")).toBeNull();
});

it.each([file,folder])("renames existing destination $objectType entries only after submitting the name",async(entry)=>{
 let current={...entry,path:destination+"\\"+entry.name};const changed=vi.fn(),base=ports.call.getMockImplementation()!;
 ports.call.mockImplementation(async(method:string,args:any)=>{
  if(method==="browse"&&args.directory===destination)return listing(destination,[current]);
  if(method==="rename"){current={...current,name:args.name,path:destination+"\\"+args.name};return {path:current.path,records:[]};}
  return base(method,args);
 });
 render(<DirectoryLinksBatch onChanged={changed}/>);const user=await openBoth();
 const target=within(screen.getByRole("region",{name:"目标文件夹"}));
 await user.click(target.getByRole("button",{name:"重命名："+entry.name}));
 const dialog=within(screen.getByRole("dialog"));
 expect(dialog.getByRole("heading",{name:entry.objectType==="file"?"重命名文件":"重命名文件夹"})).toBeTruthy();
 expect(ports.call.mock.calls.some(([method])=>method==="rename")).toBe(false);
 await user.clear(dialog.getByRole("textbox",{name:"新名称"}));await user.type(dialog.getByRole("textbox",{name:"新名称"}),"新名称.txt{Enter}");
 await waitFor(()=>expect(ports.call).toHaveBeenCalledWith("rename",{path:destination+"\\"+entry.name,name:"新名称.txt",entryIdentity:entry.identity,parentIdentity:identity(destination)}));
 await waitFor(()=>expect(screen.queryByRole("dialog")).toBeNull());
 expect(await target.findByText("新名称.txt")).toBeTruthy();expect(changed).toHaveBeenCalledWith([]);
 expect(ports.call.mock.calls.some(([method])=>String(method).startsWith("create"))).toBe(false);
});

it("supports F2, preserves a rejected name for correction, and cancels without another mutation",async()=>{
 const base=ports.call.getMockImplementation()!;
 ports.call.mockImplementation(async(method:string,args:any)=>{
  if(method==="browse"&&args.directory===destination)return listing(destination,[{...file,path:destination+"\\"+file.name}]);
  if(method==="rename")throw Error("所在文件夹已有同名项目，不能覆盖；请换一个名称");
  return base(method,args);
 });
 render(<DirectoryLinksBatch onChanged={()=>{}}/>);const user=await openBoth();
 const row=within(screen.getByRole("region",{name:"目标文件夹"})).getByRole("listitem");row.focus();await user.keyboard("{F2}");
 const dialog=within(screen.getByRole("dialog"));const input=dialog.getByRole("textbox",{name:"新名称"});
 await user.clear(input);await user.type(input,"冲突.txt");await user.click(dialog.getByRole("button",{name:"重命名"}));
 await dialog.findByText(/不能覆盖/);expect((input as HTMLInputElement).value).toBe("冲突.txt");
 await user.click(dialog.getByRole("button",{name:"取消"}));await waitFor(()=>expect(screen.queryByRole("dialog")).toBeNull());
 expect(ports.call.mock.calls.filter(([method])=>method==="rename")).toHaveLength(1);
 expect(within(screen.getByRole("region",{name:"目标文件夹"})).getByText(file.name)).toBeTruthy();
});

it("does not let an older folder response replace the newest address",async()=>{
 let finish!:(value:unknown)=>void;const defaultCall=ports.call.getMockImplementation()!;
 ports.call.mockImplementation(async(method:string,args:any)=>method==="browse"&&args.directory===source?new Promise(resolve=>{finish=resolve;}):defaultCall(method,args));
 render(<DirectoryLinksBatch onChanged={()=>{}}/>);
 const input=screen.getByRole("textbox",{name:"源文件夹"});
 fireEvent.change(input,{target:{value:source}});fireEvent.keyDown(input,{key:"Enter"});
 await waitFor(()=>expect(finish).toBeTypeOf("function"));
 fireEvent.change(input,{target:{value:"C:\\newer"}});fireEvent.keyDown(input,{key:"Enter"});
 await waitFor(()=>expect(ports.call).toHaveBeenCalledWith("browse",{directory:"C:\\newer",offset:0}));
 finish(listing(source,[file]));
 await waitFor(()=>expect((input as HTMLInputElement).value).toBe("C:\\newer"));
 expect(screen.queryByRole("checkbox",{name:"选择来源：说明.txt"})).toBeNull();
});
