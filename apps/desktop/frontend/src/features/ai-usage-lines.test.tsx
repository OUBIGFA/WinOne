// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from "vitest";
import {cleanup,fireEvent,render,screen,within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {UsageChart,createUsageRenderer} from "./ai-usage-chart";
import {UsageTotals} from "./ai-usage";
import type {Totals,UsagePage} from "./ai-usage-types";

const totals:Totals={calls:2,errors:0,input:80,output:20,cache_read:0,cache_write:0,cost:0.02,unpriced:0};
function ledger():UsagePage{
 return {rows:[],sum:totals,total:6,offset:0,limit:15,bucket:"day",by:{model:[{...totals,id:"alpha"},{...totals,id:"beta"}]},series:[0,1,2].map(i=>({...totals,time:`2026-10-${20+i}T00:00:00+08:00`,by:{model:{alpha:{calls:1,tokens:60,cost:0.012},beta:{calls:1,tokens:40,cost:0.008}}}}))};
}
beforeEach(()=>{
 // jsdom has no layout; supply only geometry, leaving the renderer and all
 // DOM interactions real. Browser checks cover actual responsive dimensions.
 vi.spyOn(HTMLElement.prototype,"clientWidth","get").mockReturnValue(600);
 vi.spyOn(HTMLElement.prototype,"clientHeight","get").mockReturnValue(220);
 localStorage.removeItem("winone.usage.chart");localStorage.removeItem("winone.usage.metric");
});
afterEach(()=>{cleanup();vi.restoreAllMocks();localStorage.removeItem("winone.usage.chart");localStorage.removeItem("winone.usage.metric");});

it("switches real columns to stacked lines, keeps day and ranking actions, and remembers the choice",async()=>{
 const user=userEvent.setup(),onDay=vi.fn(),onPick=vi.fn();
 const view=render(<UsageChart page={ledger()} onDay={onDay} onPick={onPick}/>);
 expect(view.container.querySelectorAll("rect.col")).toHaveLength(6);
 await user.click(screen.getByRole("button",{name:"折线图"}));
 expect(view.container.querySelectorAll("rect.col")).toHaveLength(0);
 expect(view.container.querySelectorAll("g.ln .band")).toHaveLength(2);
 expect(view.container.querySelectorAll("g.ln .line")).toHaveLength(2);
 const day=view.container.querySelector('[data-day="2026-10-21"].led-day')!;
 fireEvent.keyDown(day,{key:"Enter"});expect(onDay).toHaveBeenLastCalledWith("2026-10-21");
 fireEvent.keyDown(day,{key:" "});expect(onDay).toHaveBeenCalledTimes(2);
 await user.click(within(view.container.querySelector(".led-rank")!).getByRole("button",{name:/alpha/}));
 expect(onPick).toHaveBeenCalledWith("model","alpha");
 view.unmount();
 const again=render(<UsageChart page={ledger()}/>);
 expect(screen.getByRole("button",{name:"折线图"}).getAttribute("aria-pressed")).toBe("true");
 expect(again.container.querySelectorAll("g.ln")).toHaveLength(2);
 await user.click(screen.getByRole("button",{name:"柱状图"}));
 expect(again.container.querySelectorAll("g.ln")).toHaveLength(0);
 expect(again.container.querySelectorAll("rect.col")).toHaveLength(6);
});

it("breaks speed lines at unmeasured replies and keeps a separate total instead of stacking speeds",()=>{
 const page=ledger();
 page.series=page.series!.map((p,i)=>({...p,decode_out:100,decode_ms:1000,by:{model:{alpha:{calls:1,tokens:60,cost:0,decode_out:60,decode_ms:i===1?0:1000},beta:{calls:1,tokens:40,cost:0,decode_out:40,decode_ms:1000}}}}));
 const box=document.createElement("div");document.body.append(box);
 try{
  createUsageRenderer("zh-CN").drawLedLines(box,page,"model","speed",false,undefined,undefined);
  expect(box.querySelectorAll(".band")).toHaveLength(0);
  expect(box.querySelectorAll(".line.all")).toHaveLength(1);
  const line=box.querySelector('g[data-k="alpha"] .line')!;
  expect(line.getAttribute("d")?.match(/M/g)).toHaveLength(2);
  expect(line.getAttribute("d")).not.toContain("C");
  expect(box.querySelector('g[data-k="alpha"] .dots')).toBeTruthy();
  expect(box.textContent).not.toMatch(/NaN|Infinity/);
 }finally{box.remove();}
});

it("excludes unknown cache from hit rate and labels measured and lower-bound speeds separately",()=>{
 render(<UsageTotals sum={{...totals,input:180,cache_read:80,cache_write:40,cache_unknown:100,cache_unknown_calls:1,decode_out:120,decode_ms:1000,decode_n:1,floor_out:400,floor_ms:2000,floor_n:2}}/>);
 expect(screen.getByText("40.0%")).toBeTruthy();
 expect(screen.getByText("≥200 Token/秒 · 2 次回复")).toBeTruthy();
 expect(screen.getByText("120 Token/秒 · 1 次计时")).toBeTruthy();
 expect(screen.getByText("1 个请求未计入：其服务商不统计缓存")).toBeTruthy();
 cleanup();render(<UsageTotals sum={{...totals,input:100,cache_unknown:100,cache_unknown_calls:2}}/>);
 const hit=screen.getByText("缓存命中率").parentElement!;
 expect(within(hit).getByText("—")).toBeTruthy();
});
