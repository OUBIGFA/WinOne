// @vitest-environment jsdom
import {afterEach,expect,it} from "vitest";
import {hasEditorTextSelection} from "./capture-editor-shortcuts";

afterEach(()=>{document.getSelection()?.removeAllRanges();document.body.replaceChildren();});
function select(node:Node){const range=document.createRange();range.selectNodeContents(node);document.getSelection()!.removeAllRanges();document.getSelection()!.addRange(range);}
function fixtures(){
 const root=document.createElement("div"),button=document.createElement("button"),inside=document.createElement("p"),portal=document.createElement("div"),outside=document.createElement("p");
 inside.textContent="editor text";outside.textContent="other page text";portal.textContent="popup feedback";portal.dataset.slot="popover-content";portal.id="capture-popover";button.setAttribute("aria-controls",portal.id);
 root.append(button,inside);document.body.append(root,portal,outside);
 return {root,button,inside,portal,outside};
}
it("recognizes selected editor text and text in the focused portal",()=>{
 const {root,inside,portal}=fixtures();select(inside);expect(hasEditorTextSelection(root,inside)).toBe(true);
 select(portal);expect(hasEditorTextSelection(root,portal)).toBe(true);
});
it("recognizes a controlled popup while its trigger retains keyboard focus",()=>{
 const {root,button,portal}=fixtures();select(portal);expect(hasEditorTextSelection(root,button)).toBe(true);
});
it("ignores selections outside the current editor and collapsed selections",()=>{
 const {root,inside,outside}=fixtures();select(outside);expect(hasEditorTextSelection(root,inside)).toBe(false);
 select(inside);document.getSelection()!.collapseToStart();expect(hasEditorTextSelection(root,inside)).toBe(false);
});
it("ignores retired portal content without discarding whitespace selections",()=>{
 const {root,portal,inside}=fixtures();inside.textContent=" ";select(inside);expect(hasEditorTextSelection(root,inside)).toBe(true);
 select(portal);portal.remove();expect(hasEditorTextSelection(root,inside)).toBe(false);
});
