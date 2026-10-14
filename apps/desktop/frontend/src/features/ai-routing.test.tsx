// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {act,cleanup,render,screen,waitFor} from "@testing-library/react";
import {AIRouting} from "./ai-routing";

const call=vi.hoisted(()=>vi.fn());
vi.mock("../bridge",()=>({captureCall:call}));
afterEach(()=>{cleanup();call.mockReset();});

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
