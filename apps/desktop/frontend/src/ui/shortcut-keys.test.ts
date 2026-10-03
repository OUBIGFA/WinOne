import {expect,it} from "vitest";
import {recordedShortcut} from "./shortcut-keys";
const key=(key:string,extra={})=>({key,code:"",ctrlKey:false,altKey:false,shiftKey:false,metaKey:false,repeat:false,isComposing:false,altGraph:false,...extra});
it("records normalized combinations and shifted digits",()=>{
 expect(recordedShortcut(key("s",{ctrlKey:true,altKey:true}))).toEqual({kind:"shortcut",value:"Ctrl+Alt+S"});
 expect(recordedShortcut(key("!",{code:"Digit1",ctrlKey:true,shiftKey:true}))).toEqual({kind:"shortcut",value:"Ctrl+Shift+1"});
 expect(recordedShortcut(key("F9",{ctrlKey:true,shiftKey:true}))).toEqual({kind:"shortcut",value:"Ctrl+Shift+F9"});
 expect(recordedShortcut(key("1",{code:"Numpad1",ctrlKey:true}))).toEqual({kind:"shortcut",value:"Ctrl+Num1"});
});
it("does not accept modifiers alone, text input, repeats or F12",()=>{
 expect(recordedShortcut(key("Control",{ctrlKey:true}))).toEqual({kind:"modifier",value:"Ctrl"});
 expect(recordedShortcut(key("a"))).toEqual({kind:"invalid",reason:"modifier"});
 expect(recordedShortcut(key("F12",{ctrlKey:true}))).toEqual({kind:"invalid",reason:"reserved"});
 expect(recordedShortcut(key("a",{repeat:true}))).toEqual({kind:"ignore"});
 expect(recordedShortcut(key("Process",{isComposing:true}))).toEqual({kind:"invalid",reason:"key"});
});
