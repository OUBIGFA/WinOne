// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {act,cleanup,render,screen,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {useState} from "react";
import {ChatProjectContext} from "./chat-project-context";
const bridge=vi.hoisted(()=>({chatCall:vi.fn()}));vi.mock("../bridge",()=>bridge);
afterEach(()=>{cleanup();vi.clearAllMocks();});
const projects=[{id:"internal-a",name:"Alpha",path:"C:\\Alpha"},{id:"internal-b",name:"Beta",path:"C:\\Beta"}];

it('retains the branch on window focus and refreshes only on request',async()=>{
 bridge.chatCall.mockResolvedValue({projectId:'internal-a',repository:true,branch:'main'});
 render(<ChatProjectContext projects={projects} projectId="internal-a" onProject={()=>{}} onOpenProject={()=>{}} disabled={false}/>);
 await screen.findByText('main');
 await act(async()=>{window.dispatchEvent(new Event('focus'));window.dispatchEvent(new Event('focus'));});
 expect(bridge.chatCall).toHaveBeenCalledTimes(1);
 let finish!:(value:unknown)=>void;bridge.chatCall.mockImplementation(()=>new Promise(resolve=>{finish=resolve;}));
 await userEvent.click(screen.getByRole('button',{name:'刷新 Git 分支'}));
 expect(screen.getByText('main')).toBeTruthy();expect(bridge.chatCall).toHaveBeenCalledTimes(2);
 await act(async()=>finish({projectId:'internal-a',repository:true,branch:'feature/new'}));
 expect(await screen.findByText('feature/new')).toBeTruthy();
});

it("discards a slow previous project's branch and never presents its ID",async()=>{
 let finishA!:(value:unknown)=>void;bridge.chatCall.mockImplementation((_,value)=>value.projectId==="internal-a"?new Promise(resolve=>{finishA=resolve;}):Promise.resolve({projectId:"internal-b",repository:true,branch:"feature/beta"}));
 function Fixture(){const [id,setId]=useState("internal-a");return <ChatProjectContext projects={projects} projectId={id} onProject={setId} onOpenProject={()=>{}} disabled={false}/>;}
 const {container}=render(<Fixture/>);await waitFor(()=>expect(bridge.chatCall).toHaveBeenCalledWith("project.git",{projectId:"internal-a"}));
 await userEvent.click(screen.getByRole("button",{name:"项目：Alpha"}));await userEvent.click(await screen.findByRole("menuitem",{name:/Beta/}));
 expect(await screen.findByText("feature/beta")).toBeTruthy();finishA({projectId:"internal-a",repository:true,branch:"old/alpha"});await waitFor(()=>expect(screen.queryByText("old/alpha")).toBeNull());expect(container.textContent).not.toContain("internal-");
});

it("shows a non-repository without inventing a branch",async()=>{
 bridge.chatCall.mockResolvedValue({projectId:"internal-a",repository:false});render(<ChatProjectContext projects={projects} projectId="internal-a" onProject={()=>{}} onOpenProject={()=>{}} disabled={false}/>);
 await waitFor(()=>expect(screen.queryByText("正在读取分支")).toBeNull());
 expect(screen.queryByRole("status",{name:"Git 分支"})).toBeNull();expect(screen.queryByText("无分支")).toBeNull();expect(screen.queryByText("main")).toBeNull();
});

it("shows Git failures distinctly and clears the branch when the project is removed",async()=>{
 bridge.chatCall.mockRejectedValue(Error("Git unavailable"));const {rerender}=render(<ChatProjectContext projects={projects} projectId="internal-a" onProject={()=>{}} onOpenProject={()=>{}} disabled={false}/>);
 expect(await screen.findByText("无法读取分支")).toBeTruthy();rerender(<ChatProjectContext projects={projects} projectId="" onProject={()=>{}} onOpenProject={()=>{}} disabled={false}/>);expect(screen.queryByText("无法读取分支")).toBeNull();
});
