import {useEffect,useRef,useState} from "react";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {createUsageRenderer} from "./ai-usage-chart";
import type {UsagePage,UsageShare} from "./ai-usage-types";

// Every breakdown uses Magpie's actual ranking renderer, including shares,
// costs, failed counts and request filtering. Expanding keeps all identities
// selectable even when the trend groups them into "Other".
export function UsageBreakdowns({page,onPick}:{page:UsagePage;onPick:(dimension:string,id:string)=>void}){
 const {t}=useTranslation();
 return <section className="usage-breakdowns" aria-label={t("usage.breakdowns")}>
  {["agent","model","account","provider"].map(kind=><UsageRanking key={kind} kind={kind} page={page} onPick={onPick}/>)}
 </section>;
}

function UsageRanking({kind,page,onPick}:{kind:string;page:UsagePage;onPick:(dimension:string,id:string)=>void}){
 const {t,i18n}=useTranslation();const rank=useRef<HTMLDivElement>(null);const [expanded,setExpanded]=useState(true);
 const rows=page.by?.[kind]??[];
 useEffect(()=>{
  if(!rank.current)return;
  createUsageRenderer(i18n.language,expanded?rows.length:7).drawLedRank(rank.current,page,kind,"tokens","",(row:UsageShare)=>onPick(kind,row.id),true);
 },[page,kind,expanded,i18n.language,onPick,rows.length]);
 return <section className="usage-ranking" aria-label={t("usage.group"+kind)}>
  <div className="usage-ranking-heading"><h3>{t("usage.group"+kind)}</h3><span>{t("usage.groupCount",{count:rows.length})}</span></div>
  {rows.length?<div className="led-rank" ref={rank}/>:<p className="settings-help usage-ranking-empty">{t("usage.empty")}</p>}
  {rows.length>7&&<Button className="usage-ranking-more" onClick={()=>setExpanded(value=>!value)}>{t(expanded?"usage.showTopGroups":"usage.showAllGroups",{count:rows.length})}</Button>}
 </section>;
}
