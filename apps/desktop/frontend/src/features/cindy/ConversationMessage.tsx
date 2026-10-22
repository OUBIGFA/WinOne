// Layout extracted from Cindy UserMessage.tsx (a44ac274), Apache-2.0,
// Copyright 2026 XD Inc. Keep the full-width right-aligned row, constrained
// column, and independently sized bubble/actions. Host callbacks and existing
// class hooks retain WinOne copy/fork/retry/edit, attachments and navigation.
import type {ReactNode} from 'react';

export function ConversationMessage({role,children,actions,label}:{role:'user'|'assistant';children:ReactNode;actions?:ReactNode;label:string}){
 return <article className={`desktop-conversation-message desktop-conversation-message--${role}`} aria-label={label}>
  <div className="chat-message-column">
   <div className="chat-message-bubble">{children}</div>
   {actions&&<div className="desktop-conversation-message-actions">{actions}</div>}
  </div>
 </article>;
}
