import {useTranslation} from "../i18n";
import {SCRIPT_CAPABILITIES} from "../../../../../third_party/cindy/scheduler/types";
import type {AutomationDraft} from "./chat-automations-form";

/** Composes the existing form controls with Cindy's complete command/hook
 * schema. Capabilities enumerate the unchanged upstream single source. */
export function AutomationExecution({draft,change,busy}:{draft:AutomationDraft;change:(patch:Partial<AutomationDraft>)=>void;busy:boolean}){
 const {t}=useTranslation("automations");
 return <>
  <label className="chat-automation-field">{t("executionMode")}<select value={draft.executionMode} disabled={busy} onChange={event=>change(event.target.value==="script"?{executionMode:"script",workspaceKind:"project",useWorktree:false,sessionMode:draft.sessionMode==="persistent"?"fresh":draft.sessionMode,silentWhenIdle:false}:{executionMode:"agent"})}><option value="agent">{t("agentMode")}</option><option value="script">{t("scriptMode")}</option></select></label>
  {draft.executionMode==="script"&&<>
   <label className="chat-automation-field">{t("scriptCommand")}<textarea rows={3} value={draft.scriptCommand} disabled={busy} onChange={event=>change({scriptCommand:event.target.value})}/></label>
   <label className="chat-automation-field">{t("commandTimeout")}<input type="number" min="0.001" step="any" placeholder={t("noTimeout")} value={draft.scriptTimeout} disabled={busy} onChange={event=>change({scriptTimeout:event.target.value})}/></label>
   <p className="chat-automation-hint">{t("scriptHint")}</p>
   <fieldset><legend>{t("capabilities")}</legend>{SCRIPT_CAPABILITIES.map(capability=><label key={capability} className="chat-automation-check"><input type="checkbox" checked={draft.capabilities.includes(capability)} disabled={busy} onChange={event=>change({capabilities:event.target.checked?[...draft.capabilities,capability]:draft.capabilities.filter(value=>value!==capability)})}/>{capability}</label>)}</fieldset>
  </>}
  <label className="chat-automation-check"><input type="checkbox" checked={draft.hookEnabled} disabled={busy} onChange={event=>change({hookEnabled:event.target.checked})}/>{t("preRunHook")}</label>
  {draft.hookEnabled&&<><label className="chat-automation-field">{t("hookCommand")}<textarea rows={3} value={draft.hookCommand} disabled={busy} onChange={event=>change({hookCommand:event.target.value})}/></label><label className="chat-automation-field">{t("commandTimeout")}<input type="number" min="0.001" step="any" placeholder={t("noTimeout")} value={draft.hookTimeout} disabled={busy} onChange={event=>change({hookTimeout:event.target.value})}/></label><p className="chat-automation-hint">{t("hookHint")}</p></>}
 </>;
}
