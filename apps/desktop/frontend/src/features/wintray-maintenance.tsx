import {Notice} from "../ui/notice";
import {useState} from "react";
import {Dialog,DialogContent,DialogHeader,DialogTitle,DialogDescription,DialogFooter} from "../components/ui/dialog";
import {moduleCall} from "../bridge";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";

interface ResetPreview {configurationRevision:number;moduleRevision:number;programCount:number}

export function WinTrayMaintenance({disabled=false,onChanged,onBusyChange=()=>{}}:{disabled?:boolean;onChanged:()=>void|Promise<void>;onBusyChange?:(busy:boolean)=>void}) {
  const {t}=useTranslation();
  const [preview,setPreview]=useState<ResetPreview>();
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState("");
  const [done,setDone]=useState(false);
  function working(value:boolean){setBusy(value);onBusyChange(value);}
  async function review(){
    working(true);setError("");setDone(false);
    try {setPreview(await moduleCall<ResetPreview>("resetWinTray.preview"));}
    catch(error){setError(String(error));}finally{working(false);}
  }
  async function reset(){
    if(!preview||busy)return;
    working(true);setError("");
    try {
      const result=await moduleCall<{state:string;error?:string}>("resetWinTray",{configurationRevision:preview.configurationRevision,moduleRevision:preview.moduleRevision,confirmed:true});
      if(result.state!=="completed")throw Error(result.error||t("wintray.failed"));
      setDone(true);
    }catch(error){setError(String(error));}
    finally{
      setPreview(undefined);
      try{await onChanged();}catch(error){setError(String(error));}
      working(false);
    }
  }
  return <div className="space-y-3 border-t pt-5">
    <Button disabled={disabled||busy} onClick={()=>void review()}>{t("wintray.resetPrograms")}</Button>
    {error&&<Notice>{error}</Notice>}
    {done&&<p role="status">{t("wintray.resetDone")}</p>}
    <Dialog open={!!preview} disablePointerDismissal onOpenChange={open=>{if(!open&&!busy)setPreview(undefined);}}>
      <DialogContent showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>{t("wintray.resetTitle")}</DialogTitle>
          <DialogDescription>{t("wintray.resetDescription",{count:preview?.programCount??0})}</DialogDescription>
        </DialogHeader>
        <p className="text-sm text-muted-foreground">{t("wintray.resetRetained")}</p>
        <DialogFooter>
          <Button disabled={busy} onClick={()=>setPreview(undefined)}>{t("wintray.cancelReset")}</Button>
          <Button variant="danger" disabled={busy} onClick={()=>void reset()}>{t(busy?"wintray.working":"wintray.confirmReset")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>;
}
