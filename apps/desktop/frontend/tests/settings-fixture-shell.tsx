import type {ReactNode} from "react";
import {Home,ScanLine,MousePointer2,Pin,LaptopMinimal,Sparkles,Settings} from "lucide-react";
import {QuickAssistantIcon} from "../src/components/quick-assistant-icon";
import {AppShell} from "../src/components/app-shell";
import {useTranslation} from "../src/i18n";
import logo from "../../../../internal/branding/logo.png";

// Match the production navigation and content width. A standalone <main>
// misses sidebar pressure and container queries in narrow-window reviews.
export function SettingsFixtureShell({page,children}:{page:string;children:ReactNode}){
 const {t}=useTranslation();
 const pages=[{id:"overview",title:t("nav.overview"),icon:Home},{id:"capture",title:t("nav.capture"),icon:ScanLine},{id:"selection",title:t("nav.selection"),icon:MousePointer2},{id:"quick-assistant",title:t("nav.quick-assistant"),icon:QuickAssistantIcon},{id:"always-on-top",title:t("nav.always-on-top"),icon:Pin},{id:"wintray",title:t("nav.wintray"),icon:LaptopMinimal},{id:"ai-services",title:t("ai.serviceTitle"),icon:Sparkles},{id:"settings",title:t("nav.settings"),icon:Settings}];
 return <AppShell page={page} pages={pages} logo={logo} onNavigate={()=>{}}>{children}</AppShell>;
}
