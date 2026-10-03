// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,render,screen} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {AnnotationProperties} from "./capture-annotation-properties";
import {createHistory,commitMark,replaceMark,type AnnotationMark} from "./capture-editor-model";
import {setApplicationLanguage} from "../i18n";

afterEach(async()=>{cleanup();await setApplicationLanguage("zh-CN");});
const mark:AnnotationMark={kind:"text",x:10,y:20,width:100,height:50,color:"#ff0000",strokeWidth:3,text:"Old",fontSize:24};

it("does not move a mark to zero when its coordinate is cleared",async()=>{
 let history=commitMark(createHistory(400,300),mark);const close=vi.fn();
 render(<AnnotationProperties mark={mark} onApply={next=>{history=replaceMark(history,0,next);}} onClose={close}/>);
 const user=userEvent.setup();await user.click(screen.getByRole("tab",{name:"位置与尺寸"}));
 const x=screen.getByRole("spinbutton",{name:"水平位置"}) as HTMLInputElement;
 await user.clear(x);expect(x.value).toBe("");expect(x.getAttribute("aria-invalid")).toBe("true");
 await user.click(screen.getByRole("button",{name:"应用"}));
 expect(close).not.toHaveBeenCalled();expect(history.document.marks[0].x).toBe(10);
 await user.click(screen.getByRole("tab",{name:"样式"}));
 expect((screen.getByRole("button",{name:"应用"}) as HTMLButtonElement).disabled).toBe(true);
 await user.click(screen.getByRole("tab",{name:"位置与尺寸"}));
 const restored=screen.getByRole("spinbutton",{name:"水平位置"});await user.type(restored,"12.5");
 expect(restored.getAttribute("step")).toBe("any");await user.click(screen.getByRole("button",{name:"应用"}));
 expect(history.document.marks[0].x).toBe(12.5);expect(close).toHaveBeenCalledOnce();expect(history.past).toHaveLength(2);
});

it("rejects fractional effect strength without rejecting fractional geometry",async()=>{
 const apply=vi.fn(),close=vi.fn();
 render(<AnnotationProperties mark={{...mark,kind:"mosaic",effectSize:12}} onApply={apply} onClose={close}/>);
 const user=userEvent.setup(),strength=screen.getByRole("spinbutton",{name:"强度"});
 await user.clear(strength);await user.type(strength,"3.5");
 expect(strength.getAttribute("aria-invalid")).toBe("true");await user.click(screen.getByRole("button",{name:"应用"}));
 expect(apply).not.toHaveBeenCalled();
 await user.clear(strength);await user.type(strength,"8");await user.keyboard("{Escape}");
 expect(close).toHaveBeenCalledOnce();expect(apply).not.toHaveBeenCalled();
});
it("applies style changes through the annotation model without changing its position",async()=>{
  await setApplicationLanguage("en-US");
  let history=commitMark(createHistory(400,300),mark);
  const close=vi.fn();
  render(<AnnotationProperties mark={mark} onApply={next=>{history=replaceMark(history,0,next);}} onClose={close}/>);
  expect(screen.queryByRole("textbox")).toBeNull();
  const user=userEvent.setup();
  await user.clear(screen.getByRole("spinbutton",{name:"Font size"}));
  await user.type(screen.getByRole("spinbutton",{name:"Font size"}),"32");
  await user.click(screen.getByRole("button",{name:"Apply"}));
  expect(history.document.marks[0]).toEqual({...mark,fontSize:32});
  expect(close).toHaveBeenCalledOnce();
  expect(mark.text).toBe("Old");
});
it("keeps invalid edits open instead of replacing the saved mark",async()=>{
  const history=commitMark(createHistory(400,300),mark);
  const close=vi.fn();
  render(<AnnotationProperties mark={mark} onApply={next=>{replaceMark(history,0,next);}} onClose={close}/>);
  expect(screen.queryByRole("textbox")).toBeNull();
  const user=userEvent.setup();
  await user.clear(screen.getByRole("spinbutton",{name:"字号"}));
  await user.click(screen.getByRole("button",{name:"应用"}));
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(close).not.toHaveBeenCalled();
  expect(history.document.marks[0].text).toBe("Old");
});

it("keeps edits across tabs and cancels inline with Escape",async()=>{
  const apply=vi.fn(),close=vi.fn();
  render(<AnnotationProperties mark={mark} onApply={apply} onClose={close}/>);
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(screen.queryByRole("textbox")).toBeNull();
  const user=userEvent.setup();
  const text=screen.getByRole("spinbutton",{name:"字号"});
  await user.clear(text);await user.type(text,"32");
  await user.click(screen.getByRole("tab",{name:"位置与尺寸"}));
  expect(screen.getByRole("spinbutton",{name:"水平位置"})).toBeTruthy();
  await user.click(screen.getByRole("tab",{name:"样式"}));
  expect((screen.getByRole("spinbutton",{name:"字号"}) as HTMLInputElement).value).toBe("32");
  await user.keyboard("{Escape}");
  expect(close).toHaveBeenCalledOnce();expect(apply).not.toHaveBeenCalled();
});
