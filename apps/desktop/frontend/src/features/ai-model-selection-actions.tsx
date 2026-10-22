import {Button} from "../ui/button";
import {useTranslation} from "../i18n";

// Compose the existing WinOne/shadcn buttons so every model-enable list has
// the same scope and disabled behavior. Search does not narrow bulk selection.
export function AIModelSelectionActions({total,selected,hasPaused=false,disabled,onAll,onNone}:{total:number;selected:number;hasPaused?:boolean;disabled:boolean;onAll:()=>void;onNone:()=>void}){
 const {t}=useTranslation();
 return <div className="flex shrink-0 items-center gap-1" role="group" aria-label={t("ai.modelBulkActions")}>
  <Button variant="ghost" disabled={disabled||total===0||selected===total} onClick={onAll} title={t("ai.modelBulkScope")}>{t("ai.selectAll")}</Button>
  <Button variant="ghost" disabled={disabled||selected===0&&!hasPaused} onClick={onNone} title={t("ai.modelBulkScope")}>{t("ai.selectNone")}</Button>
 </div>;
}
