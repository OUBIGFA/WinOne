// The navigation and home use one order, including disabled modules.
export const moduleOrder=["capture","selection","quick-assistant","wintray","convenience"];
export function moduleRank(id:string){const rank=moduleOrder.indexOf(id==="always-on-top"?"convenience":id);return rank<0?moduleOrder.length:rank;}
