// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {cleanup,fireEvent,render,screen,waitFor} from "@testing-library/react";
import {CaptureInlineText} from "./capture-inline-text";

afterEach(cleanup);
const mark={kind:"text" as const,x:10,y:10,width:180,height:40,text:"",fontSize:24,color:"#ff0000",strokeWidth:1};
it("keeps IME confirmation keys inside the text and commits once after composition ends on blur",async()=>{
 const commit=vi.fn(),cancel=vi.fn();
 render(<CaptureInlineText mark={mark} scale={1} image={{width:400,height:300}} label="Text" hint="" onCommit={commit} onCancel={cancel}/>);
 const input=screen.getByRole("textbox");
 fireEvent.compositionStart(input);
 fireEvent.change(input,{target:{value:"中文标注"}});
 fireEvent.keyDown(input,{key:"Enter",ctrlKey:true,isComposing:true});
 fireEvent.keyDown(input,{key:"Escape",isComposing:true});
 input.blur();
 expect(commit).not.toHaveBeenCalled();expect(cancel).not.toHaveBeenCalled();
 fireEvent.compositionEnd(input);
 await waitFor(()=>expect(commit).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({text:"中文标注"})));
 fireEvent.blur(input);expect(commit).toHaveBeenCalledOnce();
});
it("cancels without committing the edited text",()=>{
 const commit=vi.fn(),cancel=vi.fn();
 render(<CaptureInlineText mark={{...mark,text:"Original"}} scale={1} image={{width:400,height:300}} label="Text" hint="" onCommit={commit} onCancel={cancel}/>);
 const input=screen.getByRole("textbox");fireEvent.change(input,{target:{value:"Changed"}});
 fireEvent.keyDown(input,{key:"Escape"});fireEvent.blur(input);
 expect(cancel).toHaveBeenCalledOnce();expect(commit).not.toHaveBeenCalled();
});
