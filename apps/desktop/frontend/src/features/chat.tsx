import {useCallback,useEffect,useLayoutEffect,useRef,useState} from "react";
import {FolderOpen,FolderInput,Download,MoreHorizontal,Archive,Pin,GitBranch,Copy,Settings2,RefreshCw,LoaderCircle,Trash2,Sparkles} from "lucide-react";
import {SidebarSimple} from '@phosphor-icons/react';
import {PathDropArea} from "../ui/path-drop-zone";
import {useDefaultLayout,usePanelRef} from "react-resizable-panels";
import welcomeLogo from "../../../../../internal/branding/chat-welcome.png";
import {chatCall,captureCall,pickDirectory,pickChatFiles,setFileDropTarget,readClipboardFiles,inspectDroppedPaths} from "../bridge";
import {importChatAttachments,uploadChatAttachment} from './chat-attachment-input';
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {usePageActivity,type PageActivityChanged} from "../ui/page-navigation";
import {DropdownMenu,DropdownMenuTrigger,DropdownMenuContent,DropdownMenuItem} from "../components/ui/dropdown-menu";
import {Dialog,DialogContent,DialogTitle,DialogDescription,DialogFooter} from "../components/ui/dialog";
import {ResizablePanelGroup,ResizablePanel,ResizableHandle} from "../components/ui/resizable";
import type {AIConfiguration,AISelection} from "./ai-configuration";
import {ChatComposer,ChatApproval} from "./chat-composer";
import {ChatQuestion} from "./chat-question";
import {ChatPlan,ChatPlanReminder} from "./chat-plan";
import {planForResponse} from './chat-plan-state';
import {ChatMessageView} from "./chat-messages";
import {ChatTranscriptChanges} from "./chat-turn-changes";
import {RetryStatus} from './cindy/RetryStatus';
import {ChatGeneratedFiles} from './chat-generated-files';
import {ChatScrollControls} from './chat-scroll-controls';
import {chatScrollMetrics,scrollChatToBottom} from './chat-scroll-position';
import {ChatSidebar,type ConversationAction} from "./chat-sidebar";
import {ChatSidePanel} from "./chat-side-panel";
import {ChatDesktopSurface} from './chat-desktop-kit';
import {MediaProvider} from './cindy/media/MediaProvider';
import {MEDIA_ATTACH_EVENT,setMediaDraftSnapshot,registerMedia,releaseTransientMedia,type MediaAttachDetail} from './cindy/media/host';
import {ChatProjectContext} from "./chat-project-context";
import {ChatProjectFields} from "./chat-project-dialog";
import {ChatSettings} from "./chat-settings";
import {ChatFeedback} from "./chat-feedback";
import {ChatMoveDialog,ChatExportDialog,chatArchiveNotice,type ChatArchiveOutcome} from "./chat-transfer-dialogs";
import {ChatAutomations} from "./chat-automations";
import {browserAnnotatedPrompt,removeBrowserAttachment} from "./chat-browser-annotations";
import type {ChatAttachment,ChatBrowserAnnotation} from "./chat-types";
import {readChatSoundEnabled,writeChatSoundEnabled,playChatDone,unlockChatAudio} from "./chat-sound";
import {messageText,mergeChatOperations,upsertChatMessage,type ChatDeliveryMode} from "./chat-state";
import {projectChatProcess} from "./chat-process";
import {WorkProcess} from "./cindy/WorkProcess";
import {QueuedSendConfirm} from './cindy/QueuedSendConfirm';
import {captureComposerSendSnapshot,isComposerSendSnapshotCurrent} from './cindy/composerSendSnapshot';
import {shouldUnpinOnWheel,shouldRepinOnWheel,shouldUnpinOnUpIntent,resolveNearBottomOnScroll} from './cindy/autoFollowIntent';
import {copyText} from "./clipboard";
import {ScrollArea} from "./lobe/scroll-area/ScrollArea";
import {ChatFileNavigation} from './chat-file-navigation';
import {ProcessNavigationProvider,ProcessCollapseAction} from "./process-navigation";
import {applyChatEvent,emptyChatState,type ChatActivities,type ChatCatalog,type ChatProject,type Conversation,type ChatPermission,type ChatSnapshot,type ChatState,type ChatPoll,type ChatMessage} from "./chat-state";
import "./chat.css";
import "./cindy/theme.css";
import "./cindy/message-tokens.css";
import "./cindy/reading.css";

const emptyCatalog:ChatCatalog={projects:[],conversations:[]};
const collapsedWorkbenchLayout={conversation:100,details:0};
const lastModelKey="winone.chat.last-model.v1";
// The composer starts from the AI service default model, but once the user
// switches a model the last choice is remembered and reused on the next chat.
function readLastModel():AISelection|undefined{try{const raw=localStorage.getItem(lastModelKey);if(!raw)return undefined;const value=JSON.parse(raw);if(value&&typeof value.provider==="string"&&typeof value.model==="string")return value;}catch{}return undefined;}
function initialModel(models:AIConfiguration,current:AISelection={provider:"",model:""}){const last=readLastModel();if(last&&models.providers.some(provider=>provider.id===last.provider&&provider.models.includes(last.model)))return last;return models.tasks.chat??models.tasks.default??models.providers.filter(provider=>provider.models.length).map(provider=>({provider:provider.id,model:provider.models[0]}))[0]??current;}
function writeLastModel(model:AISelection){try{localStorage.setItem(lastModelKey,JSON.stringify(model));}catch{}}
const errorText=(error:unknown)=>error instanceof Error?error.message:String(error);
const expandedPanelsKey='winone.chat.expanded-panels.v1';
const minimumSidebarWidth=224;
function readExpandedPanels(){const defaults={sidebar:240,right:420,bottom:300};try{const saved=JSON.parse(localStorage.getItem(expandedPanelsKey)||'{}');for(const key of ['sidebar','right','bottom'] as const)if(Number.isFinite(saved[key])&&saved[key]>=40&&saved[key]<=2000)defaults[key]=key==='sidebar'?Math.max(minimumSidebarWidth,saved[key]):saved[key];}catch{}return defaults;}

export function ChatPage({onActivity,onOpenAI,requestedConversationId,onRequestedConversationHandled}:{onActivity?:PageActivityChanged;onOpenAI?:()=>void;requestedConversationId?:string;onRequestedConversationHandled?:()=>void}){
 const {t}=useTranslation();
 const workspaceRoot=useRef<HTMLDivElement>(null);
 const [catalog,setCatalog]=useState<ChatCatalog>(emptyCatalog),[ai,setAI]=useState<AIConfiguration>(),[activeId,setActiveId]=useState("");
 const [view,setView]=useState<ChatState>(emptyChatState),[text,setText]=useState(""),[draftDirty,setDraftDirty]=useState(false),[saving,setSaving]=useState(false);
 const [submittingMessage,setSubmittingMessage]=useState<ChatMessage>();
 const [queuedSend,setQueuedSend]=useState<{id:string;text:string;attachments:ChatAttachment[]} | null>(null);
 const [busy,setBusy]=useState(false),[opening,setOpening]=useState(false),[draftLoading,setDraftLoading]=useState(false),[error,setError]=useState(""),[notice,setNotice]=useState("");
 const [modelsLoading,setModelsLoading]=useState(false),[modelsError,setModelsError]=useState('');
 const [titleJobs,setTitleJobs]=useState<Record<string,{pending:boolean;error?:string}>>({});
 const [activities,setActivities]=useState<ChatActivities>({}),[activityError,setActivityError]=useState('');
 const [detailsReview,setDetailsReview]=useState<{turnId:string;path?:string}>();
 const [detailsLine,setDetailsLine]=useState<number>();
 const [detailsURL,setDetailsURL]=useState<string>();
 const [detailsDirectory,setDetailsDirectory]=useState<string>();
 const titleRequests=useRef(new Set<string>());
 const titleEdits=useRef(new Map<string,number>());
 const [archived,setArchived]=useState(false),[sidebar,setSidebarVisible]=useState(true),[details,setDetailsVisible]=useState(false),[settingsOpen,setSettingsOpen]=useState(false);
 const [detailsMounted,setDetailsMounted]=useState(false);
 const [attachments,setAttachments]=useState<ChatAttachment[]>([]),[detailsTab,setDetailsTab]=useState<"home"|"files"|"resources"|"review">("home");
 const [detailsActivation,setDetailsActivation]=useState(0),[welcomeRevision,setWelcomeRevision]=useState(0),[historyLoading,setHistoryLoading]=useState(false);
 const historyAnchor=useRef<{height:number;top:number;nodes:Set<Element>}|undefined>(undefined),historyRequest=useRef<object|null>(null);
 const [filesDirty,setFilesDirty]=useState(false),[panelDock,setPanelDock]=useState<"right"|"bottom">("right");
 const filesDirtyRef=useRef(false),changeFilesDirty=useCallback((value:boolean)=>{filesDirtyRef.current=value;setFilesDirty(value);},[]);
 const sidebarPanel=usePanelRef(),detailsPanel=usePanelRef();
 const restoredPanels=useRef(readExpandedPanels()),sidebarGroup=useRef<HTMLDivElement>(null),workbenchGroup=useRef<HTMLDivElement>(null);
 const sidebarWidth=useRef(restoredPanels.current.sidebar),detailsSize=useRef({right:restoredPanels.current.right,bottom:restoredPanels.current.bottom}),dockTarget=useRef<number|undefined>(undefined);
 function rememberPanels(){try{localStorage.setItem(expandedPanelsKey,JSON.stringify({sidebar:sidebarWidth.current,...detailsSize.current}));}catch{}}
 // Left navigation stops at a readable width; only the explicit button hides
 // it. The right workbench retains its existing drag-to-collapse behavior.
 function setSidebar(open:boolean){const current=sidebarPanel.current?.getSize().inPixels;if(!open&&current&&current>=minimumSidebarWidth){sidebarWidth.current=current;rememberPanels();}setSidebarVisible(open);}
 useLayoutEffect(()=>{sidebarPanel.current?.resize(sidebar?Math.max(minimumSidebarWidth,sidebarWidth.current):0);},[sidebar]);
 function setDetails(open:boolean){const current=detailsPanel.current?.getSize().inPixels;if(!open&&current&&current>4){detailsSize.current[panelDock]=current;rememberPanels();}if(open)setDetailsMounted(true);detailsPanel.current?.resize(open?detailsSize.current[panelDock]:0);setDetailsVisible(open);}
 useLayoutEffect(()=>{if(dockTarget.current!==undefined){detailsPanel.current?.resize(dockTarget.current);dockTarget.current=undefined;}},[panelDock]);
 const [automationsOpen,setAutomationsOpen]=useState(false),[automationEditing,setAutomationEditing]=useState(false);
 const [moveTarget,setMoveTarget]=useState<Conversation>(),[exportTarget,setExportTarget]=useState<Conversation>(),[transferBusy,setTransferBusy]=useState(false),[reconnecting,setReconnecting]=useState(false);
 const [newProjectId,setNewProjectId]=useState(""),[location,setLocation]=useState(""),[saveLocation,setSaveLocation]=useState(""),[newModel,setNewModel]=useState<AISelection>(()=>readLastModel()??{provider:"",model:""}),[newPermission,setNewPermission]=useState<ChatPermission>("ask"),[projectPermission,setProjectPermission]=useState<ChatPermission>("ask");
 const [newThinking,setNewThinking]=useState(""),[newFastMode,setNewFastMode]=useState(false),[soundEnabled,setSoundEnabled]=useState(()=>readChatSoundEnabled());
 // Poll callbacks read the preference through a ref: a stale closure must not
 // replay or silence the completion chime after the toggle changes.
 const soundRef=useRef(soundEnabled);soundRef.current=soundEnabled;
 const [projectMode,setProjectMode]=useState<"open"|"create">("open");
 const [dialog,setDialog]=useState<"project"|"rename"|"delete"|"renameProject"|"removeProject"|"locations"|null>(null),[dialogConversation,setDialogConversation]=useState<Conversation>(),[dialogProject,setDialogProject]=useState<ChatProject>(),[name,setName]=useState(""),[parent,setParent]=useState(""),[editWorking,setEditWorking]=useState(""),[editSaved,setEditSaved]=useState("");
 const panelLayout=useDefaultLayout({id:"winone-chat-panels",panelIds:["sidebar","workbench"]});
 const modelsRevision=useRef(0);
 const [refreshToken,setRefreshToken]=useState(0),[catalogLoaded,setCatalogLoaded]=useState(false);
 const requestedNavigation=useRef({id:requestedConversationId,handled:onRequestedConversationHandled}),handledNavigation=useRef<string|undefined>(undefined);
 requestedNavigation.current={id:requestedConversationId,handled:onRequestedConversationHandled};
 const inFlight=useRef(false),draftWrites=useRef(Promise.resolve()),draftRevision=useRef(0),openDraftRevision=useRef(0),pendingInput=useRef<{id:string;text:string;attachments?:string[];deliveryMode?:ChatDeliveryMode}|undefined>(undefined),scroll=useRef<HTMLDivElement>(null),follow=useRef(true),resizeDetached=useRef(false),newDraft=useRef(""),newAttachments=useRef<ChatAttachment[]>([]);
 const awaitingInput=useRef<string|undefined>(undefined),retryReceipt=useRef<{key:string;id:string}|undefined>(undefined);
 const [sendCleared,setSendCleared]=useState(false),[stopping,setStopping]=useState(false);
 const stopInFlight=useRef(false),sendCancelGeneration=useRef(0);
 const pendingFiles=useRef(new Map<string,File>());
 function releasePendingFile(path:string){if(pendingFiles.current.delete(path)){URL.revokeObjectURL(path);releaseTransientMedia(path);}}
 useEffect(()=>()=>{for(const url of pendingFiles.current.keys())releasePendingFile(url);},[]);
 const latestComposer=useRef({text,attachments}),lastScrollTop=useRef(0);latestComposer.current={text,attachments};
 const selection=useRef(""),selectionRevision=useRef(0),openingRequests=useRef(new Map<string,Promise<ChatSnapshot>>()),lastPrompt=useRef("");
 const silentReload=useRef(""),configuring=useRef("");
 const sidebarActivities:ChatActivities={...activities,...(activeId&&!opening?{[activeId]:{phase:view.status==='stopped'?'interrupted':view.status==='failed'?'error':view.approvals.length?'needs-interaction':view.running||view.backgroundRunning?'running':view.status==='completed'?'completed':'idle',attention:view.status==='failed',paused:view.paused}}:{})};
 const conversation=catalog.conversations.find(value=>value.id===activeId);
 const visibleMessages=submittingMessage&&!view.messages.some(message=>message.requestId===submittingMessage.requestId||message.id===submittingMessage.id)?[...view.messages,submittingMessage]:view.messages;
 const [detailsFile,setDetailsFile]=useState<string>();
 const processItems=projectChatProcess(visibleMessages,view.tools,view.running,view.operations);
 const activeRetry=view.running?[...(view.operations??[])].reverse().find(operation=>operation.kind==='retry'&&operation.status==='running'):undefined;
 const changedTurns=processItems.filter(item=>item.kind==='response'&&item.entries.some(entry=>entry.kind==='tool'&&!entry.tool.running&&!entry.tool.isError&&(/^(write|edit|apply_patch)$/.test(entry.tool.name)||!!entry.tool.files?.length)));
 // Generated file chips describe the conversation's current outputs. Keep
 // them at the transcript end with turn changes, rather than on an old reply.
 const generatedFiles=[...new Set(processItems.flatMap(item=>item.kind==='response'?item.entries.flatMap(entry=>entry.kind==='tool'&&!entry.tool.running&&!entry.tool.isError?(entry.tool.files?.map(file=>file.path)??(/^(write|edit)$/.test(entry.tool.name)&&typeof entry.tool.args.path==='string'?[entry.tool.args.path]:[])):[]):[]))];
 const latestReviewTurn=[...processItems].reverse().find(item=>item.kind==='response');
 const currentPlan=latestReviewTurn?.kind==='response'?planForResponse(latestReviewTurn):undefined;
 const currentPlanAnchor=currentPlan&&`${activeId}:${latestReviewTurn!.key}:${currentPlan.anchor}`;
 const hasInlinePlan=processItems.some(item=>item.kind==='response'&&planForResponse(item));
 const latestReviewPaths=latestReviewTurn?.kind==='response'?latestReviewTurn.entries.flatMap(entry=>entry.kind==='tool'&&!entry.tool.running&&!entry.tool.isError?(entry.tool.files?.map(file=>file.path)??(/^(write|edit)$/.test(entry.tool.name)&&typeof entry.tool.args.path==='string'?[entry.tool.args.path]:[])):[]):[];
 function openFile(path:string,line?:number){setDetailsFile(path);setDetailsLine(line);setDetailsTab('home');setDetailsActivation(value=>value+1);setDetails(true);}
 function openTurnReview(turnId:string,path?:string){setDetailsFile(undefined);setDetailsReview({turnId,path});setDetailsTab('review');setDetailsActivation(value=>value+1);setDetails(true);}
 // Refresh saved AI services on each settings opening without discarding its draft.
 useEffect(()=>{if(!settingsOpen)return;let alive=true;const revision=++modelsRevision.current;setModelsLoading(true);setModelsError('');void captureCall<AIConfiguration>('ai.configuration.get').then(models=>{if(alive&&revision===modelsRevision.current)setAI(models);}).catch(error=>{if(alive)setModelsError(errorText(error));}).finally(()=>{if(alive)setModelsLoading(false);});return()=>{alive=false;};},[settingsOpen]);
 usePageActivity(onActivity,draftDirty||automationEditing||filesDirty,busy||saving||opening||transferBusy||reconnecting,"chat");
 async function refresh(){const revision=++modelsRevision.current;const [value,models]=await Promise.all([chatCall<ChatCatalog>("catalog"),captureCall<AIConfiguration>("ai.configuration.get")]);setCatalog(value);setCatalogLoaded(true);if(revision===modelsRevision.current){setAI(models);setNewModel(current=>initialModel(models,current));}}
 useEffect(()=>{let alive=true;const revision=++modelsRevision.current;void Promise.all([chatCall<ChatCatalog>("catalog"),captureCall<AIConfiguration>("ai.configuration.get")]).then(([value,models])=>{if(alive){setCatalog(value);setCatalogLoaded(true);if(revision===modelsRevision.current){setAI(models);setNewModel(initialModel(models));}}}).catch(error=>{if(alive)setError(errorText(error));});return()=>{alive=false;};},[]);
 // One lightweight projection for the entire sidebar, including background
 // sessions. No per-row workers/history reads, and no overlapping polls.
 useEffect(()=>{
  if(!sidebar)return;
  let alive=true,timer:ReturnType<typeof setTimeout>|undefined;
  const poll=async()=>{try{if(!document.hidden){const value=await chatCall<ChatActivities>('activity');if(alive){setActivities(current=>JSON.stringify(current)===JSON.stringify(value??{})?current:value??{});setActivityError('');}}}catch(error){if(alive)setActivityError(errorText(error));}finally{if(alive)timer=setTimeout(()=>void poll(),1500);}};
  void poll();return()=>{alive=false;if(timer)clearTimeout(timer);};
 },[sidebar]);
 const run=async(action:()=>Promise<void>)=>{
  if(inFlight.current)return;inFlight.current=true;setBusy(true);setError("");setNotice("");
  try{await action();}catch(error){setError(errorText(error));}finally{inFlight.current=false;setBusy(false);}
 };
 useEffect(()=>{setMediaDraftSnapshot(activeId,text,attachments);},[activeId,text,attachments]);
 useEffect(()=>{
  const append=(event:Event)=>{const detail=(event as CustomEvent<MediaAttachDetail>).detail;if(detail?.id!==activeId)return;detail.handled=true;if(busy||opening||draftLoading){detail.reject(Error(t('chat.browserDraftBusy')));return;}const next=detail.replace?attachments.flatMap(file=>file.path===detail.replace?detail.files:[file]):[...attachments,...detail.files.filter(file=>!attachments.some(old=>old.path===file.path))];updateText(text,next);void draftWrites.current.then(detail.resolve,detail.reject);};
  window.addEventListener(MEDIA_ATTACH_EVENT,append);return()=>window.removeEventListener(MEDIA_ATTACH_EVENT,append);
 },[activeId,text,attachments,busy,opening,draftLoading]);
 function updateText(value:string,nextAttachments=attachments,id=activeId){
  latestComposer.current={text:value,attachments:nextAttachments};setText(value);setAttachments(nextAttachments);if(!id){newDraft.current=value;newAttachments.current=nextAttachments;setDraftDirty(value.length>0||nextAttachments.length>0);return;}
  const revision=++draftRevision.current;setDraftDirty(true);
  // Failed first-send uploads remain a protected local draft. Never persist a
  // blob URL as if it were an imported Host file; resending retries the bytes.
  if(nextAttachments.some(file=>pendingFiles.current.has(file.path))){setSaving(false);return;}
  setSaving(true);
  // Serialize writes across navigation. A slow earlier save cannot overwrite
  // a newer draft or clear its unsaved marker after the user has kept typing.
  const write=draftWrites.current.catch(()=>{}).then(()=>chatCall("draft.save",{id,draft:{text:value,attachments:nextAttachments}})).then(()=>{
   if(revision===draftRevision.current){setDraftDirty(false);setSaving(false);}
  },error=>{if(revision===draftRevision.current){setSaving(false);setError(t("chat.saveFailed")+": "+errorText(error));}throw error;});
  draftWrites.current=write;void write.catch(()=>{});
 }
 async function choose(id:string,stillRequested=()=>true){
  // A repeated selection is not a navigation: clearing its transcript would not
  // rerun the id-keyed effect. A revision also prevents a delayed save changing
  // the destination after the user has selected a newer conversation.
  if(id===selection.current&&id)return;
  if(selection.current&&latestComposer.current.attachments.some(file=>pendingFiles.current.has(file.path)))throw Error(t('chat.attachmentPending'));
  if(filesDirtyRef.current)throw new Error(t("chat.saveFileBeforeNavigate"));
  const revision=++selectionRevision.current;await draftWrites.current;if(revision!==selectionRevision.current||!stillRequested())return;
  // An editor can become dirty while an earlier composer write is pending.
  // Recheck the live value before unmounting the old workbench.
  if(filesDirtyRef.current)throw new Error(t("chat.saveFileBeforeNavigate"));
  setDetails(false);setDetailsMounted(false);
  historyRequest.current=null;historyAnchor.current=undefined;setHistoryLoading(false);
  // A new selection owns its own runtime state. In particular, leaving for an
  // empty chat has no open callback that could clear the previous reconnect.
  configuring.current="";silentReload.current="";setReconnecting(false);
  selection.current=id;openDraftRevision.current=++draftRevision.current;pendingInput.current=undefined;awaitingInput.current=undefined;retryReceipt.current=undefined;setQueuedSend(null);setDetailsURL(undefined);setDetailsFile(undefined);setDetailsDirectory(undefined);setDetailsReview(undefined);setDetailsLine(undefined);setDetailsTab('home');setDetailsActivation(0);setError("");setActiveId(id);setOpening(!!id);setDraftLoading(!!id);setDraftDirty(!id&&(newDraft.current.length>0||newAttachments.current.length>0));setText(id?"":newDraft.current);setAttachments(id?[]:newAttachments.current);setView(emptyChatState());setNotice("");
 }
 useEffect(()=>{
  if(!requestedConversationId){handledNavigation.current=undefined;return;}
  if(!catalogLoaded||busy||handledNavigation.current===requestedConversationId)return;
  const id=requestedConversationId;handledNavigation.current=id;
  // Cross-page navigation uses the same draft barrier as a sidebar selection.
  // A newer request invalidates an older request that is still waiting to save.
  void run(async()=>{
   try{
    const target=catalog.conversations.find(value=>value.id===id);
    if(!target)throw new Error(t("chat.requestedConversationMissing"));
    await choose(id,()=>requestedNavigation.current.id===id);
    if(requestedNavigation.current.id!==id)return;
    setArchived(!!target.archived);setAutomationsOpen(false);setSettingsOpen(false);setDialog(null);
   }finally{if(requestedNavigation.current.id===id)requestedNavigation.current.handled?.();}
  });
 },[requestedConversationId,catalogLoaded,catalog,busy]);
 function openConversation(id:string){
  let request=openingRequests.current.get(id);
  if(!request){request=chatCall<ChatSnapshot>("open",{id});openingRequests.current.set(id,request);void request.finally(()=>openingRequests.current.delete(id)).catch(()=>{});}
  return request;
 }
 useEffect(()=>{
  if(!activeId)return;let disposed=false,timer:ReturnType<typeof setTimeout>|undefined,cursor=0;const loadRevision=openDraftRevision.current;
  // Cindy keeps a conversation mounted when its settings change. Reconnect the
  // Pi event cursor independently, without replacing the transcript or draft.
  const silent=silentReload.current===activeId;silentReload.current="";
  if(!silent){setOpening(true);setDraftLoading(true);}
  const poll=async()=>{
   if(configuring.current===activeId){timer=setTimeout(()=>void poll(),160);return;}
   try{
    const page=await chatCall<ChatPoll>("poll",{id:activeId,cursor});if(disposed)return;
    const waiting=awaitingInput.current;
    if(waiting&&page.events.some(({data})=>data.type==='input_saved'&&data.id===waiting||data.type==='settled'&&data.requestId===waiting||data.state?.messages.some(message=>message.requestId===waiting)))awaitingInput.current=undefined;
    if(page.status==='stopped'||page.status==='paused')awaitingInput.current=undefined;
    if(page.reset){const state=await chatCall<ChatSnapshot>("state",{id:activeId});if(disposed)return;if(awaitingInput.current&&state.messages.some(message=>message.requestId===awaitingInput.current))awaitingInput.current=undefined;setView({...emptyChatState(),...state,running:page.running||!!awaitingInput.current,status:page.status,approvals:page.approvals});}
    else{
     setView(current=>{let next=current;for(const event of page.events)next=applyChatEvent(next,event.data);return {...next,running:page.running||!!awaitingInput.current,status:page.status,approvals:page.approvals};});
     // Compaction feedback and the completion chime are page effects: they
     // belong to the transcript once, not to the pure event reducer.
     for(const event of page.events){
      if(event.data.type==="compacted"&&typeof event.data.before==="number"&&typeof event.data.after==="number")setNotice(t("chat.compacted",{before:String(event.data.before),after:String(event.data.after)}));
      if(event.data.type==="settled"&&event.data.status==="completed"&&soundRef.current)playChatDone();
     }
    }
    cursor=page.cursor;if(page.error)setError(page.error);
    timer=setTimeout(()=>void poll(),page.more?0:page.running?160:1200);
   }catch(error){if(!disposed){if(configuring.current!==activeId)setError(errorText(error));timer=setTimeout(()=>void poll(),2000);}}
  };
  // Draft I/O is independent of session startup. Publish each result as soon
  // as it arrives; a slow or failed draft must not hide a valid transcript.
  if(!silent)void chatCall<{text:string;pendingId?:string;pendingDelivery?:ChatDeliveryMode;attachments?:ChatAttachment[]}>("draft.get",{id:activeId}).then(draft=>{
   if(disposed)return;
   if(loadRevision===draftRevision.current){if(draft.pendingId)pendingInput.current={id:draft.pendingId,text:draft.text,attachments:draft.attachments?.map(file=>file.path),deliveryMode:draft.pendingDelivery??'queue'};setText(draft.text);setAttachments(draft.attachments??[]);setDraftDirty(false);}
  }).catch(error=>{if(!disposed)setError(t("chat.draftLoadFailed")+": "+errorText(error));}).finally(()=>{if(!disposed)setDraftLoading(false);});
  void openConversation(activeId).then(state=>{
   if(disposed)return;
   if(silent)setView(current=>({...current,modelError:state.modelError}));
   else{setView({...emptyChatState(),...state,running:!!state.streaming});setOpening(false);resizeDetached.current=false;follow.current=true;}
   configuring.current="";setReconnecting(false);void poll();
  }).catch(error=>{if(!disposed){configuring.current="";setReconnecting(false);setOpening(false);setError(errorText(error));}});
  return()=>{disposed=true;if(timer)clearTimeout(timer);};
 },[activeId,refreshToken]);
 useEffect(()=>()=>{if(activeId)void chatCall("release",{id:activeId}).catch(error=>console.error("Chat release failed",error));},[activeId]);
 // Dragging or keyboard-resizing the composer is an explicit reading intent.
 // Suspend streaming follow until the user returns to the bottom, rather than
 // snapping the current transcript to the newly moved input edge on each token.
 // Idle polls return a fresh approval array; only actual content changes may
 // advance the transcript. Otherwise polling can interrupt a resize or read.
 const approvalKeys=view.approvals.map(value=>value.id).join('\n');
 function followLatest(){resizeDetached.current=false;follow.current=true;const viewport=scroll.current;if(viewport){scrollChatToBottom(viewport);lastScrollTop.current=viewport.scrollTop;}}
 useLayoutEffect(()=>{if(follow.current)followLatest();},[view.messages,view.tools,approvalKeys]);
 // A deliberate history request navigates to the newly loaded earlier page.
 // Disable live follow before inserting it so an arriving poll cannot undo it.
 useLayoutEffect(()=>{const anchor=historyAnchor.current,viewport=scroll.current;if(!anchor||!viewport)return;viewport.scrollTop=0;
  if(!matchMedia('(prefers-reduced-motion: reduce)').matches)viewport.querySelectorAll('.desktop-conversation-message').forEach(node=>{if(!anchor.nodes.has(node))node.animate?.([{opacity:.45},{opacity:1}],{duration:200,easing:'ease-out'});});
  historyAnchor.current=undefined;},[view.messages]);
 async function loadEarlier(){
  if(historyRequest.current||!activeId||opening||!view.hasOlder||!view.before)return;
  const id=activeId,revision=selectionRevision.current,request={};historyRequest.current=request;setHistoryLoading(true);
  try{
   const page=await chatCall<ChatSnapshot>('state',{id,before:view.before});
   if(selection.current!==id||selectionRevision.current!==revision||historyRequest.current!==request)return;
   const viewport=scroll.current;if(viewport)historyAnchor.current={height:viewport.scrollHeight,top:viewport.scrollTop,nodes:new Set(Array.from(viewport.querySelectorAll('.desktop-conversation-message')))};
   follow.current=false;
   setView(current=>{const ids=new Set(current.messages.map(message=>message.id).filter(Boolean));return {...current,hasOlder:page.hasOlder,before:page.before,messages:[...page.messages.filter(message=>!message.id||!ids.has(message.id)),...current.messages],operations:mergeChatOperations(page.operations,current.operations)};});
  }catch(error){if(selection.current===id&&selectionRevision.current===revision&&historyRequest.current===request)setError(errorText(error));}
  finally{if(historyRequest.current===request){historyRequest.current=null;setHistoryLoading(false);}}
 }
 async function startConversation(projectId=newProjectId){
  let created=await chatCall<Conversation>("conversation.new",{projectId,directory:projectId?"":location,saveDirectory:saveLocation,model:newModel,title:"Chat",...(newThinking?{thinkingLevel:newThinking}:{}),...(newFastMode?{fastMode:true}:{})});
  if(!projectId&&newPermission!=="ask")created=await chatCall<Conversation>("configure",{id:created.id,permission:newPermission});
  setCatalog(current=>({...current,conversations:[...current.conversations,created]}));await choose(created.id);return created;
 }
 async function send(override?:{text:string;attachments:ChatAttachment[]},deliveryMode:ChatDeliveryMode='queue'){
  const originalContent=override?override.text:text;let files=override?override.attachments:attachments;const originalFiles=files;const content=browserAnnotatedPrompt(originalContent,files);if(view.paused)return;if(!content.trim()&&!files.length)return;
  const queued=view.running&&deliveryMode==='queue';
  unlockChatAudio();
  const receipt=pendingInput.current?.text===content&&(pendingInput.current.deliveryMode??'queue')===deliveryMode&&JSON.stringify(pendingInput.current.attachments??[])===JSON.stringify(files.map(file=>file.path))?pendingInput.current:{id:crypto.randomUUID(),text:content,attachments:files.map(file=>file.path),deliveryMode};
  const sendSnapshot=captureComposerSendSnapshot(originalContent,files,[]),previousWrites=draftWrites.current,cancelGeneration=sendCancelGeneration.current;
  let target=conversation,releaseDraftWrites=()=>{},cleared=false;
  // Cindy ChatInput.dispatchSend freezes this input, clears it before dispatch,
  // then allows typing while keeping send/settings locked until acknowledgement.
  // WinOne serializes newer draft writes behind the receipt so Host.SaveDraft
  // cannot overwrite something typed while its preflight is still running.
  function clearSubmittedDraft(){
   const current=latestComposer.current;
   if(isComposerSendSnapshotCurrent(sendSnapshot,current.text,current.attachments,[])){
    draftRevision.current++;latestComposer.current={text:'',attachments:[]};setText('');setAttachments([]);setDraftDirty(false);cleared=true;
   }
   const receiptBarrier=new Promise<void>(resolve=>{releaseDraftWrites=resolve;});
   draftWrites.current=previousWrites.catch(()=>{}).then(()=>receiptBarrier);setSendCleared(true);
  }
  if(target)clearSubmittedDraft();
  setSubmittingMessage({id:receipt.id,requestId:receipt.id,turnId:receipt.id,pending:true,queued,deliveryMode,role:'user',content:originalContent,attachments:files,timestamp:Date.now()});followLatest();
  try{
   await previousWrites;
   if(!target){
    target=await startConversation();
    if(files.length)files=await importChatAttachments(target.id,files,pendingFiles.current);
    await openConversation(target.id);newDraft.current='';newAttachments.current=[];clearSubmittedDraft();
   }
   // Retry an interrupted first-send import before handing any path to Pi.
   for(let index=0;index<files.length;index++){
    const pending=pendingFiles.current.get(files[index].path);
    if(pending){files=[...files];files[index]=await uploadChatAttachment(target.id,pending);}
   }
   if(cancelGeneration!==sendCancelGeneration.current)throw Error(t('chat.sendCancelled'));
   const paths=files.map(file=>file.path);receipt.attachments=paths;pendingInput.current=receipt;lastPrompt.current=content;awaitingInput.current=receipt.id;
   const result=await chatCall<{accepted?:boolean;error?:string}>('prompt',{id:target.id,requestId:receipt.id,text:content,deliveryMode,...(paths.length?{attachments:paths}:{})});
   if(result?.accepted===false)throw Error(result.error||t('chat.sendNotAccepted'));
   if(pendingInput.current?.id===receipt.id)pendingInput.current=undefined;
   // A late acknowledgement never clears newer content or re-enables following
   // after the reader deliberately scrolled upward (Cindy autoFollowIntent).
   setView(current=>{const optimistic:ChatMessage={id:receipt.id,requestId:receipt.id,turnId:receipt.id,pending:true,queued,deliveryMode,role:'user',content:originalContent,attachments:files,timestamp:Date.now()};return {...current,messages:current.messages.some(message=>message.id===receipt.id||message.requestId===receipt.id)?current.messages:upsertChatMessage(current.messages,optimistic),running:!!awaitingInput.current||current.running,status:awaitingInput.current?'running':current.status};});
   for(const file of originalFiles)releasePendingFile(file.path);
   if(target.title==='Chat')void generateTitle(target,content);
  }catch(error){
   if(awaitingInput.current===receipt.id)awaitingInput.current=undefined;
   if(selection.current===(target?.id??'')){
    const current=latestComposer.current,unchanged=!cleared&&isComposerSendSnapshotCurrent(sendSnapshot,current.text,current.attachments,[]);
    const restored=unchanged?current.text:[originalContent,current.text].filter(Boolean).join('\n\n');
    updateText(restored,[...new Map([...files,...current.attachments].map(file=>[file.path,file])).values()],target?.id??'');
   }
   throw error;
  }finally{releaseDraftWrites();setSubmittingMessage(undefined);setSendCleared(false);}
 }
 function requestSend(deliveryMode?:ChatDeliveryMode){if(inFlight.current||view.paused||!text.trim()&&!attachments.length)return;if(view.running&&activeId&&!deliveryMode)setQueuedSend({id:activeId,text,attachments:[...attachments]});else void run(()=>send(undefined,deliveryMode));}
 function confirmSend(deliveryMode:ChatDeliveryMode){const intent=queuedSend;setQueuedSend(null);if(intent&&intent.id===activeId)void run(()=>send({text:intent.text,attachments:intent.attachments},deliveryMode));}
 async function addBrowserComment(annotation:ChatBrowserAnnotation){
  if(busy||opening||draftLoading)throw new Error(t("chat.browserDraftBusy"));
  const imported=activeId?(await chatCall<{attachments:ChatAttachment[]}>("attachments.add",{id:activeId,paths:[annotation.screenshotPath]})).attachments[0]:{name:`#${annotation.markerNumber}.png`,path:annotation.screenshotPath,size:0,mimeType:"image/png"};
  updateText(text,[...attachments,{...imported,annotation}]);
 }
 async function addAttachments(chosen?:string[]){
  const id=activeId,paths=chosen??await pickChatFiles();if(!paths.length)return;
  const entries=await inspectDroppedPaths(paths),directories=entries.filter(entry=>entry.kind==='directory'),filePaths=entries.filter(entry=>entry.kind==='file').map(entry=>entry.path);
  if(latestComposer.current.attachments.length+filePaths.length>16)throw Error(t('chat.attachmentCountLimit'));
  const added=filePaths.length?(id?(await chatCall<{attachments:ChatAttachment[]}>("attachments.add",{id,paths:filePaths})).attachments:filePaths.map(path=>({name:path.split(/[\\/]/).pop()??path,path,size:0,mimeType:""}))):[];
  if(selection.current!==id)throw Error(t('chat.browserDraftBusy'));
  const current=latestComposer.current,reference=directories.map(entry=>'@'+entry.path).join('\n');
  updateText([current.text,reference].filter(Boolean).join('\n'),[...new Map([...current.attachments,...added].map(file=>[file.path,file])).values()]);
 }
 async function addBrowserFiles(files:File[],clipboard=false){
  const id=activeId;
  // Explorer files/folders use native paths; screenshots use captured bytes.
  if(clipboard&&window.go){const paths=await readClipboardFiles();if(paths.length){await addAttachments(paths);return;}}
  if(!files.length)return;
  if(latestComposer.current.attachments.length+files.length>16)throw Error(t('chat.attachmentCountLimit'));
  for(const file of files)if(file.size>(file.type.startsWith('image/')?100:20)*1024*1024)throw Error(t('chat.attachmentSizeLimit'));
  for(const file of files){
   let added:ChatAttachment;
   if(id)added=await uploadChatAttachment(id,file);
   else{const path=URL.createObjectURL(file);pendingFiles.current.set(path,file);added={path,name:file.name||'image.png',size:file.size,mimeType:file.type};registerMedia({id:'',url:path,name:added.name,mimeType:file.type,size:file.size,local:false,attachment:added});}
   if(selection.current!==id)throw Error(t('chat.browserDraftBusy'));
   const current=latestComposer.current;updateText(current.text,[...current.attachments,added]);
  }
 }
 useEffect(()=>{
  void setFileDropTarget(true).catch(error=>setError(String(error)));
  return()=>{void setFileDropTarget(false).catch(console.error);};
 },[]);
 useEffect(()=>{
  const drop=(event:Event)=>{const paths=(event as CustomEvent<{paths:string[]}>).detail?.paths;if(paths?.length&&!busy&&!opening&&!draftLoading&&!view.paused)void run(()=>addAttachments(paths));};
  window.addEventListener('winone:chat-files-drop',drop);
  return()=>{window.removeEventListener('winone:chat-files-drop',drop);};
 },[activeId,busy,opening,draftLoading,view.paused,text,attachments]);
 async function configure(value:{model?:AISelection;permission?:ChatPermission}){
  if(value.model)writeLastModel(value.model);
  const projectId=conversation?.projectId??newProjectId;
  if(value.permission&&projectId){await draftWrites.current;const updated=await chatCall<ChatCatalog>("project.permission",{id:projectId,permission:value.permission});setCatalog(updated);setNewPermission(value.permission);if(conversation){silentReload.current=conversation.id;setRefreshToken(value=>value+1);}return;}
  if(!conversation){if(value.model)setNewModel(value.model);if(value.permission)setNewPermission(value.permission);return;}
  await draftWrites.current;configuring.current=conversation.id;setReconnecting(true);
  try{
   const updated=await chatCall<Conversation>("configure",{id:conversation.id,...value});
   setCatalog(current=>({...current,conversations:current.conversations.map(c=>c.id===updated.id?updated:c)}));
   silentReload.current=conversation.id;setRefreshToken(value=>value+1);
  }catch(error){configuring.current="";setReconnecting(false);throw error;}
 }
 async function setThinkingLevel(value:string){
  if(!conversation){setNewThinking(value);return;}
  const updated=await chatCall<Conversation>("thinking",{id:conversation.id,thinkingLevel:value});
  setCatalog(current=>({...current,conversations:current.conversations.map(c=>c.id===updated.id?updated:c)}));
 }
 async function setFastMode(value:boolean){
  if(!conversation){setNewFastMode(value);return;}
  const updated=await chatCall<Conversation>("fast",{id:conversation.id,fastMode:value});
  setCatalog(current=>({...current,conversations:current.conversations.map(c=>c.id===updated.id?updated:c)}));
 }
 async function updateConversation(value:Partial<Conversation>,target=conversation){
  if(!target)return;const updated=await chatCall<Conversation>("conversation.update",{id:target.id,...value});
  if(value.title!==undefined)titleEdits.current.set(target.id,(titleEdits.current.get(target.id)??0)+1);
  setCatalog(current=>({...current,conversations:current.conversations.map(c=>c.id===updated.id?updated:c)}));
  if(value.archived&&target.id===activeId)await choose("");
 }
 // Title requests are independent background work. Never lock navigation,
 // draft editing or unrelated row actions for the duration of an AI call.
 async function generateTitle(target:Conversation,initialText?:string){
  if(titleRequests.current.has(target.id))return;
  const editRevision=titleEdits.current.get(target.id)??0;
  titleRequests.current.add(target.id);setTitleJobs(current=>({...current,[target.id]:{pending:true}}));
  try{
   // The Host reads the saved branch for Cindy's regeneration prompt. A closed
   // conversation must not depend on the live runtime's state command.
   const method=initialText===undefined?'title.regenerate':'title.generate';
   const params={id:target.id,...(initialText===undefined?{}:{text:initialText})};
   // The Host owns bounded provider retries. A disconnected RPC may already
   // have saved its title, so repeating it here can race a manual rename.
   const updated=await chatCall<Conversation>(method,params);
   // Do not overwrite other row fields with a snapshot from a slow title call.
   // The backend may commit an AI title before its response arrives. A manual
   // save after that commit still wins locally, including an identical name.
   setCatalog(current=>({...current,conversations:current.conversations.map(value=>value.id===updated.id&&(titleEdits.current.get(target.id)??0)===editRevision&&value.title===target.title?{...value,title:updated.title}:value)}));
   setTitleJobs(current=>({...current,[target.id]:{pending:false}}));
  }catch(error){
   // The Host persists a conversation-text fallback even when AI fails. Read
   // that saved result once; never repeat the failed generation request.
   try{const saved=await chatCall<ChatCatalog>('catalog');const updated=saved.conversations.find(value=>value.id===target.id);if(updated)setCatalog(current=>({...current,conversations:current.conversations.map(value=>value.id===target.id&&(titleEdits.current.get(target.id)??0)===editRevision&&value.title===target.title?{...value,title:updated.title}:value)}));}
   catch(refreshError){setError(errorText(refreshError));}
   setTitleJobs(current=>({...current,[target.id]:{pending:false,...((titleEdits.current.get(target.id)??0)===editRevision?{error:errorText(error)}:{})}}));
  }
  finally{titleRequests.current.delete(target.id);}
 }
 function conversationAction(target:Conversation,action:ConversationAction){
  if(action==="rename"||action==="delete"){setDialogConversation(target);setName(target.title);setDialog(action);}
  else if(action==="aiRename")void generateTitle(target);
  else if(action==="move"){setError("");setMoveTarget(target);}
  else if(action==="export")setExportTarget(target);
  else if(action==="pin")void run(()=>updateConversation({pinned:!target.pinned},target));
  else if(action==="archive")void run(()=>updateConversation({archived:!target.archived},target));
  else copy(target.directory);
 }
 async function moveConversation(projectId:string){
  if(!moveTarget)return;await draftWrites.current;
  const targetId=moveTarget.id,reconnect=targetId===activeId;
  // The Host retires this runtime before it commits the project change. Mark
  // that interval now so an in-flight poll cannot report a false open error.
  if(reconnect){configuring.current=targetId;setReconnecting(true);}
  try{
   const updated=await chatCall<Conversation>("conversation.move",{id:targetId,projectId});
   setCatalog(current=>({...current,conversations:current.conversations.map(value=>value.id===updated.id?updated:value)}));
   if(reconnect){silentReload.current=updated.id;setRefreshToken(value=>value+1);}
   setNotice(t("chat.moveDone",{name:updated.title,project:catalog.projects.find(project=>project.id===projectId)?.name??t("chat.noProject")}));setMoveTarget(undefined);
  }catch(error){if(reconnect){configuring.current="";silentReload.current="";setReconnecting(false);}throw error;}
 }
 async function moveCurrentConversation(projectId:string){if(!conversation||conversation.projectId===projectId)return;await draftWrites.current;configuring.current=conversation.id;setReconnecting(true);try{const updated=await chatCall<Conversation>("conversation.move",{id:conversation.id,projectId});setCatalog(current=>({...current,conversations:current.conversations.map(value=>value.id===updated.id?updated:value)}));silentReload.current=conversation.id;setRefreshToken(value=>value+1);setNotice(t("chat.moveDone",{name:updated.title,project:catalog.projects.find(project=>project.id===projectId)?.name??t("chat.noProject")}));}catch(error){configuring.current="";silentReload.current="";setReconnecting(false);throw error;}}
 async function fork(entry?:string){if(!activeId)return;const created=await chatCall<Conversation>("fork",{id:activeId,entry});await refresh();await choose(created.id);}
 async function dispatchRetry(entry:string,text?:string,restoreRevision?:string){
  const key=JSON.stringify({id:activeId,entry,text,restoreRevision});
  const receipt=retryReceipt.current?.key===key?retryReceipt.current:{key,id:crypto.randomUUID()};retryReceipt.current=receipt;
  awaitingInput.current=receipt.id;followLatest();setView(current=>({...current,running:true,status:'running'}));
  try{
   await chatCall(text===undefined?'retry':'edit',{id:activeId,entry,requestId:receipt.id,...(text===undefined?{}:{text,restoreRevision})});
   if(retryReceipt.current===receipt)retryReceipt.current=undefined;
  }catch(error){if(awaitingInput.current===receipt.id)awaitingInput.current=undefined;throw error;}
 }
 async function editMessage(entry:string,text:string,restoreRevision?:string){if(inFlight.current)throw Error(t('chat.browserDraftBusy'));const id=activeId;inFlight.current=true;setBusy(true);try{await draftWrites.current;if(view.running)await chatCall('stop',{id});await dispatchRetry(entry,text,restoreRevision);}finally{inFlight.current=false;setBusy(false);}}
 // The Worker emits the replacement branch before starting inference. Do not
 // fetch a second snapshot here: it can arrive behind streaming deltas and
 // overwrite the new reply. The ordered poll stream is the sole state owner.
 async function retryMessage(entry?:string){const user=[...view.messages].reverse().find(message=>message.role==="user"&&!message.pending);const target=entry??user?.id;if(!activeId||!target||view.running||view.paused)return;await dispatchRetry(target);}
 function editLocations(){setEditWorking(conversation?.directory??catalog.projects.find(p=>p.id===newProjectId)?.path??location);setEditSaved(conversation?.sessionDirectory??saveLocation);setDialog("locations");}
 function openProjectDialog(){setName("");setParent("");setProjectPermission("ask");setProjectMode("open");setError("");setDialog("project");}
 async function chooseProjectDirectory(chosen?:string){const path=chosen??await pickDirectory();if(path){setParent(path);if(projectMode==="open")setName(path.replace(/[\\/]+$/,"").split(/[\\/]/).pop()??"");}}
 async function saveProject(){
  const project=await chatCall<ChatProject>(projectMode==="open"?"project.add":"project.create",projectMode==="open"?{path:parent,name:name.trim(),permission:projectPermission}:{parent,name:name.trim(),permission:projectPermission});
  await refresh();await choose("");setNewProjectId(project.id);setNewPermission(projectPermission);setAutomationsOpen(false);
 }
 async function saveLocations(){
  if(conversation&&editSaved!==conversation.sessionDirectory){const updated=await chatCall<Conversation>("location",{id:conversation.id,saveDirectory:editSaved});setCatalog(current=>({...current,conversations:current.conversations.map(c=>c.id===updated.id?updated:c)}));setRefreshToken(value=>value+1);setNotice(t("chat.locationChanged"));}
  else if(!conversation){setLocation(editWorking);setSaveLocation(editSaved);}
  setDialog(null);
 }
 const copy=(value:string)=>void copyText(value).catch(error=>setError(errorText(error)));
 function toggleSound(){const next=!soundEnabled;setSoundEnabled(next);writeChatSoundEnabled(next);if(next)unlockChatAudio();}
 // Cindy's Stop is independent of send's dispatch lock. Cancel any unfinished
 // local preflight, then stop the source session without clearing its new draft.
 async function stopCurrent(){
  if(stopInFlight.current||!activeId)return;stopInFlight.current=true;sendCancelGeneration.current++;setStopping(true);
  try{await chatCall('stop',{id:activeId});if(selection.current===activeId)awaitingInput.current=undefined;}
  catch(error){if(selection.current===activeId)setError(errorText(error));}
  finally{stopInFlight.current=false;setStopping(false);}
 }
 // Keep the composer draft untouched while an agent waits for answers.
 function respondQuestion(answers?:string[]){
  const request=view.approvals[0];if(request?.kind!=="question")return;
  void run(async()=>{await chatCall("question",{id:activeId,approvalId:request.id,...(answers?{answers}:{cancel:true})});setView(current=>({...current,approvals:current.approvals.filter(value=>value.id!==request.id)}));});
 }
 const queuedCount=view.messages.filter(message=>message.pending&&message.queued).length;
 const inputHint=queuedCount?t('chat.queuedCount',{count:queuedCount}):view.messages.some(message=>message.pending&&message.deliveryMode==='steer')?t('chat.steerSubmitted'):t(view.running?'chat.runningInputHint':'chat.inputHint');
 const composer=<ChatComposer onResizeIntent={()=>{resizeDetached.current=true;follow.current=false;}} key={activeId||`welcome:${welcomeRevision}`} conversationId={activeId} text={text} onText={updateText} onSend={requestSend} onStop={()=>void stopCurrent()} running={view.running} stopping={stopping} inputLocked={busy&&!sendCleared||opening||draftLoading||reconnecting||transferBusy} busy={busy||stopping||opening||draftLoading||reconnecting||transferBusy} saving={saving} providers={ai?.providers??[]} model={conversation?.model??newModel} permission={catalog.projects.find(project=>project.id===(conversation?.projectId??newProjectId))?.permission??conversation?.permission??newPermission} onModel={model=>void run(()=>configure({model}))} onPermission={permission=>void run(()=>configure({permission}))} onLocation={!activeId?editLocations:undefined} location={!activeId&&saveLocation?saveLocation:undefined} attachments={attachments} onAttach={()=>void run(addAttachments)} onImportFiles={(files,clipboard)=>void run(()=>addBrowserFiles(files,clipboard))} onImportPaths={paths=>void run(()=>addAttachments(paths))} onImportError={error=>setError(String(error))} onRemoveAttachment={path=>{releasePendingFile(path);updateText(text,removeBrowserAttachment(attachments,path));}} onTools={tab=>{setDetailsFile(undefined);setDetailsReview(undefined);setDetailsTab(tab);setDetailsActivation(value=>value+1);setDetails(true);}} thinkingLevel={conversation?.thinkingLevel??newThinking} fastMode={conversation?.fastMode??newFastMode} onThinkingLevel={value=>void run(()=>setThinkingLevel(value))} onFastMode={value=>void run(()=>setFastMode(value))} contextUsage={view.contextUsage} usage={view.usage} onCompact={conversation&&view.messages.length?()=>void run(async()=>{await chatCall("compact",{id:conversation.id});}):undefined} paused={view.paused} taskGraph={view.taskGraph} onPause={activeId?()=>void run(async()=>{await chatCall("pause",{id:activeId});}):undefined} onResume={activeId?()=>void run(async()=>{await chatCall("resume",{id:activeId});}):undefined} soundEnabled={soundEnabled} onSoundToggle={toggleSound} subagents={view.subagents} onStopSubagent={name=>void run(async()=>{if(activeId)await chatCall("agents",{id:activeId,action:"stop",name});})}/>;
 return <ChatDesktopSurface><MediaProvider id={activeId} cwd={conversation?.directory}><ProcessNavigationProvider><ChatFileNavigation id={activeId} cwd={conversation?.directory??""} onBrowser={url=>{setDetailsFile(undefined);setDetailsReview(undefined);setDetailsURL(url);setDetailsTab('home');setDetailsActivation(value=>value+1);setDetails(true);}} files={view.messages.flatMap(message=>message.attachments??[])} onDirectory={path=>{setDetailsFile(undefined);setDetailsDirectory(path);setDetailsTab('files');setDetailsActivation(value=>value+1);setDetails(true);}} onOpen={(path,line)=>{setDetailsFile(path);setDetailsLine(line);setDetailsTab('home');setDetailsActivation(value=>value+1);setDetails(true);}}><div ref={workspaceRoot} className={"chat-workspace"+(sidebar?"":" chat-workspace--collapsed")}>
  <ResizablePanelGroup elementRef={sidebarGroup} orientation="horizontal" defaultLayout={panelLayout.defaultLayout} onLayoutChanged={(layout,meta)=>{panelLayout.onLayoutChanged(layout,meta);if(meta.isUserInteraction&&sidebarGroup.current){const pixels=(sidebarGroup.current.clientWidth-1)*layout.sidebar/100;if(sidebar&&pixels>=minimumSidebarWidth-1){sidebarWidth.current=Math.max(minimumSidebarWidth,pixels);rememberPanels();}}}}>
  <ResizablePanel className="chat-panel" style={{overflow:"hidden"}} id="sidebar" panelRef={sidebarPanel} defaultSize={restoredPanels.current.sidebar} minSize={sidebar?minimumSidebarWidth:0} maxSize={sidebar?400:0} groupResizeBehavior="preserve-pixel-size"><div hidden={!sidebar} className="h-full"><ChatSidebar catalog={catalog} activities={sidebarActivities} activityError={activityError} activeId={activeId} busy={busy} titleJobs={titleJobs} archived={archived} onArchived={setArchived} onChoose={id=>{setAutomationsOpen(false);void run(()=>choose(id));}} onNew={projectId=>{setAutomationsOpen(false);void run(async()=>{await choose("");setWelcomeRevision(value=>value+1);setNewProjectId(projectId??"");setNewThinking("");setNewFastMode(false);});}} onProject={openProjectDialog} onHide={()=>setSidebar(false)} onSettings={()=>setSettingsOpen(true)} onAutomations={()=>{setDetails(false);setAutomationsOpen(true);}} automationsOpen={automationsOpen} onAction={conversationAction} onProjectAction={(project,action)=>{setDialogProject(project);setName(project.name);setDialog(action==="rename"?"renameProject":"removeProject");}}/></div></ResizablePanel><ResizableHandle aria-label={t("chat.resizeSidebar")}/>
  <ResizablePanel className="chat-panel" style={{overflow:"hidden"}} id="workbench" minSize="30%"><ResizablePanelGroup elementRef={workbenchGroup} orientation={panelDock==='right'?'horizontal':'vertical'} defaultLayout={collapsedWorkbenchLayout} onLayoutChanged={(layout,meta)=>{if(meta.isUserInteraction&&workbenchGroup.current){const pixels=((panelDock==='right'?workbenchGroup.current.clientWidth:workbenchGroup.current.clientHeight)-1)*layout.details/100;if(pixels<=4)detailsPanel.current?.resize(0);else{detailsSize.current[panelDock]=pixels;rememberPanels();}}}}><ResizablePanel className="chat-panel" style={{overflow:"hidden"}} id="conversation" minSize={panelDock==='right'?'240px':'20%'}>{automationsOpen?<ChatAutomations providers={ai?.providers??[]} defaultModel={newModel} conversations={catalog.conversations} onBack={()=>setAutomationsOpen(false)} onEditingChange={setAutomationEditing} onConversation={id=>void run(async()=>{await refresh();await choose(id);setAutomationsOpen(false);})}/>:<main className="chat-main">
   <header className="chat-header">{!sidebar&&<Button variant="ghost" size="icon" aria-label={t("chat.showSidebar")} onClick={()=>setSidebar(true)}><SidebarSimple size={16}/></Button>}
    <div className="chat-header-title"><strong>{conversation?.title??t("chat.newChat")}</strong><span title={conversation?.directory}>{conversation?.directory??catalog.projects.find(p=>p.id===newProjectId)?.name??t("chat.noProject")}</span></div>
    {conversation&&<DropdownMenu><DropdownMenuTrigger render={<Button variant="ghost" size="icon" icon={MoreHorizontal} aria-label={t("chat.more")}/>}/><DropdownMenuContent align="end" className="chat-row-menu">
     <DropdownMenuItem disabled={busy} onClick={()=>conversationAction(conversation,"rename")}>{t("chat.rename")}</DropdownMenuItem>
     <DropdownMenuItem disabled={busy||view.running||titleJobs[conversation.id]?.pending} onClick={()=>conversationAction(conversation,"aiRename")}><Sparkles/>{t("chat.aiRename")}</DropdownMenuItem>
     <DropdownMenuItem onClick={()=>void run(()=>updateConversation({pinned:!conversation.pinned}))}><Pin/>{t(conversation.pinned?"chat.unpin":"chat.pin")}</DropdownMenuItem>
     <DropdownMenuItem disabled={view.running} onClick={()=>void run(()=>updateConversation({archived:!conversation.archived}))}><Archive/>{t(conversation.archived?"chat.restore":"chat.archive")}</DropdownMenuItem>
     <DropdownMenuItem disabled={view.running||!view.messages.length} onClick={()=>void run(()=>fork())}><GitBranch/>{t("chat.fork")}</DropdownMenuItem>
     <DropdownMenuItem disabled={view.running||!view.messages.length} onClick={()=>void run(async()=>{await chatCall("compact",{id:activeId});})}><Settings2/>{t("chat.compact")}</DropdownMenuItem>
     <DropdownMenuItem onClick={()=>copy(conversation.directory)}><Copy/>{t("chat.sourcePath")}</DropdownMenuItem>
     <DropdownMenuItem disabled={view.running||busy} onClick={editLocations}><FolderOpen/>{t("chat.saveLocation")}</DropdownMenuItem>
     <DropdownMenuItem disabled={view.running||busy} onClick={()=>conversationAction(conversation,"move")}><FolderInput/>{t("chat.moveToProject")}</DropdownMenuItem>
     <DropdownMenuItem disabled={busy} onClick={()=>conversationAction(conversation,"export")}><Download/>{t("chat.exportTask")}</DropdownMenuItem>
     <DropdownMenuItem disabled={view.running||busy} onClick={()=>conversationAction(conversation,"delete")}><Trash2/>{t("chat.delete")}</DropdownMenuItem>
    </DropdownMenuContent></DropdownMenu>}
    {!activeId&&<Button variant="ghost" size="icon" icon={FolderOpen} aria-label={t("chat.saveLocation")} onClick={editLocations}/>}
    {!details&&<Button variant="ghost" size="icon" aria-label={t("chat.showDetails")} onClick={()=>setDetails(true)}><SidebarSimple size={16}/></Button>}
   </header>
   <ChatFeedback key={'notice:'+activeId} message={notice||view.notice}/>
   <ChatFeedback key={'error:'+activeId} message={dialog==='project'?'':error} role="alert" className="chat-error"><Button variant="ghost" icon={RefreshCw} onClick={()=>void run(async()=>{await refresh();if(activeId)setRefreshToken(value=>value+1);})}>{t("chat.retry")}</Button></ChatFeedback>
   <ChatFeedback key={'model:'+activeId} message={view.modelError} className="chat-error">{onOpenAI&&<Button onClick={onOpenAI}>{t("chat.openAI")}</Button>}</ChatFeedback>
   <div className="chat-conversation-body"><div className="chat-reading-pane">
   {!activeId&&!opening&&!submittingMessage?<div className="chat-welcome-stage"><div className="chat-welcome"><img className="chat-welcome-logo" src={welcomeLogo} alt="" draggable={false}/><div className="chat-welcome-composer"><div className="chat-composer-stack"><ChatProjectContext projects={catalog.projects} projectId={newProjectId} onProject={setNewProjectId} onOpenProject={openProjectDialog} disabled={busy}/>{composer}</div></div></div></div>:<ScrollArea disableContentFit className="chat-transcript" viewportProps={{"data-scroll-container":"",ref:scroll,className:"chat-transcript-viewport",onWheel:event=>{const el=event.currentTarget;if(shouldUnpinOnWheel({deltaX:event.deltaX,deltaY:event.deltaY,scrollHeight:el.scrollHeight,clientHeight:el.clientHeight}))follow.current=false;else if(!resizeDetached.current&&shouldRepinOnWheel({deltaX:event.deltaX,deltaY:event.deltaY,distanceFromBottom:chatScrollMetrics(el).bottomTop-el.scrollTop}))follow.current=true;},onKeyDown:event=>{if(['PageUp','Home','ArrowUp'].includes(event.key)&&!(event.target as HTMLElement).closest('input,textarea,[contenteditable=true]')&&shouldUnpinOnUpIntent(event.currentTarget))follow.current=false;},onScroll:()=>{const el=scroll.current;if(el){if(!resizeDetached.current)follow.current=resolveNearBottomOnScroll({wasNearBottom:follow.current,distanceFromBottom:chatScrollMetrics(el).bottomTop-el.scrollTop,scrollDelta:el.scrollTop-lastScrollTop.current,thresholdPx:80,directionDeadZonePx:1});lastScrollTop.current=el.scrollTop;}}}}>
    {opening&&!submittingMessage?<div className="chat-loading"><LoaderCircle className="animate-spin" size={18}/>{t("chat.loading")}</div>:<div className="client-thread client-thread--chat">
     {view.hasOlder&&<Button className="chat-load-older" variant="ghost" busy={historyLoading} disabled={busy} onClick={()=>void loadEarlier()}>{t(historyLoading?'chat.loadingOlder':'chat.loadOlder')}</Button>}
     {processItems.map((item,index)=>{
      if(item.kind==="user")return <div key={"user:"+item.key} data-chat-turn={item.key}><ChatMessageView conversationId={activeId} message={item.message} running={view.running} busy={busy||view.paused} onEdit={!item.message.pending&&item.message.id&&item.message.id===[...view.messages].reverse().find(message=>message.role==='user')?.id?(text,revision)=>editMessage(item.message.id!,text,revision):undefined} onStopForEdit={view.running?async()=>{await chatCall('stop',{id:activeId});}:undefined} onRetry={!item.message.pending&&item.message.id?()=>void run(()=>retryMessage(item.message.id)):undefined} onCopy={copy} canFork={!view.running&&!busy} onFork={id=>void run(()=>fork(id))}/></div>;
      const group=ai?.showReasoning===false?{...item,entries:item.entries.filter(entry=>entry.kind!=="thinking")}:item;
      // Thinking-only and tool turns share one Cindy work surface, including
      // completion. No empty-start or completed-thinking legacy fallback.
      const grouped=group.entries.length>0;
      return <ChatMessageView key={"response:"+item.key} message={item.answer??{role:"assistant",content:[]}} providers={ai?.providers} onOpenFile={openFile} onCopy={copy} canFork={!view.running&&!busy} onFork={id=>void run(()=>fork(id))} onRetry={index===processItems.length-1&&view.messages.some(message=>message.role==="user"&&message.id&&!message.pending)?()=>void run(()=>retryMessage()):undefined} running={item.running} busy={busy||view.paused||view.running} showReasoning={ai?.showReasoning!==false} conversationId={activeId} process={grouped?<WorkProcess group={group} conversationId={activeId} onCopy={copy} providers={ai?.providers} canFork={!view.running&&!busy} onFork={id=>void run(()=>fork(id))} onOpenFile={openFile}/>:undefined}/>;
     })}
     {!hasInlinePlan&&<ChatPlan key={activeId} workflow={view.workflow} animated={view.running}/>}
       {/* Files have one transcript footer; no response owns or repositions it. */}
     {(generatedFiles.length>0||changedTurns.length>0)&&<section className="chat-conversation-files" aria-label={t('chat.conversationFiles')}>
      {generatedFiles.length>0&&<ChatGeneratedFiles id={activeId} paths={generatedFiles} onOpen={openFile}/>}
      {changedTurns.length>0&&<ChatTranscriptChanges key={activeId} id={activeId} turns={changedTurns.flatMap(item=>item.kind==='response'?[{turnId:item.key,title:messageText(visibleMessages.find(message=>message.role==='user'&&(message.turnId??message.requestId??message.id)===item.key)??{role:'user',content:''}),version:item.entries.filter(entry=>entry.kind==='tool'&&!entry.tool.running).map(entry=>entry.key).join(':')}]:[])} disabled={view.running||busy} onReview={openTurnReview} onOpen={openFile}/>}
     </section>}
    </div>}
    <div className="chat-transcript-end-space" aria-hidden="true"/>
   </ScrollArea>}
   {activeId&&<footer className="chat-input-area"><div className="chat-input-content">
    <ChatPlanReminder key={activeId} workflow={currentPlan?.workflow} anchor={currentPlanAnchor} active={!!currentPlan&&(view.running||!!view.paused)} animated={view.running&&!view.paused} viewport={scroll}/>
    <ProcessCollapseAction/>
    {activeRetry&&<RetryStatus operation={activeRetry} stopping={stopping} onStop={()=>void stopCurrent()}/>}
    <div className="chat-composer-stack"><ChatProjectContext projects={catalog.projects} projectId={conversation?.projectId??""} onProject={id=>{if(id)void run(()=>moveCurrentConversation(id));}} onOpenProject={openProjectDialog} disabled={busy}/>
    {view.approvals[0]?.kind==="question"?<ChatQuestion key={view.approvals[0].id} request={view.approvals[0]} busy={busy} onStop={()=>void run(async()=>{await chatCall("stop",{id:activeId});})} onAnswer={answers=>respondQuestion(answers)} onCancel={()=>respondQuestion()}/>:view.approvals[0]?<ChatApproval request={view.approvals[0]} busy={busy} onStop={()=>void run(async()=>{await chatCall("stop",{id:activeId});})} onRespond={(allow,scope)=>void run(async()=>{const approvalId=view.approvals[0].id;await chatCall("permission",{id:activeId,approvalId,allow,scope});setView(current=>({...current,approvals:current.approvals.filter(a=>a.id!==approvalId)}));})}/>:composer}</div>
    <p className="chat-input-hint" role="status"><span title={inputHint}>{inputHint}</span></p>
   </div></footer>}
   {activeId&&<ChatScrollControls viewportRef={scroll} conversationId={activeId} onNavigate={latest=>{resizeDetached.current=false;follow.current=latest;}}/>}</div></div>
  </main>}</ResizablePanel>
  <ResizableHandle aria-label={t("chat.resizeDetails")}/><ResizablePanel className="chat-panel" style={{overflow:"hidden"}} id="details" panelRef={detailsPanel} defaultSize={0} minSize={0} maxSize="80%" groupResizeBehavior="preserve-pixel-size" onResize={size=>{if(size.inPixels>4)setDetailsMounted(true);setDetailsVisible(size.inPixels>4);}}><div hidden={!details} className="h-full">{detailsMounted&&<ChatSidePanel key={`${activeId}:${conversation?.directory??""}`} visible={details} running={view.running} lastTurnPaths={latestReviewPaths} id={activeId} initialTab={detailsTab} initialTabRequest={detailsActivation} initialURL={detailsURL} onURLHandled={()=>setDetailsURL(undefined)} initialFile={detailsFile} initialLine={detailsLine} initialDirectory={detailsDirectory} initialReview={detailsReview} onClose={()=>setDetails(false)} onDirtyChange={changeFilesDirty} dock={panelDock} onDock={()=>{const next=panelDock==='right'?'bottom':'right';dockTarget.current=detailsSize.current[next];setPanelDock(next);}} onReference={path=>{if(!busy&&!opening&&!draftLoading)updateText(text+(text?"\n":"")+"@"+path);}} onReviewComment={value=>{if(busy||opening||draftLoading)throw Error(t("chat.browserDraftBusy"));updateText(text+(text?"\n\n":"")+value);}} onComment={addBrowserComment} commentMarkerNumbers={attachments.flatMap(file=>file.annotation?[file.annotation.markerNumber]:[])}/>}</div></ResizablePanel>
  </ResizablePanelGroup></ResizablePanel>
  </ResizablePanelGroup>
  {moveTarget&&<ChatMoveDialog key={moveTarget.id} conversation={moveTarget} projects={catalog.projects} busy={busy} error={error} onClose={()=>setMoveTarget(undefined)} onMove={projectId=>void run(()=>moveConversation(projectId))}/>}
  {exportTarget&&<ChatExportDialog key={exportTarget.id} conversation={exportTarget} onBusy={setTransferBusy} onClose={()=>setExportTarget(undefined)} onExport={async options=>{await draftWrites.current;return chatCall<ChatArchiveOutcome>("export",{id:exportTarget.id,...options});}} onExported={value=>setNotice(chatArchiveNotice(value,t))}/>}
  <QueuedSendConfirm open={!!queuedSend} text={queuedSend?.text??""} onCancel={()=>setQueuedSend(null)} onConfirm={()=>confirmSend('queue')} onSteer={()=>confirmSend('steer')}/>
  <ChatSettings modelsLoading={modelsLoading} modelsError={modelsError} open={settingsOpen} onOpenChange={setSettingsOpen} providers={ai?.providers??[]} defaultModel={newModel} onSaved={()=>{}} onConversation={id=>void run(async()=>{await refresh();await choose(id);})}/>
  <Dialog open={dialog!==null} onOpenChange={open=>{if(!open&&!busy)setDialog(null);}}><DialogContent className={dialog==="project"||dialog==="locations"?"chat-configuration-dialog":undefined}><DialogTitle>{t(dialog==="project"?"chat.openOrCreateProject":dialog==="locations"?"chat.saveLocation":dialog==="delete"?"chat.delete":dialog==="removeProject"?"chat.removeProject":"chat.rename")}</DialogTitle>
   {dialog==="delete"||dialog==="removeProject"?<DialogDescription>{t(dialog==="delete"?"chat.deleteDescription":"chat.removeProjectHint",{name:dialogConversation?.title})}</DialogDescription>:<>
   {dialog==="locations"?<>
    <PathDropArea kind="directory" disabled={busy||!!conversation||!!newProjectId} onError={setError} onPaths={paths=>setEditWorking(paths[0])} className="chat-dialog-field"><span>{t("chat.sourcePath")}</span><div><span>{editWorking||t("chat.defaultLocation")}</span>{!conversation&&!newProjectId&&<Button disabled={busy} onClick={()=>void run(async()=>{const path=await pickDirectory();if(path)setEditWorking(path);})}>{t("chat.chooseLocation")}</Button>}</div></PathDropArea>
    <PathDropArea kind="directory" disabled={busy} onError={setError} onPaths={paths=>setEditSaved(paths[0])} className="chat-dialog-field"><span>{t("chat.saveLocation")}</span><div><span>{editSaved||t("chat.defaultLocation")}</span><Button disabled={busy} onClick={()=>void run(async()=>{const path=await pickDirectory();if(path)setEditSaved(path);})}>{t("chat.chooseLocation")}</Button></div><p>{t("chat.storageHint")}</p></PathDropArea>
   </>:dialog==="project"?<PathDropArea kind="directory" disabled={busy} onError={setError} onPaths={paths=>run(()=>chooseProjectDirectory(paths[0]))}><ChatProjectFields mode={projectMode} onMode={setProjectMode} name={name} onName={setName} directory={parent} onChooseDirectory={()=>void run(chooseProjectDirectory)} busy={busy} permission={projectPermission} onPermission={setProjectPermission}/></PathDropArea>:<label className="chat-dialog-field">{t("chat.newName")}<input autoFocus value={name} onChange={event=>setName(event.target.value)}/></label>}
   </>}
   {dialog&&dialog!=="project"&&error&&<p role="alert" className="chat-error">{error}</p>}
   {dialog==="project"&&error&&<div role="alert" className="chat-error">{error}</div>}
   <DialogFooter><Button disabled={busy} onClick={()=>setDialog(null)}>{t("chat.cancel")}</Button><Button variant="primary" disabled={busy||!["locations","delete","removeProject"].includes(dialog??"")&&(!name.trim()||dialog==="project"&&!parent)} onClick={()=>void run(async()=>{if(dialog==="locations"){await saveLocations();return;}if(dialog==="delete"&&dialogConversation){await draftWrites.current;const updated=await chatCall<ChatCatalog>("conversation.delete",{id:dialogConversation.id});setCatalog(updated);if(activeId===dialogConversation.id)await choose("");}else if(dialog==="project"){await saveProject();}else if((dialog==="renameProject"||dialog==="removeProject")&&dialogProject){await chatCall(dialog==="renameProject"?"project.update":"project.remove",{id:dialogProject.id,...(dialog==="renameProject"?{name}:{})});if(dialog==="removeProject"&&newProjectId===dialogProject.id)setNewProjectId("");await refresh();}else await updateConversation({title:name},dialogConversation);setDialog(null);})}>{t(dialog==="project"?(projectMode==="open"?"chat.openProject":"chat.create"):dialog==="locations"?"chat.apply":dialog==="delete"?"chat.delete":dialog==="removeProject"?"chat.remove":"chat.rename")}</Button></DialogFooter></DialogContent></Dialog>
 </div></ChatFileNavigation></ProcessNavigationProvider></MediaProvider></ChatDesktopSurface>;
}
