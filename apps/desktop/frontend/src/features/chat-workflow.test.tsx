// @vitest-environment jsdom
import {cleanup,render,screen,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {afterEach,expect,it} from "vitest";
import {WorkProcess} from "./cindy/WorkProcess";
import type {ProcessResponse} from "./chat-process";

afterEach(cleanup);
const process=(key:string,running:boolean):ProcessResponse=>({kind:"response",key,messages:[],running,hasTools:true,startedAt:100,durationMs:400,entries:[
 {kind:"tool",key:"read",at:100,tool:{id:"read",name:"read",args:{path:"README.md"},running:false,result:"Read the project"}},
 {kind:"commentary",key:"note",at:200,text:"Now update the file."},
 {kind:"tool",key:"write",at:300,tool:{id:"write",name:"write",args:{path:"result.txt"},running,result:running?undefined:"Saved the result"}},
]});

it("keeps consecutive work segments separate while the turn is running",()=>{
 const {container}=render(<WorkProcess group={process("live-segments",true)} conversationId="test" onCopy={()=>{}}/>);
 expect(container.querySelectorAll('[data-slot="cindy-work"]')).toHaveLength(2);
 expect(screen.getByText("Now update the file.")).toBeTruthy();
});

it("shows Cindy's live thinking row before the first work event arrives",()=>{
 const {container}=render(<WorkProcess group={{kind:"response",key:"pending",messages:[],entries:[],running:true,hasTools:false,startedAt:100}} conversationId="test" onCopy={()=>{}}/>);
 expect(container.querySelector('[data-slot="cindy-thinking"]')).toBeTruthy();
 expect(screen.getByText("正在思考")).toBeTruthy();
});

it("retains nested work segments and details inside the completed disclosure",async()=>{
 const {container}=render(<WorkProcess group={process("completed-segments",false)} conversationId="test" onCopy={()=>{}}/>);
 expect(screen.queryByText("Now update the file.")).toBeNull();
 await userEvent.click(container.querySelector('[data-slot="cindy-work"]')!);
 expect(await screen.findByText("Now update the file.")).toBeTruthy();
 await waitFor(()=>expect(container.querySelectorAll('[data-slot="cindy-work"]')).toHaveLength(3));
 expect(screen.queryByText("Saved the result")).toBeNull();
 await userEvent.click(container.querySelectorAll('[data-slot="cindy-work"]')[2]);
 const row=await screen.findByRole("button",{name:/write.*result.txt|写入.*result.txt/});await userEvent.click(row);
 expect(await screen.findByText("Saved the result")).toBeTruthy();
});
