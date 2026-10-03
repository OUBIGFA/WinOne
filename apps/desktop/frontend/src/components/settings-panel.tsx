import {useState,type ReactNode} from "react";
import {Tabs,TabsList,TabsTrigger,TabsContent} from "./ui/tabs";
import {Diagnostics} from "../features/diagnostics";
import {useTranslation} from "../i18n";
import {Updates} from "../features/updates";

export function SettingsPanel({children}:{children:ReactNode}) {
 const {t}=useTranslation();const [tab,setTab]=useState("general");
 return <Tabs value={tab} onValueChange={value=>setTab(String(value))}>
  <TabsList aria-label={t("nav.settings")}><TabsTrigger value="general">{t("shell.general")}</TabsTrigger><TabsTrigger value="logs">{t("diagnostics.title")}</TabsTrigger></TabsList>
  <TabsContent value="general" keepMounted>{children}<Updates/></TabsContent>
  <TabsContent value="logs">{tab==="logs"&&<Diagnostics moduleId="host"/>}</TabsContent>
 </Tabs>;
}
