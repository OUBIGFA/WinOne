// FileTargetChip view reused from makecindy/cindy a44ac274d1689699ea00de89ae2920d74a087d51
// apps/desktop/src/renderer/components/chat/MarkdownRenderer.tsx, Apache-2.0.
// WinOne supplies scoped file resolution, media/side-pane opening and the shared
// useFileChipContextMenu handlers; no second path classifier or menu is added.
import type {HTMLAttributes,ReactNode} from 'react';
import {FileTypeIcon} from './panel-ui/file-type-icon';
import {activateInlineLinkFromKeyboard} from './linkInteraction';
export function FileTargetChip({path,directory,children,...props}:HTMLAttributes<HTMLElement>&{path:string;directory?:boolean;children:ReactNode}){
 // Keep code, not button: copied rich text must retain the path in editors
 // whose clipboard allowlist removes button elements (Cindy's upstream rule).
 return <code {...props} role="button" tabIndex={0} title={props.title??path} onKeyDown={activateInlineLinkFromKeyboard} className="inline rounded-[6px] border-0 px-1 py-0.5 font-mono text-14 align-baseline bg-[var(--msg-md-inline-code-bg)] underline underline-offset-2 text-left break-all transition-colors cursor-pointer hover:bg-[var(--cmd-palette-item-hover)]">
  {!directory&&<FileTypeIcon name={path} size={14} className="mr-1 inline-block align-[-0.125em]"/>}{children}
 </code>;
}
