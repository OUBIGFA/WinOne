import {useEffect,useRef} from "react";
import {useTranslation} from "../i18n";
import {magpieUsageHeatmap} from "./magpie-usage-heatmap";
import type {UsageHeatmapData} from "./ai-usage-types";

const chinese:Record<string,string>={"Usage heatmap":"请求活动热图","Tokens":"Token","Requests":"请求数","Cost":"费用","Less":"少","More":"多","No requests":"没有请求","{n} tokens":"{n} Token","{n} request":"{n} 次请求","{n} requests":"{n} 次请求"};

export function UsageHeatmap({data}:{data:UsageHeatmapData}){
 const {t,i18n}=useTranslation(),box=useRef<HTMLElement>(null),renderer=useRef<ReturnType<typeof magpieUsageHeatmap>>(null);
 useEffect(()=>{
  if(!box.current)return;
  renderer.current=magpieUsageHeatmap(box.current,i18n.language,(key:string,values:Record<string,unknown>={})=>{let text=i18n.language.startsWith("zh")?chinese[key]??key:key;for(const [name,value] of Object.entries(values))text=text.replaceAll("{"+name+"}",String(value));return text;});
  renderer.current.update(data);
  return()=>{renderer.current?.dispose();renderer.current=null;};
 },[i18n.language]);
 useEffect(()=>{renderer.current?.update(data);},[data]);
 return <section id="ledHeat" className="led-heat" ref={box} aria-label={t("usage.heatmap")}>
  <div className="usage-heat-heading"><div className="heat-title"><h3>{t("usage.heatmap")}</h3><span>{t("usage.heatmapPeriod")}</span></div><div id="ledHeatMetric" className="usage-heat-metrics" aria-label={t("usage.metric")}/></div>
  <div id="ledHeatScroll" className="heat-scroll"/><div className="heat-foot"/><div id="ledHeatTip" className="heat-tip" hidden/>
 </section>;
}
