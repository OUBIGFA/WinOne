// Presentation adapter only: tab lifecycle, file data and actions stay in the
// existing WinOne workbench. The Cindy bar receives verified display metadata.
import {FileText,FolderTree,Globe,Terminal,GitPullRequestArrow,Puzzle,Plus,Pin} from 'lucide-react';
import type {PanelTabLabel} from './types';
const icons:Record<string,typeof FileText>={file:FileText,files:FolderTree,browser:Globe,terminal:Terminal,review:GitPullRequestArrow,resources:Puzzle,home:Plus};
const plugin={
 TabPillTitle:({state}:{state:PanelTabLabel;sessionId?:string|null;active?:boolean;t?:unknown})=><span title={state.title}>{state.title}{state.dirty?' *':''}</span>,
 TabPillIcon:({state}:{state:PanelTabLabel;sessionId?:string|null;active?:boolean})=>{const Icon=state.pinned?Pin:icons[state.kind]??Puzzle;return <Icon size={13}/>;},
};
export function getTabKind(_kind:string){return plugin;}
export function hydrateTabState(_plugin:unknown,value:unknown):PanelTabLabel{return value&&typeof value==='object'?value as PanelTabLabel:{title:'',kind:'home'};}
