import {expect,it} from "vitest";
import {editorShortcut} from "./capture-editor-shortcuts";

const key={key:"c",ctrlKey:true,metaKey:false,shiftKey:false,altKey:false};
it("routes canvas copy/save while leaving text editing and unrelated chords alone",()=>{
 expect(editorShortcut(key,false)).toBe("copy");
 expect(editorShortcut(key,true)).toBeUndefined();
 expect(editorShortcut({...key,key:"s"},false)).toBe("save");
 expect(editorShortcut({...key,key:"s",shiftKey:true},false)).toBe("saveAs");
 expect(editorShortcut({...key,ctrlKey:false},false)).toBeUndefined();
 expect(editorShortcut({...key,altKey:true},false)).toBeUndefined();
 expect(editorShortcut({...key,shiftKey:true},false)).toBeUndefined();
});
it("preserves undo/redo and deletion semantics",()=>{
 expect(editorShortcut({...key,key:"z"},false)).toBe("undo");
 expect(editorShortcut({...key,key:"z",shiftKey:true},false)).toBe("redo");
 expect(editorShortcut({...key,key:"y"},false)).toBe("redo");
 expect(editorShortcut({...key,key:"Delete",ctrlKey:false},false)).toBe("delete");
 expect(editorShortcut({...key,key:"Delete",ctrlKey:false},true)).toBeUndefined();
});
