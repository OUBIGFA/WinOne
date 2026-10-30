// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {act,cleanup,render,screen,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {useState} from "react";
import {usePageActivity,usePageNavigation,type PageActivityChanged,type SettingsClosePort} from "./page-navigation";
import {setApplicationLanguage} from "../i18n";
import {Tabs,TabsContent,TabsList,TabsTrigger} from "../components/ui/tabs";
import {TabChangeGuardContext} from "../components/tab-change-guard";

afterEach(async()=>{cleanup();await setApplicationLanguage("zh-CN");});
it("clicking the backdrop keeps unsaved edits and removes the modal",async()=>{
 render(<Preview/>);const user=userEvent.setup();
 await user.type(screen.getByLabelText("draft")," changed");await user.click(screen.getByText("next",{selector:"button"}));
 await screen.findByRole("dialog");
 await user.click(document.querySelector('[data-slot=dialog-overlay]')!);
 await waitFor(()=>expect(screen.queryByRole("dialog")).toBeNull());
 expect((screen.getByLabelText("draft") as HTMLInputElement).value).toBe("saved changed");
 expect(document.querySelector('[data-slot=dialog-overlay]')).toBeNull();
});
it("can keep editing while activity synchronization is stalled without a late close",async()=>{
 let requestClose=()=>{},release!:()=>void;
 const blocked=new Promise<void>(resolve=>{release=resolve;});
 const port:SettingsClosePort={setActivity:()=>blocked,pending:async()=>false,subscribe:listener=>{requestClose=listener;return()=>{};},cancel:vi.fn().mockResolvedValue(undefined),approve:vi.fn().mockResolvedValue(undefined)};
 render(<Preview port={port}/>);const user=userEvent.setup();await user.type(screen.getByLabelText("draft")," changed");act(()=>requestClose());
 await user.click(await screen.findByRole("button",{name:"放弃并关闭"}));
 expect((screen.getByRole("button",{name:"继续编辑"}) as HTMLButtonElement).disabled).toBe(false);
 await user.click(screen.getByRole("button",{name:"继续编辑"}));
 await waitFor(()=>expect(screen.queryByRole("dialog")).toBeNull());
 await act(async()=>release());expect(port.approve).not.toHaveBeenCalled();
 expect((screen.getByLabelText("draft") as HTMLInputElement).value).toBe("saved changed");
});
function Editor({onActivity}:{onActivity:(dirty:boolean,busy:boolean)=>void}) {
 const [value,setValue]=useState("saved"),[busy,setBusy]=useState(false);
 usePageActivity(onActivity,value!=="saved",busy);
 return <><input aria-label="draft" value={value} onChange={e=>setValue(e.target.value)}/><button onClick={()=>setValue("saved")}>save</button><button onClick={()=>setBusy(!busy)}>work</button></>;
}
function Preview({port,onArrive}:{port?:SettingsClosePort;onArrive?:()=>void}) {
 const [page,setPage]=useState("edit");
 const navigation=usePageNavigation(page,setPage,port);
 return <><button onClick={()=>navigation.requestPage("next",onArrive)}>next</button><p>{page}</p>{page==="edit"&&<Editor onActivity={navigation.onActivity}/>} {navigation.dialog}</>;
}
it("opens a requested item only after the guarded navigation is accepted",async()=>{
 const arrived=vi.fn();render(<Preview onArrive={arrived}/>);const user=userEvent.setup();
 await user.type(screen.getByLabelText("draft")," changed");await user.click(screen.getByRole("button",{name:"next"}));
 await user.click(await screen.findByRole("button",{name:"继续编辑"}));expect(arrived).not.toHaveBeenCalled();
 await user.click(screen.getByRole("button",{name:"next"}));await user.click(await screen.findByRole("button",{name:"放弃并离开"}));
 expect(arrived).toHaveBeenCalledOnce();
});
it("keeps edits when navigation is cancelled and discards only on confirmation",async()=>{
 render(<Preview/>);const user=userEvent.setup();
 await user.type(screen.getByLabelText("draft")," changed");await user.click(screen.getByText("next",{selector:"button"}));
 await screen.findByRole("dialog");await user.click(screen.getByRole("button",{name:"继续编辑"}));
 expect((screen.getByLabelText("draft") as HTMLInputElement).value).toBe("saved changed");
 await user.click(screen.getByText("next",{selector:"button"}));await user.click(screen.getByRole("button",{name:"放弃并离开"}));
 expect(screen.queryByLabelText("draft")).toBeNull();
});

it("protects a native window close and preserves edits on cancel",async()=>{
 let requestClose=()=>{};
 const port:SettingsClosePort={setActivity:vi.fn().mockResolvedValue(undefined),pending:async()=>false,subscribe:listener=>{requestClose=listener;return()=>{};},cancel:vi.fn().mockResolvedValue(undefined),approve:vi.fn().mockResolvedValue(undefined)};
 render(<Preview port={port}/>);const user=userEvent.setup();await user.type(screen.getByLabelText("draft")," changed");
 act(()=>requestClose());await screen.findByRole("dialog");await user.click(screen.getByRole("button",{name:"继续编辑"}));
 expect(port.cancel).toHaveBeenCalledOnce();expect((screen.getByLabelText("draft") as HTMLInputElement).value).toBe("saved changed");expect(port.approve).not.toHaveBeenCalled();
 act(()=>requestClose());await user.click(await screen.findByRole("button",{name:"放弃并关闭"}));
 await waitFor(()=>expect(port.approve).toHaveBeenCalledOnce());
});

it("keeps the draft and dialog when the close acknowledgement fails",async()=>{
 let requestClose=()=>{};
 const port:SettingsClosePort={setActivity:async()=>{},pending:async()=>false,subscribe:listener=>{requestClose=listener;return()=>{};},cancel:async()=>{},approve:async()=>{throw Error("close failed");}};
 render(<Preview port={port}/>);const user=userEvent.setup();await user.type(screen.getByLabelText("draft")," changed");act(()=>requestClose());
 await user.click(await screen.findByRole("button",{name:"放弃并关闭"}));expect((await screen.findByRole("alert")).textContent).toContain("close failed");
 expect((screen.getByLabelText("draft") as HTMLInputElement).value).toBe("saved changed");expect(screen.queryByRole("dialog")).not.toBeNull();
});
it("does not interrupt a running operation",async()=>{
 render(<Preview/>);const user=userEvent.setup();await user.click(screen.getByText("work"));await user.click(screen.getByText("next",{selector:"button"}));
 expect((await screen.findByRole("dialog")).textContent).toContain("操作尚未完成");expect(screen.queryByRole("button",{name:"放弃并离开"})).toBeNull();
 await user.click(screen.getByRole("button",{name:"留在此页"}));expect(screen.getByLabelText("draft")).not.toBeNull();
});
it("leaves a saved page without confirmation",async()=>{
 render(<Preview/>);const user=userEvent.setup();await user.type(screen.getByLabelText("draft")," changed");await user.click(screen.getByText("save"));await user.click(screen.getByText("next",{selector:"button"}));
 expect(screen.queryByRole("dialog")).toBeNull();expect(screen.queryByLabelText("draft")).toBeNull();
});

it("keeps another section's unsaved changes when one section becomes clean",async()=>{
 function MultipleSections(){
  const [page,setPage]=useState("edit");const navigation=usePageNavigation(page,setPage);
  return <><button onClick={()=>navigation.onActivity(true,false,"preferences")}>edit preferences</button><button onClick={()=>navigation.onActivity(false,false,"shortcuts")}>clean shortcuts</button><button onClick={()=>navigation.requestPage("next")}>leave</button>{navigation.dialog}<p>{page}</p></>;
 }
 render(<MultipleSections/>);const user=userEvent.setup();await user.click(screen.getByText("edit preferences"));await user.click(screen.getByText("clean shortcuts"));await user.click(screen.getByText("leave"));
 expect(await screen.findByRole("dialog")).toBeTruthy();expect(screen.getByText("edit",{selector:"p"})).toBeTruthy();
});

// A draft shared by several tabs stays mounted, so discarding must revert it.
function SharedDraft({onActivity}:{onActivity:PageActivityChanged}){
 const [value,setValue]=useState("saved");
 usePageActivity(onActivity,value!=="saved",false,"shared",()=>setValue("saved"));
 return <input aria-label="draft" value={value} onChange={event=>setValue(event.target.value)}/>;
}
function GuardedTabs({uncontrolled=false,navigation=true}:{uncontrolled?:boolean;navigation?:boolean}){
 const [page,setPage]=useState("edit"),[tab,setTab]=useState("first");
 const guard=usePageNavigation(page,setPage);
 const content=<><TabsList><TabsTrigger value="first">first</TabsTrigger><TabsTrigger value="second">second</TabsTrigger></TabsList>
  <TabsContent value="first" keepMounted><SharedDraft onActivity={guard.onActivity}/></TabsContent><TabsContent value="second" keepMounted><p>second panel</p></TabsContent></>;
 return <TabChangeGuardContext.Provider value={guard.requestChange}>
  {uncontrolled?<Tabs navigation={navigation} defaultValue="first">{content}</Tabs>:<Tabs navigation={navigation} value={tab} onValueChange={value=>setTab(String(value))}>{content}</Tabs>}
  {guard.dialog}
 </TabChangeGuardContext.Provider>;
}
// The modal dialog hides the page from the accessibility tree while open.
const selected=(name:string)=>screen.getByRole("tab",{name,hidden:true}).getAttribute("aria-selected");
const draft=()=>(screen.getByLabelText("draft") as HTMLInputElement).value;
it.each([false,true])("asks before a tab switch discards unsaved changes (uncontrolled=%s)",async uncontrolled=>{
 render(<GuardedTabs uncontrolled={uncontrolled}/>);const user=userEvent.setup();
 await user.click(screen.getByRole("tab",{name:"second"}));expect(selected("second")).toBe("true");
 await user.click(screen.getByRole("tab",{name:"first"}));expect(screen.queryByRole("dialog")).toBeNull();
 await user.type(screen.getByLabelText("draft")," changed");await user.click(screen.getByRole("tab",{name:"second"}));
 expect((await screen.findByRole("dialog")).textContent).toContain("切换标签会放弃这些更改");expect(selected("first")).toBe("true");
 await user.click(screen.getByRole("button",{name:"继续编辑"}));
 await waitFor(()=>expect(screen.queryByRole("dialog")).toBeNull());
 expect(selected("first")).toBe("true");expect(draft()).toBe("saved changed");
 await user.click(screen.getByRole("tab",{name:"second"}));await user.click(await screen.findByRole("button",{name:"放弃并离开"}));
 await waitFor(()=>expect(selected("second")).toBe("true"));expect(draft()).toBe("saved");
 await user.click(screen.getByRole("tab",{name:"first"}));expect(screen.queryByRole("dialog")).toBeNull();expect(selected("first")).toBe("true");
});
it("does not guard tabs that only represent a form value",async()=>{
 render(<GuardedTabs navigation={false}/>);const user=userEvent.setup();
 await user.type(screen.getByLabelText("draft")," changed");await user.click(screen.getByRole("tab",{name:"second"}));
 expect(screen.queryByRole("dialog")).toBeNull();expect(selected("second")).toBe("true");expect(draft()).toBe("saved changed");
});
it("switches tabs only after a running action finishes",async()=>{
 let report!:PageActivityChanged;
 function Busy(){
  const [page,setPage]=useState("edit"),[tab,setTab]=useState("first");const guard=usePageNavigation(page,setPage);report=guard.onActivity;
  return <TabChangeGuardContext.Provider value={guard.requestChange}><Tabs value={tab} onValueChange={value=>setTab(String(value))}><TabsList><TabsTrigger value="first">first</TabsTrigger><TabsTrigger value="second">second</TabsTrigger></TabsList></Tabs>{guard.dialog}</TabChangeGuardContext.Provider>;
 }
 render(<Busy/>);const user=userEvent.setup();
 act(()=>report(false,true,"work"));await user.click(screen.getByRole("tab",{name:"second"}));
 expect((await screen.findByRole("dialog")).textContent).toContain("操作完成后将切换标签");
 expect(screen.queryByRole("button",{name:"放弃并离开"})).toBeNull();expect(selected("first")).toBe("true");
 act(()=>report(false,false,"work"));
 await waitFor(()=>expect(selected("second")).toBe("true"));expect(screen.queryByRole("dialog")).toBeNull();
});
