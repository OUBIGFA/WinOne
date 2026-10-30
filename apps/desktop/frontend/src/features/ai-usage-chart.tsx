import {useEffect,useRef,useState} from "react";
import {useTranslation} from "../i18n";
import {CaptureOptionSelect} from "../ui/capture-option-select";
import {ChartColumn,ChartLine} from "lucide-react";
import {Button} from "../ui/button";
import {magpieLedgerCharts} from "./magpie-ledger-charts";
import {magpieLedgerChinese} from "./magpie-ledger-messages";
import type {UsageFilter,UsagePage,UsageShare} from "./ai-usage-types";

const chartChinese:Record<string,string>={"Usage trend":"用量趋势","Other":"其他","Other ({n})":"其他（{n}）","Total":"合计","All":"全部","Nothing in this period":"此范围内还没有请求记录。","No known price for these requests":"这些请求尚无已知价格","No streamed replies timed in this period":"此范围内没有已计时的流式回复","{n} request":"{n} 次请求","{n} requests":"{n} 次请求","{n} tokens":"{n} Token","{n} tok/s":"{n} Token/秒","TTFT {ms}":"首字延迟 {ms}","hit rate {p}":"缓存命中率 {p}","{n} failed":"{n} 次失败","week of {label}":"{label} 所在周"};

export function usageMessage(language:string,key:string,values:Record<string,string|number>={}){
 let text=language.startsWith("zh")?chartChinese[key]??magpieLedgerChinese[key]??key:key;
 for(const [name,value] of Object.entries(values))text=text.replaceAll("{"+name+"}",String(value));
 return text;
}
export function createUsageRenderer(language:string,shown=7){
 return magpieLedgerCharts((key:string,values:Record<string,string|number>={})=>usageMessage(language,key,values),language,{shown});
}

// These are viewing preferences, not AI settings. Match Magpie's persistence
// without reading or writing its own localStorage namespace.
function readChartChoice(key:string,allowed:string[],fallback:string){
 try{const saved=localStorage.getItem("winone.usage."+key);return saved&&allowed.includes(saved)?saved:fallback;}
 catch{return fallback; /* Embedded storage may be unavailable; rendering still works. */}
}
function saveChartChoice(key:string,value:string){
 try{localStorage.setItem("winone.usage."+key,value);}catch{/* Retain the in-memory viewing choice. */}
}

// The original Magpie drawing functions retain their metric arithmetic and
// interactions; React owns only lifecycle, selectors and WinOne's locale.
export function UsageChart({page,filter,onPick,onDay}:{page:UsagePage;filter?:UsageFilter;onPick?:(dimension:string,id:string)=>void;onDay?:(day:string)=>void}){
 const {t,i18n}=useTranslation();
 const [metric,setMetric]=useState(()=>readChartChoice("metric",["tokens","cost","calls","speed"],"tokens"));
 const [style,setStyle]=useState(()=>readChartChoice("chart",["bars","line"],"bars")),[split,setSplit]=useState("model");
 const keyed=!!page.callerKeys?.length,dimension=split==="callerKey"&&!keyed?"model":split;
 const picked=dimension==="modelAt"?filter?.provider&&filter.model?filter.provider+"/"+filter.model:"":filter?.[dimension==="agent"?"source":dimension as keyof UsageFilter]||"";
 useEffect(()=>{if(!keyed&&split==="callerKey")setSplit("model");},[keyed,split]);
 const chart=useRef<HTMLDivElement>(null),rank=useRef<HTMLDivElement>(null);
 useEffect(()=>{
  if(!chart.current||!rank.current)return;
  const renderer=createUsageRenderer(i18n.language);
  const draw=()=>{
   const drawChart=style==="line"?renderer.drawLedLines:renderer.drawLedColumns;
   drawChart(chart.current,page,dimension,metric,false,onDay,undefined);
   Object.assign(rank.current!,{chart:chart.current});
   createUsageRenderer(i18n.language,page.by?.[dimension]?.length??7).drawLedRank(rank.current,page,dimension,metric,picked,(row:UsageShare)=>onPick?.(dimension,row.id),false);
  };
  // The upstream renderer replaces its plot and changes the chart height.
  // React must not redraw that same observed subtree inside RO delivery.
  // Only width changes require a new plot (the plot height is fixed by CSS).
  let frame=0,width=chart.current.clientWidth;
  const resize=()=>{if(!frame)frame=requestAnimationFrame(()=>{frame=0;const next=chart.current?.clientWidth??0;if(next>0&&next!==width){width=next;draw();}});};
  draw();const observer=typeof ResizeObserver!=="undefined"?new ResizeObserver(resize):null;observer?.observe(chart.current);
  return()=>{observer?.disconnect();if(frame)cancelAnimationFrame(frame);};
 },[page,metric,dimension,style,picked,i18n.language,onPick,onDay]);
 return <section className="settings-group" aria-label={t("usage.trend")}>
  <div className="usage-chart-controls">
   <CaptureOptionSelect stacked label={t("usage.metric")} value={metric} disabled={false} onChange={value=>{setMetric(value);saveChartChoice("metric",value);}} items={["tokens","cost","calls","speed"].map(value=>({value,label:t("usage."+(value==="tokens"?"tokenTotal":value))}))}/>
   <div role="group" aria-label={t("usage.chartStyle")} className="usage-chart-style">{(["bars","line"] as const).map(value=><Button key={value} icon={value==="bars"?ChartColumn:ChartLine} variant="ghost" title={t("usage."+value)} aria-label={t("usage."+value)} aria-pressed={style===value} onClick={()=>{setStyle(value);saveChartChoice("chart",value);}}/>)}</div>
   <CaptureOptionSelect stacked label={t("usage.split")} value={dimension} disabled={false} onChange={setSplit} items={["model","modelAt","provider","agent",...(keyed?["callerKey"]:[])].map(value=>({value,label:t("usage."+value)}))}/>
  </div>
  <div className="usage-chart-layout"><div className="led-chart" ref={chart}/><div className="led-rank" ref={rank}/></div>
 </section>;
}
