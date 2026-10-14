import {useEffect,useRef,useState} from "react";
import {useTranslation} from "../i18n";
import {CaptureOptionSelect} from "../ui/capture-option-select";
import {magpieLedgerCharts} from "./magpie-ledger-charts";
import type {UsagePage,UsageShare} from "./ai-usage-types";

const chartChinese:Record<string,string>={"Usage trend":"用量趋势","Other":"其他","Other ({n})":"其他（{n}）","Total":"合计","All":"全部","Nothing in this period":"此范围内还没有请求记录。","No known price for these requests":"这些请求尚无已知价格","No streamed replies timed in this period":"此范围内没有已计时的流式回复","{n} request":"{n} 次请求","{n} requests":"{n} 次请求","{n} tokens":"{n} Token","{n} tok/s":"{n} Token/秒","TTFT {ms}":"首字延迟 {ms}","hit rate {p}":"缓存命中率 {p}","{n} failed":"{n} 次失败","week of {label}":"{label} 所在周"};

export function createUsageRenderer(language:string,shown=7){
 return magpieLedgerCharts((key:string,values:Record<string,string|number>={})=>{let text=language.startsWith("zh")?chartChinese[key]??key:key;for(const [name,value] of Object.entries(values))text=text.replaceAll("{"+name+"}",String(value));return text;},language,{shown});
}

// The original Magpie drawing functions retain their metric arithmetic and
// interactions; React owns only lifecycle, selectors and WinOne's locale.
export function UsageChart({page,onPick,onDay}:{page:UsagePage;onPick?:(dimension:string,id:string)=>void;onDay?:(day:string)=>void}){
 const {t,i18n}=useTranslation();const [metric,setMetric]=useState("tokens"),[split,setSplit]=useState("model");
 const chart=useRef<HTMLDivElement>(null),rank=useRef<HTMLDivElement>(null);
 useEffect(()=>{
  if(!chart.current||!rank.current)return;
  const renderer=createUsageRenderer(i18n.language);
  const draw=()=>{renderer.drawLedColumns(chart.current,page,split,metric,false,onDay,undefined);Object.assign(rank.current!,{chart:chart.current});createUsageRenderer(i18n.language,page.by?.[split]?.length??7).drawLedRank(rank.current,page,split,metric,"",(row:UsageShare)=>onPick?.(split,row.id),false);};
  draw();const observer=typeof ResizeObserver!=="undefined"?new ResizeObserver(draw):null;observer?.observe(chart.current);
  return()=>observer?.disconnect();
 },[page,metric,split,i18n.language,onPick,onDay]);
 return <section className="settings-group" aria-label={t("usage.trend")}><div className="grid gap-4 sm:grid-cols-2"><CaptureOptionSelect stacked label={t("usage.metric")} value={metric} disabled={false} onChange={setMetric} items={["tokens","cost","calls","speed"].map(value=>({value,label:t("usage."+(value==="tokens"?"tokenTotal":value))}))}/><CaptureOptionSelect stacked label={t("usage.split")} value={split} disabled={false} onChange={setSplit} items={["model","modelAt","provider","agent"].map(value=>({value,label:t("usage."+(value==="tokens"?"tokenTotal":value))}))}/></div><div className="usage-chart-layout"><div className="led-chart" ref={chart}/><div className="led-rank" ref={rank}/></div></section>;
}
