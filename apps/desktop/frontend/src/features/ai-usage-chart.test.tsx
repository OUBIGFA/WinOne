// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {act,cleanup,render} from "@testing-library/react";
import {UsageChart} from "./ai-usage-chart";
import type {UsagePage} from "./ai-usage-types";

const draw=vi.hoisted(()=>vi.fn());
vi.mock("./magpie-ledger-charts",()=>({magpieLedgerCharts:()=>({drawLedColumns:draw,drawLedRank:vi.fn()})}));
afterEach(()=>{cleanup();vi.restoreAllMocks();vi.unstubAllGlobals();draw.mockReset();});

it("coalesces width changes outside ResizeObserver delivery without redrawing height-only changes",()=>{
 let width=400,id=0;const frames=new Map<number,FrameRequestCallback>();
 const observers:{resize:()=>void;target?:Element}[]=[];
 vi.spyOn(HTMLElement.prototype,"clientWidth","get").mockImplementation(()=>width);
 vi.stubGlobal("ResizeObserver",class {
  item:{resize:()=>void;target?:Element};
  constructor(resize:()=>void){this.item={resize};observers.push(this.item);}
  observe(target:Element){this.item.target=target;}disconnect(){}
 });
 vi.stubGlobal("requestAnimationFrame",(callback:FrameRequestCallback)=>{frames.set(++id,callback);return id;});
 vi.stubGlobal("cancelAnimationFrame",(id:number)=>frames.delete(id));
 const flush=()=>act(()=>{const callbacks=[...frames.values()];frames.clear();callbacks.forEach(callback=>callback(0));});
 const view=render(<UsageChart page={{rows:[],sum:{},by:{},series:[]} as unknown as UsagePage}/>);
 const resize=observers.find(item=>item.target?.classList.contains("led-chart"))!.resize;
 const initial=draw.mock.calls.length;
 act(()=>{resize();resize();});expect(draw).toHaveBeenCalledTimes(initial);
 flush();expect(draw).toHaveBeenCalledTimes(initial);
 width=520;act(()=>{resize();resize();});expect(draw).toHaveBeenCalledTimes(initial);
 flush();expect(draw).toHaveBeenCalledTimes(initial+1);
 // Hidden panels must not rebuild the plot at zero width.
 width=0;act(resize);flush();expect(draw).toHaveBeenCalledTimes(initial+1);
 width=640;act(resize);view.unmount();flush();expect(draw).toHaveBeenCalledTimes(initial+1);
});
