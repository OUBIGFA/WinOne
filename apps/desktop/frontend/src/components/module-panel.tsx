import {type ReactNode} from "react";
import {Shortcuts} from "../features/shortcuts";
import {useTranslation} from "../i18n";
import type {PageActivityChanged} from "../ui/page-navigation";

// Shortcuts remain visible above preferences. Each module owns its settings
// grouping; switching diagnostic tabs must not discard a preference draft.
export function ModulePanel({moduleId,children,onSaved,onActivity}:{moduleId:"capture"|"always-on-top"|"wintray";children:ReactNode;onSaved:()=>void;onActivity:PageActivityChanged}) {
 const {t}=useTranslation();
 if(moduleId==="wintray")return <>{children}</>;
 return <div className="module-sections">
  <section aria-label={t("shortcuts.title")} className="module-section">
   <h2 className="section-heading">{t("shortcuts.title")}</h2>
   <Shortcuts moduleId={moduleId} onSaved={onSaved} onActivity={onActivity}/>
  </section>
  {moduleId==="capture"?children:<section className="module-section">{children}</section>}
 </div>;
}
