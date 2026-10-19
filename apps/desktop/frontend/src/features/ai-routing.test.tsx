// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {act,cleanup,fireEvent,render,screen,waitFor} from "@testing-library/react";
import {AIRouting} from "./ai-routing";

const call=vi.hoisted(()=>vi.fn());
vi.mock("../bridge",()=>({captureCall:call}));
afterEach(()=>{cleanup();call.mockReset();});
it("sends the native theme attribute to the routing document",async()=>{
 document.documentElement.dataset.theme="dark";
 try{
  call.mockResolvedValue({url:"http://127.0.0.1:3456/"});render(<AIRouting active enabled onEnabled={()=>{}}/>);
  const frame=await screen.findByTitle("路由") as HTMLIFrameElement,send=vi.spyOn(frame.contentWindow!,"postMessage");
  fireEvent.load(frame);expect(send).toHaveBeenLastCalledWith(expect.objectContaining({theme:"dark"}),"http://127.0.0.1:3456");
  await act(async()=>{document.documentElement.dataset.theme="light";});
  expect(send).toHaveBeenLastCalledWith(expect.objectContaining({theme:"light"}),"http://127.0.0.1:3456");
 }finally{delete document.documentElement.dataset.theme;}
});
it("releases a routing viewer whose open finishes after leaving",async()=>{
 let finish!:(value:unknown)=>void;call.mockImplementation(method=>method==="ai.routing.open"?new Promise(resolve=>{finish=resolve;}):Promise.resolve({}));
 const view=render(<AIRouting active enabled={false} onEnabled={()=>{}}/>);
 view.unmount();
 expect(call.mock.calls.filter(([method])=>method==="ai.routing.close")).toHaveLength(1);
 await act(async()=>{finish({url:"http://127.0.0.1:3456/"});});
 expect(call.mock.calls.filter(([method])=>method==="ai.routing.close")).toHaveLength(2);
});

it("fits the complete routing editor and ignores resize messages from other windows",async()=>{
 call.mockResolvedValue({url:"http://127.0.0.1:3456/?view=routing"});
 render(<AIRouting active enabled onEnabled={()=>{}}/>);
 const frame=await waitFor(()=>{const node=screen.getByTitle("路由") as HTMLIFrameElement;expect(node.tagName).toBe("IFRAME");return node;});
 const resize=(source:MessageEventSource|null,origin:string,height:unknown)=>act(()=>window.dispatchEvent(new MessageEvent("message",{source,origin,data:{type:"winone-routing-height",height}})));
 resize(window,"http://127.0.0.1:3456",2500);expect(frame.style.height).toBe("600px");
 resize(frame.contentWindow,"http://untrusted.invalid",2500);expect(frame.style.height).toBe("600px");
 resize(frame.contentWindow,"http://127.0.0.1:3456",2500);expect(frame.style.height).toBe("2500px");
 resize(frame.contentWindow,"http://127.0.0.1:3456",900);expect(frame.style.height).toBe("900px");
 resize(frame.contentWindow,"http://127.0.0.1:3456",-1);expect(frame.style.height).toBe("900px");
});
