// @vitest-environment jsdom
import {useState,type ComponentType} from "react";
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {usePageNavigation,type PageActivityChanged} from "../ui/page-navigation";
import {AISettings} from "./ai-settings";
import {AIGateway} from "./ai-gateway";
import {CaptureAISettings} from "./capture-ai-settings";
import {AlwaysOnTopPage} from "./always-on-top";
import {SelectionPage} from "./selection";
import {Shortcuts} from "./shortcuts";

const call=vi.hoisted(()=>vi.fn());
vi.mock("../bridge",()=>({captureCall:call,alwaysOnTopCall:call,selectionCall:call,quickAssistantCall:call,getSnapshot:call,getShortcuts:call,saveSettings:call,shortcutRecordingPort:{begin:call,end:call}}));
afterEach(()=>{cleanup();vi.resetAllMocks();});
type Page=ComponentType<{onActivity:PageActivityChanged}>;
function Harness({Page}:{Page:Page}){
 const [page,setPage]=useState("settings");const navigation=usePageNavigation(page,setPage);
 return <><button onClick={()=>navigation.requestPage("other")}>切换页面</button>{page==="settings"?<Page onActivity={navigation.onActivity}/>:<p>其他页面</p>}{navigation.dialog}</>;
}
const pages:[string,Page][]=[
 ["AI configuration",AISettings],
 ["gateway",props=><AIGateway {...props} active providers={[]}/>],
 ["capture AI",CaptureAISettings],
 ["pinning",AlwaysOnTopPage],
 ["selection",SelectionPage],
 ["quick assistant",props=><SelectionPage {...props} moduleId="quick-assistant"/>],
 ["shortcuts",props=><Shortcuts {...props} moduleId="capture" onSaved={()=>{}}/>],
];
it.each(pages)("allows leaving %s while its initial read is pending",async(_name,Page)=>{
 // The real navigation guard must not mistake a stalled backend read for a
 // transaction that would lose edits if this component unmounted.
 call.mockImplementation(()=>new Promise(()=>{}));render(<Harness Page={Page}/>);
 await userEvent.setup().click(screen.getByRole("button",{name:"切换页面"}));
 expect(screen.queryByRole("dialog")).toBeNull();expect(screen.getByText("其他页面")).toBeTruthy();
});

it("continues to protect a pending settings save",async()=>{
 call.mockImplementation((method:string)=>method==="settings.save"?new Promise(()=>{}):Promise.resolve({schemaVersion:1,revision:1,automatic:true,keyboard:true,excludedApps:[]}));
 render(<Harness Page={SelectionPage}/>);const user=userEvent.setup();
 await user.click(screen.getByRole("tab",{name:"常规"}));await user.click(await screen.findByRole("switch",{name:"划词后显示工具栏"}));
 await user.click(screen.getByRole("button",{name:"保存设置"}));await user.click(screen.getByRole("button",{name:"切换页面"}));
 expect(screen.getByRole("dialog")).toBeTruthy();expect(screen.queryByText("其他页面")).toBeNull();
});
