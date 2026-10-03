import {InlineError} from "../ui/notice";
import {useEffect,useId,useRef,useState} from "react";
import {Button} from "../ui/button";
import {Input} from "../components/ui/input";
import {NumberField,validNumber} from "../ui/number-field";
import {Label} from "../components/ui/label";
import {Tabs,TabsList,TabsTrigger,TabsContent} from "../components/ui/tabs";
import {useTranslation} from "../i18n";
import type {AnnotationMark} from "./capture-editor-model";

// One local draft becomes one undo step on Apply. The parent prevents changes
// to the selected mark until these fields are applied or cancelled.
export function AnnotationProperties({mark,onApply,onClose,floating}:{floating?:{left:number;top:number;maxHeight:number};mark:AnnotationMark;onApply:(mark:AnnotationMark)=>void;onClose:()=>void}) {
  const {t}=useTranslation();
  const id=useId(),panel=useRef<HTMLElement>(null);
  const [draft,setDraft]=useState(mark);
  const [error,setError]=useState(false);
  const [tab,setTab]=useState("style");
  useEffect(()=>{panel.current?.focus({preventScroll:true});},[]);
  const effect=mark.kind==="mosaic"||mark.kind==="blur";
  type NumericKey="x"|"y"|"width"|"height"|"strokeWidth"|"fontSize"|"effectSize";
  const fields:Array<{name:NumericKey;label:string;min:number;max?:number;integer?:boolean}>=[];
  if(!effect&&mark.kind!=="text"&&mark.kind!=="cover")fields.push({name:"strokeWidth",label:"capture.strokeWidth",min:1,max:64});
  if(effect)fields.push({name:"effectSize",label:"capture.effectStrength",min:2,max:64,integer:true});
  if(mark.kind==="text")fields.push({name:"fontSize",label:"capture.fontSize",min:8,max:256});
  if(mark.kind!=="arrow"&&mark.kind!=="pen")fields.push({name:"x",label:"capture.positionX",min:0},{name:"y",label:"capture.positionY",min:0},{name:"width",label:"capture.width",min:1},{name:"height",label:"capture.height",min:1});
  const invalidFields=fields.filter(field=>!validNumber(draft[field.name]??NaN,field.min,field.max,field.integer??false));
  const hiddenErrors=invalidFields.filter(field=>(["x","y","width","height"].includes(field.name)?"position":"style")!==tab);
  function apply() {
    if(invalidFields.length)return;
    try {onApply(draft);onClose();}
    catch {setError(true);}
  }
  const number=(name:NumericKey)=>{
    const field=fields.find(field=>field.name===name);
    if(!field)return null;
    return <NumberField id={`${id}-${name}`} label={t(field.label)} min={field.min} max={field.max} integer={field.integer??false} value={draft[name]??NaN} onChange={value=>{setDraft({...draft,[name]:value});setError(false);}}/>;
  };
  return <aside style={floating?{position:"absolute",...floating,zIndex:5,padding:16,background:"var(--panel)",border:"1px solid var(--line)"}:undefined} ref={panel} tabIndex={-1} aria-labelledby={`${id}-title`} className="w-80 max-w-[45%] shrink-0 space-y-4 overflow-y-auto border-l pl-4 outline-none" onKeyDown={event=>{if(event.key==="Escape"){event.preventDefault();event.stopPropagation();onClose();}}}>
    <h2 id={`${id}-title`} className="text-base font-medium">{t("capture.editAnnotation")}</h2>
    <Tabs value={tab} onValueChange={value=>setTab(String(value))}>
      <TabsList aria-label={t("capture.editAnnotation")}><TabsTrigger value="style">{t("capture.annotationStyle")}</TabsTrigger>{mark.kind!=="arrow"&&mark.kind!=="pen"&&<TabsTrigger value="position">{t("capture.positionSize")}</TabsTrigger>}</TabsList>
      <TabsContent value="style" className="space-y-4 pt-3">
        {!effect&&<div className="space-y-2"><Label htmlFor={`${id}-color`}>{t("capture.color")}</Label><Input id={`${id}-color`} type="color" className="w-12 px-1" value={draft.color} onChange={e=>setDraft({...draft,color:e.target.value})}/></div>}
        {number("strokeWidth")}{number("effectSize")}{number("fontSize")}
      </TabsContent>
      <TabsContent value="position" className="grid gap-4 pt-3">
        {number("x")}{number("y")}{number("width")}{number("height")}
      </TabsContent>
    </Tabs>
    {error&&<InlineError>{t("capture.invalidProperties")}</InlineError>}
    {!!hiddenErrors.length&&<InlineError>{t("common.checkFields",{fields:hiddenErrors.map(field=>t(field.label)).join(", ")})}</InlineError>}
    <footer className="flex flex-wrap gap-2"><Button variant="primary" disabled={!!invalidFields.length} onClick={apply}>{t("capture.apply")}</Button><Button onClick={onClose}>{t("capture.cancelEdit")}</Button></footer>
  </aside>;
}
