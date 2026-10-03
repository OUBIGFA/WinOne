// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {act,cleanup,render,waitFor} from "@testing-library/react";
import {CaptureDraftWorkspace} from "./capture-draft-workspace";
import type {AnnotationDocument,EditPreview} from "./capture-editor-model";

const boundary=vi.hoisted(()=>({props:undefined as undefined|{onPreview:(document:AnnotationDocument)=>Promise<EditPreview>;onDocumentChange:(document:AnnotationDocument)=>void;captureNotice?:string;initialZoom?:string},job:vi.fn()}));
vi.mock("./capture-editor",()=>({default:(props:typeof boundary.props)=>{boundary.props=props;return null;}}));
vi.mock("./capture-draft-api",()=>({runCaptureJob:boundary.job,readCaptureDraft:vi.fn()}));
vi.mock("../bridge",()=>({subscribeCaptureClose:()=>()=>{},captureClosePending:async()=>false,setCaptureEditing:async()=>{},cancelCaptureClose:async()=>{}}));
afterEach(()=>{cleanup();boundary.props=undefined;boundary.job.mockReset();});

it("keeps the stopped-capture warning visible in the unified editor",async()=>{
 const document:AnnotationDocument={version:1,width:100,height:80,marks:[]};
 render(<CaptureDraftWorkspace draft={{id:"a".repeat(32),width:100,height:80,revision:0,previewReady:true,document,notice:"scrollStopped"}} onDone={()=>{}}/>);
 await waitFor(()=>expect(boundary.props?.captureNotice).toBe("已停止，请检查截图后保存。"));
});

it("opens scrolling captures at fit-width",async()=>{
 const document:AnnotationDocument={version:1,width:600,height:20000,marks:[]};
 render(<CaptureDraftWorkspace draft={{id:"a".repeat(32),width:600,height:20000,revision:0,previewReady:true,document,sourceKind:"scroll"}} onDone={()=>{}}/>);
 await waitFor(()=>expect(boundary.props?.initialZoom).toBe("width"));
});

it("renders a partial preview without persisting it as the draft",async()=>{
  const id="a".repeat(32);
  const document:AnnotationDocument={version:2,width:100,height:80,crop:{x:5,y:5,width:50,height:50},marks:[]};
  boundary.job.mockResolvedValue({state:"completed",result:{id:"b".repeat(32),sourceId:id,width:100,height:80}});
  render(<CaptureDraftWorkspace draft={{id,width:100,height:80,revision:3,previewReady:true,document}} onDone={()=>{}}/>);
  await waitFor(()=>expect(boundary.props).toBeTruthy());
  const prefix:AnnotationDocument={...document,crop:undefined};
  await act(async()=>{await boundary.props!.onPreview(prefix);});
  expect(boundary.job).toHaveBeenCalledExactlyOnceWith("edit.preview",{expectedId:id,document:prefix});
});

it("waits for autosave before starting a renderer task without persisting its partial document",async()=>{
 const id="a".repeat(32),document:AnnotationDocument={version:2,width:100,height:80,crop:{x:5,y:5,width:50,height:50},marks:[]};
 let updated!:(value:unknown)=>void;
 boundary.job.mockImplementationOnce(()=>new Promise(resolve=>{updated=resolve;})).mockResolvedValue({state:"completed",result:{id:"b".repeat(32),sourceId:id,width:100,height:80}});
 render(<CaptureDraftWorkspace draft={{id,width:100,height:80,revision:3,previewReady:true,document}} onDone={()=>{}}/>);
 await waitFor(()=>expect(boundary.props).toBeTruthy());
 const changed={...document,crop:{x:5,y:5,width:60,height:50}};
 act(()=>boundary.props!.onDocumentChange(changed));
 await waitFor(()=>expect(boundary.job).toHaveBeenCalledOnce());
 let painting!:Promise<EditPreview>;
 act(()=>{painting=boundary.props!.onPreview({...changed,crop:undefined});});
 expect(boundary.job).toHaveBeenCalledOnce();
 await act(async()=>{updated({state:"completed",result:{draftId:id,revision:4}});await painting;});
 expect(boundary.job).toHaveBeenNthCalledWith(1,"draft.update",{expectedId:id,revision:3,document:changed});
 expect(boundary.job).toHaveBeenNthCalledWith(2,"edit.preview",{expectedId:id,document:{...changed,crop:undefined}});
});
