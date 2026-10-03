import {afterEach,expect,it,vi} from "vitest";
import {retakeScrollingDraft} from "./capture-draft-api";

const native=vi.hoisted(()=>({call:vi.fn()}));
vi.mock("../bridge",()=>({captureCall:native.call}));
afterEach(()=>native.call.mockReset());
const old="a".repeat(32);
const next={id:"b".repeat(32),revision:0,width:100,height:80,previewReady:true,sourceKind:"scroll",replaces:{id:old,revision:3},document:{version:1,width:100,height:80,marks:[]}};

it("recovers a committed replacement after losing the response without starting twice",async()=>{
 native.call.mockImplementation(async(method:string)=>{if(method==="scroll.retake")throw Error("connection lost");return {draft:next};});
 expect(await retakeScrollingDraft(old,3)).toMatchObject({id:next.id,replaces:next.replaces});
 expect(native.call.mock.calls.filter(([method])=>method==="scroll.retake")).toHaveLength(1);
});
it("does not adopt an unrelated replacement",async()=>{
 native.call.mockImplementation(async(method:string)=>{if(method==="scroll.retake")throw Error("connection lost");return {draft:{...next,replaces:{id:old,revision:2}}};});
 await expect(retakeScrollingDraft(old,3)).rejects.toThrow("connection lost");
});
it("leaves cancellation alone and never deletes the old draft",async()=>{
 native.call.mockResolvedValue({id:"job",state:"cancelled"});
 expect(await retakeScrollingDraft(old,3)).toBeUndefined();
 expect(native.call).toHaveBeenCalledExactlyOnceWith("scroll.retake",{expectedId:old,revision:3});
});
