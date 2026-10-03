import { useState } from "react";
import { useTranslation } from "../i18n";
import { Button } from "./button";
import {Notice} from "./notice";
import "./capture-preview.css";

export function CapturePreview({id,ready,width,height,onRefresh}:{id?:string;ready?:boolean;width:number;height:number;onRefresh:()=>void}) {
  const {t}=useTranslation();
  const [failed,setFailed]=useState(false);
  if(!ready||!id)return <p className="muted">{t("capture.noPreview")}</p>;
  if(failed)return <Notice actions={<Button onClick={()=>{setFailed(false);onRefresh();}}>{t("capture.refreshPreview")}</Button>}>{t("capture.previewFailed")}</Notice>;
  return <div className="capture-preview"><img src={`/capture-preview/${encodeURIComponent(id)}.png`} alt={t("capture.preview")} width={width} height={height} decoding="async" loading="lazy" onError={()=>setFailed(true)}/></div>;
}
