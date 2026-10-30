import "./ai-model-menu.css";
import {useEffect,useRef,useState} from "react";
import {Bot,Brain,ChevronDown,ChevronRight,Check,FolderOpen,Gauge,Minimize2,MoreHorizontal,Play,Plus,LoaderCircle,Paperclip,FileText,Phone,Volume2,VolumeX,Wrench,X} from "lucide-react";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {Popover,PopoverTrigger,PopoverContent} from "../components/ui/popover";
import {Dialog,DialogContent,DialogTitle,DialogDescription,DialogFooter} from "../components/ui/dialog";
import {DropdownMenu,DropdownMenuCheckboxItem,DropdownMenuContent,DropdownMenuItem,DropdownMenuTrigger} from "../components/ui/dropdown-menu";
import {AIModelIcon} from "./ai-model-icon";
import {AIProviderIcon} from "./ai-provider-icon";
import {AIModelBadge} from "./ai-model-badge";
import {PermissionSelector} from "./cindy/PermissionSelector";
import {resolveComposerEnterIntent} from './cindy/composerEnterIntent';
import type {ChatDeliveryMode} from './chat-state';
import {SendButton} from "./cindy/SendButton";
import {ListComposerTextarea} from "./cindy/ListComposerTextarea";
import {FILE_REFERENCE_MIME} from "./chat-panel-state";
import {ComposerFrame} from "./cindy/ComposerFrame";
import {ComposerAttachments} from "./cindy/ComposerAttachments";
import {modelLabel} from "./ai-model-label";
import {ScrollArea} from "./lobe/scroll-area/ScrollArea";
import type {AIProvider,AISelection} from "./ai-configuration";
import {ChatTasks} from "./chat-tasks";
import {ChatAgents} from "./chat-agents";
import type {ChatTaskGraph,Approval,ChatPermission,ChatContextUsage,ChatSubagent,ChatUsage} from "./chat-state";
import {composerTransferFiles,hasComposerFiles} from './cindy/composerFileTransfer';
import {droppedTextPaths} from '../ui/path-drop';
import type {ChatAttachment} from "./chat-types";
import type {ComposerBotMention} from "./cindy/lib/fileTypes";
import {activeBotMentionQuery,insertBotMention,matchingBotMentions,BotMentionPanel,type BotMentionQuery} from "./chat-bot-mentions";

export function ChatApproval({request,busy,onRespond,onStop}:{request:Approval;busy:boolean;onRespond:(allow:boolean,scope:"once"|"session")=>void;onStop:()=>void}){
 const {t}=useTranslation(),root=useRef<HTMLDivElement>(null);
 useEffect(()=>{
  const key=(event:KeyboardEvent)=>{
   const target=event.target as HTMLElement|null;
   if(event.defaultPrevented||event.isComposing||event.keyCode===229||busy||!root.current||target?.closest("input,textarea,button,a,select,[contenteditable=true],[role=menu],[role=listbox]"))return;
   if(event.key==="Escape"){event.preventDefault();onRespond(false,"once");}
   if(event.key==="Enter"){event.preventDefault();onRespond(true,(event.ctrlKey||event.metaKey)&&request.scopeLabel?"session":"once");}
  };window.addEventListener("keydown",key);return()=>window.removeEventListener("keydown",key);
 },[request,busy,onRespond]);
 return <div ref={root} className="chat-approval" aria-label={t("chat.permissionTitle")}>
  <div className="chat-approval-heading"><h3>{t("chat.permissionTitle")}</h3><span className="cindy-chat-ui"><SendButton disabled={busy} isStreaming onClick={onStop}/></span></div>
  {request.autoReviewUnavailable&&<p role="status">{t("chat.unavailable")}</p>}
  {request.reason&&<p>{request.reason}</p>}
  <div className="chat-approval-operation"><strong>{request.tool}</strong><pre>{typeof request.args.command==="string"?request.args.command:JSON.stringify(request.args,null,2)}</pre></div>
  {request.scopeLabel&&<details><summary>{t("chat.scope")}</summary><pre>{request.scopeLabel}</pre></details>}
  <div className="chat-approval-actions"><Button disabled={busy} onClick={()=>onRespond(false,"once")}>{t("chat.deny")}<kbd aria-hidden="true">Esc</kbd></Button>
   {request.scopeLabel&&<Button disabled={busy} onClick={()=>onRespond(true,"session")}>{t("chat.allowSession")}<kbd aria-hidden="true">Ctrl ↵</kbd></Button>}
   <Button variant="primary" busy={busy} onClick={()=>onRespond(true,"once")}>{t("chat.allowOnce")}<kbd aria-hidden="true">↵</kbd></Button>
  </div>
 </div>;
}

const thinkingLevels=["","off","minimal","low","medium","high","xhigh","max"];
const compactCount=(value:number)=>value>=1_000_000?(value/1_000_000).toFixed(1)+"M":value>=1_000?(value/1_000).toFixed(1)+"k":String(value);
// Thinking level, fast mode, usage, compaction, sound and subagent status are
// composer-adjacent session controls; every action runs through the Host so
// the durable session stays the single source of truth.
// Cindy's composer suggestion panel shows at most this many teammate rows.
const MENTION_ROWS=8;
export function ChatComposer({queueEditing=false,text,onText,onSend,onStop,onResizeIntent,running,busy,inputLocked=busy,stopping=false,providers,model,permission,onModel,onPermission,onLocation,location,saving,attachments=[],onAttach,onImportFiles,onImportPaths,onImportError,onRemoveAttachment,onTools,conversationId="",thinkingLevel="",fastMode=false,onThinkingLevel,onFastMode,contextUsage,usage,onCompact,soundEnabled=true,onSoundToggle,subagents=[],onStopSubagent,paused=false,taskGraph,onPause,onResume,placeholder,hideRuntimeControls=false,mentions,onVoice}:{
 queueEditing?:boolean;text:string;onResizeIntent?:()=>void;onText:(value:string)=>void;onSend:(deliveryMode?:ChatDeliveryMode)=>void;onStop:()=>void;running:boolean;busy:boolean;inputLocked?:boolean;stopping?:boolean;providers:AIProvider[];model:AISelection;permission:ChatPermission;onModel:(value:AISelection)=>void;onPermission:(value:ChatPermission)=>void;onLocation?:()=>void;location?:string;saving?:boolean;attachments?:ChatAttachment[];onAttach?:()=>void;onImportFiles?:(files:File[],clipboard?:boolean)=>void;onImportPaths?:(paths:string[])=>void;onImportError?:(error:unknown)=>void;onRemoveAttachment?:(path:string)=>void;onTools?:(tab:"files"|"resources")=>void;conversationId?:string;thinkingLevel?:string;fastMode?:boolean;onThinkingLevel?:(value:string)=>void;onFastMode?:(value:boolean)=>void;contextUsage?:ChatContextUsage;usage?:ChatUsage;onCompact?:()=>void;soundEnabled?:boolean;onSoundToggle?:()=>void;paused?:boolean;taskGraph?:ChatTaskGraph;onPause?:()=>void;onResume?:()=>void;subagents?:ChatSubagent[];onStopSubagent?:(name:string)=>void;
 /** Cindy's teammate prompt ("跟小柴说点什么…"); ordinary Chat keeps its own. */
 placeholder?:string;
 /** A OneBot teammate's model and effort belong to its settings (Cindy hideRuntimeControls). */
 hideRuntimeControls?:boolean;
 /** Teammates the owner can reference with @ in a teammate's chat (Cindy botMentions). */
 mentions?:readonly ComposerBotMention[];
 /** The canonical OneBot chat supplies the original voice call surface. */
 onVoice?:()=>void;
}){
 const {t}=useTranslation();const input=useRef<HTMLTextAreaElement>(null),selectedModel=useRef<HTMLButtonElement>(null);
 const thinkingKey=thinkingLevel?"chat.thinking_"+thinkingLevel:"chat.thinkingAuto";
 const contextPercent=usage&&contextUsage&&contextUsage.contextWindow>0?Math.max(0,Math.min(100,contextUsage.percent??contextUsage.tokens/contextUsage.contextWindow*100)):undefined;
 const usageView=usage&&contextUsage&&contextPercent!==undefined?{tokens:contextUsage.tokens,window:contextUsage.contextWindow,percent:contextPercent,input:usage.input,output:usage.output,cacheRead:usage.cacheRead,cacheWrite:usage.cacheWrite,totalTokens:usage.totalTokens,cost:usage.cost,costKnown:usage.costKnown}:undefined;
 const [modelsOpen,setModelsOpen]=useState(false),[search,setSearch]=useState(""),[fullConfirm,setFullConfirm]=useState(false),[collapsedProviders,setCollapsedProviders]=useState(new Set<string>());
 function openModels(open:boolean){
  if(open){setSearch('');setCollapsedProviders(current=>{const next=new Set(current);next.delete(model.provider);return next;});}
  setModelsOpen(open);
 }
 useEffect(()=>{
  if(!modelsOpen)return;
  // Wait for the portalled list to lay out without stealing focus from search.
  const frame=requestAnimationFrame(()=>selectedModel.current?.scrollIntoView({block:'center',inline:'nearest',behavior:'instant'}));
  return()=>cancelAnimationFrame(frame);
 },[modelsOpen]);
 // Cindy treats every selected file as message content. A file-only prompt
 // must therefore remain sendable even when it has no browser annotation.
 const canSend=!!text.trim()||attachments.length>0;
 // A removed model is not a usable selection. Keep historical call metadata
 // in the transcript, but never leave a stale provider/model in the composer.
 const provider=providers.find(value=>value.id===model.provider&&value.models.includes(model.model)),providerName=provider?.name??'',modelName=provider?modelLabel(provider,model.model,t("ai.free")):t("chat.chooseModel");
 // A teammate's Host resolves its model chain at send time and reports a missing route itself.
 const canSubmit=canSend&&(hideRuntimeControls||!!provider);
 // Cindy ChatInput's two slots: Send owns the rightmost position. While it is
 // dispatching or there is no draft, Stop replaces it without shifting tools.
 const mainSlotIsStop=!queueEditing&&running&&(busy||!canSend),showSecondaryStop=running&&canSend&&!busy;
 const mutationLocked=busy||paused||inputLocked;
 // Cindy's @ panel: typing @ lists the teammates. A queued message is edited
 // as plain text, so it has no panel. Esc closes the list until the caret
 // leaves that @.
 const [mentionQuery,setMentionQuery]=useState<BotMentionQuery|null>(null),[mentionFocus,setMentionFocus]=useState(0);
 const dismissedMention=useRef<number|null>(null);
 const mentionItems=mentionQuery&&mentions?.length&&!queueEditing?matchingBotMentions(mentions,mentionQuery.query).slice(0,MENTION_ROWS):[];
 const mentionList=`chat-bot-mentions-${conversationId||'new'}`,mentionCurrent=Math.min(mentionFocus,Math.max(0,mentionItems.length-1));
 function trackMention(element:HTMLTextAreaElement){
  const query=mentions?.length&&element.selectionStart===element.selectionEnd?activeBotMentionQuery(element.value,element.selectionStart):null;
  if(!query)dismissedMention.current=null;
  const next=query&&dismissedMention.current!==query.start?query:null;
  if(next?.query!==mentionQuery?.query||next?.start!==mentionQuery?.start){setMentionQuery(next);setMentionFocus(0);}
 }
 function chooseMention(mention:ComposerBotMention){
  const element=input.current;
  if(!element||!mentionQuery)return;
  const next=insertBotMention(element.value,mentionQuery,element.selectionStart,mention);
  onText(next.text);setMentionQuery(null);
  // The new value renders first; then the caret follows the inserted name.
  requestAnimationFrame(()=>{element.focus();element.setSelectionRange(next.caret,next.caret);});
 }
 /** ↑↓ move, Enter/Tab choose and Esc closes while the list is open; other keys type. */
 function mentionKey(event:React.KeyboardEvent<HTMLTextAreaElement>){
  if(!mentionItems.length||event.nativeEvent.isComposing||event.keyCode===229)return false;
  if(event.key==='ArrowDown'||event.key==='ArrowUp')setMentionFocus((mentionCurrent+(event.key==='ArrowDown'?1:mentionItems.length-1))%mentionItems.length);
  else if((event.key==='Enter'||event.key==='Tab')&&!event.shiftKey&&!event.ctrlKey&&!event.altKey&&!event.metaKey)chooseMention(mentionItems[mentionCurrent]);
  else if(event.key==='Escape'){event.stopPropagation();dismissedMention.current=mentionQuery!.start;setMentionQuery(null);}
  else return false;
  event.preventDefault();
  return true;
 }
 const mentionAria=mentions?.length?{role:'combobox','aria-autocomplete':'list' as const,'aria-expanded':mentionItems.length>0,'aria-controls':mentionList,
  'aria-activedescendant':mentionItems.length?`${mentionList}-${mentionCurrent}`:undefined}:{};
 // Snapshot DOM Files while the browser still grants access. Prefer native
 // paths on Windows, but never also upload their duplicate bitmap/file entry.
 function paste(event:React.ClipboardEvent<HTMLDivElement>){
  const transfer=event.clipboardData;
  if(mutationLocked){event.preventDefault();return;}
  if(!hasComposerFiles(transfer)&&transfer.getData('text/plain'))return;
  if(!onImportFiles||!onImportPaths)return;
  event.preventDefault();
  let files:File[];try{files=composerTransferFiles(transfer,true);}catch(error){onImportError?.(error);return;}
  onImportFiles(files,true);
 }
 function drop(event:React.DragEvent<HTMLDivElement>){
  if(event.defaultPrevented)return;
  if(!onImportFiles||!onImportPaths)return;
  event.preventDefault();event.stopPropagation();if(mutationLocked)return;
  try{const files=composerTransferFiles(event.dataTransfer);if(files.length)onImportFiles(files);else{const paths=droppedTextPaths(event.dataTransfer);if(paths.length)onImportPaths(paths);}}catch(error){onImportError?.(error instanceof Error&&error.message==='fileDrop.localOnly'?Error(t(error.message)):error);}
 }
 return <>
  <div style={{display:'contents'}} onPaste={paste} onDrop={drop} onDragOver={event=>{if(hasComposerFiles(event.dataTransfer)){event.preventDefault();event.dataTransfer.dropEffect=mutationLocked?'none':'copy';}}}>
  <ComposerFrame onResizeIntent={onResizeIntent} newConversation={!!onLocation} editor={input} content={text} attachmentCount={attachments.length}>
   {mentionItems.length>0&&<BotMentionPanel id={mentionList} items={mentionItems} focused={mentionCurrent} onFocus={setMentionFocus} onChoose={chooseMention}/>}
   <ComposerAttachments id={conversationId} attachments={attachments} onRemove={path=>onRemoveAttachment?.(path)} disabled={busy||paused}/>
   <ListComposerTextarea ref={input} maxLength={100000} aria-label={t("chat.message")} placeholder={placeholder??t("chat.placeholder")} value={text} disabled={inputLocked} {...mentionAria} onChange={event=>{onText(event.target.value);trackMention(event.target);}} onSelect={event=>trackMention(event.currentTarget)} onBlur={()=>setMentionQuery(null)} onDragOver={event=>{if(event.dataTransfer.types.includes(FILE_REFERENCE_MIME)){event.preventDefault();event.dataTransfer.dropEffect="copy";}}} onDrop={event=>{const path=event.dataTransfer.getData(FILE_REFERENCE_MIME);if(path&&!mutationLocked){event.preventDefault();event.stopPropagation();onText(text+(text?"\n":"")+"@"+path);}}} onKeyDown={event=>{if(mentionKey(event))return;const intent=resolveComposerEnterIntent({...event.nativeEvent,key:event.key,shiftKey:event.shiftKey,altKey:event.altKey,ctrlKey:event.ctrlKey,metaKey:event.metaKey,repeat:event.repeat,isComposing:event.nativeEvent.isComposing||event.keyCode===229},'enter',{turnRunning:running,platform:'win32'});if(intent==='ignore'){event.preventDefault();return;}if(intent==='queue'||intent==='steer'){event.preventDefault();if(canSubmit&&!paused&&!busy)onSend(intent==='steer'?'steer':undefined);}}}/>
   <div className="chat-composer-toolbar">
    <div className="chat-composer-tools">
    {onVoice&&<Button variant="ghost" size="icon" icon={Phone} aria-label={t('bots.voiceMode.title')} title={t('bots.voiceMode.title')} onClick={onVoice} disabled={busy||paused}/>}
    <DropdownMenu><DropdownMenuTrigger render={<Button variant="ghost" size="icon" icon={Plus} aria-label={t("chat.attachments")} disabled={busy||paused}/>}/><DropdownMenuContent side="top" className="chat-row-menu"><DropdownMenuItem disabled={!onAttach} onClick={onAttach}><Paperclip/>{t("chat.uploadAttachment")}</DropdownMenuItem><DropdownMenuItem disabled={!onTools} onClick={()=>onTools?.("files")}><FileText/>{t("chat.insertFile")}</DropdownMenuItem><DropdownMenuItem disabled={!onTools} onClick={()=>onTools?.("resources")}><Wrench/>{t("chat.toolsAvailable")}</DropdownMenuItem></DropdownMenuContent></DropdownMenu>
    <PermissionSelector vendorKey="pi" permissionMode={permission} disabled={busy||running||paused||queueEditing} onPermissionModeChange={mode=>{if(mode==="bypassPermissions"&&permission!==mode)setFullConfirm(true);else onPermission(mode);}}/>
    {onThinkingLevel&&!hideRuntimeControls&&<DropdownMenu><DropdownMenuTrigger render={<Button variant="ghost" size="sm" className="chat-pill" icon={Brain} aria-label={t("chat.thinkingLevel")+" · "+t(thinkingKey)} disabled={busy||running||paused||queueEditing}/>}><span>{t(thinkingKey)}</span></DropdownMenuTrigger><DropdownMenuContent side="top" align="start" className="chat-row-menu">{thinkingLevels.map(level=><DropdownMenuItem key={level||"auto"} onClick={()=>{if(level!==thinkingLevel)onThinkingLevel(level);}}>{t(level?"chat.thinking_"+level:"chat.thinkingAuto")}{level===thinkingLevel&&<span className="chat-model-check"><Check size={14}/></span>}</DropdownMenuItem>)}{onFastMode&&<DropdownMenuCheckboxItem checked={fastMode} onCheckedChange={value=>onFastMode(!!value)}>{t("chat.fastMode")}</DropdownMenuCheckboxItem>}</DropdownMenuContent></DropdownMenu>}
    {(onCompact||onSoundToggle||taskGraph?.total||running||paused)&&<Popover><PopoverTrigger render={<Button variant="ghost" size="icon" icon={MoreHorizontal} aria-label={t('chat.sessionTools')}/>}/><PopoverContent side="top" align="start" className="chat-session-controls">
     {onCompact&&<Button variant="ghost" icon={Minimize2} disabled={busy||running||paused||queueEditing} onClick={onCompact}>{t('chat.compact')}</Button>}
     {onSoundToggle&&<Button variant="ghost" icon={soundEnabled?Volume2:VolumeX} aria-pressed={soundEnabled} disabled={busy} onClick={onSoundToggle}>{t(soundEnabled?'chat.disableSound':'chat.enableSound')}</Button>}
     <ChatTasks graph={taskGraph} paused={paused} running={running} busy={busy} onPause={onPause}/>
    </PopoverContent></Popover>}
    {paused&&onResume&&<Button variant="ghost" size="sm" icon={Play} disabled={busy} onClick={onResume}>{t('chat.resumeTask')}</Button>}
    <ChatAgents conversationId={conversationId} agents={subagents} busy={busy} paused={paused} onStop={onStopSubagent}/>
    </div><div className="chat-composer-model-actions">
    {usageView&&<Popover><PopoverTrigger render={<Button variant="ghost" size="icon" icon={Gauge} aria-label={t("chat.contextUsage")+" · "+Math.round(usageView.percent)+"%"} disabled={busy}/>}/><PopoverContent side="top" align="end" className="chat-usage-panel"><dl><div><dt>{t("chat.contextUsage")}</dt><dd>{compactCount(usageView.tokens)} / {compactCount(usageView.window)}</dd></div><div><dt>{t("chat.usageInput")}</dt><dd>{compactCount(usageView.input)}</dd></div><div><dt>{t("chat.usageOutput")}</dt><dd>{compactCount(usageView.output)}</dd></div><div><dt>{t("chat.usageCacheRead")}</dt><dd>{compactCount(usageView.cacheRead)}</dd></div><div><dt>{t("chat.usageCacheWrite")}</dt><dd>{compactCount(usageView.cacheWrite)}</dd></div><div><dt>{t("chat.usageTotal")}</dt><dd>{compactCount(usageView.totalTokens)}</dd></div><div><dt>{t("chat.usageCost")}</dt><dd>{usageView.costKnown?usageView.cost.toFixed(4):t("chat.usageCostUnknown")}</dd></div></dl></PopoverContent></Popover>}
    {!hideRuntimeControls&&<Popover open={modelsOpen} onOpenChange={openModels}><PopoverTrigger render={<button data-slot="cindy-model" type="button" className="chat-model-trigger" disabled={busy||running||paused||queueEditing} aria-label={t("chat.model")} title={providerName?`${providerName} · ${modelName}`:modelName}/>}>{provider?<AIProviderIcon icon={provider.avatar||provider.icon}/>:<Bot size={15}/>}{providerName&&<><span className="chat-provider-name">{providerName}</span><span aria-hidden="true">·</span></>}<span className="chat-model-name">{modelName}</span>{provider&&model.model&&<AIModelBadge provider={provider} id={model.model}/>}<ChevronDown size={14}/></PopoverTrigger>
     <PopoverContent side="top" align="end" className="ai-model-menu chat-model-menu"><input autoFocus aria-label={t("chat.searchModels")} placeholder={t("chat.searchModels")} value={search} onChange={event=>setSearch(event.target.value)}/><ScrollArea disableContentFit viewportProps={{className:"chat-model-viewport"}}><div role="listbox" aria-label={t("chat.model")}>{providers.map(p=>{
      const matches=p.models.filter(id=>(p.name+" "+id+" "+p.modelNames?.[id]).toLowerCase().includes(search.toLowerCase()));const expanded=!!search||!collapsedProviders.has(p.id);return matches.length>0&&<section key={p.id}><h4><button type="button" className="chat-model-group" aria-label={p.name} aria-expanded={expanded} onClick={()=>setCollapsedProviders(current=>{const next=new Set(current);if(next.has(p.id))next.delete(p.id);else next.add(p.id);return next;})}><AIProviderIcon icon={p.avatar||p.icon}/><span className="chat-model-label">{p.name}</span><small>{matches.length}</small>{expanded?<ChevronDown size={13}/>:<ChevronRight size={13}/>}</button></h4>{expanded&&matches.map(id=><button key={id} ref={model.provider===p.id&&model.model===id?selectedModel:undefined} type="button" role="option" aria-selected={model.provider===p.id&&model.model===id} onClick={()=>{onModel({provider:p.id,model:id});setModelsOpen(false);}}><AIModelIcon model={id} name={p.modelNames?.[id]}/><span className="chat-model-label">{modelLabel(p,id)}</span><AIModelBadge provider={p} id={id}/><span className="chat-model-check" aria-hidden="true">{model.provider===p.id&&model.model===id&&<Check size={14}/>}</span></button>)}</section>;
     })}</div></ScrollArea>{!providers.some(p=>p.models.length)&&<p>{t("chat.modelUnavailable")}</p>}</PopoverContent>
    </Popover>}
    {saving&&<span className="sr-only" role="status">{t("chat.saving")}</span>}
    {showSecondaryStop&&!paused&&<SendButton disabled={stopping} isStreaming onClick={onStop}/>}
    <span className="inline-flex rounded-full" onMouseDown={event=>{if(document.activeElement===input.current)event.preventDefault();}}>
     <SendButton action={queueEditing?"save":"send"} ariaLabel={queueEditing?t("chat.queue_save"):undefined} disabled={mainSlotIsStop?stopping:busy||paused||!canSubmit} isStreaming={mainSlotIsStop} onClick={mainSlotIsStop?onStop:()=>onSend()}/>
    </span>
    </div>
   </div>
  </ComposerFrame>
  </div>
  {location&&<p className="chat-composer-location"><FolderOpen size={13}/><span title={location}>{location}</span></p>}
  <Dialog open={fullConfirm} onOpenChange={setFullConfirm}><DialogContent><DialogTitle>{t("chat.fullTitle")}</DialogTitle><DialogDescription>{t("chat.fullDescription")}</DialogDescription><DialogFooter><Button onClick={()=>setFullConfirm(false)}>{t("chat.cancel")}</Button><Button variant="primary" onClick={()=>{setFullConfirm(false);onPermission("bypassPermissions");}}>{t("chat.enableFull")}</Button></DialogFooter></DialogContent></Dialog>
 </>;
}
