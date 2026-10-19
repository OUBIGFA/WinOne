import {type ReactNode} from "react";

// Shortcuts are one of the module's own tabs now, so no separate block sits
// above the task navigation. This wrapper keeps only the module page stacking
// and its own section boundary.
export function ModulePanel({moduleId,children}:{moduleId:"capture"|"selection"|"quick-assistant"|"always-on-top"|"wintray"|"stroke";children:ReactNode}) {
 if(moduleId==="wintray"||moduleId==="stroke")return <>{children}</>;
 return <div className="settings-section-stack module-settings-layout">
  {moduleId==="capture"?children:<section className="module-section">{children}</section>}
 </div>;
}
