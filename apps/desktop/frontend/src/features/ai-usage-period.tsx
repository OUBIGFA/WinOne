import {useEffect,useState} from "react";
import {useTranslation} from "../i18n";
import {Input} from "../components/ui/input";
import {Button} from "../ui/button";
import {CaptureOptionSelect} from "../ui/capture-option-select";

const dateInput=(day:Date)=>[day.getFullYear(),String(day.getMonth()+1).padStart(2,"0"),String(day.getDate()).padStart(2,"0")].join("-");

// Native date fields adapt Magpie's inclusive custom-period picker. Editing
// dates has no effect on requests until Apply is pressed; reversed dates are
// ordered exactly as upstream RangeOf does.
export function UsagePeriod({value,onChange}:{value:string;onChange:(period:string)=>void}){
 const {t}=useTranslation();const [custom,setCustom]=useState(value.includes(".."));
 const [from,setFrom]=useState(()=>value.split("..")[0].match(/^\d{4}-/)?value.split("..")[0]:dateInput(new Date(Date.now()-6*86400000)));
 const [to,setTo]=useState(()=>value.split("..")[1]||dateInput(new Date()));
 useEffect(()=>{if(value.includes("..")){const [first,last]=value.split("..");setCustom(true);setFrom(first);setTo(last);}},[value]);
 return <div className={"usage-period"+(custom?" usage-period-custom":"")}><CaptureOptionSelect stacked label={t("usage.period")} value={custom?"custom":value} disabled={false} items={[{value:"today",label:t("usage.today")},{value:"7d",label:t("usage.week")},{value:"30d",label:t("usage.month")},{value:"all",label:t("usage.all")},{value:"custom",label:t("usage.customPeriod")}]} onChange={period=>{setCustom(period==="custom");if(period!=="custom")onChange(period);}}/>
  {custom&&<div className="usage-date-range"><label><span>{t("usage.fromDate")}</span><Input type="date" aria-label={t("usage.fromDate")} value={from} onChange={event=>setFrom(event.target.value)}/></label><label><span>{t("usage.toDate")}</span><Input type="date" aria-label={t("usage.toDate")} value={to} onChange={event=>setTo(event.target.value)}/></label><Button disabled={!from||!to} onClick={()=>onChange([from,to].sort().join(".."))}>{t("usage.applyDates")}</Button></div>}
 </div>;
}
