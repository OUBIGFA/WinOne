import {InlineError} from "../ui/notice";
import { useEffect, useState } from "react";
import { getResidency } from "../bridge";
import { useTranslation } from "../i18n";

export function ResidencyStatus() {
  const {t}=useTranslation();
  const [state,setState]=useState<{requested:boolean;reasons:string[]}>();
  const [error,setError]=useState("");
  useEffect(()=>{
    let cancelled=false;
    let timer:ReturnType<typeof setTimeout>;
    const refresh=async()=>{
      try{const next=await getResidency();if(!cancelled){setState(next);setError("");}}
      catch(e){if(!cancelled)setError(String(e));}
      finally{if(!cancelled)timer=setTimeout(()=>void refresh(),3000);}
    };
    void refresh();return()=>{cancelled=true;clearTimeout(timer);};
  },[]);
  return <div className="settings-state-panel">
    <h3 className="settings-group-title">{t("wintray.residency")}</h3>
    {error ? <InlineError>{error}</InlineError> : state && <>
      <p>{t(state.requested ? "wintray.exitRequested" : "wintray.exitHint")}</p>
      {state.reasons.length>0 && <ul>{state.reasons.map(reason=><li key={reason}>{reason==="ai-gateway"?t("gateway.title"):reason}</li>)}</ul>}
    </>}
  </div>;
}
