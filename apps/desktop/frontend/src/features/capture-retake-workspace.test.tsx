// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {act,cleanup,render,waitFor} from "@testing-library/react";
import {CaptureDraftWorkspace} from "./capture-draft-workspace";

const mocks=vi.hoisted(()=>({props:undefined as any,retake:vi.fn(),call:vi.fn(),reveal:vi.fn()}));
vi.mock("./capture-editor",()=>({default:(props:any)=>{mocks.props=props;return null;}}));
vi.mock("./capture-draft-api",()=>({retakeScrollingDraft:mocks.retake,readCaptureDraft:vi.fn(),runCaptureJob:vi.fn()}));
vi.mock("../bridge",()=>({captureCall:mocks.call,revealCaptureEditor:mocks.reveal,captureClosePending:async()=>false,subscribeCaptureClose:()=>()=>{},cancelCaptureClose:vi.fn(),setCaptureEditing:vi.fn(),captureSurfacePort:{}}));
afterEach(()=>{cleanup();vi.clearAllMocks();mocks.props=undefined;});
const draft={id:"a".repeat(32),revision:3,width:100,height:80,previewReady:true,sourceKind:"scroll" as const,document:{version:1 as const,width:100,height:80,marks:[]}};

it("keeps the old workspace when retaking is cancelled",async()=>{
 mocks.retake.mockResolvedValue(undefined);
 const replace=vi.fn(),done=vi.fn();
 render(<CaptureDraftWorkspace draft={draft} onReplace={replace} onDone={done}/>);
 await waitFor(()=>expect(mocks.props?.onRetake).toBeTypeOf("function"));
 await act(()=>mocks.props.onRetake(draft.document));
 expect(replace).not.toHaveBeenCalled();expect(done).not.toHaveBeenCalled();
 expect(mocks.props.finalized).toBe(false);expect(mocks.props.image.id).toBe(draft.id);
 expect(mocks.reveal).toHaveBeenCalled();
});
it("retries opening a committed replacement without capturing again",async()=>{
 const next={...draft,id:"b".repeat(32),revision:0,replaces:{id:draft.id,revision:3}};
 mocks.retake.mockResolvedValue(next);
 const replace=vi.fn().mockRejectedValueOnce(Error("Open failed")).mockResolvedValue(undefined);
 render(<CaptureDraftWorkspace draft={draft} onReplace={replace} onDone={()=>{}}/>);
 await waitFor(()=>expect(mocks.props?.onRetake).toBeTypeOf("function"));
 await act(async()=>{await expect(mocks.props.onRetake(draft.document)).rejects.toThrow("Open failed");});
 expect(mocks.props.finalized).toBe(true);expect(mocks.props.retakeLabel).toBe("打开新截图");
 await act(()=>mocks.props.onRetake(draft.document));
 expect(mocks.retake).toHaveBeenCalledTimes(1);expect(replace).toHaveBeenLastCalledWith(draft.id,next);
});
