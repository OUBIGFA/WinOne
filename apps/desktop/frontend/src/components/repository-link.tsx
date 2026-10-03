import {InlineError} from "../ui/notice";
import {useState} from "react";
import {Button} from "./ui/button";
import {openRepository} from "../bridge";
import {useTranslation} from "../i18n";
import github from "../assets/github.svg";

export function RepositoryLink(){
 const {t}=useTranslation();
 const [busy,setBusy]=useState(false),[error,setError]=useState("");
 async function open(){
  setBusy(true);setError("");
  try{await openRepository();}catch(error){setError(String(error));}finally{setBusy(false);}
 }
 return <div className="mt-6 space-y-2">
  <Button variant="outline" disabled={busy} onClick={()=>void open()}><img src={github} alt="" className="size-4 dark:invert"/>{t("shell.repositoryExample")}</Button>
  {error&&<InlineError className="text-sm">{error}</InlineError>}
 </div>;
}
