import {useState,type ReactNode} from "react";
import {Tabs,TabsList,TabsTrigger,TabsContent} from "./ui/tabs";
import {Diagnostics} from "../features/diagnostics";
import {useTranslation} from "../i18n";
import {Updates} from "../features/updates";
import {type PageActivityChanged} from "../ui/page-navigation";
import {FileText} from "lucide-react";
import {IconLabel} from "../ui/icon-label";

export function SettingsPanel({children}:{children:ReactNode;onActivity?:PageActivityChanged}) {
 const {t}=useTranslation();
 const [logsOpened,setLogsOpened]=useState(false);
 // AI services have their own page. Diagnostics remains an independent final
 // section, and opening it never unmounts the general preference forms.
 return <div className="module-sections settings-panel">
  <section className="module-section">{children}<Updates/></section>
  <section className="module-section"><Tabs value={logsOpened?"logs":null} onValueChange={value=>setLogsOpened(value==="logs")}>
   <TabsList variant="line" aria-label={t("diagnostics.title")}><TabsTrigger value="logs"><IconLabel icon={FileText}>{t("diagnostics.title")}</IconLabel></TabsTrigger></TabsList>
   <TabsContent value="logs" className="">{logsOpened&&<Diagnostics moduleId="host"/>}</TabsContent>
  </Tabs></section>
 </div>;
}
