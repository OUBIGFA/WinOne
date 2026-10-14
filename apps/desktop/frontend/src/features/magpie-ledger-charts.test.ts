// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {magpieLedgerCharts} from "./magpie-ledger-charts";
import {magpieQuotaCurve} from "./magpie-quota-curve";
afterEach(()=>{vi.restoreAllMocks();document.body.replaceChildren();});
it("keeps every account selectable after expanding an overview ranking",()=>{
 const base={calls:1,errors:0,input:100,output:20,cache_read:0,cache_write:0,cost:0.02,unpriced:0};
 const accounts=Array.from({length:9},(_,i)=>({...base,id:"account-"+i}));
 const rank=document.createElement("div");document.body.append(rank);
 const renderer=magpieLedgerCharts((key:string)=>key,"en",{shown:9});
 const pick=vi.fn();renderer.drawLedRank(rank,{by:{account:accounts}},"account","tokens","",pick,true);
 expect(rank.querySelectorAll("button:not(.plain)")).toHaveLength(9);
 rank.querySelectorAll<HTMLButtonElement>("button")[8].click();
 expect(pick).toHaveBeenCalledWith(expect.objectContaining({id:"account-8"}));
});
it("retains Magpie's Top 7 plus Other and keyboard-selectable daily columns",()=>{
 vi.spyOn(HTMLElement.prototype,"clientWidth","get").mockReturnValue(640);
 vi.spyOn(HTMLElement.prototype,"clientHeight","get").mockReturnValue(300);
 const base={calls:1,errors:0,input:100,output:20,cache_read:30,cache_write:10,cost:0.02,unpriced:0,timed:1,ttft_ms:500,decode_ms:1000,decode_out:20};
 const models=Array.from({length:9},(_,i)=>({...base,id:"model-"+i}));
 const chart=document.createElement("div"),rank=document.createElement("div");document.body.append(chart,rank);
 const renderer=magpieLedgerCharts((key:string,vars:Record<string,unknown>={})=>key.replace(/\{(\w+)\}/g,(_,key)=>String(vars[key])),"en");
 const page={by:{model:models},bucket:"day",series:[{...base,time:"2026-10-10T00:00:00Z",by:{model:{"model-0":{...base,tokens:160}}}}]};
 const pick=vi.fn();renderer.drawLedColumns(chart,page,"model","tokens",false,pick,undefined);renderer.drawLedRank(rank,page,"model","tokens","",vi.fn(),false);
 expect(rank.querySelectorAll("button")).toHaveLength(8);expect(rank.textContent).toContain("Other (2)");
 const day=chart.querySelector('[role="button"]')!;day.dispatchEvent(new KeyboardEvent("keydown",{key:"Enter",bubbles:true}));expect(pick).toHaveBeenCalledWith("2026-10-10");
 expect(chart.querySelector("svg")?.getAttribute("aria-label")).toBe("Usage trend");
});

it("uses recorded quota readings and preserves keyboard zoom and the off setting",()=>{
 const now=Date.now(),reset=new Date(now+3600000).toISOString(),start=new Date(now-3600000).toISOString();
 const quota={provider:"fixture",user:"a@example.invalid",windows:[{name:"Window",used:30,resetsAt:reset}]};
 const histories=[{provider:"fixture",user:quota.user,lines:[{name:"Window",points:[{at:start,left:90,start,resetsAt:reset},{at:new Date(now).toISOString(),left:70,start,resetsAt:reset}]}]}];
 const draw=magpieQuotaCurve("en",(key:string)=>key,histories,"cycle"),box=draw(quota)!;document.body.append(box);
 expect(box.querySelectorAll(".qc-line")).toHaveLength(1);expect(box.textContent).toContain("70%");
 const plot=box.querySelector("svg")!;plot.dispatchEvent(new KeyboardEvent("keydown",{key:"Enter",bubbles:true}));expect(plot.getAttribute("aria-expanded")).toBe("true");
 expect(magpieQuotaCurve("en",(key:string)=>key,histories,"off")(quota)).toBeNull();
});
