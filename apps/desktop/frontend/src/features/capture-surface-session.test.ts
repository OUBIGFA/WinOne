import {expect,it,vi} from "vitest";
import {prepareCaptureSurface,openCaptureEditor} from "./capture-surface-session";

const draft={id:"a".repeat(32),revision:0,width:100,height:80,previewReady:true,document:{version:1 as const,width:100,height:80,marks:[]},placement:{x:0,y:0,desktopX:0,desktopY:0,desktopWidth:100,desktopHeight:80},desktopPreviewId:"b".repeat(32)};
const monitor={left:0,top:0,right:100,bottom:80};
it("does not substitute an ordinary editor for a fresh screenshot",async()=>{
 const port={begin:vi.fn(async()=>false),monitor:vi.fn(),end:vi.fn(),recovery:vi.fn(),setEditing:vi.fn(async()=>{}),abortOpening:vi.fn(async()=>{}),revealEditor:vi.fn()};
 await expect(openCaptureEditor({...draft,sourceKind:"region",selectionPending:true},port,"Surface unavailable")).rejects.toThrow("Surface unavailable");
 expect(port.revealEditor).not.toHaveBeenCalled();expect(port.abortOpening).toHaveBeenCalledOnce();
});
it("releases the close guard when opening an ordinary editor fails",async()=>{
 const port={begin:vi.fn(),monitor:vi.fn(),end:vi.fn(),recovery:vi.fn(),setEditing:vi.fn(async()=>{}),abortOpening:vi.fn(async()=>{}),revealEditor:vi.fn(async()=>{throw Error("window unavailable");})};
 await expect(openCaptureEditor({...draft,placement:undefined},port,"fallback")).rejects.toThrow("window unavailable");
 expect(port.setEditing).toHaveBeenCalledExactlyOnceWith(true);expect(port.abortOpening).toHaveBeenCalledOnce();
});
it("retains activation and cleanup errors when acknowledgement is uncertain",async()=>{
 const port={begin:vi.fn(),monitor:vi.fn(),end:vi.fn(),recovery:vi.fn(),setEditing:vi.fn(async()=>{throw Error("activation lost");}),abortOpening:vi.fn(async()=>{throw Error("cleanup lost");}),revealEditor:vi.fn()};
 await expect(openCaptureEditor(draft,port,"fallback")).rejects.toThrow(/activation lost.*cleanup lost/);
 expect(port.abortOpening).toHaveBeenCalledOnce();expect(port.begin).not.toHaveBeenCalled();
});
it("keeps the guard active after a successful surface opening",async()=>{
 const port={begin:vi.fn(async()=>true),monitor:vi.fn(async()=>monitor),end:vi.fn(),recovery:vi.fn(),setEditing:vi.fn(async()=>{}),abortOpening:vi.fn(),revealEditor:vi.fn()};
 expect((await openCaptureEditor(draft,port,"fallback")).surface).toBeDefined();
 expect(port.abortOpening).not.toHaveBeenCalled();expect(port.revealEditor).not.toHaveBeenCalled();
});
it("uses ordinary editing for a legacy image without moving the window",async()=>{
 const port={begin:vi.fn(),monitor:vi.fn(),end:vi.fn(),recovery:vi.fn()};
 expect(await prepareCaptureSurface({...draft,placement:undefined},port,"fallback")).toEqual({warning:"",recoveryShown:false});
 expect(port.begin).not.toHaveBeenCalled();
});
it("returns the monitor only after successful entry",async()=>{
 const port={begin:vi.fn(async()=>true),monitor:vi.fn(async()=>monitor),end:vi.fn(),recovery:vi.fn()};
 const result=await prepareCaptureSurface(draft,port,"fallback");
 expect(result.surface?.monitor).toEqual(monitor);expect(port.end).not.toHaveBeenCalled();
});
it("exposes a recovery editor if entry and restoration both fail",async()=>{
 const port={begin:vi.fn(async()=>true),monitor:vi.fn(async()=>{throw Error("monitor failed");}),end:vi.fn(async()=>{throw Error("restore failed");}),recovery:vi.fn(async()=>{})};
 const result=await prepareCaptureSurface(draft,port,"fallback");
 expect(result.surface).toBeUndefined();expect(result.recoveryShown).toBe(true);expect(result.warning).toContain("monitor failed");expect(result.warning).toContain("restore failed");expect(port.recovery).toHaveBeenCalledExactlyOnceWith(draft.id);
});
it("retains every failure when even the recovery window cannot be shown",async()=>{
 const port={begin:async()=>{throw Error("entry failed");},monitor:vi.fn(),end:async()=>{throw Error("restore failed");},recovery:async()=>{throw Error("show failed");}};
 await expect(prepareCaptureSurface(draft,port,"fallback")).rejects.toThrow(/entry failed.*restore failed.*show failed/);
});
