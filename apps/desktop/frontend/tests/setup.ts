import { afterAll, beforeAll, vi } from "vitest";
let addedAnimations=false,addedScrollIntoView=false;

// JSDOM 26 lacks PointerEvent. Base UI synthesizes keyboard and controlled
// activation clicks with modifier flags; MouseEvent supplies those test semantics.
// Production WebView2 uses its own PointerEvent implementation.
beforeAll(() => {
  if (typeof window !== "undefined" && !window.PointerEvent) vi.stubGlobal("PointerEvent", MouseEvent);
  // JSDOM has no layout engine; resizing itself is verified in the browser.
  if (typeof window !== "undefined" && !window.ResizeObserver) vi.stubGlobal("ResizeObserver", class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  // Base UI waits for Web Animations before measuring a scroll thumb. JSDOM
  // has no animations; browser layout tests exercise the actual API and motion.
  if(typeof Element!=="undefined"&&!Element.prototype.getAnimations){Object.defineProperty(Element.prototype,"getAnimations",{configurable:true,value:()=>[]});addedAnimations=true;}
  // Model-menu centering is measured in Playwright. JSDOM cannot scroll, but
  // components must still be able to invoke the standard browser method.
  if(typeof Element!=="undefined"&&!Element.prototype.scrollIntoView){Object.defineProperty(Element.prototype,"scrollIntoView",{configurable:true,writable:true,value:()=>{}});addedScrollIntoView=true;}
});
afterAll(() => {vi.unstubAllGlobals();if(addedAnimations)Reflect.deleteProperty(Element.prototype,"getAnimations");if(addedScrollIntoView)Reflect.deleteProperty(Element.prototype,"scrollIntoView");});
// jsdom has no media-query API. Real browser checks cover live theme changes;
// unit tests need the standard subscription contract used by DesktopProvider.
if(typeof window!=='undefined'&&!window.matchMedia){
 window.matchMedia=(query:string)=>({matches:false,media:query,onchange:null,addListener:()=>{},removeListener:()=>{},addEventListener:()=>{},removeEventListener:()=>{},dispatchEvent:()=>true});
}
