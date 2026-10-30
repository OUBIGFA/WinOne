// WinOne host adapter for the teammate items of Cindy's composer @ panel
// (Cindy b7e46bc, Apache-2.0: ChatInput atResources bot rows, AtMentionPanel
// resource rows and composerContentSerialization's bot chip). WinOne's
// composer is a textarea, so a chosen teammate is written as `@Name`. On send,
// each `@Name` of a current teammate becomes Cindy's wire link
// `[Name](cindy://bot/<id>)` with its exact agentReferences range, which the
// Host resolves to that teammate.
import {Children,useEffect,useRef,type ReactElement} from 'react';
import {Bot} from 'lucide-react';
import {useTranslation} from '../i18n';
import {buildBotReferenceHref} from '../../../../../modules/chat-pi/cindy/tasks/core/agentInputProjection';
import type {ComposerBotMention} from './cindy/lib/fileTypes';
import {COMPOSER_MENU_ROW,MenuHighlightLayer,menuPanelAttrs,menuRowAttrs,useMenuPanel,withMenuLabels} from './cindy/menu-row';
import {MENU_CURRENT_ROW_ATTR,currentMarkedRow} from './cindy/dropdown-menu-highlight';
import {Tip} from './cindy/panel-ui/tooltip';
import {cn} from './cindy/adapter';

/** Cindy AgentInputReference of kind `bot`, as the composer sends it. */
export interface BotAgentReference {kind:'bot';start:number;end:number;href:string;botId:string;name:string}
/** The `@query` the caret is in: where its `@` starts and what follows it. */
export interface BotMentionQuery {start:number;query:string}

// An `@` right after these belongs to an address or a word, not a mention.
const WORD_BEFORE=/[A-Za-z0-9_.@]/;
const ASCII_WORD=/[A-Za-z0-9_]/;
/** Cindy's chip label: one line, trimmed. */
const mentionLabel=(name:string)=>name.replace(/\s+/g,' ').trim();

/** The `@query` being typed at a collapsed caret; like Cindy's panel, a query ends at whitespace. */
export function activeBotMentionQuery(text:string,caret:number):BotMentionQuery|null{
 const before=text.slice(0,caret),start=before.lastIndexOf('@');
 if(start<0||start>0&&WORD_BEFORE.test(before[start-1]))return null;
 const query=before.slice(start+1);
 return /\s/.test(query)||query.length>64?null:{start,query};
}

/** Teammates for a query: name prefix first, then name or id substring, in roster order. */
export function matchingBotMentions(mentions:readonly ComposerBotMention[],query:string):ComposerBotMention[]{
 const wanted=query.toLowerCase();
 const rank=(mention:ComposerBotMention)=>{
  const name=mention.name.toLowerCase();
  return name.startsWith(wanted)?0:name.includes(wanted)?1:mention.id.toLowerCase().includes(wanted)?2:3;
 };
 return mentions.map(mention=>({mention,score:rank(mention)})).filter(item=>item.score<3).sort((a,b)=>a.score-b.score).map(item=>item.mention);
}

/** Replace the typed `@query` with the chosen teammate and a space; the caret follows it. */
export function insertBotMention(text:string,query:BotMentionQuery,caret:number,mention:ComposerBotMention){
 const inserted='@'+mentionLabel(mention.name)+' ';
 return {text:text.slice(0,query.start)+inserted+text.slice(caret),caret:query.start+inserted.length};
}

/**
 * Cindy composerContentSerialization for bot chips: every `@Name` of a current
 * teammate becomes `[Name](cindy://bot/<id>)` with its exact range in the sent
 * text. Longer names win, so `@Reader Pro` is not read as `@Reader`. A name two
 * teammates share stays text, because the composer cannot tell them apart.
 */
export function serializeBotMentions(text:string,mentions:readonly ComposerBotMention[]):{text:string;agentReferences:BotAgentReference[]}{
 const byName=new Map<string,ComposerBotMention|null>();
 for(const mention of mentions){const name=mentionLabel(mention.name);if(name)byName.set(name,byName.has(name)?null:mention);}
 const names=[...byName.keys()].filter(name=>byName.get(name)).sort((a,b)=>b.length-a.length);
 const agentReferences:BotAgentReference[]=[];
 let output='',cursor=0;
 for(let at=text.indexOf('@');at>=0&&names.length;at=text.indexOf('@',at+1)){
  if(at<cursor||at>0&&WORD_BEFORE.test(text[at-1]))continue;
  // A Latin name must end at a word boundary; CJK text needs no space after it.
  const name=names.find(candidate=>text.startsWith(candidate,at+1)&&!(ASCII_WORD.test(candidate.at(-1)!)&&ASCII_WORD.test(text[at+1+candidate.length]??'')));
  if(!name)continue;
  const mention=byName.get(name)!,href=buildBotReferenceHref(mention.id),wire=`[${name.replace(/([[\]\\])/g,'\\$1')}](${href})`;
  output+=text.slice(cursor,at);
  agentReferences.push({kind:'bot',start:output.length,end:output.length+wire.length,href,botId:mention.id,name});
  output+=wire;cursor=at+1+name.length;
 }
 return {text:output+text.slice(cursor),agentReferences};
}

/**
 * Cindy AtMentionPanel's teammate rows above the composer. The textarea keeps
 * focus: rows never take it, and ↑↓/Enter/Tab/Esc are handled by the composer.
 */
export function BotMentionPanel({id,items,focused,onFocus,onChoose}:{id:string;items:readonly ComposerBotMention[];focused:number;onFocus:(index:number)=>void;onChoose:(mention:ComposerBotMention)=>void}){
 const {t}=useTranslation();
 const panelRef=useRef<HTMLDivElement|null>(null),focusedRef=useRef<HTMLButtonElement>(null);
 // Cindy's glide highlight follows the keyboard row while the editor keeps focus.
 const highlightPanelRef=useMenuPanel(panelRef,{lockWidth:false,options:{current:currentMarkedRow,currentAttributes:[MENU_CURRENT_ROW_ATTR],keyboardSource:document}});
 useEffect(()=>{focusedRef.current?.scrollIntoView?.({block:'nearest'});},[focused]);
 return <div className="chat-bot-mentions pointer-events-auto">
  <div ref={highlightPanelRef} id={id} role="listbox" aria-label={t('onebot.teammates')} {...menuPanelAttrs}
   className="relative w-full overflow-x-hidden overflow-y-auto rounded-[12px] border bg-[var(--cmd-palette-bg)] border-[var(--cmd-palette-border)] p-[6px]"
   style={{boxShadow:'var(--shadow-menu)',maxHeight:320}}>
   <MenuHighlightLayer/>
   {items.map((mention,index)=>{
    const current=index===focused;
    return <button key={mention.id} id={`${id}-${index}`} ref={current?focusedRef:undefined} type="button" role="option" aria-selected={current} tabIndex={-1}
     onMouseDown={event=>event.preventDefault()} onClick={()=>onChoose(mention)} onMouseEnter={()=>onFocus(index)}
     {...menuRowAttrs({current})} className={cn(COMPOSER_MENU_ROW,'flex w-full items-center gap-2 h-[44px] px-[10px] text-left')}>
     <Bot size={16} className="shrink-0 text-[var(--cmd-palette-item-icon)]"/>
     {/* Cindy shows a teammate's description beside its row; here it is the name's tooltip. */}
     <Tip text={mention.description||null}>{Children.toArray(withMenuLabels(<span className="min-w-0 truncate">{mention.name}</span>))[0] as ReactElement}</Tip>
     <span className="ml-auto shrink-0 truncate text-12 font-normal text-[var(--cmd-palette-item-meta)]">{t('onebot.mention.action')}</span>
    </button>;
   })}
  </div>
 </div>;
}
