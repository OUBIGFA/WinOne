// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from "vitest";
import {act,cleanup,fireEvent,render,screen,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type {ReactNode} from "react";
import CaptureEditor from "./capture-editor";
import type {AnnotationDocument} from "./capture-editor-model";

// Geometry and pixels are tested in the renderer; here exercise the actual
// editor controls, history, image-loading boundary and document callbacks.
vi.mock("react-konva",()=>{
  const Group=({children}:{children?:ReactNode})=><div>{children}</div>;
  const Shape=()=>null;
  return {Stage:Group,Layer:Group,Group,Rect:Shape,Ellipse:Shape,Arrow:Shape,Line:Shape,Text:Shape};
});
beforeEach(()=>{vi.stubGlobal("PointerEvent",MouseEvent);vi.stubGlobal("ResizeObserver",class {observe(){}unobserve(){}disconnect(){}});});
afterEach(()=>{cleanup();vi.unstubAllGlobals();vi.restoreAllMocks();});

it("keeps the viewed source point when zooming the live canvas",async()=>{
 const changed=vi.fn();
 render(<CaptureEditor image={{id:"a".repeat(32),width:600,height:40000}} initialZoom="width" sourceImage onDocumentChange={changed} onSave={async()=>false} onClose={()=>{}} onPreview={async()=>({id:"b".repeat(32),sourceId:"a".repeat(32),width:600,height:40000})}/>);
 fireEvent.load(await screen.findByAltText("最近截图预览"));
 const user=userEvent.setup();
 const target=screen.getByRole("group",{name:"标注画布"});
 const scroll=target.closest(".capture-editor-scroll") as HTMLDivElement;
 Object.defineProperties(scroll,{clientWidth:{value:800},clientHeight:{value:500}});
 scroll.getBoundingClientRect=()=>new DOMRect(0,0,800,500);
 target.getBoundingClientRect=()=>new DOMRect(-scroll.scrollLeft,-scroll.scrollTop,parseFloat(target.style.width),parseFloat(target.style.height));
 scroll.scrollTop=15000;
 const calls=changed.mock.calls.length;
 await user.click(screen.getByRole("combobox",{name:"缩放"}));
 await user.click(await screen.findByRole("option",{name:"200%"}));
 await waitFor(()=>expect(scroll.scrollTop).toBe(30250));
 expect(scroll.scrollLeft).toBe(400);
 expect(changed).toHaveBeenCalledTimes(calls);
});

it("reads a long image at fit-width while allocating vectors only for its visible rows",async()=>{
 const changed=vi.fn();
 render(<CaptureEditor image={{id:"a".repeat(32),width:600,height:40000}} initialZoom="width" sourceImage onDocumentChange={changed} onSave={async()=>false} onClose={()=>{}}/>);
 const canvas=screen.getByRole("group",{name:"标注画布"});
 const scroll=canvas.closest(".capture-editor-scroll") as HTMLElement;
 Object.defineProperties(scroll,{clientWidth:{value:800},clientHeight:{value:500}});
 scroll.getBoundingClientRect=()=>new DOMRect(0,0,800,500);
 canvas.getBoundingClientRect=()=>new DOMRect(0,-scroll.scrollTop,600,40000);
 fireEvent.load(await screen.findByAltText("最近截图预览"));
 expect(canvas.style.width).toBe("600px");expect(canvas.style.height).toBe("40000px");
 const calls=changed.mock.calls.length;
 scroll.scrollTop=30000;fireEvent.scroll(scroll);
 await waitFor(()=>expect((document.querySelector(".capture-editor-vector-slice") as HTMLElement)?.style.top).toBe("30000px"));
 const vectors=document.querySelector(".capture-editor-vector-slice") as HTMLElement;
 expect(vectors.style.width).toBe("600px");expect(vectors.style.height).toBe("500px");
 expect(changed).toHaveBeenCalledTimes(calls);
});

it("keeps the initial desktop selection and expands it without exposing a clear-crop action",async()=>{
 const changed=vi.fn();
 const initial:AnnotationDocument={version:2,width:200,height:160,crop:{x:20,y:30,width:100,height:80},marks:[]};
 render(<CaptureEditor image={{id:"a".repeat(32),width:200,height:160}} initialDocument={initial} selectionRequired sourceImage onDocumentChange={changed} onSave={async()=>false} onClose={()=>{}}/>);
 fireEvent.load(await screen.findByAltText("最近截图预览"));
 expect(screen.getByAltText("最近截图预览").getAttribute("src")).toContain("/capture-source/");
 expect(screen.queryByRole("button",{name:"取消裁剪"})).toBeNull();
 const left=screen.getByRole("button",{name:"调整左边缘"});
 left.focus();await userEvent.setup().keyboard("{Shift>}{ArrowLeft}{/Shift}");
 expect(changed.mock.lastCall?.[0].crop).toEqual({x:10,y:30,width:110,height:80});
});

it("resizes a crop without losing marks, supports undo and cancels a drag with Escape",async()=>{
 const changed=vi.fn(),close=vi.fn();
 const initial:AnnotationDocument={version:2,width:200,height:160,crop:{x:20,y:30,width:100,height:80},marks:[{kind:"rectangle",x:25,y:35,width:20,height:20,color:"#ff0000",strokeWidth:3}]};
 render(<CaptureEditor image={{id:"a".repeat(32),width:200,height:160}} initialDocument={initial} onDocumentChange={changed} onSave={async()=>false} onClose={close}/>);
 fireEvent.load(await screen.findByAltText("最近截图预览"));
 const user=userEvent.setup();
 await user.click(screen.getByRole("button",{name:"裁剪"}));
 let right=screen.getByRole("button",{name:"调整右边缘"});
 right.focus();await user.keyboard("{Shift>}{ArrowRight}{/Shift}");
 expect(changed.mock.lastCall?.[0].crop).toEqual({...initial.crop,width:110});
 expect(changed.mock.lastCall?.[0].marks).toEqual(initial.marks);
 await user.click(screen.getByRole("button",{name:"撤销"}));
 expect(changed.mock.lastCall?.[0].crop).toEqual(initial.crop);
 right=screen.getByRole("button",{name:"调整右边缘"});
 right.setPointerCapture=vi.fn();right.releasePointerCapture=vi.fn();
 fireEvent.pointerDown(right,{button:0,clientX:120,clientY:70});
 fireEvent.pointerMove(right,{clientX:150,clientY:70});
 expect((screen.getByRole("button",{name:"保存图片"}) as HTMLButtonElement).disabled).toBe(true);
 fireEvent.keyDown(right,{key:"Escape"});
 expect(close).not.toHaveBeenCalled();
 expect(changed.mock.lastCall?.[0].crop).toEqual(initial.crop);
 expect((screen.getByRole("button",{name:"保存图片"}) as HTMLButtonElement).disabled).toBe(false);
});

it("offers Save as and keeps the editor open when its picker is cancelled",async()=>{
 const save=vi.fn(async()=>true),saveAs=vi.fn(async()=>false),close=vi.fn();
 render(<CaptureEditor image={{id:"a".repeat(32),width:100,height:80}} onSave={save} onSaveAs={saveAs} onSaveAndCopy={async()=>true} onClose={close}/>);
 fireEvent.load(await screen.findByAltText("最近截图预览"));
 expect(screen.getByText(/双击截图保存并复制/)).toBeTruthy();
 await userEvent.setup().click(screen.getByRole("button",{name:"另存为"}));
 await screen.findByText("图片未保存，标注已保留");
 expect(saveAs).toHaveBeenCalledOnce();expect(save).not.toHaveBeenCalled();expect(close).not.toHaveBeenCalled();
});

it("locks a finalized document but permits retrying close without discarding again",async()=>{
 const close=vi.fn(async()=>{});
 render(<CaptureEditor image={{id:"a".repeat(32),width:100,height:80}} initialDocument={{version:1,width:100,height:80,marks:[]}} finalized onSave={async()=>true} onClose={close}/>);
 expect((screen.getByRole("button",{name:"保存图片"}) as HTMLButtonElement).disabled).toBe(true);
 expect(screen.getByRole("button",{name:"矩形"}).matches(":disabled")).toBe(true);
 expect(document.querySelector(".capture-editor-scroll")?.hasAttribute("inert")).toBe(true);
 await userEvent.setup().click(screen.getByRole("button",{name:"关闭"}));
 expect(close).toHaveBeenCalledOnce();expect(screen.queryByText("关闭并放弃当前草稿？")).toBeNull();
});

it("positions a surface image at its capture origin and waits for both images",async()=>{
 const ready=vi.fn(async()=>{});
 const surface={desktopPreviewId:"b".repeat(32),placement:{x:100,y:120,desktopX:0,desktopY:0,desktopWidth:window.innerWidth,desktopHeight:window.innerHeight},monitor:{left:0,top:0,right:window.innerWidth,bottom:window.innerHeight}};
 render(<CaptureEditor image={{id:"a".repeat(32),width:100,height:80}} surface={surface} onSurfaceReady={ready} onSave={async()=>false} onClose={()=>{}}/>);
 const canvas=screen.getByRole("group",{name:"标注画布"});
 expect(canvas.style.left).toBe("100px");expect(canvas.style.top).toBe("120px");
 expect(screen.getByAltText("最近截图预览").getAttribute("src")).toBe(`/capture-source/${"a".repeat(32)}.png`);
 fireEvent.load(await screen.findByAltText("最近截图预览"));expect(ready).not.toHaveBeenCalled();
 fireEvent.load(document.querySelector(".capture-surface-background")!);
 await waitFor(()=>expect(ready).toHaveBeenCalledOnce());
 expect(screen.queryByRole("combobox",{name:"缩放"})).toBeNull();
});

it("requests recovery once when the displayed surface changes size",async()=>{
 const ready=vi.fn(async()=>{}),failed=vi.fn();
 const original=Object.getOwnPropertyDescriptor(window,"innerWidth")!;
 const surface={desktopPreviewId:"b".repeat(32),placement:{x:100,y:120,desktopX:0,desktopY:0,desktopWidth:window.innerWidth,desktopHeight:window.innerHeight},monitor:{left:0,top:0,right:window.innerWidth,bottom:window.innerHeight}};
 render(<CaptureEditor image={{id:"a".repeat(32),width:100,height:80}} surface={surface} onSurfaceReady={ready} onSurfaceError={failed} onSave={async()=>false} onClose={()=>{}}/>);
 fireEvent.load(await screen.findByAltText("最近截图预览"));fireEvent.load(document.querySelector(".capture-surface-background")!);
 await waitFor(()=>expect(ready).toHaveBeenCalledOnce());
 try {
  Object.defineProperty(window,"innerWidth",{configurable:true,value:surface.placement.desktopWidth+100});
  fireEvent.resize(window);fireEvent.resize(window);
  expect(failed).toHaveBeenCalledOnce();
  expect((screen.getByRole("button",{name:"保存图片"}) as HTMLButtonElement).disabled).toBe(true);
 } finally {Object.defineProperty(window,"innerWidth",original);}
});

it("double click saves and copies the current document through one completion action",async()=>{
 const save=vi.fn(async()=>true),saveCopy=vi.fn(async()=>true),close=vi.fn();
 render(<CaptureEditor image={{id:"a".repeat(32),width:100,height:80}} onSave={save} onSaveAndCopy={saveCopy} onClose={close}/>);
 fireEvent.load(await screen.findByAltText("最近截图预览"));
 fireEvent.doubleClick(screen.getByRole("group",{name:"标注画布"}));
 await waitFor(()=>expect(close).toHaveBeenCalledOnce());
 expect(saveCopy).toHaveBeenCalledOnce();expect(save).not.toHaveBeenCalled();
 expect(saveCopy.mock.calls[0]).toHaveLength(1);
});

it("edits annotation properties beside the canvas without a second dialog",async()=>{
  const initial:AnnotationDocument={version:2,width:100,height:80,marks:[{kind:"rectangle",x:10,y:10,width:30,height:30,color:"#ff0000",strokeWidth:3}]};
  const changed=vi.fn();
  render(<CaptureEditor image={{id:"a".repeat(32),width:100,height:80}} initialDocument={initial} onDocumentChange={changed} onSave={async()=>false} onClose={()=>{}}/>);
  fireEvent.load(await screen.findByAltText("最近截图预览"));
  const user=userEvent.setup();
  await user.click(screen.getByRole("button",{name:"选择"}));
  const canvas=screen.getByRole("group",{name:"标注画布"});
  vi.spyOn(canvas,"getBoundingClientRect").mockReturnValue({left:0,top:0,width:100,height:80,right:100,bottom:80,x:0,y:0,toJSON:()=>({})});
  canvas.setPointerCapture=vi.fn();canvas.releasePointerCapture=vi.fn();
  fireEvent.pointerDown(canvas,{button:0,pointerId:1,clientX:20,clientY:20});
  fireEvent.pointerUp(canvas,{button:0,pointerId:1,clientX:20,clientY:20});
  await user.click(screen.getByRole("button",{name:"工具设置"}));
  await user.click(await screen.findByRole("button",{name:"修改标注"}));
  await waitFor(()=>expect(screen.getAllByRole("dialog")).toHaveLength(1));
  expect(screen.getByRole("complementary",{name:"修改标注"})).toBeTruthy();
  expect(canvas.closest("[inert]")).not.toBeNull();
  expect((screen.getByRole("button",{name:"保存图片"}) as HTMLButtonElement).disabled).toBe(true);
  await user.click(screen.getByRole("button",{name:"取消"}));
  expect(screen.queryByRole("complementary")).toBeNull();
  expect(canvas.closest("[inert]")).toBeNull();
  expect((screen.getByRole("button",{name:"保存图片"}) as HTMLButtonElement).disabled).toBe(false);
  expect(changed.mock.lastCall?.[0].marks).toEqual(initial.marks);
});

it("accepts consecutive mosaic strokes while the first raster is still rendering",async()=>{
  vi.stubGlobal("URL",{createObjectURL:()=>"blob:effect",revokeObjectURL:vi.fn()});
  vi.stubGlobal("Image",class {src="";decode=async()=>{};});
  vi.stubGlobal("fetch",vi.fn(async()=>({ok:true,blob:async()=>new Blob(["png"],{type:"image/png"})})));
  const result={id:"b".repeat(32),sourceId:"a".repeat(32),width:100,height:80};
  let finish!:(value:typeof result)=>void;
  const preview=vi.fn().mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve;})).mockResolvedValue(result);
  const changed=vi.fn();
  render(<CaptureEditor image={{id:result.sourceId,width:100,height:80}} onPreview={preview} onDocumentChange={changed} onSave={async()=>false} onClose={()=>{}}/>);
  fireEvent.load(await screen.findByAltText("最近截图预览"));
  await userEvent.setup().click(screen.getByRole("button",{name:"马赛克"}));
  const canvas=screen.getByRole("group",{name:"标注画布"});
  vi.spyOn(canvas,"getBoundingClientRect").mockReturnValue({left:0,top:0,width:100,height:80,right:100,bottom:80,x:0,y:0,toJSON:()=>({})});
  canvas.setPointerCapture=vi.fn();canvas.releasePointerCapture=vi.fn();
  const stroke=(x:number)=>{
    fireEvent.pointerDown(canvas,{button:0,pointerId:1,clientX:x,clientY:5});
    fireEvent.pointerMove(canvas,{pointerId:1,clientX:x+15,clientY:25});
    fireEvent.pointerUp(canvas,{button:0,pointerId:1,clientX:x+15,clientY:25});
  };
  stroke(5);await waitFor(()=>expect(preview).toHaveBeenCalledOnce());
  expect((screen.getByRole("button",{name:"保存图片"}) as HTMLButtonElement).disabled).toBe(false);
  stroke(40);
  expect(changed.mock.lastCall?.[0].marks.map((mark:{kind:string})=>mark.kind)).toEqual(["mosaic","mosaic"]);
  expect(screen.queryByRole("button",{name:"继续编辑"})).toBeNull();
  await act(async()=>finish(result));
  await waitFor(()=>expect(screen.queryByText("正在处理")).toBeNull());
  expect(preview).toHaveBeenCalledTimes(2);
  expect(preview.mock.lastCall?.[0].marks).toHaveLength(2);
});

it("keeps effects on the editable canvas and preserves the full cropped document",async()=>{
  vi.stubGlobal("URL",{createObjectURL:()=>"blob:effect",revokeObjectURL:vi.fn()});
  vi.stubGlobal("Image",class {src="";decode=async()=>{};});
  vi.stubGlobal("fetch",vi.fn(async()=>({ok:true,blob:async()=>new Blob(["png"],{type:"image/png"})})));
  const initial:AnnotationDocument={version:2,width:100,height:80,crop:{x:10,y:10,width:50,height:40},marks:[
    {kind:"mosaic",x:1,y:1,width:20,height:20,effectSize:8,color:"#ff0000",strokeWidth:1},
    {kind:"rectangle",x:10,y:10,width:50,height:50,color:"#ff0000",strokeWidth:1},
  ]};
  const preview=vi.fn(async(_document:AnnotationDocument)=>({id:"b".repeat(32),sourceId:"a".repeat(32),width:100,height:80}));
  const changed=vi.fn(),save=vi.fn(async()=>false);
  const view=render(<CaptureEditor image={{id:"a".repeat(32),width:100,height:80}} initialDocument={initial} onPreview={preview} onDocumentChange={changed} onSave={save} onClose={()=>{}}/>);
  fireEvent.load(await screen.findByAltText("最近截图预览"));
  await waitFor(()=>expect(view.baseElement.querySelector('img[src="blob:effect"]')).not.toBeNull());
  expect(screen.getByRole("group",{name:"标注画布"})).toBeTruthy();
  expect(screen.queryByAltText("编辑结果预览")).toBeNull();
  expect(preview.mock.calls[0][0]).toEqual({...initial,crop:undefined,marks:initial.marks});
  expect(changed).toHaveBeenLastCalledWith(initial);
  await userEvent.setup().click(screen.getByRole("button",{name:"保存图片"}));
  expect(save).toHaveBeenCalledWith(initial);
  await userEvent.setup().click(screen.getByRole("button",{name:"工具设置"}));
  await userEvent.setup().click(await screen.findByRole("button",{name:"取消裁剪"}));
  await waitFor(()=>expect(changed.mock.lastCall?.[0].crop).toBeUndefined());
  expect(preview).toHaveBeenCalledOnce();
  await userEvent.setup().click(screen.getByRole("button",{name:"撤销"}));
  expect(changed).toHaveBeenLastCalledWith(initial);
  expect(preview).toHaveBeenCalledOnce();
});
