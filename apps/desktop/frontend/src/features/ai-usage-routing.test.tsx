// @vitest-environment jsdom
import {afterEach,expect,it} from "vitest";
import {cleanup,render,screen} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {AIUsage} from "./ai-usage";

const previous=window.go;
afterEach(()=>{cleanup();window.go=previous;});

// Keep the real bridge: replacing captureCall hid requests sent to the wrong
// native namespace. Only the native transport is replaced by an isolated host.
function connectUsageHost(){
 const totals={calls:1,errors:0,input:80,output:20,cache_read:0,cache_write:0,cost:0.03,unpriced:0};
 const counts={...totals,sessions:1,messages:2,reasoning:0,compactions:0,activeMs:1000};
 const session={...counts,id:"registered",title:"已登记的测试会话",project:"project",provider:"fixture",model:"model",createdAt:"2026-10-10T01:00:00Z",updatedAt:"2026-10-10T01:00:01Z",archived:false,context:[],tools:{},skills:{},roles:{user:1,assistant:1}};
 const prompt={window:1000,tokens:80,counted:true,parts:[{kind:"chat",tokens:80,items:[]}]};
 Object.assign(window,{go:{main:{App:{
  async Capture(method:string){
   if(method!=="ai.usage.get")throw Error("不支持的模块操作 (unsupported)");
   return JSON.stringify({rows:[],sum:totals,offset:0,limit:15,total:1,by:{},context:{agents:[],sessions:[{agent:"fixture",key:"registered",model:"model",requests:1,peak:80,window:1000,latest:prompt,latestId:1,points:[]}]},contextTotal:1,contextRequests:[]});
  },
  async Chat(method:string){
   if(method==="catalog")return JSON.stringify({projects:[{id:"project",name:"测试项目"}],conversations:[{id:"registered",title:"已登记的测试会话"}]});
   if(method==="usage")return JSON.stringify({rows:[session],sum:counts,series:[],by:{},total:1,offset:0,limit:15,median:100,p90:100,top:[],tools:{},skills:{},warnings:[],shape:{}});
   throw Error("不支持的模块操作 (unsupported)");
  },
 }}}});
}

it("routes session statistics through native Chat and renders the registered history",async()=>{
 connectUsageHost();render(<AIUsage active providers={[]}/>);
 await userEvent.setup().click(screen.getByRole("tab",{name:"会话"}));
 expect(await screen.findByRole("button",{name:/已登记的测试会话/})).toBeTruthy();
 expect(screen.getByText("$0.030000")).toBeTruthy();
 expect(screen.queryByRole("alert")).toBeNull();
});

it("uses the Chat catalog to label context requests without crossing the Capture namespace",async()=>{
 connectUsageHost();render(<AIUsage active providers={[]}/>);
 await userEvent.setup().click(screen.getByRole("tab",{name:"上下文"}));
 expect(await screen.findByRole("button",{name:/已登记的测试会话/})).toBeTruthy();
 expect(screen.queryByRole("alert")).toBeNull();
});
