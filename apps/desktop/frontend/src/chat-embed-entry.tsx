// The Chat page's React tools inside the MyGo window. The MyGo window draws
// Chat natively and shows this page in its web views for what stays React
// (the user's hybrid plan): the side panel with its library, editors, PDF,
// review, terminal and browser tabs; the automations page; Chat settings;
// the OneBot page (Cindy's teammates). `?view=` chooses which. The window binds `App`, the methods the Rust
// window's bridge offered (window.go.main.App), and tells this page what
// to show with `winone:embed` events; what the page asks of the Chat page
// goes back through App.Embed.
import {Component,lazy,Suspense,useEffect,useState,type ReactNode} from 'react';
import {createRoot} from 'react-dom/client';
import {CSPProvider} from '@base-ui/react/csp-provider';
import {setApplicationLanguage,useTranslation} from './i18n';
import './styles/shadcn.css';
import './style.css';
import './features/chat.css';
import './features/cindy/theme.css';
import './features/cindy/message-tokens.css';
import './features/cindy/reading.css';
import './chat-embed.css';
import {ChatDesktopSurface} from './features/chat-desktop-kit';
import {Notice} from './ui/notice';
import {MediaProvider} from './features/cindy/media/MediaProvider';
import {ProcessNavigationProvider} from './features/process-navigation';
import {ChatSidePanel} from './features/chat-side-panel';
import {ChatSettings} from './features/chat-settings';
import {SchedulerHost} from './features/cindy/host/SchedulerHost';
import {publishCindyHost,setCindyNavigation,type CindyHostSnapshot} from './features/cindy/host/store';
import type {AIProvider,AISelection} from './features/ai-configuration';
import type {ChatBrowserAnnotation} from './features/chat-types';
// Loaded only by the OneBot view: the teammate store reads profiles as
// soon as its module loads, as main.tsx loaded it on first use.
const OneBotPage=lazy(()=>import('./features/onebot').then(module=>({default:module.OneBotPage})));

/** What the window tells every view: its look, and the Chat data Cindy's
 * hooks read (publishCindyHost). */
interface EmbedCommon{language:string;theme:'light'|'dark';host:Partial<CindyHostSnapshot>}
/** The side panel's props, as chat.tsx gave ChatSidePanel. `key` remounts it
 * for another conversation or folder, as React's key did. */
interface PanelProps{key:string;id:string;cwd:string;initialTab:'home'|'files'|'resources'|'review';initialTabRequest:number;initialDirectory?:string;initialURL?:string;initialFile?:string;initialLine?:number;initialReview?:{turnId:string;path?:string};dock:'right'|'bottom';visible:boolean;running:boolean;lastTurnPaths:string[];commentMarkerNumbers:number[];
 /** The native composer cannot take text now (busy, opening, loading its draft). */
 composerLocked:boolean}
interface SettingsProps{providers:AIProvider[];defaultModel:AISelection;modelsLoading:boolean;modelsError:string}
type EmbedState=EmbedCommon&{panel?:PanelProps;settings?:SettingsProps};

const view=new URLSearchParams(location.search).get('view')??'panel';
// The window's App.Embed: the page's requests of the native Chat page. It is
// bound for this page only, beside the methods bridge.ts declares.
type EmbedApp={Embed(kind:string,payload:string):Promise<string>};
function ask<T=unknown>(kind:string,value?:unknown):Promise<T>{
 const app=window.go?.main?.App as unknown as EmbedApp|undefined;
 if(!app)return Promise.reject(Error('WinOne is not available'));
 return app.Embed(kind,JSON.stringify(value??null)).then(text=>text?JSON.parse(text) as T:undefined as T);
}
const tell=(kind:string,value?:unknown)=>{void ask(kind,value).catch(console.error);};

// Links into the window, as main.tsx took them: a Chat conversation (at a
// file of it) or a page by id (Chat settings, AI services, OneBot). The
// native window follows them through its unsaved-change guard.
window.addEventListener('winone-open-chat',event=>{
 const detail=(event as CustomEvent<{id?:unknown;file?:{path?:unknown;line?:unknown;directory?:unknown}}>).detail;
 if(typeof detail?.id!=='string'||!detail.id)return;
 const file=detail.file,path=file&&typeof file.path==='string'?file.path:'';
 tell('navigate',{to:'conversation',id:detail.id,file:path?{path,line:typeof file!.line==='number'&&Number.isSafeInteger(file!.line)&&file!.line>0?file!.line:0,directory:file!.directory===true}:undefined});
});
window.addEventListener('winone-open-page',event=>{
 const detail=(event as CustomEvent<{id?:unknown;section?:unknown}>).detail;
 if(typeof detail?.id==='string')tell('navigate',{to:'page',id:detail.id,section:typeof detail.section==='string'?detail.section:''});
});
const openConversation=(id:string)=>tell('navigate',{to:'conversation',id});
const openPage=(id:string,section='')=>tell('navigate',{to:'page',id,section});

function applyLook(state:EmbedCommon){
 const root=document.documentElement;
 root.dataset.theme=state.theme;root.classList.toggle('dark',state.theme==='dark');
 void setApplicationLanguage(state.language==='en-US'?'en-US':'zh-CN');
 publishCindyHost(state.host);
}

function useEmbedState():EmbedState|undefined{
 const [state,setState]=useState<EmbedState>();
 useEffect(()=>{
  let alive=true;
  const receive=(event:Event)=>{const next=(event as CustomEvent<EmbedState>).detail;applyLook(next);setState(next);};
  window.addEventListener('winone:embed',receive);
  // The first state answers ready; later ones arrive as events.
  void ask<EmbedState>('ready',{view}).then(first=>{if(alive&&first){applyLook(first);setState(current=>current??first);}}).catch(console.error);
  return()=>{alive=false;window.removeEventListener('winone:embed',receive);};
 },[]);
 return state;
}

/** A view that fails to render says why, in place of a blank web view. */
class Failure extends Component<{children:ReactNode},{error?:string}>{
 state:{error?:string}={};
 static getDerivedStateFromError(error:unknown){return {error:error instanceof Error?error.message:String(error)};}
 render(){return this.state.error?<div role="alert" className="chat-embed-failure"><Notice>{this.state.error}</Notice></div>:this.props.children;}
}

function Surface({children}:{children:ReactNode}){
 return <ChatDesktopSurface><div className="chat-embed-root"><Failure>{children}</Failure></div></ChatDesktopSurface>;
}

function Panel({props}:{props:PanelProps}){
 const {t}=useTranslation();
 // A review comment the native composer cannot take now throws, as
 // chat.tsx's handler did, so the review keeps the comment.
 const reviewComment=(text:string)=>{if(props.composerLocked)throw Error(t('chat.browserDraftBusy'));tell('reviewComment',text);};
 return <MediaProvider id={props.id} cwd={props.cwd}><ProcessNavigationProvider><ChatSidePanel key={props.key}
  id={props.id} initialTab={props.initialTab} initialTabRequest={props.initialTabRequest} initialDirectory={props.initialDirectory}
  initialURL={props.initialURL} onURLHandled={()=>tell('urlHandled')} initialFile={props.initialFile} initialLine={props.initialLine}
  initialReview={props.initialReview} dock={props.dock} visible={props.visible} running={props.running} lastTurnPaths={props.lastTurnPaths}
  commentMarkerNumbers={props.commentMarkerNumbers}
  onClose={()=>tell('close')} onDirtyChange={value=>tell('dirty',value)} onDock={()=>tell('dock')}
  onReference={path=>tell('reference',path)} onReviewComment={reviewComment}
  onComment={(item:ChatBrowserAnnotation)=>ask('comment',item).then(()=>{})}/></ProcessNavigationProvider></MediaProvider>;
}

function Embed(){
 const state=useEmbedState();
 // Cindy's links to a conversation, AI services or IM settings go to the
 // native window, which navigates through its own unsaved-change guard.
 useEffect(()=>setCindyNavigation({
  openConversation,
  openAIServices:()=>openPage('ai-services'),
  openIMSettings:()=>openPage('onebot','im'),
 }),[]);
 if(!state)return null;
 if(view==='automations')return <Surface><SchedulerHost providers={state.host.providers??null} defaultModel={state.host.defaultModel?.model?state.host.defaultModel:null}
  onOpenConversation={openConversation} onOpenAIServices={()=>openPage('ai-services')}
  onOpenIMSettings={()=>openPage('onebot','im')}/></Surface>;
 if(view==='settings'&&state.settings)return <Surface><ChatSettings open onOpenChange={open=>{if(!open)tell('close');}}
  providers={state.settings.providers} defaultModel={state.settings.defaultModel} modelsLoading={state.settings.modelsLoading}
  modelsError={state.settings.modelsError} onSaved={()=>tell('saved')} onConversation={openConversation}/></Surface>;
 // OneBot sets its own Cindy navigation, which raises the window events above.
 if(view==='onebot')return <Surface><Suspense fallback={null}><OneBotPage/></Suspense></Surface>;
 if(view==='panel'&&state.panel)return <Surface><Panel props={state.panel}/></Surface>;
 return null;
}

createRoot(document.getElementById('root')!).render(<CSPProvider disableStyleElements><Embed/></CSPProvider>);
