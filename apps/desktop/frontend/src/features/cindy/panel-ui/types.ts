// Cindy TabBar's view contract. WinOne stores its existing tab identities.
export type BuiltinTabKindId='file-browser'|'web-browser'|'retired-feature'|'terminal'|'review'|'orca-workers'|'subagents'|'background-tasks'|'routines'|'bot-workbench'|'resource-usage'|'cindy-make';
export type TabKindId=string;
export interface TabState {id:string;kind:TabKindId;state:unknown}
export interface PanelTabLabel{title:string;kind:string;dirty?:boolean;pinned?:boolean}
