import {useTranslation} from "../i18n";
import {isFreeModel} from "./ai-model-label";
import type {AIProvider} from "./ai-configuration";

// Magpie ede1e32 freeBadge/rateTag/contextTag. Preserve every upstream label;
// badges never participate in the model name's ellipsis or change its route.
export function AIModelBadge({provider,id}:{provider:Pick<AIProvider,"modelNames"|"modelFree"|"modelMetadata">;id:string}){
 const {t}=useTranslation();
 const metadata=provider.modelMetadata?.[id],rate=metadata?.rate??0,was=metadata?.rateWas??0;
 const context=metadata?.listedContextWindow??metadata?.contextWindow??0,name=provider.modelNames?.[id]??id;
 const times=(value:number)=>(value<.01?"<0.01":String(+value.toFixed(2)))+"×";
 const contextLabel=context/1e6;
 const caption="shrink-0 whitespace-nowrap rounded px-1.5 py-0.5 text-xs leading-4 font-medium";
 return <>
  {isFreeModel(provider,id)&&<span data-model-badge="free" className={caption+" bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300"}>{t("ai.free")}</span>}
  {(rate>0||was>rate)&&<span data-model-badge="rate" className={caption+" inline-flex gap-1 bg-muted text-muted-foreground"} title={t(rate<=0&&was>rate?"ai.modelFreeAfterDiscount":was>rate?"ai.modelDiscountRate":"ai.modelRate",{rate:times(rate),was:times(was)})}>{was>rate&&<s>{times(was)}</s>}{rate>0&&<span>{times(rate)}</span>}</span>}
  {context>=1e6&&!/\b\d+M\b/.test(name)&&<span data-model-badge="context" className={caption+" bg-muted text-muted-foreground"} title={t("ai.modelContextTokens",{count:context})}>{Number.isInteger(contextLabel)?contextLabel:contextLabel.toFixed(1)}M</span>}
 </>;
}
