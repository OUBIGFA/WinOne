import {useEffect,useLayoutEffect,useRef,useState} from "react";
import {FolderOpen,FolderInput,Download,MoreHorizontal,ChevronsLeftRight,Archive,Pin,GitBranch,Copy,Settings2,RefreshCw,LoaderCircle,Trash2,Sparkles} from "lucide-react";
import {useDefaultLayout} from "react-resizable-panels";
import welcomeLogo from "../../../../../internal/branding/chat-welcome.png";
import {chatCall,captureCall,pickDirectory,pickChatFiles,setFileDropTarget} from "../bridge";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {usePageActivity,type PageActivityChanged} from "../ui/page-navigation";
import {DropdownMenu,DropdownMenuTrigger,DropdownMenuContent,DropdownMenuItem} from "../components/ui/dropdown-menu";
import {Dialog,DialogContent,DialogTitle,DialogDescription,DialogFooter} from "../components/ui/dialog";
import {ResizablePanelGroup,ResizablePanel,ResizableHandle} from "../components/ui/resizable";
import type {AIConfiguration,AISelection} from "./ai-configuration";
import {ChatComposer,ChatApproval} from "./chat-composer";
import {ChatMessageView} from "./chat-messages";
import {ChatSidebar,type ConversationAction} from "./chat-sidebar";
import {ChatSidePanel} from "./chat-side-panel";
import {ChatDesktopSurface} from './chat-desktop-kit';
import {ChatProjectContext} from "./chat-project-context";
import {ChatProjectFields} from "./chat-project-dialog";
import {ChatSettings} from "./chat-settings";
import {ChatFeedback} from "./chat-feedback";
import {ChatMoveDialog,ChatExportDialog,chatArchiveNotice,type ChatArchiveOutcome} from "./chat-transfer-dialogs";
import {ChatAutomations} from "./chat-automations";
import {browserAnnotatedPrompt,removeBrowserAttachment} from "./chat-browser-annotations";
import type {ChatAttachment,ChatBrowserAnnotation} from "./chat-types";
import {mergeChatOperations} from "./chat-state";
import {projectChatProcess} from "./chat-process";
import {WorkProcess} from "./cindy/WorkProcess";
import {copyText} from "./clipboard";
import {ScrollArea} from "./lobe/scroll-area/ScrollArea";
import {ChatFileNavigation} from './chat-file-navigation';
import {ProcessNavigationProvider,ProcessCollapseAction} from "./process-navigation";
import {applyChatEvent,emptyChatState,type ChatCatalog,type ChatProject,type Conversation,type ChatPermission,type ChatSnapshot,type ChatState,type ChatPoll,type ChatMessage} from "./chat-state";
import "./chat.css";
import "./cindy/theme.css";
import "./cindy/message-tokens.css";
import "./cindy/reading.css";

const emptyCatalog:ChatCatalog={projects:[],conversations:[]};
const lastModelKey="winone.chat.last-model.v1";
// The composer starts from the AI service default model, but once the user
// switches a model the last choice is remembered and reused on the next chat.
function readLastModel():AISelection|undefined{try{const raw=localStorage.getItem(lastModelKey);if(!raw)return undefined;const value=JSON.parse(raw);if(value&&typeof value.provider==="string"&&typeof value.model==="string")return value;}catch{}return undefined;}
function initialModel(models:AIConfiguration,current:AISelection={provider:"",model:""}){const last=readLastModel();if(last&&models.providers.some(provider=>provider.id===last.provider&&provider.models.includes(last.model)))return last;return models.tasks.chat??models.tasks.default??models.providers.filter(provider=>provider.models.length).map(provider=>({provider:provider.id,model:provider.models[0]}))[0]??current;}
function writeLastModel(model:AISelection){try{localStorage.setItem(lastModelKey,JSON.stringify(model));}catch{}}
const errorText=(error:unknown)=>error instanceof Error?error.message:String(error);

export function ChatPage({onActivity,onOpenAI,requestedConversationId,onRequestedConversationHandled}:{onActivity?:PageActivityChanged;onOpenAI?:()=>void;requestedConversationId?:string;onRequestedConversationHandled?:()=>void}){
 const {t}=useTranslation();
 const workspaceRoot=useRef<HTMLDivElement>(null);
 const [catalog,setCatalog]=useState<ChatCatalog>(emptyCatalog),[ai,setAI]=useState<AIConfiguration>(),[activeId,setActiveId]=useState("");
 const [view,setView]=useState<ChatState>(emptyChatState),[text,setText]=useState(""),[draftDirty,setDraftDirty]=useState(false),[saving,setSaving]=useState(false);
 const [submittingMessage,setSubmittingMessage]=useState<ChatMessage>();
 const [busy,setBusy]=useState(false),[opening,setOpening]=useState(false),[draftLoading,setDraftLoading]=useState(false),[error,setError]=useState(""),[notice,setNotice]=useState("");
 const [titleJobs,setTitleJobs]=useState<Record<string,{pending:boolean;error?:string}>>({});
 const [detailsReview,setDetailsReview]=useState<{turnId:string;path?:string}>();
 const [detailsLine,setDetailsLine]=useState<number>();
 const [detailsDirectory,setDetailsDirectory]=useState<string>();
 const titleRequests=useRef(new Set<string>());
 const titleEdits=useRef(new Map<string,number>());
 const [archived,setArchived]=useState(false),[sidebar,setSidebar]=useState(true),[details,setDetails]=useState(false),[settingsOpen,setSettingsOpen]=useState(false);
 const [attachments,setAttachments]=useState<ChatAttachment[]>([]),[detailsTab,setDetailsTab]=useState<"home"|"files"|"resources"|"review">("home");
 const [detailsActivation,setDetailsActivation]=useState(0),[welcomeRevision,setWelcomeRevision]=useState(0),[historyLoading,setHistoryLoading]=useState(false);
 const historyAnchor=useRef<{height:number;top:number;nodes:Set<Element>}|undefined>(undefined),historyRequest=useRef<object|null>(null);
 const [filesDirty,setFilesDirty]=useState(false),[panelDock,setPanelDock]=useState<"right"|"bottom">("right");
 const [automationsOpen,setAutomationsOpen]=useState(false),[automationEditing,setAutomationEditing]=useState(false);
 const [moveTarget,setMoveTarget]=useState<Conversation>(),[exportTarget,setExportTarget]=useState<Conversation>(),[transferBusy,setTransferBusy]=useState(false),[reconnecting,setReconnecting]=useState(false);
 const [newProjectId,setNewProjectId]=useState(""),[location,setLocation]=useState(""),[saveLocation,setSaveLocation]=useState(""),[newModel,setNewModel]=useState<AISelection>(()=>readLastModel()??{provider:"",model:""}),[newPermission,setNewPermission]=useState<ChatPermission>("ask"),[projectPermission,setProjectPermission]=useState<ChatPermission>("ask");
 const [projectMode,setProjectMode]=useState<"open"|"create">("open");
 const [dialog,setDialog]=useState<"project"|"rename"|"delete"|"renameProject"|"removeProject"|"locations"|null>(null),[dialogConversation,setDialogConversation]=useState<Conversation>(),[dialogProject,setDialogProject]=useState<ChatProject>(),[name,setName]=useState(""),[parent,setParent]=useState(""),[editWorking,setEditWorking]=useState(""),[editSaved,setEditSaved]=useState("");
 const panelLayout=useDefaultLayout({id:"winone-chat-panels",panelIds:[...(sidebar?["sidebar"]:[]),"workbench"]});
 const [refreshToken,setRefreshToken]=useState(0),[catalogLoaded,setCatalogLoaded]=useState(false);
 const requestedNavigation=useRef({id:requestedConversationId,handled:onRequestedConversationHandled}),handledNavigation=useRef<string|undefined>(undefined);
 requestedNavigation.current={id:requestedConversationId,handled:onRequestedConversationHandled};
 const inFlight=useRef(false),draftWrites=useRef(Promise.resolve()),draftRevision=useRef(0),openDraftRevision=useRef(0),pendingInput=useRef<{id:string;text:string;attachments?:string[]}|undefined>(undefined),scroll=useRef<HTMLDivElement>(null),follow=useRef(true),newDraft=useRef(""),newAttachments=useRef<ChatAttachment[]>([]);
 const selection=useRef(""),selectionRevision=useRef(0),openingRequests=useRef(new Map<string,Promise<ChatSnapshot>>()),lastPrompt=useRef("");
 const silentReload=useRef(""),configuring=useRef("");
 const conversation=catalog.conversations.find(value=>value.id===activeId);
 const visibleMessages=submittingMessage&&!view.messages.some(message=>message.requestId===submittingMessage.requestId||message.id===submittingMessage.id)?[...view.messages,submittingMessage]:view.messages;
 const [detailsFile,setDetailsFile]=useState<string>();
 const processItems=projectChatProcess(visibleMessages,view.tools,view.running,view.operations);
 usePageActivity(onActivity,draftDirty||automationEditing||filesDirty,busy||saving||opening||transferBusy||reconnecting,"chat");
 async function refresh(){const [value,models]=await Promise.all([chatCall<ChatCatalog>("catalog"),captureCall<AIConfiguration>("ai.configuration.get")]);setCatalog(value);setCatalogLoaded(true);setAI(models);setNewModel(current=>initialModel(models,current));}
 useEffect(()=>{let alive=true;void Promise.all([chatCall<ChatCatalog>("catalog"),captureCall<AIConfiguration>("ai.configuration.get")]).then(([value,models])=>{if(alive){setCatalog(value);setCatalogLoaded(true);setAI(models);setNewModel(initialModel(models));}}).catch(error=>{if(alive)setError(errorText(error));});return()=>{alive=false;};},[]);
 const run=async(action:()=>Promise<void>)=>{
  if(inFlight.current)return;inFlight.current=true;setBusy(true);setError("");setNotice("");
  try{await action();}catch(error){setError(errorText(error));}finally{inFlight.current=false;setBusy(false);}
 };
 function updateText(value:string,nextAttachments=attachments){
  setText(value);setAttachments(nextAttachments);const id=activeId;if(!id){newDraft.current=value;newAttachments.current=nextAttachments;setDraftDirty(value.length>0||nextAttachments.length>0);return;}
  const revision=++draftRevision.current;setDraftDirty(true);setSaving(true);
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
  if(id===selection.current)return;
  if(filesDirty)throw new Error(t("chat.saveFileBeforeNavigate"));
  const revision=++selectionRevision.current;await draftWrites.current;if(revision!==selectionRevision.current||!stillRequested())return;
  historyRequest.current=null;historyAnchor.current=undefined;setHistoryLoading(false);
  // A new selection owns its own runtime state. In particular, leaving for an
  // empty chat has no open callback that could clear the previous reconnect.
  configuring.current="";silentReload.current="";setReconnecting(false);
  selection.current=id;openDraftRevision.current=++draftRevision.current;pendingInput.current=undefined;setError("");setActiveId(id);setOpening(!!id);setDraftLoading(!!id);setDraftDirty(!id&&(newDraft.current.length>0||newAttachments.current.length>0));setText(id?"":newDraft.current);setAttachments(id?[]:newAttachments.current);setView(emptyChatState());setNotice("");
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
    if(page.reset){const state=await chatCall<ChatSnapshot>("state",{id:activeId});if(disposed)return;setView({...emptyChatState(),...state,running:page.running,status:page.status,approvals:page.approvals});}
    else setView(current=>{let next=current;for(const event of page.events)next=applyChatEvent(next,event.data);return {...next,running:page.running,status:page.status,approvals:page.approvals};});
    cursor=page.cursor;if(page.error)setError(page.error);
    timer=setTimeout(()=>void poll(),page.more?0:page.running?160:1200);
   }catch(error){if(!disposed){if(configuring.current!==activeId)setError(errorText(error));timer=setTimeout(()=>void poll(),2000);}}
  };
  // Draft I/O is independent of session startup. Publish each result as soon
  // as it arrives; a slow or failed draft must not hide a valid transcript.
  if(!silent)void chatCall<{text:string;pendingId?:string;attachments?:ChatAttachment[]}>("draft.get",{id:activeId}).then(draft=>{
   if(disposed)return;
   if(loadRevision===draftRevision.current){if(draft.pendingId)pendingInput.current={id:draft.pendingId,text:draft.text,attachments:draft.attachments?.map(file=>file.path)};setText(draft.text);setAttachments(draft.attachments??[]);setDraftDirty(false);}
  }).catch(error=>{if(!disposed)setError(t("chat.draftLoadFailed")+": "+errorText(error));}).finally(()=>{if(!disposed)setDraftLoading(false);});
  void openConversation(activeId).then(state=>{
   if(disposed)return;
   if(silent)setView(current=>({...current,modelError:state.modelError}));
   else{setView({...emptyChatState(),...state,running:!!state.streaming});setOpening(false);follow.current=true;}
   configuring.current="";setReconnecting(false);void poll();
  }).catch(error=>{if(!disposed){configuring.current="";setReconnecting(false);setOpening(false);setError(errorText(error));}});
  return()=>{disposed=true;if(timer)clearTimeout(timer);};
 },[activeId,refreshToken]);
 useEffect(()=>()=>{if(activeId)void chatCall("release",{id:activeId}).catch(error=>console.error("Chat release failed",error));},[activeId]);
 // Idle polls return a fresh approval array; only actual content changes may
 // advance the transcript. Otherwise polling can interrupt a resize or read.
 const approvalKeys=view.approvals.map(value=>value.id).join('\n');
 useEffect(()=>{if(follow.current&&scroll.current)scroll.current.scrollTop=scroll.current.scrollHeight;},[view.messages,view.tools,approvalKeys]);
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
  let created=await chatCall<Conversation>("conversation.new",{projectId,directory:projectId?"":location,saveDirectory:saveLocation,model:newModel,title:"Chat"});
  if(!projectId&&newPermission!=="ask")created=await chatCall<Conversation>("configure",{id:created.id,permission:newPermission});
  setCatalog(current=>({...current,conversations:[...current.conversations,created]}));await choose(created.id);return created;
 }
 async function send(override?:{text:string;attachments:ChatAttachment[]}){
  const originalContent=override?override.text:text;let files=override?override.attachments:attachments;const content=browserAnnotatedPrompt(originalContent,files);if(!content.trim()&&!files.length)return;
  const receipt=pendingInput.current?.text===content&&JSON.stringify(pendingInput.current.attachments??[])===JSON.stringify(files.map(file=>file.path))?pendingInput.current:{id:crypto.randomUUID(),text:content,attachments:files.map(file=>file.path)};
  // The user row is an immediate acknowledgement, independent of session/file
  // startup. Keep it outside snapshots until prompt acceptance can reconcile it.
  setSubmittingMessage({id:receipt.id,requestId:receipt.id,role:'user',content:originalContent,attachments:files,timestamp:Date.now()});follow.current=true;
  let target=conversation;
  let acceptedReceipt:{id:string;text:string;attachments?:string[]}|undefined;
  await draftWrites.current;
  // Pi owns the durable follow-up queue; every submission has its own receipt.
  // Keeping it in the page would lose inputs on navigation or overwrite them.
  try{
   if(!target){target=await startConversation();if(files.length){const originals=files;files=(await chatCall<{attachments:ChatAttachment[]}>("attachments.add",{id:target.id,paths:files.map(file=>file.path)})).attachments.map((file,index)=>({...file,annotation:originals[index]?.annotation}));}await openConversation(target.id);}
   const paths=files.map(file=>file.path);
   receipt.attachments=paths;acceptedReceipt=receipt;pendingInput.current=receipt;lastPrompt.current=content;
   await chatCall("prompt",{id:target.id,requestId:receipt.id,text:content,...(paths.length?{attachments:paths}:{})});
  }catch(error){setSubmittingMessage(undefined);draftRevision.current++;setText(originalContent);setAttachments(files);setDraftDirty(true);throw error;}
  draftRevision.current++;newDraft.current="";newAttachments.current=[];setText("");setAttachments([]);setDraftDirty(false);setView(current=>{const optimistic=acceptedReceipt?{id:acceptedReceipt.id,requestId:acceptedReceipt.id,role:"user",content:originalContent,attachments:files,timestamp:Date.now()}:undefined;return {...current,messages:optimistic&&!current.messages.some(message=>message.id===optimistic.id||message.requestId===optimistic.requestId)?[...current.messages,optimistic]:current.messages,running:true,status:"running"};});setSubmittingMessage(undefined);
  if(target.title==="Chat")void generateTitle(target,content);
 }
 async function addBrowserComment(annotation:ChatBrowserAnnotation){
  if(busy||opening||draftLoading)throw new Error(t("chat.browserDraftBusy"));
  const imported=activeId?(await chatCall<{attachments:ChatAttachment[]}>("attachments.add",{id:activeId,paths:[annotation.screenshotPath]})).attachments[0]:{name:`#${annotation.markerNumber}.png`,path:annotation.screenshotPath,size:0,mimeType:"image/png"};
  updateText(text,[...attachments,{...imported,annotation}]);
 }
 async function addAttachments(chosen?:string[]){
  const paths=chosen??await pickChatFiles();if(!paths.length)return;
  const added=activeId?(await chatCall<{attachments:ChatAttachment[]}>("attachments.add",{id:activeId,paths})).attachments:paths.map(path=>({name:path.split(/[\\/]/).pop()??path,path,size:0,mimeType:""}));
  updateText(text,[...new Map([...attachments,...added].map(file=>[file.path,file])).values()]);
 }
 useEffect(()=>{
  void setFileDropTarget(true).catch(error=>setError(String(error)));
  return()=>{void setFileDropTarget(false).catch(console.error);};
 },[]);
 useEffect(()=>{
  const drop=(event:Event)=>{const paths=(event as CustomEvent<{paths:string[]}>).detail?.paths;if(paths?.length&&!busy&&!opening&&!draftLoading&&!view.running)void run(()=>addAttachments(paths));};
  window.addEventListener('winone:chat-files-drop',drop);
  return()=>{window.removeEventListener('winone:chat-files-drop',drop);};
 },[activeId,busy,opening,draftLoading,view.running,text,attachments]);
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
  }catch(error){setTitleJobs(current=>({...current,[target.id]:{pending:false,...((titleEdits.current.get(target.id)??0)===editRevision?{error:errorText(error)}:{})}}));}
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
 async function editMessage(entry:string,text:string,restoreRevision?:string){if(inFlight.current)throw Error(t('chat.browserDraftBusy'));const id=activeId;inFlight.current=true;setBusy(true);try{await draftWrites.current;if(view.running)await chatCall('stop',{id});await chatCall('edit',{id,entry,text,restoreRevision,requestId:crypto.randomUUID()});const state=await chatCall<ChatSnapshot>('state',{id});if(selection.current===id){setView({...emptyChatState(),...state,running:!!state.streaming});follow.current=true;}}finally{inFlight.current=false;setBusy(false);}}
 async function retryMessage(entry?:string){const user=[...view.messages].reverse().find(message=>message.role==="user");const target=entry??user?.id;if(!activeId||!target||view.running)return;await chatCall("retry",{id:activeId,entry:target,requestId:crypto.randomUUID()});const state=await chatCall<ChatSnapshot>("state",{id:activeId});setView({...emptyChatState(),...state,running:true,status:"running"});follow.current=true;}
 function editLocations(){setEditWorking(conversation?.directory??catalog.projects.find(p=>p.id===newProjectId)?.path??location);setEditSaved(conversation?.sessionDirectory??saveLocation);setDialog("locations");}
 function openProjectDialog(){setName("");setParent("");setProjectPermission("ask");setProjectMode("open");setError("");setDialog("project");}
 async function chooseProjectDirectory(){const path=await pickDirectory();if(path){setParent(path);if(projectMode==="open")setName(path.replace(/[\\/]+$/,"").split(/[\\/]/).pop()??"");}}
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
 const composer=<ChatComposer key={activeId||`welcome:${welcomeRevision}`} conversationId={activeId} text={text} onText={updateText} onSend={()=>void run(send)} onStop={()=>void run(async()=>{await chatCall("stop",{id:activeId});})} running={view.running} busy={busy||opening||draftLoading||reconnecting||transferBusy} saving={saving} providers={ai?.providers??[]} model={conversation?.model??newModel} permission={catalog.projects.find(project=>project.id===(conversation?.projectId??newProjectId))?.permission??conversation?.permission??newPermission} onModel={model=>void run(()=>configure({model}))} onPermission={permission=>void run(()=>configure({permission}))} onLocation={!activeId?editLocations:undefined} location={!activeId&&saveLocation?saveLocation:undefined} attachments={attachments} onAttach={()=>void run(addAttachments)} onRemoveAttachment={path=>updateText(text,removeBrowserAttachment(attachments,path))} onTools={tab=>{setDetailsTab(tab);setDetails(true);}}/>;
 return <ChatDesktopSurface><ProcessNavigationProvider><ChatFileNavigation id={activeId} cwd={conversation?.directory??""} files={view.messages.flatMap(message=>message.attachments??[])} onDirectory={path=>{setDetailsFile(undefined);setDetailsDirectory(path);setDetailsTab('files');setDetailsActivation(value=>value+1);setDetails(true);}} onOpen={(path,line)=>{setDetailsFile(path);setDetailsLine(line);setDetailsTab('home');setDetailsActivation(value=>value+1);setDetails(true);}}><div ref={workspaceRoot} className={"chat-workspace"+(sidebar?"":" chat-workspace--collapsed")}>
  <ResizablePanelGroup orientation="horizontal" defaultLayout={panelLayout.defaultLayout} onLayoutChanged={panelLayout.onLayoutChanged}>
  {sidebar&&<><ResizablePanel className="chat-panel" style={{overflow:"hidden"}} id="sidebar" defaultSize={230} minSize={170} maxSize="36%"><ChatSidebar catalog={catalog} activeId={activeId} busy={busy} titleJobs={titleJobs} archived={archived} onArchived={setArchived} onChoose={id=>{setAutomationsOpen(false);void run(()=>choose(id));}} onNew={projectId=>{setAutomationsOpen(false);void run(async()=>{await choose("");setWelcomeRevision(value=>value+1);setNewProjectId(projectId??"");});}} onProject={openProjectDialog} onHide={()=>setSidebar(false)} onSettings={()=>setSettingsOpen(true)} onAutomations={()=>setAutomationsOpen(true)} automationsOpen={automationsOpen} onAction={conversationAction} onProjectAction={(project,action)=>{setDialogProject(project);setName(project.name);setDialog(action==="rename"?"renameProject":"removeProject");}}/></ResizablePanel><ResizableHandle aria-label={t("chat.resizeSidebar")}/></>}
  <ResizablePanel className="chat-panel" style={{overflow:"hidden"}} id="workbench" minSize="30%"><ResizablePanelGroup orientation={panelDock==='right'?'horizontal':'vertical'}><ResizablePanel className="chat-panel" style={{overflow:"hidden"}} id="conversation" minSize="20%">{automationsOpen?<ChatAutomations providers={ai?.providers??[]} defaultModel={newModel} conversations={catalog.conversations} onBack={()=>setAutomationsOpen(false)} onEditingChange={setAutomationEditing} onConversation={id=>void run(async()=>{await refresh();await choose(id);setAutomationsOpen(false);})}/>:<main className="chat-main">
   <header className="chat-header">{!sidebar&&<Button variant="ghost" size="icon" icon={ChevronsLeftRight} aria-label={t("chat.showSidebar")} onClick={()=>setSidebar(true)}/>}
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
    {!details&&<Button variant="ghost" size="icon" icon={ChevronsLeftRight} aria-label={t("chat.showDetails")} onClick={()=>setDetails(true)}/>}
   </header>
   <ChatFeedback key={'notice:'+activeId} message={notice||view.notice}/>
   <ChatFeedback key={'error:'+activeId} message={dialog==='project'?'':error} role="alert" className="chat-error"><Button variant="ghost" icon={RefreshCw} onClick={()=>void run(async()=>{await refresh();if(activeId)setRefreshToken(value=>value+1);})}>{t("chat.retry")}</Button></ChatFeedback>
   <ChatFeedback key={'model:'+activeId} message={view.modelError} className="chat-error">{onOpenAI&&<Button onClick={onOpenAI}>{t("chat.openAI")}</Button>}</ChatFeedback>
   {!activeId&&!opening&&!submittingMessage?<div className="chat-welcome-stage"><div className="chat-welcome"><img className="chat-welcome-logo" src={welcomeLogo} alt="" draggable={false}/><div className="chat-welcome-composer"><div className="chat-composer-stack"><ChatProjectContext projects={catalog.projects} projectId={newProjectId} onProject={setNewProjectId} onOpenProject={openProjectDialog} disabled={busy}/>{composer}</div></div></div></div>:<ScrollArea disableContentFit className="chat-transcript" viewportProps={{ref:scroll,className:"chat-transcript-viewport",onScroll:()=>{const el=scroll.current;if(el)follow.current=el.scrollHeight-el.scrollTop-el.clientHeight<80;}}}>
    {opening&&!submittingMessage?<div className="chat-loading"><LoaderCircle className="animate-spin" size={18}/>{t("chat.loading")}</div>:<div className="client-thread client-thread--chat">
     {view.hasOlder&&<Button className="chat-load-older" variant="ghost" busy={historyLoading} disabled={busy} onClick={()=>void loadEarlier()}>{t(historyLoading?'chat.loadingOlder':'chat.loadOlder')}</Button>}
     {processItems.map((item,index)=>{
      if(item.kind==="user")return <ChatMessageView key={"user:"+item.key} conversationId={activeId} message={item.message} onEdit={item.message.id&&item.message.id===[...view.messages].reverse().find(message=>message.role==='user')?.id?(text,revision)=>editMessage(item.message.id!,text,revision):undefined} onStopForEdit={view.running?async()=>{await chatCall('stop',{id:activeId});}:undefined} onRetry={item.message.id&&!view.running&&!busy?()=>void run(()=>retryMessage(item.message.id)):undefined} onCopy={copy} canFork={!view.running&&!busy} onFork={id=>void run(()=>fork(id))}/>;
      const group=ai?.showReasoning===false?{...item,entries:item.entries.filter(entry=>entry.kind!=="thinking")}:item;
      // Keep the Cindy live-working disclosure mounted from the first running
      // poll, even before Pi has emitted a tool or commentary entry.
      const grouped=item.running||item.hasTools||item.entries.some(entry=>entry.kind==="commentary"||entry.kind==="operation");
      return <ChatMessageView key={"response:"+item.key} turnId={item.key} changeVersion={item.entries.filter(entry=>entry.kind==='tool'&&!entry.tool.running).map(entry=>entry.key).join(':')} message={item.answer??{role:"assistant",content:[]}} providers={ai?.providers} files={item.entries.flatMap(entry=>entry.kind==='tool'&&!entry.tool.running&&!entry.tool.isError?(entry.tool.files?.map(file=>file.path)??(/^(write|edit)$/.test(entry.tool.name)&&typeof entry.tool.args.path==='string'?[entry.tool.args.path]:[])):[])} onOpenFile={(path,line)=>{setDetailsFile(path);setDetailsLine(line);setDetailsTab('home');setDetailsActivation(value=>value+1);setDetails(true);}} onCopy={copy} canFork={!view.running&&!busy} onFork={id=>void run(()=>fork(id))} onRetry={index===processItems.length-1&&!view.running&&view.messages.some(message=>message.role==="user"&&message.id)?()=>void run(()=>retryMessage()):undefined} onOpenReview={item.entries.some(entry=>entry.kind==='tool'&&!entry.tool.running&&!entry.tool.isError&&(/^(write|edit|apply_patch)$/.test(entry.tool.name)||!!entry.tool.files?.length))?path=>{setDetailsFile(undefined);setDetailsReview({turnId:item.key,path});setDetailsTab('review');setDetailsActivation(value=>value+1);setDetails(true);}:undefined} running={item.running} showReasoning={ai?.showReasoning!==false} conversationId={activeId} process={grouped?<WorkProcess group={group} conversationId={activeId} onCopy={copy}/>:undefined}/>;
     })}
    </div>}
    <div className="chat-transcript-end-space" aria-hidden="true"/>
   </ScrollArea>}
   {activeId&&<footer className="chat-input-area"><div className="chat-input-content">
    <ProcessCollapseAction/>
    <div className="chat-composer-stack"><ChatProjectContext projects={catalog.projects} projectId={conversation?.projectId??""} onProject={id=>{if(id)void run(()=>moveCurrentConversation(id));}} onOpenProject={openProjectDialog} disabled={busy}/>
    {view.approvals[0]?<ChatApproval request={view.approvals[0]} busy={busy} onStop={()=>void run(async()=>{await chatCall("stop",{id:activeId});})} onRespond={(allow,scope)=>void run(async()=>{const approvalId=view.approvals[0].id;await chatCall("permission",{id:activeId,approvalId,allow,scope});setView(current=>({...current,approvals:current.approvals.filter(a=>a.id!==approvalId)}));})}/>:composer}</div>
    <p className="chat-input-hint">{t("chat.inputHint")}</p>
   </div></footer>}
  </main>}</ResizablePanel>
  {details&&!automationsOpen&&<><ResizableHandle aria-label={t("chat.resizeDetails")}/><ResizablePanel className="chat-panel" style={{overflow:"hidden"}} id="details" defaultSize="42%" minSize={panelDock==='right'?280:200} maxSize="80%"><ChatSidePanel key={`${activeId}:${conversation?.directory??""}`} id={activeId} initialTab={detailsTab} initialTabRequest={detailsActivation} initialFile={detailsFile} initialLine={detailsLine} initialDirectory={detailsDirectory} initialReview={detailsReview} onClose={()=>setDetails(false)} onDirtyChange={setFilesDirty} dock={panelDock} onDock={()=>setPanelDock(value=>value==='right'?'bottom':'right')} onReference={path=>{if(!busy&&!opening&&!draftLoading)updateText(text+(text?"\n":"")+"@"+path);}} onReviewComment={value=>{if(busy||opening||draftLoading)throw Error(t("chat.browserDraftBusy"));updateText(text+(text?"\n\n":"")+value);}} onComment={addBrowserComment} commentMarkerNumbers={attachments.flatMap(file=>file.annotation?[file.annotation.markerNumber]:[])}/></ResizablePanel></>}
  </ResizablePanelGroup></ResizablePanel>
  </ResizablePanelGroup>
  {moveTarget&&<ChatMoveDialog key={moveTarget.id} conversation={moveTarget} projects={catalog.projects} busy={busy} error={error} onClose={()=>setMoveTarget(undefined)} onMove={projectId=>void run(()=>moveConversation(projectId))}/>}
  {exportTarget&&<ChatExportDialog key={exportTarget.id} conversation={exportTarget} onBusy={setTransferBusy} onClose={()=>setExportTarget(undefined)} onExport={async options=>{await draftWrites.current;return chatCall<ChatArchiveOutcome>("export",{id:exportTarget.id,...options});}} onExported={value=>setNotice(chatArchiveNotice(value,t))}/>}
  <ChatSettings open={settingsOpen} onOpenChange={setSettingsOpen} providers={ai?.providers??[]} defaultModel={newModel} onSaved={()=>{}} onConversation={id=>void run(async()=>{await refresh();await choose(id);})}/>
  <Dialog open={dialog!==null} onOpenChange={open=>{if(!open&&!busy)setDialog(null);}}><DialogContent><DialogTitle>{t(dialog==="project"?"chat.openOrCreateProject":dialog==="locations"?"chat.saveLocation":dialog==="delete"?"chat.delete":dialog==="removeProject"?"chat.removeProject":"chat.rename")}</DialogTitle>
   {dialog==="delete"||dialog==="removeProject"?<DialogDescription>{t(dialog==="delete"?"chat.deleteDescription":"chat.removeProjectHint",{name:dialogConversation?.title})}</DialogDescription>:<>
   {dialog==="locations"?<>
    <div className="chat-dialog-field"><span>{t("chat.sourcePath")}</span><div><span>{editWorking||t("chat.defaultLocation")}</span>{!conversation&&!newProjectId&&<Button disabled={busy} onClick={()=>void run(async()=>{const path=await pickDirectory();if(path)setEditWorking(path);})}>{t("chat.chooseLocation")}</Button>}</div></div>
    <div className="chat-dialog-field"><span>{t("chat.saveLocation")}</span><div><span>{editSaved||t("chat.defaultLocation")}</span><Button disabled={busy} onClick={()=>void run(async()=>{const path=await pickDirectory();if(path)setEditSaved(path);})}>{t("chat.chooseLocation")}</Button></div><p>{t("chat.storageHint")}</p></div>
   </>:dialog==="project"?<ChatProjectFields mode={projectMode} onMode={setProjectMode} name={name} onName={setName} directory={parent} onChooseDirectory={()=>void run(chooseProjectDirectory)} busy={busy} permission={projectPermission} onPermission={setProjectPermission}/>:<label className="chat-dialog-field">{t("chat.newName")}<input autoFocus value={name} onChange={event=>setName(event.target.value)}/></label>}
   </>}
   {dialog==="project"&&error&&<div role="alert" className="chat-error">{error}</div>}
   <DialogFooter><Button disabled={busy} onClick={()=>setDialog(null)}>{t("chat.cancel")}</Button><Button variant="primary" disabled={busy||!["locations","delete","removeProject"].includes(dialog??"")&&(!name.trim()||dialog==="project"&&!parent)} onClick={()=>void run(async()=>{if(dialog==="locations"){await saveLocations();return;}if(dialog==="delete"&&dialogConversation){await draftWrites.current;const updated=await chatCall<ChatCatalog>("conversation.delete",{id:dialogConversation.id});setCatalog(updated);if(activeId===dialogConversation.id)await choose("");}else if(dialog==="project"){await saveProject();}else if((dialog==="renameProject"||dialog==="removeProject")&&dialogProject){await chatCall(dialog==="renameProject"?"project.update":"project.remove",{id:dialogProject.id,...(dialog==="renameProject"?{name}:{})});if(dialog==="removeProject"&&newProjectId===dialogProject.id)setNewProjectId("");await refresh();}else await updateConversation({title:name},dialogConversation);setDialog(null);})}>{t(dialog==="project"?(projectMode==="open"?"chat.openProject":"chat.create"):dialog==="locations"?"chat.apply":dialog==="delete"?"chat.delete":dialog==="removeProject"?"chat.remove":"chat.rename")}</Button></DialogFooter></DialogContent></Dialog>
 </div></ChatFileNavigation></ProcessNavigationProvider></ChatDesktopSurface>;
}
