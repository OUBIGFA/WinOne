import {useEffect,useRef,useState} from "react";
import {Braces,Plus,X} from "lucide-react";
import {Button} from "../ui/button";
import {Input} from "../components/ui/input";
import {Textarea} from "../components/ui/textarea";
import {useTranslation} from "../i18n";
import {headerRows,headersOf,prettyJSON,minifyJSON,type HeaderRow} from "./magpie-headers";

// Magpie's ordered name/value/JSON editor assembled with WinOne's existing
// input and button components. Save/discovery consume only the serialized map.
export function AIProviderHeaders({value,disabled,onChange,resetToken}:{value?:Record<string,string>;disabled:boolean;onChange:(value:Record<string,string>)=>void;resetToken?:unknown}){
 const {t}=useTranslation();const [rows,setRows]=useState<HeaderRow[]>(()=>headerRows(value));const initial=useRef(true);
 useEffect(()=>{if(initial.current){initial.current=false;return;}setRows(headerRows(value));},[resetToken]);
 function update(next:HeaderRow[]){setRows(next);onChange(headersOf(next));}
 function patch(index:number,name:string,content:string,json:boolean){update(rows.map((row,i)=>i===index?[name,content,json]:row));}
 return <section className="settings-group">
  <div><h3 className="settings-group-title">{t("ai.headers")}</h3><p className="settings-group-description">{t("ai.headersHint")}</p></div>
  <div className="space-y-2">{rows.map(([name,content,json],index)=><div key={index} className="space-y-2"><div className="flex items-center gap-2">
   <Input className={json?"flex-1":"min-w-0 flex-1"} aria-label={`${t("ai.headerName")} ${index+1}`} placeholder="Header-Name" value={name} disabled={disabled} onChange={event=>patch(index,event.target.value,content,json)}/>
   {!json&&<Input className="min-w-0 flex-[2]" aria-label={`${t("ai.headerValue")} ${index+1}`} placeholder={t("ai.headerValue")} value={content} disabled={disabled} onChange={event=>patch(index,name,event.target.value,json)}/>}
   <Button variant="ghost" size="icon" icon={Braces} aria-label={t(json?"ai.headerSingleLine":"ai.headerJSON")} title={t(json?"ai.headerSingleLine":"ai.headerJSON")} disabled={disabled} onClick={()=>patch(index,name,json?minifyJSON(content):prettyJSON(content),!json)}/>
   <Button variant="ghost" size="icon" icon={X} aria-label={`${t("ai.removeHeader")} ${index+1}`} title={t("ai.removeHeader")} disabled={disabled} onClick={()=>update(rows.filter((_,i)=>i!==index))}/>
  </div>{json&&<Textarea className="min-h-24 font-mono text-sm" spellCheck={false} wrap="off" aria-label={`${t("ai.headerValue")} ${index+1}`} value={content} disabled={disabled} onChange={event=>patch(index,name,event.target.value,json)}/>}</div>)}</div>
  <div><Button variant="ghost" icon={Plus} disabled={disabled||rows.length>=64} onClick={()=>update([...rows,["","",false]])}>{t("ai.addHeader")}</Button></div>
 </section>;
}
