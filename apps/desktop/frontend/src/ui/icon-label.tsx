import type {ReactNode} from "react";
import type {LucideIcon} from "lucide-react";

// Decorative icons preserve the existing accessible label and shared type scale.
export function IconLabel({icon:Icon,children}:{icon:LucideIcon;children:ReactNode}){
 return <span className="inline-flex min-w-0 items-center gap-2"><Icon aria-hidden="true" className="size-4 shrink-0"/><span>{children}</span></span>;
}
