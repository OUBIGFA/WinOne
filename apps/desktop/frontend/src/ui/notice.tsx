import {Children,isValidElement,useState,type ComponentProps,type ReactNode} from "react";
import {CircleAlert,Info,X} from "lucide-react";
import {cn} from "cn";
import {Alert,AlertDescription,AlertTitle} from "../components/ui/alert";
import {FieldError} from "../components/ui/field";
import {Button} from "./button";
import {useTranslation} from "../i18n";

// Content identity, not freshly allocated JSX identity: polling the same
// message must not undo dismissal. No validation/permission state is changed.
function feedbackKey(value:ReactNode):string {
 return Children.toArray(value).map(child=>isValidElement<{children?:ReactNode}>(child)?feedbackKey(child.props.children):String(child)).join("\u0000");
}

export function Notice({children,title,actions,tone="error",className}:{children:ReactNode;title?:ReactNode;actions?:ReactNode;tone?:"error"|"info";className?:string}) {
 const {t}=useTranslation();
 const key=tone+"\u0000"+feedbackKey(title)+"\u0000"+feedbackKey(children);
 const [dismissed,setDismissed]=useState<string|null>(null);
 if(dismissed===key)return null;
 const Icon=tone==="error"?CircleAlert:Info;
 return <Alert variant={tone==="error"?"destructive":"default"} role={tone==="error"?"alert":"status"} className={cn("relative my-4 pr-10",className)}>
  <Icon aria-hidden="true"/>
  {title&&<AlertTitle className="min-w-0 [overflow-wrap:anywhere]">{title}</AlertTitle>}
  <AlertDescription className="min-w-0 whitespace-pre-line [overflow-wrap:anywhere]">
   {children}
   {actions&&<div className="mt-3 flex flex-wrap items-center gap-2 text-foreground">{actions}</div>}
  </AlertDescription>
  <Button variant="ghost" size="icon" className="absolute right-2 top-2 size-6 min-h-6 p-0" aria-label={t("common.dismissNotice")} onClick={()=>setDismissed(key)}><X className="size-4" aria-hidden="true"/></Button>
 </Alert>;
}

// Compact process/rich-answer feedback keeps its original typography while
// sharing the same explicit content-only dismissal as application notices.
export function InlineNotice({children,className,role="status"}:{children:ReactNode;className?:string;role?:"status"|"alert"}) {
 const {t}=useTranslation();
 const key=feedbackKey(children);
 const [dismissed,setDismissed]=useState<string|null>(null);
 if(dismissed===key)return null;
 return <div role={role} className={cn("flex min-w-0 items-start gap-2",className)}>
  <span className="min-w-0 flex-1 [overflow-wrap:anywhere]">{children}</span>
  <Button variant="ghost" size="icon" className="size-6 min-h-6 shrink-0 p-0" aria-label={t("common.dismissNotice")} onClick={()=>setDismissed(key)}><X className="size-4" aria-hidden="true"/></Button>
 </div>;
}

export function InlineError({className,...props}:ComponentProps<typeof FieldError>) {
 const {t}=useTranslation();
 const key=feedbackKey(props.children)+"\u0000"+JSON.stringify(props.errors?.map(error=>error?.message)??[]);
 const [dismissed,setDismissed]=useState<string|null>(null);
 if(dismissed===key||!props.children&&!props.errors?.some(error=>error?.message))return null;
 return <div className="flex min-w-0 items-start gap-2">
  <FieldError className={cn("min-w-0 flex-1 whitespace-pre-line [overflow-wrap:anywhere]",className)} {...props}/>
  <Button variant="ghost" size="icon" className="size-6 min-h-6 shrink-0 p-0" aria-label={t("common.dismissError")} onClick={()=>setDismissed(key)}><X className="size-4" aria-hidden="true"/></Button>
 </div>;
}
