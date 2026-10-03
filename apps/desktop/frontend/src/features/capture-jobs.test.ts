import {afterEach,expect,it,vi} from "vitest";
import {runCaptureJob} from "./capture-draft-api";
const call=vi.hoisted(()=>vi.fn());
vi.mock("../bridge",()=>({captureCall:call}));
afterEach(()=>{call.mockReset();vi.useRealTimers();vi.restoreAllMocks();});

it("returns a completed result only for the requested job",async()=>{
 vi.useFakeTimers();call.mockResolvedValueOnce({id:"one",state:"running"}).mockResolvedValueOnce({id:"one",state:"completed",result:{copied:true}});
 const pending=runCaptureJob("draft.copy",{expectedId:"draft"});
 await vi.advanceTimersByTimeAsync(300);
 expect(await pending).toEqual({state:"completed",result:{copied:true}});
 expect(call).toHaveBeenLastCalledWith("jobs.get",{id:"one"});
});
it("rejects a result from a different job",async()=>{
 vi.useFakeTimers();call.mockResolvedValueOnce({id:"one",state:"running"}).mockResolvedValueOnce({id:"other",state:"completed"});
 const assertion=expect(runCaptureJob("draft.copy",{})).rejects.toThrow("无法确认操作结果");
 await vi.advanceTimersByTimeAsync(300);await assertion;
});
it("ends a long wait without reporting success or cancelling the worker",async()=>{
 vi.spyOn(performance,"now").mockReturnValueOnce(0).mockReturnValue(300_000);
 call.mockResolvedValue({id:"slow",state:"running"});
 await expect(runCaptureJob("edit.preview",{})).rejects.toThrow("任务可能仍在进行");
 expect(call).toHaveBeenCalledExactlyOnceWith("edit.preview",{});
});
it("does not expire an interactive selection while the user is deciding",async()=>{
 vi.useFakeTimers();vi.spyOn(performance,"now").mockReturnValueOnce(0).mockReturnValue(600_000);
 call.mockResolvedValueOnce({id:"selection",state:"running"}).mockResolvedValueOnce({id:"selection",state:"cancelled"});
 const pending=runCaptureJob("capture.unified",{});
 await vi.advanceTimersByTimeAsync(300);
 expect(await pending).toEqual({state:"cancelled",result:undefined});
});
it("keeps waiting for a mutation acknowledgement instead of losing its outcome",async()=>{
 vi.useFakeTimers();vi.spyOn(performance,"now").mockReturnValueOnce(0).mockReturnValue(600_000);
 call.mockResolvedValueOnce({id:"save",state:"running"}).mockResolvedValueOnce({id:"save",state:"completed",result:{path:"saved.png"}});
 const pending=runCaptureJob("draft.complete",{});
 await vi.advanceTimersByTimeAsync(300);
 expect(await pending).toEqual({state:"completed",result:{path:"saved.png"}});
});
