import {useState} from "react";
// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import CaptureEditor from "./capture-editor";
import { setApplicationLanguage } from "../i18n";

afterEach(async()=>{cleanup();await setApplicationLanguage("zh-CN");});

it("keeps the same canvas while selecting and then exposes the toolbar",async()=>{
 const commit=vi.fn().mockResolvedValue(undefined),confirm=vi.fn().mockResolvedValue(true),close=vi.fn();
 function Preview(){const [selecting,setSelecting]=useState(true);return <CaptureEditor image={{id:"a".repeat(32),width:400,height:300}} onSave={async()=>false} onConfirm={confirm} onClose={close} selection={selecting?{mode:"region",candidates:async()=>({region:null,x:0,y:0,windows:[]}),commit:async crop=>{await commit(crop);setSelecting(false);}}:undefined}/>;}
 render(<Preview/>);fireEvent.load(await screen.findByAltText("最近截图预览"));
 const canvas=screen.getByRole("group",{name:"标注画布"}) as HTMLElement;
 canvas.getBoundingClientRect=()=>new DOMRect(0,0,400,300);canvas.setPointerCapture=vi.fn();canvas.releasePointerCapture=vi.fn();
 expect(screen.queryByRole("button",{name:"矩形"})).toBeNull();
 fireEvent(canvas,new MouseEvent("pointerdown",{bubbles:true,button:0,clientX:20,clientY:30}));
 fireEvent(canvas,new MouseEvent("pointerup",{bubbles:true,button:0,clientX:220,clientY:180}));
 await screen.findByRole("button",{name:"矩形"});
 expect(commit).toHaveBeenCalledWith({x:20,y:30,width:200,height:150});
 expect(screen.getByRole("group",{name:"标注画布"})).toBe(canvas);
 fireEvent.doubleClick(canvas);await waitFor(()=>expect(confirm).toHaveBeenCalledOnce());
 expect(confirm.mock.calls[0][0].crop).toEqual({x:20,y:30,width:200,height:150});expect(close).toHaveBeenCalledOnce();
});

it("right-click cancels the screenshot without a discard dialog",async()=>{
 const close=vi.fn();render(<CaptureEditor image={{id:"a".repeat(32),width:400,height:300}} initialDocument={{version:2,width:400,height:300,crop:{x:10,y:10,width:100,height:90},marks:[]}} onSave={async()=>false} onConfirm={async()=>true} onClose={close}/>);
 fireEvent.contextMenu(await screen.findByRole("group",{name:"标注画布"}));
 await waitFor(()=>expect(close).toHaveBeenCalledOnce());expect(screen.queryByText("关闭并放弃当前草稿？")).toBeNull();
});

it("moves the selected crop directly and records one undo step",async()=>{
 const changed=vi.fn();
 render(<CaptureEditor image={{id:"a".repeat(32),width:400,height:300}} initialDocument={{version:2,width:400,height:300,crop:{x:20,y:30,width:100,height:90},marks:[]}} onDocumentChange={changed} onSave={async()=>false} onConfirm={async()=>true} onClose={()=>{}}/>);
 fireEvent.load(await screen.findByAltText("最近截图预览"));await userEvent.setup().click(screen.getByRole("button",{name:"选择"}));
 const canvas=screen.getByRole("group",{name:"标注画布"}) as HTMLElement;
 canvas.getBoundingClientRect=()=>new DOMRect(0,0,400,300);canvas.setPointerCapture=vi.fn();canvas.releasePointerCapture=vi.fn();
 const pointer=(type:string,x:number,y:number)=>fireEvent(canvas,new MouseEvent(type,{bubbles:true,button:0,clientX:x,clientY:y}));
 pointer("pointerdown",50,60);pointer("pointermove",70,90);pointer("pointermove",90,110);pointer("pointerup",90,110);
 await waitFor(()=>expect(changed.mock.lastCall?.[0].crop).toEqual({x:60,y:80,width:100,height:90}));
 await userEvent.setup().click(screen.getByRole("button",{name:"撤销"}));
 expect(changed.mock.lastCall?.[0].crop).toEqual({x:20,y:30,width:100,height:90});
});

it("types directly on the image and commits multiline text without a preview step",async()=>{
 const changed=vi.fn();
 render(<CaptureEditor image={{id:"a".repeat(32),width:400,height:300}} onSave={async()=>false} onClose={()=>{}} onDocumentChange={changed}/>);
 fireEvent.load(await screen.findByAltText("最近截图预览"));
 const user=userEvent.setup();await user.click(screen.getByRole("button",{name:"文字"}));
 const canvas=screen.getByRole("group",{name:"标注画布"});
 canvas.getBoundingClientRect=()=>new DOMRect(0,0,400,300);
 fireEvent(canvas,new MouseEvent("pointerdown",{bubbles:true,button:0,clientX:40,clientY:50}));
 const input=await screen.findByRole("textbox",{name:"输入文字"});
 expect(canvas.contains(input)).toBe(true);
 await user.type(input,"Hello{Enter}World");
 await user.keyboard("{Control>}{Enter}{/Control}");
 expect(changed.mock.lastCall?.[0].marks[0]).toMatchObject({kind:"text",x:40,y:50,text:"Hello\nWorld"});
 expect(screen.queryByRole("textbox",{name:"输入文字"})).toBeNull();
 expect(screen.queryByRole("button",{name:"预览结果"})).toBeNull();
});

it("retains inline text when closing is cancelled and lets it be edited on the image",async()=>{
 const changed=vi.fn(),close=vi.fn();
 render(<CaptureEditor image={{id:"a".repeat(32),width:400,height:300}} onSave={async()=>false} onClose={close} onDocumentChange={changed}/>);
 fireEvent.load(await screen.findByAltText("最近截图预览"));
 const user=userEvent.setup();await user.click(screen.getByRole("button",{name:"文字"}));
 const canvas=screen.getByRole("group",{name:"标注画布"});canvas.getBoundingClientRect=()=>new DOMRect(0,0,400,300);
 fireEvent(canvas,new MouseEvent("pointerdown",{bubbles:true,button:0,clientX:40,clientY:50}));
 await user.type(await screen.findByRole("textbox",{name:"输入文字"}),"保留文字");
 await user.click(screen.getByRole("button",{name:"关闭"}));
 expect(close).not.toHaveBeenCalled();
 await user.click(await screen.findByRole("button",{name:"继续编辑"}));
 expect(changed.mock.lastCall?.[0].marks[0].text).toBe("保留文字");
 fireEvent(canvas,new MouseEvent("pointerdown",{bubbles:true,button:0,clientX:45,clientY:55}));
 expect((await screen.findByRole("textbox",{name:"输入文字"}) as HTMLTextAreaElement).value).toBe("保留文字");
 await user.keyboard("{Escape}");
 expect(changed.mock.lastCall?.[0].marks[0].text).toBe("保留文字");
});

it("drags the toolbar directly without changing the screenshot",async()=>{
 const changed=vi.fn();
 const surface={desktopPreviewId:"b".repeat(32),placement:{x:0,y:0,desktopX:0,desktopY:0,desktopWidth:window.innerWidth,desktopHeight:window.innerHeight},monitor:{left:0,top:0,right:window.innerWidth,bottom:window.innerHeight}};
 const view=render(<CaptureEditor image={{id:"a".repeat(32),width:window.innerWidth,height:window.innerHeight}} surface={surface} onDocumentChange={changed} onSave={async()=>false} onClose={()=>{}}/>);
 const toolbar=document.querySelector(".capture-surface-toolbar") as HTMLElement;
 const canvas=document.querySelector(".capture-editor-canvas")!;
 const before=canvas.getAttribute("style"),calls=changed.mock.calls.length;
 toolbar.setPointerCapture=vi.fn();toolbar.releasePointerCapture=vi.fn();
 const pointer=(type:string,x:number,y:number,target:HTMLElement=toolbar)=>{const event=new MouseEvent(type,{bubbles:true,button:0,clientX:x,clientY:y});Object.defineProperty(event,"pointerId",{value:1});fireEvent(target,event);};
 const grip=screen.getByRole("button",{name:"拖动工具栏"});
 pointer("pointerdown",100,20,grip);pointer("pointermove",100,180);
 expect(toolbar.style.top).toBe("168px");
 pointer("pointerup",100,180);
 expect(toolbar.style.top).toBe("168px");
 fireEvent.keyDown(toolbar,{key:"ArrowDown",shiftKey:true});expect(toolbar.style.top).toBe("178px");
 pointer("pointerdown",100,180);pointer("pointermove",100,9999);
 expect(Number.parseFloat(toolbar.style.top)).toBeLessThan(window.innerHeight);
 fireEvent.keyDown(toolbar,{key:"Escape"});expect(toolbar.style.top).toBe("178px");
 expect(canvas.getAttribute("style")).toBe(before);expect(changed).toHaveBeenCalledTimes(calls);
 view.unmount();
});

it("returns focus to the restored editor when closing is cancelled",async()=>{
  const keep=vi.fn();
  render(<CaptureEditor image={{id:"f".repeat(32),width:100,height:80}} initialDocument={{version:1,width:100,height:80,marks:[]}} onSave={async()=>false} onClose={()=>{}} onKeepEditing={keep}/>);
  const user=userEvent.setup();
  await user.click(await screen.findByRole("button",{name:"关闭"}));
  await user.keyboard("{Escape}");
  await waitFor(()=>expect(screen.queryByText("关闭并放弃当前草稿？")).toBeNull());
  expect(keep).toHaveBeenCalledOnce();
  expect(document.activeElement?.closest('[role="dialog"]')).not.toBeNull();
});

it("offers effect strength instead of color or line width for blur",async()=>{
  await setApplicationLanguage("en-US");
  render(<CaptureEditor image={{id:"f".repeat(32),width:100,height:80}} onSave={async()=>false} onClose={()=>{}} onPreview={async()=>({id:"a".repeat(32),sourceId:"f".repeat(32),width:100,height:80})}/>);
  await userEvent.setup().click(await screen.findByRole("button",{name:"Blur"}));
  await userEvent.setup().click(screen.getByRole("button",{name:"Tool settings"}));
  expect(screen.getByRole("combobox",{name:"Strength"})).toBeTruthy();
  expect(screen.queryByLabelText("Color")).toBeNull();
  expect(screen.queryByRole("combobox",{name:"Line width"})).toBeNull();
});

it("hides drawing-only options while choosing a crop",async()=>{
  await setApplicationLanguage("en-US");
  render(<CaptureEditor image={{id:"e".repeat(32),width:100,height:80}} onSave={async()=>false} onClose={()=>{}}/>);
  await userEvent.setup().click(await screen.findByRole("button",{name:"Crop"}));
  expect(screen.queryByLabelText("Color")).toBeNull();
  expect(screen.queryByRole("combobox",{name:"Line width"})).toBeNull();
  expect(screen.getByText("Drag to select the area to keep.")).toBeTruthy();
});

it("does not allow saving before the source preview loads",async()=>{
  render(<CaptureEditor image={{id:"a".repeat(32),width:3200,height:1600}} onSave={async()=>false} onClose={()=>{}}/>);
  expect((await screen.findByRole("button",{name:"保存图片"}) as HTMLButtonElement).disabled).toBe(true);
  fireEvent.error(screen.getByAltText("最近截图预览"));
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect((screen.getByRole("button",{name:"保存图片"}) as HTMLButtonElement).disabled).toBe(true);
});

it("offers the same drawing controls in English and closes a clean draft",async()=>{
  await setApplicationLanguage("en-US");
  const close=vi.fn();
  render(<CaptureEditor image={{id:"b".repeat(32),width:100,height:80}} onSave={async()=>false} onClose={close}/>);
  expect(await screen.findByRole("button",{name:"Rectangle"})).toBeTruthy();
  expect(await screen.findByRole("button",{name:"Solid cover"})).toBeTruthy();
  expect(screen.getByRole("button",{name:"Customize toolbar"})).toBeTruthy();
  await userEvent.setup().click(screen.getByRole("button",{name:"Close"}));
  expect(close).toHaveBeenCalledOnce();
});
