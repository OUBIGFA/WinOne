// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from "vitest";
import {act,cleanup,fireEvent,render,screen,waitFor} from "@testing-library/react";
import {useRef} from "react";
import {PageScrollControls} from "./page-scroll-controls";

let resize:()=>void;
beforeEach(()=>{
 vi.stubGlobal("ResizeObserver",class {constructor(callback:()=>void){resize=callback;}observe(){}disconnect(){}});
 vi.stubGlobal("requestAnimationFrame",(callback:FrameRequestCallback)=>setTimeout(()=>callback(0),0));
 vi.stubGlobal("cancelAnimationFrame",clearTimeout);
 vi.stubGlobal("matchMedia",()=>({matches:true}));
});
afterEach(()=>{cleanup();vi.unstubAllGlobals();});

function Fixture({nested=false}:{nested?:boolean}){
 const root=useRef<HTMLDivElement>(null);
 return <><div ref={root} data-testid="viewport" style={{overflowY:nested?"hidden":"auto"}}><div data-slot="page-content">{nested?<><div data-page-scroll-target="" data-testid="first" style={{overflowY:"auto"}}><button>First field</button></div><div data-page-scroll-target="" data-testid="second" style={{overflowY:"auto"}}><button>Second field</button></div></>:<p>Settings content</p>}</div></div><PageScrollControls viewportRef={root}/></>;
}
function measure(node:HTMLElement,height:number,contentHeight:number){
 Object.defineProperties(node,{clientHeight:{configurable:true,value:height},clientWidth:{configurable:true,value:300},scrollHeight:{configurable:true,value:contentHeight}});
 const scrollTo=vi.fn(({top}:{top:number})=>{node.scrollTop=top;fireEvent.scroll(node);});Object.defineProperty(node,"scrollTo",{configurable:true,value:scrollTo});
 return scrollTo;
}

it("hides both controls without overflow and updates when content or viewport changes",async()=>{
 render(<Fixture/>);const root=screen.getByTestId("viewport");measure(root,400,400);act(()=>resize());
 expect(screen.queryByRole("button",{name:"返回顶部"})).toBeNull();
 measure(root,400,900);act(()=>resize());await screen.findByRole("button",{name:"返回底部"});
 measure(root,1000,900);act(()=>resize());await waitFor(()=>expect(screen.queryByRole("button",{name:"返回顶部"})).toBeNull());
});

it("moves the active page to both ends and disables the reached boundary",async()=>{
 render(<Fixture/>);const root=screen.getByTestId("viewport"),scroll=measure(root,400,900);act(()=>resize());
 const top=await screen.findByRole<HTMLButtonElement>("button",{name:"返回顶部"});const bottom=screen.getByRole<HTMLButtonElement>("button",{name:"返回底部"});
 expect(top.disabled).toBe(true);expect(bottom.disabled).toBe(false);
 fireEvent.click(bottom);expect(scroll).toHaveBeenLastCalledWith({top:500,behavior:"auto"});
 await waitFor(()=>expect(bottom.disabled).toBe(true));expect(top.disabled).toBe(false);
 fireEvent.click(top);expect(scroll).toHaveBeenLastCalledWith({top:0,behavior:"auto"});
});

it("uses the recently focused or scrolled settings pane when the page has nested scroll areas",async()=>{
 render(<Fixture nested/>);measure(screen.getByTestId("viewport"),400,400);
 const first=screen.getByTestId("first"),second=screen.getByTestId("second");const scrollFirst=measure(first,300,900),scrollSecond=measure(second,300,1200);act(()=>resize());
 await screen.findByRole("button",{name:"返回底部"});fireEvent.focusIn(screen.getByRole("button",{name:"Second field"}));
 fireEvent.click(screen.getByRole("button",{name:"返回底部"}));expect(scrollSecond).toHaveBeenLastCalledWith({top:900,behavior:"auto"});expect(scrollFirst).not.toHaveBeenCalled();
 first.scrollTop=50;fireEvent.scroll(first);fireEvent.click(screen.getByRole("button",{name:"返回顶部"}));expect(scrollFirst).toHaveBeenLastCalledWith({top:0,behavior:"auto"});
});

it("ignores an inactive zero-height settings pane",async()=>{
 render(<Fixture nested/>);measure(screen.getByTestId("viewport"),400,400);measure(screen.getByTestId("first"),0,900);const scroll=measure(screen.getByTestId("second"),300,1200);act(()=>resize());
 fireEvent.click(await screen.findByRole("button",{name:"返回底部"}));expect(scroll).toHaveBeenCalledWith({top:900,behavior:"auto"});
});
