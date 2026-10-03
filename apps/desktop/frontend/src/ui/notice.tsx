import type {ComponentProps,ReactNode} from "react";
import {CircleAlert,Info} from "lucide-react";
import {cn} from "cn";
import {Alert,AlertDescription,AlertTitle} from "../components/ui/alert";
import {FieldError} from "../components/ui/field";

// Application composition of the bundled shadcn primitives. Actions flow below
// the message so long translations cannot overlap it in narrow panels.
export function Notice({children,title,actions,tone="error",className}:{children:ReactNode;title?:ReactNode;actions?:ReactNode;tone?:"error"|"info";className?:string}) {
 const Icon=tone==="error"?CircleAlert:Info;
 return <Alert variant={tone==="error"?"destructive":"default"} role={tone==="error"?"alert":"status"} className={cn("my-4",className)}>
  <Icon aria-hidden="true"/>
  {title&&<AlertTitle className="min-w-0 [overflow-wrap:anywhere]">{title}</AlertTitle>}
  <AlertDescription className="min-w-0 whitespace-pre-line [overflow-wrap:anywhere]">
   {children}
   {actions&&<div className="mt-3 flex flex-wrap items-center gap-2 text-foreground">{actions}</div>}
  </AlertDescription>
 </Alert>;
}

export function InlineError({className,...props}:ComponentProps<typeof FieldError>) {
 return <FieldError className={cn("min-w-0 whitespace-pre-line [overflow-wrap:anywhere]",className)} {...props}/>;
}
