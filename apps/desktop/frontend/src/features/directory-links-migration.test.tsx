// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from "vitest";
import {cleanup,fireEvent,render,screen,waitFor,within} from "@testing-library/react";
import {DirectoryLinksMigration} from "./directory-links-migration";
import type {DirectoryLinkRecord} from "./directory-links";
const ports=vi.hoisted(()=>({call:vi.fn(),pick:vi.fn()}));
vi.mock("../bridge",()=>({directoryLinksCall:ports.call,pickDirectory:ports.pick}));
const plan={id:"plan-one",operation:"migrate",source:"C:\\source",destination:"D:\\target\\source",files:2,bytes:120,warnings:[] as string[],expiresAt:"2099-01-01T00:00:00Z"};
const row:DirectoryLinkRecord={id:"moved",name:"source",linkPath:plan.source,targetPath:plan.destination,kind:"junction",objectType:"directory",targetText:plan.destination,relative:false,createdAt:"2026-10-26T00:00:00Z",checkedAt:"2026-10-26T01:00:00Z",status:"active",reason:"target-accessible",accessible:true,migration:{operation:"migrate",phase:"complete",source:plan.source,destination:plan.destination,backup:"C:\\backup",staging:"D:\\stage",bytes:120,files:2,retained:[]}};
const job=(requestId:string,state="completed",extra:object={})=>({id:"job-one",requestId,state,canCancel:state==="running",progress:{phase:"copying",files:0,totalFiles:2,bytes:0,totalBytes:120},...extra});
function fill(){fireEvent.change(screen.getByLabelText("要搬迁的目录"),{target:{value:plan.source}});fireEvent.change(screen.getByLabelText("新的存放文件夹"),{target:{value:"D:\\target"}});}
beforeEach(()=>ports.pick.mockResolvedValue("C:\\chosen"));
afterEach(()=>{cleanup();vi.resetAllMocks();});
it("previews without moving, requires confirmation, and admits execution exactly once",async()=>{
 const changed=vi.fn();ports.call.mockImplementation(async(method:string,args:any)=>job(args.requestId,"completed",method==="migration.preview"?{plan}:{record:row}));
 render(<DirectoryLinksMigration onChanged={changed}/>);fill();fireEvent.click(screen.getByRole("button",{name:"检查并预览"}));
 await screen.findByText("预览已准备好，尚未搬迁");expect(ports.call).toHaveBeenCalledTimes(1);expect(changed).not.toHaveBeenCalled();
 fireEvent.click(screen.getByRole("button",{name:"开始搬迁"}));await screen.findByRole("dialog");expect(ports.call).toHaveBeenCalledTimes(1);
 fireEvent.click(screen.getByRole("button",{name:"确认执行"}));await waitFor(()=>expect(changed).toHaveBeenCalledWith([row]));
 expect(ports.call.mock.calls.filter(([method])=>method==="migration.execute")).toHaveLength(1);
 expect(ports.call).toHaveBeenLastCalledWith("migration.execute",{token:plan.id,confirmed:true,acknowledged:false,requestId:expect.any(String)});
});
it("changing a checked path invalidates its execution plan",async()=>{
 ports.call.mockImplementation(async(_:string,args:any)=>job(args.requestId,"completed",{plan}));render(<DirectoryLinksMigration onChanged={()=>{}}/>);fill();fireEvent.click(screen.getByRole("button",{name:"检查并预览"}));await screen.findByText("预览已准备好，尚未搬迁");
 fireEvent.change(screen.getByLabelText("要搬迁的目录"),{target:{value:"C:\\different"}});
 expect((screen.getByRole("button",{name:"开始搬迁"}) as HTMLButtonElement).disabled).toBe(true);
});
it("requires acknowledgment for a warning-level directory",async()=>{
 ports.call.mockImplementation(async(_:string,args:any)=>job(args.requestId,"completed",{plan:{...plan,warnings:["关闭正在使用的应用"]}}));render(<DirectoryLinksMigration onChanged={()=>{}}/>);fill();fireEvent.click(screen.getByRole("button",{name:"检查并预览"}));await screen.findByText("预览已准备好，尚未搬迁");fireEvent.click(screen.getByRole("button",{name:"开始搬迁"}));
 const dialog=await screen.findByRole("dialog"),button=within(dialog).getByRole("button",{name:"确认执行"});expect((button as HTMLButtonElement).disabled).toBe(true);
 fireEvent.click(within(dialog).getByRole("checkbox"));await waitFor(()=>expect((button as HTMLButtonElement).disabled).toBe(false));
});
it("recovers a lost admission reply by request identity without resubmitting the preview",async()=>{
 let requestId="";ports.call.mockImplementation(async(method:string,args:any)=>{if(method==="migration.preview"){requestId=args.requestId;throw Error("reply lost");}expect(args).toEqual({requestId});return job(requestId,"completed",{plan});});
 render(<DirectoryLinksMigration onChanged={()=>{}}/>);fill();fireEvent.click(screen.getByRole("button",{name:"检查并预览"}));await screen.findByText(/reply lost/);
 fireEvent.click(screen.getByRole("button",{name:"重新读取本次任务"}));await screen.findByText("预览已准备好，尚未搬迁");expect(ports.call.mock.calls.filter(([method])=>method==="migration.preview")).toHaveLength(1);
});
it("keeps activity protection after a polling failure and can cancel the known job",async()=>{
 const activity=vi.fn();let requestId="";
 ports.call.mockImplementation(async(method:string,args:any)=>{if(method==="migration.preview")return job(args.requestId,"completed",{plan});if(method==="migration.execute"){requestId=args.requestId;return job(requestId,"running");}if(method==="migration.cancel")return job(requestId,"cancelled");throw Error("poll temporarily unavailable");});
 render(<DirectoryLinksMigration onChanged={()=>{}} onActivity={activity}/>);fill();fireEvent.click(screen.getByRole("button",{name:"检查并预览"}));await screen.findByText("预览已准备好，尚未搬迁");fireEvent.click(screen.getByRole("button",{name:"开始搬迁"}));fireEvent.click(await screen.findByRole("button",{name:"确认执行"}));await screen.findByText(/poll temporarily unavailable/);
 expect(activity.mock.calls.at(-1)?.[1]).toBe(true);fireEvent.click(screen.getByRole("button",{name:"取消任务"}));await screen.findByText("任务已取消");await waitFor(()=>expect(activity.mock.calls.at(-1)?.[1]).toBe(false));
 expect(ports.call.mock.calls.filter(([method])=>method==="migration.execute")).toHaveLength(1);
});
it("restores by the inspected record and binds cleanup to the displayed backup path",async()=>{
 ports.call.mockImplementation(async(method:string,args:any)=>job(args.requestId,"completed",method==="migration.preview"?{plan:{...plan,operation:"restore",source:plan.destination,destination:plan.source}}:{record:row}));
 const rendered=render(<DirectoryLinksMigration selection={{record:row,action:"restore",nonce:1}} onChanged={()=>{}}/>);fireEvent.click(await screen.findByRole("button",{name:"检查并预览"}));await screen.findByText("预览已准备好，尚未搬迁");
 expect(ports.call).toHaveBeenCalledWith("migration.preview",{recordId:row.id,checkedAt:row.checkedAt,requestId:expect.any(String)});
 const retained={...row,migration:{...row.migration!,retained:[{path:"C:\\retained-data",kind:"directory"}]}};
 rendered.rerender(<DirectoryLinksMigration selection={{record:retained,action:"cleanup",index:0,nonce:2}} onChanged={()=>{}}/>);const dialog=await screen.findByRole("dialog");expect(within(dialog).getByText("C:\\retained-data")).toBeTruthy();fireEvent.click(within(dialog).getByRole("button",{name:"确认执行"}));
 await waitFor(()=>expect(ports.call).toHaveBeenCalledWith("migration.cleanup",{id:row.id,index:0,path:"C:\\retained-data",confirmed:true,requestId:expect.any(String)}));
});
