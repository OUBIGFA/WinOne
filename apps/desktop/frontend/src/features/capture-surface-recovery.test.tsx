// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {act,cleanup,render,screen,waitFor} from "@testing-library/react";
import {CaptureDraftWorkspace} from "./capture-draft-workspace";

const mocks=vi.hoisted(()=>({end:vi.fn(),show:vi.fn(),release:vi.fn(),recovery:vi.fn(),reveal:vi.fn(),editing:vi.fn(),job:vi.fn(),editor:{props:undefined as any}}));
vi.mock("../bridge",()=>({captureCall:vi.fn(),cancelCaptureClose:vi.fn(),captureClosePending:async()=>false,setCaptureEditing:mocks.editing,subscribeCaptureClose:()=>()=>{},captureSurfacePort:{end:mocks.end,show:mocks.show,release:mocks.release,recovery:mocks.recovery},revealCaptureEditor:mocks.reveal}));
vi.mock("./capture-draft-api",()=>({readCaptureDraft:vi.fn(),runCaptureJob:mocks.job}));
vi.mock("./capture-editor",()=>({default:(props:any)=>{mocks.editor.props=props;return <div><span>{props.surface?"surface":"regular"}</span><p role="status">{props.saveError}</p></div>;}}));
afterEach(()=>{cleanup();vi.resetAllMocks();vi.unstubAllGlobals();mocks.editor.props=undefined;});
const draft={id:"a".repeat(32),revision:0,width:100,height:80,previewReady:true,document:{version:1 as const,width:100,height:80,marks:[]}};
const surface={desktopPreviewId:"b".repeat(32),placement:{x:0,y:0,desktopX:0,desktopY:0,desktopWidth:100,desktopHeight:80},monitor:{left:0,top:0,right:100,bottom:80}};

it("keeps the native frame until the shown editor has crossed a paint boundary",async()=>{
 const frames:FrameRequestCallback[]=[];
 vi.stubGlobal("requestAnimationFrame",(callback:FrameRequestCallback)=>{frames.push(callback);return frames.length;});
 let shown!:()=>void;
 mocks.show.mockImplementation(()=>new Promise<void>(resolve=>{shown=resolve;}));
 render(<CaptureDraftWorkspace draft={draft} initialSurface={surface} onDone={()=>{}}/>);
 await screen.findByText("surface");
 let ready!:Promise<void>;
 act(()=>{ready=mocks.editor.props.onSurfaceReady();});
 expect(mocks.release).not.toHaveBeenCalled();expect(frames).toHaveLength(0);
 await act(async()=>shown());expect(frames).toHaveLength(1);
 await act(async()=>frames.shift()!(0));expect(mocks.release).not.toHaveBeenCalled();
 await act(async()=>{frames.shift()!(16);await ready;});
 expect(mocks.release).toHaveBeenCalledExactlyOnceWith(draft.id);
});

it("passes an explicit save-as request without changing the draft on cancellation",async()=>{
 mocks.job.mockResolvedValue({state:"cancelled"});
 render(<CaptureDraftWorkspace draft={draft} onDone={()=>{}}/>);await screen.findByText("regular");
 let saved=true;await act(async()=>{saved=await mocks.editor.props.onSaveAs(draft.document);});
 expect(saved).toBe(false);expect(mocks.editor.props.finalized).toBe(false);
 expect(mocks.job).toHaveBeenCalledExactlyOnceWith("draft.complete",{expectedId:draft.id,revision:0,saveAs:true});
});

it("coalesces recovery requests and retains its warning after autosave",async()=>{
 let finish!:()=>void;
 mocks.end.mockImplementation(()=>new Promise<void>(resolve=>{finish=resolve;}));mocks.reveal.mockResolvedValue(undefined);
 render(<CaptureDraftWorkspace draft={draft} initialSurface={surface} onDone={()=>{}}/>);await screen.findByText("surface");
 act(()=>{mocks.editor.props.onSurfaceError(Error("changed"));mocks.editor.props.onSurfaceError(Error("changed"));});
 await waitFor(()=>expect(mocks.end).toHaveBeenCalledOnce());
 await act(async()=>finish());await screen.findByText("regular");
 await act(async()=>mocks.editor.props.onDocumentChange(draft.document));
 expect(screen.getByRole("status").textContent).toContain("changed");expect(mocks.reveal).toHaveBeenCalledOnce();
});

it("keeps restoration errors visible in a recoverable editor",async()=>{
 mocks.end.mockRejectedValue(Error("restore unavailable"));
 render(<CaptureDraftWorkspace draft={draft} initialSurface={surface} onDone={()=>{}}/>);await screen.findByText("surface");
 act(()=>mocks.editor.props.onSurfaceError(Error("changed")));
 await waitFor(()=>expect(screen.getByRole("status").textContent).toContain("restore unavailable"));
 await act(async()=>mocks.editor.props.onDocumentChange(draft.document));
 expect(screen.getByRole("status").textContent).toContain("restore unavailable");expect(screen.getByText("regular")).toBeTruthy();expect(mocks.recovery).toHaveBeenCalledOnce();expect(mocks.reveal).not.toHaveBeenCalled();
});

it("retries window restoration without cancelling a discarded draft twice",async()=>{
 mocks.job.mockResolvedValue({state:"completed"});mocks.end.mockRejectedValueOnce(Error("retry restore")).mockResolvedValue(undefined);mocks.editing.mockResolvedValue(undefined);
 const done=vi.fn();render(<CaptureDraftWorkspace draft={draft} initialSurface={surface} onDone={done}/>);await screen.findByText("surface");
 await act(async()=>{await expect(mocks.editor.props.onClose()).rejects.toThrow("retry restore");});
 expect(mocks.editor.props.finalized).toBe(true);
 await act(async()=>mocks.editor.props.onClose());
 expect(mocks.job).toHaveBeenCalledExactlyOnceWith("draft.cancel",{expectedId:draft.id,revision:0});expect(mocks.end).toHaveBeenCalledTimes(2);expect(done).toHaveBeenCalledOnce();
});
