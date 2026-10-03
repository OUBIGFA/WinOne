// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from "vitest";
import {act,cleanup,fireEvent,render,screen,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import CaptureEditor from "./capture-editor";
import type {AnnotationDocument,EditPreview} from "./capture-editor-model";

beforeEach(()=>{vi.stubGlobal("PointerEvent",MouseEvent);vi.stubGlobal("ResizeObserver",class {observe(){}disconnect(){}});});
afterEach(()=>{window.getSelection()?.removeAllRanges();cleanup();vi.unstubAllGlobals();vi.restoreAllMocks();});
const image={id:"a".repeat(32),width:400,height:300};

it("keeps Ctrl+C on selected feedback text instead of copying the image",async()=>{
 const copy=vi.fn(async()=>{});
 render(<CaptureEditor image={image} saveError="Selected feedback" onSave={async()=>false} onCopy={copy} onClose={()=>{}}/>);
 fireEvent.load(await screen.findByAltText("最近截图预览"));
 const message=await screen.findByText("Selected feedback");
 const range=document.createRange();range.selectNodeContents(message);
 window.getSelection()!.removeAllRanges();
 window.getSelection()!.addRange(range);
 expect(window.getSelection()!.toString()).toBe("Selected feedback");
 expect(fireEvent.keyDown(message,{key:"c",ctrlKey:true})).toBe(true);
 await act(async()=>{});
 expect(copy).not.toHaveBeenCalled();
});

it("does not let another page's selection block screenshot copying",async()=>{
 const copy=vi.fn(async()=>{});
 render(<CaptureEditor image={image} onSave={async()=>false} onCopy={copy} onClose={()=>{}}/>);
 fireEvent.load(await screen.findByAltText("最近截图预览"));
 const outside=document.createElement("p");outside.textContent="Outside selection";document.body.append(outside);
 try {
  const range=document.createRange();range.selectNodeContents(outside);window.getSelection()!.removeAllRanges();window.getSelection()!.addRange(range);
  expect(fireEvent.keyDown(screen.getByRole("group",{name:"标注画布"}),{key:"c",ctrlKey:true})).toBe(false);
  await waitFor(()=>expect(copy).toHaveBeenCalledOnce());
 } finally {outside.remove();}
});
function rasterAssets(){
 vi.stubGlobal("URL",{createObjectURL:()=>"blob:text",revokeObjectURL:vi.fn()});
 vi.stubGlobal("Image",class {src="";decode=async()=>{};});
 vi.stubGlobal("fetch",vi.fn(async()=>({ok:true,blob:async()=>new Blob(["png"],{type:"image/png"})})));
}
async function typeOnImage(){
 fireEvent.load(await screen.findByAltText("最近截图预览"));
 const user=userEvent.setup();await user.click(screen.getByRole("button",{name:"文字"}));
 const canvas=screen.getByRole("group",{name:"标注画布"});canvas.getBoundingClientRect=()=>new DOMRect(0,0,400,300);
 fireEvent.pointerDown(canvas,{button:0,clientX:40,clientY:50});
 const input=await screen.findByRole("textbox",{name:"输入文字"});
 await user.type(input,"最后输入的文字");return {user,input};
}
it.each([false,true])("saves active text with the document shortcut (save as: %s)",async shift=>{
 const save=vi.fn(async(_document:AnnotationDocument)=>false),saveAs=vi.fn(async(_document:AnnotationDocument)=>false);
 render(<CaptureEditor image={image} onSave={save} onSaveAs={saveAs} onClose={()=>{}}/>);
 const {input}=await typeOnImage();
 expect(fireEvent.keyDown(input,{key:"s",ctrlKey:true,shiftKey:shift})).toBe(false);
 const action=shift?saveAs:save;
 await waitFor(()=>expect(action).toHaveBeenCalledOnce());
 expect(action.mock.calls[0][0].marks[0].text).toBe("最后输入的文字");
 expect(shift?save:saveAs).not.toHaveBeenCalled();
});

it("waits for active IME text to finish before a keyboard save",async()=>{
 const save=vi.fn(async(_document:AnnotationDocument)=>false);
 render(<CaptureEditor image={image} onSave={save} onClose={()=>{}}/>);
 const {input}=await typeOnImage();fireEvent.compositionStart(input);
 fireEvent.keyDown(input,{key:"s",ctrlKey:true,isComposing:true});
 expect(save).not.toHaveBeenCalled();
 fireEvent.change(input,{target:{value:"完成选字"}});fireEvent.compositionEnd(input);
 await waitFor(()=>expect(save).toHaveBeenCalledOnce());
 expect(save.mock.calls[0][0].marks[0].text).toBe("完成选字");
});

it("cancels a keyboard save during composition without discarding the text",async()=>{
 const save=vi.fn(async(_document:AnnotationDocument)=>false),changed=vi.fn();
 render(<CaptureEditor image={image} onSave={save} onDocumentChange={changed} onClose={()=>{}}/>);
 const {input}=await typeOnImage();fireEvent.compositionStart(input);
 fireEvent.keyDown(input,{key:"s",ctrlKey:true,isComposing:true});
 fireEvent.keyDown(input,{key:"Escape",isComposing:true});
 fireEvent.change(input,{target:{value:"保留文字"}});fireEvent.compositionEnd(input);
 await waitFor(()=>expect(changed.mock.lastCall?.[0].marks[0]?.text).toBe("保留文字"));
 expect(save).not.toHaveBeenCalled();
});

it("leaves copy and undo inside the inline text field",async()=>{
 const save=vi.fn(async()=>false),copy=vi.fn(async()=>{});
 render(<CaptureEditor image={image} onSave={save} onCopy={copy} onClose={()=>{}}/>);
 const {input}=await typeOnImage();
 expect(fireEvent.keyDown(input,{key:"c",ctrlKey:true})).toBe(true);
 expect(fireEvent.keyDown(input,{key:"z",ctrlKey:true})).toBe(true);
 expect(copy).not.toHaveBeenCalled();expect(save).not.toHaveBeenCalled();
 expect(screen.getByRole("textbox",{name:"输入文字"})).toBeTruthy();
});
it.each(["保存图片","另存为","复制","截图置顶"])("includes active text on the first click of %s",async name=>{
 const action=vi.fn(async(_document:AnnotationDocument)=>false);
 render(<CaptureEditor image={image} onSave={action} onSaveAs={action} onCopy={async doc=>{await action(doc);}} onPin={async doc=>{await action(doc);}} onClose={()=>{}}/>);
 const {user}=await typeOnImage();await user.click(screen.getByRole("button",{name}));
 await waitFor(()=>expect(action).toHaveBeenCalledOnce());
 expect(action.mock.calls[0][0].marks[0]).toMatchObject({kind:"text",text:"最后输入的文字"});
});
it("waits for the new text raster and performs the requested save only once",async()=>{
 rasterAssets();
 let finish!:(value:EditPreview)=>void;
 const raster=vi.fn(()=>new Promise<EditPreview>(resolve=>{finish=resolve;})),save=vi.fn(async(_document:AnnotationDocument)=>false);
 render(<CaptureEditor image={image} onSave={save} onPreview={raster} onClose={()=>{}}/>);
 const {user}=await typeOnImage();await user.click(screen.getByRole("button",{name:"保存图片"}));
 await waitFor(()=>expect(raster).toHaveBeenCalledOnce());expect(save).not.toHaveBeenCalled();
 await user.click(screen.getByRole("button",{name:"保存图片"}));
 await act(async()=>finish({id:"b".repeat(32),sourceId:image.id,width:400,height:300}));
 await waitFor(()=>expect(save).toHaveBeenCalledOnce());
 expect(save.mock.calls[0][0].marks[0].text).toBe("最后输入的文字");
});
it("does not save an unfinished IME composition",async()=>{
 const save=vi.fn(async(_document:AnnotationDocument)=>false);
 render(<CaptureEditor image={image} onSave={save} onClose={()=>{}}/>);
 const {user,input}=await typeOnImage();fireEvent.compositionStart(input);
 await user.click(screen.getByRole("button",{name:"保存图片"}));expect(save).not.toHaveBeenCalled();
 fireEvent.change(input,{target:{value:"最终选字"}});fireEvent.compositionEnd(input);
 await waitFor(()=>expect(save).toHaveBeenCalledOnce());
 expect(save.mock.calls[0][0].marks[0].text).toBe("最终选字");
});
it("cancels a queued save with Escape while retaining the text",async()=>{
 rasterAssets();let finish!:(value:EditPreview)=>void;
 const changed=vi.fn(),save=vi.fn(async(_document:AnnotationDocument)=>false);
 render(<CaptureEditor image={image} onSave={save} onDocumentChange={changed} onPreview={()=>new Promise(resolve=>{finish=resolve;})} onClose={()=>{}}/>);
 const {user}=await typeOnImage();await user.click(screen.getByRole("button",{name:"保存图片"}));
 await waitFor(()=>expect(finish).toBeDefined());
 fireEvent.keyDown(screen.getByRole("dialog"),{key:"Escape"});
 await act(async()=>finish({id:"b".repeat(32),sourceId:image.id,width:400,height:300}));
 await waitFor(()=>expect(screen.queryByText("正在处理")).toBeNull());
 expect(save).not.toHaveBeenCalled();expect(changed.mock.lastCall?.[0].marks[0].text).toBe("最后输入的文字");
});
it("does not export stale pixels when the queued raster fails",async()=>{
 const save=vi.fn(async(_document:AnnotationDocument)=>false);let reject!:(error:Error)=>void;
 render(<CaptureEditor image={image} onSave={save} onPreview={()=>new Promise((_,fail)=>{reject=fail;})} onClose={()=>{}}/>);
 const {user}=await typeOnImage();await user.click(screen.getByRole("button",{name:"保存图片"}));
 await waitFor(()=>expect(reject).toBeDefined());await act(async()=>reject(Error("renderer unavailable")));
 expect(await screen.findByRole("alert")).toBeTruthy();expect(save).not.toHaveBeenCalled();
 await user.click(screen.getByRole("button",{name:"矩形"}));
 expect(screen.getByRole("button",{name:"矩形"}).getAttribute("aria-pressed")).toBe("true");
});
