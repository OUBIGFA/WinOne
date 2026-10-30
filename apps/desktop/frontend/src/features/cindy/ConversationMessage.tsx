// Layout extracted from Cindy UserMessage.tsx (a44ac274), Apache-2.0,
// Copyright 2026 XD Inc. Keep the full-width right-aligned row, constrained
// column, and independently sized bubble/actions. Host callbacks and existing
// class hooks retain WinOne copy/fork/retry/edit, attachments and navigation.
// A OneBot chat adds Cindy's companion variant: a bordered reply bubble and a
// footer row (topic entry, starter choices, learning notes) under the bubble.
import type {ReactNode} from 'react';

export function ConversationMessage({role,children,actions,label,footer,bot=false}:{role:'user'|'assistant';children:ReactNode;actions?:ReactNode;label:string;footer?:ReactNode;bot?:boolean}){
 return <article className={`desktop-conversation-message desktop-conversation-message--${role}`+(bot?' desktop-conversation-message--bot':'')} aria-label={label}>
  <div className="chat-message-column">
   <div className="chat-message-bubble">{children}</div>
   {actions&&<div className="desktop-conversation-message-actions">{actions}</div>}
   {footer}
  </div>
 </article>;
}
