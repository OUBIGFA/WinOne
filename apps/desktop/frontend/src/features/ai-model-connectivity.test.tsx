// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {act,cleanup,render,screen,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {AIProviderModels} from "./ai-provider-models";
const api=vi.hoisted(()=>({captureCall:vi.fn()}));vi.mock("../bridge",()=>api);
afterEach(()=>{cleanup();vi.clearAllMocks();});
const provider={id:"draft",name:"Draft",authMode:"api-key" as const,hasKey:false,baseUrl:"https://fixture.invalid/v1",protocol:"responses",headers:{"X-Team":"fixture"},models:["one","two"],availableModels:["one","two","disabled"],modelMetadata:{one:{upstreamName:"actual",api:"responses"}}};
function fixture(onBusy=vi.fn()){return render(<AIProviderModels provider={provider} canFetch draftConnection keyUpdates={{primary:"not-a-real-key"}} disabled={false} onBusy={onBusy} onChange={()=>{}}/>);}

it("tests only enabled models using the unsaved connection and shows each real result",async()=>{
 let request:any;api.captureCall.mockImplementation(async(method,value)=>{expect(method).toBe("ai.models.test");request=value;return {results:[{model:"one",ok:true,ms:123,protocol:"responses"},{model:"two",ok:false,ms:90,status:404,error:"Model unavailable"}],clients:{codex:"","claude-code":"Run Claude Code through the gateway once"}};});
 fixture();await userEvent.click(screen.getByRole("button",{name:"测试已选模型"}));
 expect(await screen.findByText(/Model unavailable/)).toBeTruthy();expect(screen.getByText(/123 ms/)).toBeTruthy();
 expect(request).toMatchObject({provider:"draft",models:["one","two"],connection:{baseUrl:"https://fixture.invalid/v1",protocol:"responses",headers:{"X-Team":"fixture"}},keyUpdates:{primary:"not-a-real-key"},metadata:{one:{upstreamName:"actual",api:"responses"}}});
 expect(screen.queryByText(/全部模型测试通过/)).toBeNull();
 expect(screen.getByText(/Run Claude Code through the gateway once/)).toBeTruthy();
});

it("tests a single model as Codex and blocks overlapping tests until it finishes",async()=>{
 let finish!:(value:unknown)=>void,request:any;api.captureCall.mockImplementation((method,value)=>{request=value;return new Promise(resolve=>finish=resolve);});const busy=vi.fn();fixture(busy);const user=userEvent.setup();
 await user.click(screen.getByRole("button",{name:"模型操作：one"}));await user.click(await screen.findByRole("menuitem",{name:"以 Codex 测试"}));
 expect(screen.getByRole("button",{name:"测试已选模型"}).hasAttribute("disabled")).toBe(true);expect(busy).toHaveBeenLastCalledWith(true);
 await act(async()=>finish({results:[{model:"one",ok:true,ms:77,protocol:"responses"}]}));
 expect(await screen.findByText(/77 ms/)).toBeTruthy();expect(request).toMatchObject({models:["one"],as:"codex"});expect(busy).toHaveBeenLastCalledWith(false);
});

it("keeps an IPC failure visible and permits retry without reporting success",async()=>{
 api.captureCall.mockRejectedValueOnce(Error("Connection refused")).mockResolvedValue({results:[{model:"one",ok:true,ms:1},{model:"two",ok:true,ms:2}]});fixture();const user=userEvent.setup();await user.click(screen.getByRole("button",{name:"测试已选模型"}));expect(await screen.findByText(/Connection refused/)).toBeTruthy();
 await waitFor(()=>expect(screen.getByRole("button",{name:"测试已选模型"}).hasAttribute("disabled")).toBe(false));await user.click(screen.getByRole("button",{name:"测试已选模型"}));expect(await screen.findByText("全部 2 个模型测试通过")).toBeTruthy();
});
