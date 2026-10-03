// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {act,cleanup,renderHook,waitFor} from "@testing-library/react";
import {useRef} from "react";
import {rasterDocument,useEffectPreview} from "./capture-effect-preview";
import type {AnnotationDocument,EditPreview} from "./capture-editor-model";

const document:AnnotationDocument={version:2,width:100,height:80,crop:{x:10,y:10,width:50,height:40},marks:[
  {kind:"cover",x:0,y:0,width:20,height:20,color:"#ff0000",strokeWidth:1},
  {kind:"blur",x:5,y:5,width:30,height:30,color:"#ff0000",strokeWidth:1,effectSize:8},
  {kind:"rectangle",x:10,y:10,width:40,height:40,color:"#ff0000",strokeWidth:1},
]};
const preview:EditPreview={id:"b".repeat(32),sourceId:"a".repeat(32),width:100,height:80};
afterEach(()=>{cleanup();vi.unstubAllGlobals();vi.restoreAllMocks();});

function mockImages() {
  const create=vi.fn(()=>"blob:effect"),revoke=vi.fn();
  vi.stubGlobal("URL",{createObjectURL:create,revokeObjectURL:revoke});
  vi.stubGlobal("fetch",vi.fn(async()=>({ok:true,blob:async()=>new Blob(["png"],{type:"image/png"})})));
  vi.stubGlobal("Image",class {src="";decode=async()=>{};});
  return {create,revoke};
}

it("shares the native render and decode queue between text backgrounds and the full image",async()=>{
 mockImages();let decoded!:()=>void;
 const decode=vi.fn().mockImplementationOnce(()=>new Promise<void>(resolve=>{decoded=resolve;})).mockResolvedValue(undefined);
 vi.stubGlobal("Image",class {src="";decode=decode;});
 const render=vi.fn(async()=>preview);
 const hook=renderHook(()=>{
  const queue=useRef(Promise.resolve());
  return [useEffectPreview(document,preview.sourceId,render,queue),useEffectPreview({...document,marks:document.marks.slice(1)},preview.sourceId,render,queue)];
 });
 await waitFor(()=>expect(decoded).toBeDefined());
 expect(render).toHaveBeenCalledOnce();expect(fetch).toHaveBeenCalledOnce();
 await act(async()=>decoded());
 await waitFor(()=>expect(render).toHaveBeenCalledTimes(2));
 await waitFor(()=>expect(hook.result.current.every(result=>!!result.url)).toBe(true));
});

it("accepts a full-resolution raster larger than the thumbnail limit",async()=>{
  mockImages();
  const blob=new Blob(["png"],{type:"image/png"});
  Object.defineProperty(blob,"size",{value:20*1024*1024});
  vi.stubGlobal("fetch",vi.fn(async()=>({ok:true,blob:async()=>blob})));
  const hook=renderHook(()=>useEffectPreview(document,preview.sourceId,async()=>preview));
  await waitFor(()=>expect(hook.result.current.url).toBe("blob:effect"));
  expect(hook.result.current.error).toBeUndefined();
});

it("renders all completed marks in original coordinates without applying the crop twice",()=>{
  const prefix=rasterDocument(document);
  expect(prefix?.marks).toEqual(document.marks);
  expect(prefix?.crop).toBeUndefined();
  expect(prefix?.width).toBe(100);
  expect(rasterDocument({...document,marks:[]})).toBeUndefined();
});

it("keeps the raster when only the crop changes and releases it on unmount",async()=>{
  const {revoke}=mockImages();
  const render=vi.fn(async()=>preview);
  const hook=renderHook(({doc})=>useEffectPreview(doc,preview.sourceId,render),{initialProps:{doc:document}});
  await waitFor(()=>expect(hook.result.current.url).toBe("blob:effect"));
  hook.rerender({doc:{...document,crop:undefined}});
  expect(render).toHaveBeenCalledOnce();
  expect(hook.result.current.count).toBe(3);
  hook.unmount();
  expect(revoke).toHaveBeenCalledWith("blob:effect");
});

it("retains finished effects while appended marks render and releases the old raster after replacement",async()=>{
 const {create,revoke}=mockImages();create.mockReturnValueOnce("blob:first").mockReturnValueOnce("blob:second");
 let finish!:(value:EditPreview)=>void;
 const render=vi.fn().mockResolvedValueOnce(preview).mockImplementationOnce(()=>new Promise<EditPreview>(resolve=>{finish=resolve;}));
 const hook=renderHook(({doc})=>useEffectPreview(doc,preview.sourceId,render),{initialProps:{doc:document}});
 await waitFor(()=>expect(hook.result.current.url).toBe("blob:first"));
 hook.rerender({doc:{...document,marks:[...document.marks,{kind:"mosaic",x:60,y:10,width:20,height:20,effectSize:8,color:"#000000",strokeWidth:1}]}});
 await waitFor(()=>expect(render).toHaveBeenCalledTimes(2));
 expect(hook.result.current.url).toBe("blob:first");expect(hook.result.current.count).toBe(3);
 expect(hook.result.current.pending).toBe(true);expect(revoke).not.toHaveBeenCalled();
 await act(async()=>finish(preview));
 await waitFor(()=>expect(hook.result.current.url).toBe("blob:second"));
 expect(hook.result.current.count).toBe(4);expect(revoke).toHaveBeenCalledExactlyOnceWith("blob:first");
 hook.unmount();expect(revoke).toHaveBeenCalledWith("blob:second");
});

it.each(["undo","edit"])("does not retain obsolete effects after %s",async(operation)=>{
 const {revoke}=mockImages();
 const render=vi.fn().mockResolvedValueOnce(preview).mockImplementation(()=>new Promise<EditPreview>(()=>{}));
 const hook=renderHook(({doc})=>useEffectPreview(doc,preview.sourceId,render),{initialProps:{doc:document}});
 await waitFor(()=>expect(hook.result.current.url).toBe("blob:effect"));
 hook.rerender({doc:{...document,marks:operation==="undo"?document.marks.slice(0,1):[{...document.marks[0],x:1},...document.marks.slice(1)]}});
 expect(hook.result.current.url).toBeUndefined();
 await waitFor(()=>expect(revoke).toHaveBeenCalledWith("blob:effect"));
});

it("keeps the valid earlier raster visible when an appended render fails",async()=>{
 const {revoke}=mockImages();
 const render=vi.fn().mockResolvedValueOnce(preview).mockRejectedValueOnce(Error("renderer unavailable"));
 const hook=renderHook(({doc})=>useEffectPreview(doc,preview.sourceId,render),{initialProps:{doc:document}});
 await waitFor(()=>expect(hook.result.current.url).toBe("blob:effect"));
 hook.rerender({doc:{...document,marks:[...document.marks,document.marks[0]]}});
 await waitFor(()=>expect(hook.result.current.error).toContain("renderer unavailable"));
 expect(hook.result.current.url).toBe("blob:effect");expect(hook.result.current.count).toBe(3);
 expect(hook.result.current.pending).toBe(false);expect(revoke).not.toHaveBeenCalled();
});

it("discards stale work and serializes native rendering",async()=>{
  mockImages();
  let resolve!:(value:EditPreview)=>void;
  const render=vi.fn().mockImplementationOnce(()=>new Promise<EditPreview>(done=>{resolve=done;})).mockResolvedValue(preview);
  const hook=renderHook(({doc})=>useEffectPreview(doc,preview.sourceId,render),{initialProps:{doc:document}});
  await waitFor(()=>expect(render).toHaveBeenCalledOnce());
  hook.rerender({doc:{...document,marks:[{...document.marks[0],x:1},...document.marks.slice(1)]}});
  expect(hook.result.current.url).toBeUndefined();
  expect(render).toHaveBeenCalledOnce();
  await act(async()=>resolve(preview));
  await waitFor(()=>expect(hook.result.current.url).toBe("blob:effect"));
  expect(render).toHaveBeenCalledTimes(2);
  expect(fetch).toHaveBeenCalledOnce();
});

it("reports a failed image fetch and permits retry",async()=>{
  mockImages();
  vi.mocked(fetch).mockRejectedValueOnce(Error("image unavailable"));
  const hook=renderHook(()=>useEffectPreview(document,preview.sourceId,async()=>preview));
  await waitFor(()=>expect(hook.result.current.error).toContain("image unavailable"));
  expect(hook.result.current.url).toBeUndefined();
  act(()=>hook.result.current.retry());
  await waitFor(()=>expect(hook.result.current.url).toBe("blob:effect"));
});

it("keeps exports waiting when an effect is undone during native rendering",async()=>{
  mockImages();
  let finish!:(value:EditPreview)=>void;
  const render=vi.fn(()=>new Promise<EditPreview>(resolve=>{finish=resolve;}));
  const hook=renderHook(({doc})=>useEffectPreview(doc,preview.sourceId,render),{initialProps:{doc:document}});
  await waitFor(()=>expect(hook.result.current.rendering).toBe(true));
  hook.rerender({doc:{...document,marks:[]}});
  expect(hook.result.current.pending).toBe(false);
  expect(hook.result.current.rendering).toBe(true);
  await act(async()=>finish(preview));
  expect(hook.result.current.rendering).toBe(false);
  expect(fetch).not.toHaveBeenCalled();
});

it("releases an image when unmounted during decoding",async()=>{
  const {revoke}=mockImages();
  let decoded!:()=>void;
  vi.stubGlobal("Image",class {src="";decode=()=>new Promise<void>(resolve=>{decoded=resolve;});});
  const hook=renderHook(()=>useEffectPreview(document,preview.sourceId,async()=>preview));
  await waitFor(()=>expect(decoded).toBeDefined());
  hook.unmount();
  expect(revoke).toHaveBeenCalledExactlyOnceWith("blob:effect");
  await act(async()=>decoded());
  expect(revoke).toHaveBeenCalledOnce();
});

it("does not fetch a preview belonging to another screenshot",async()=>{
  mockImages();
  const hook=renderHook(()=>useEffectPreview(document,preview.sourceId,async()=>({...preview,sourceId:"c".repeat(32)})));
  await waitFor(()=>expect(hook.result.current.error).toBeTruthy());
  expect(fetch).not.toHaveBeenCalled();
});
