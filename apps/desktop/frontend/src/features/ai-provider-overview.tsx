import {useTranslation} from "../i18n";
import {Button} from "../components/ui/button";
import {AIProviderIcon} from "./ai-provider-icon";
import type {AIProvider} from "./ai-configuration";
import {ChevronRight} from "lucide-react";
import "./ai-provider-overview.css";

// Reuse the shared shadcn/Base UI button and bundled provider icons. The flat
// summary stays mounted across service tabs and reflects the current draft.
export function AIProviderOverview({providers,disabled,unavailableProviders,onOpenProvider}:{providers:AIProvider[];disabled:boolean;unavailableProviders:string[];onOpenProvider:(id:string)=>void}){
 const {t}=useTranslation();
 const enabled=providers.filter(provider=>provider.models.length>0);
 return <section aria-label={t("ai.providerOverview")} className="ai-provider-summary">
  <div className="ai-provider-summary-heading"><h2>{t("ai.providerOverview")}</h2><p>{t("ai.providerTotal",{count:enabled.length,models:enabled.reduce((total,p)=>total+p.models.length,0)})}</p></div>
  {enabled.length>0?<ul className="ai-provider-summary-list">{enabled.map(provider=><li key={provider.id}><Button variant="outline" className="ai-provider-summary-link" aria-label={t("ai.providerCount",{name:provider.name||t("ai.unnamed"),count:provider.models.length})} title={provider.name||t("ai.unnamed")} disabled={disabled||unavailableProviders.includes(provider.id)} onClick={()=>onOpenProvider(provider.id)}><AIProviderIcon icon={provider.avatar||provider.icon}/><span className="ai-provider-summary-name">{provider.name||t("ai.unnamed")}</span><span className="ai-provider-summary-models">{t("ai.overviewModelCount",{count:provider.models.length})}</span><ChevronRight size={16} aria-hidden="true"/></Button></li>)}</ul>:<p className="settings-help">{t("ai.providerOverviewEmpty")}</p>}
 </section>;
}
