// @vitest-environment jsdom
import {afterEach,expect,it} from "vitest";
import {installWindowResizeFeedback} from "./window-resize";
const flags={enableResize:true,borderThickness:5,resizeEdge:undefined as string|undefined,defaultCursor:""};
let dispose:(()=>void)|undefined;
afterEach(()=>{dispose?.();delete (window as unknown as {wails?:unknown}).wails;});
it("uses the client viewport for all eight edges even when outer dimensions differ",()=>{
 Object.assign(window,{wails:{flags},innerWidth:800,innerHeight:600,outerWidth:1200,outerHeight:900});
 dispose=installWindowResizeFeedback(false);
 for(const [x,y,cursor] of [[1,1,"nw-resize"],[400,1,"n-resize"],[799,1,"ne-resize"],[1,300,"w-resize"],[799,300,"e-resize"],[1,599,"sw-resize"],[400,599,"s-resize"],[799,599,"se-resize"]] as const){
  window.dispatchEvent(new MouseEvent("mousemove",{clientX:x,clientY:y}));
  expect(flags.resizeEdge).toBe(cursor);expect(document.documentElement.style.getPropertyValue("--window-resize-cursor")).toBe(cursor);
 }
 window.dispatchEvent(new MouseEvent("mousemove",{clientX:200,clientY:200}));
 expect(flags.resizeEdge).toBeUndefined();expect(document.documentElement.hasAttribute("data-window-resizing")).toBe(false);
});
it("clears a stale edge when maximized and recomputes it on the actual press",()=>{
 Object.assign(window,{wails:{flags},innerWidth:800,innerHeight:600});
 flags.resizeEdge="se-resize";dispose=installWindowResizeFeedback(true);
 window.dispatchEvent(new MouseEvent("mousedown",{clientX:799,clientY:599}));expect(flags.resizeEdge).toBeUndefined();
 dispose();dispose=installWindowResizeFeedback(false);
 window.dispatchEvent(new MouseEvent("mousedown",{clientX:799,clientY:300}));expect(flags.resizeEdge).toBe("e-resize");
 window.dispatchEvent(new Event("blur"));expect(flags.resizeEdge).toBeUndefined();
});
