// Cindy's declarative answer renderer shares WinOne's existing Markdown,
// scoped file previews and media elements. No generated HTML or script runs.
import {useEffect,useMemo,useState} from 'react';
import {chatCall} from '../bridge';
import {MarkdownRenderer as PlainMarkdown,type MarkdownRendererProps} from './cindy/markdown/MarkdownRenderer';
import {RichAnswer} from './cindy/rich/RichAnswer';
import './cindy/host/onebotLocales';
import {splitRichAnswer,richMediaSource,youtubeVideo,type RichNode} from '../../../../../third_party/cindy/maker-shared/richAnswer';
import {ChatFileImage,useChatFileNavigation} from './chat-file-navigation';
import {ChatSessionFileProvider} from './cindy/host/onebotTaskFileContext';
import type {ChatFilePreviewData} from './chat-file-preview';
import {useTranslation} from '../i18n';

type Props=MarkdownRendererProps&{workingDir?:string;currentSessionId?:string;renderRichAnswers?:boolean};
/** Questions use the same file-aware Markdown and media previews as Chat. */
export function QuestionReferenceText({text,onPreviewOpenChange}:{text:string;onPreviewOpenChange?:(open:boolean)=>void}){
 useEffect(()=>{const listener=(event:Event)=>onPreviewOpenChange?.(!!(event as CustomEvent).detail);window.addEventListener('winone:media-overlay',listener);return()=>{window.removeEventListener('winone:media-overlay',listener);onPreviewOpenChange?.(false);};},[onPreviewOpenChange]);
 return <MarkdownRenderer content={text}/>;
}
export function MarkdownRenderer({workingDir,currentSessionId,renderRichAnswers=false,...props}:Props){
 const context=useChatFileNavigation();
 const parts=useMemo(()=>renderRichAnswers?splitRichAnswer(props.content):null,[props.content,renderRichAnswers]);
 const body=parts?.some(part=>part.kind==='rich')?<div className="cindy-chat-ui">
  {parts.map(part=>part.kind==='markdown'?<PlainMarkdown key={part.key} {...props} content={part.source}/>:<RichAnswer key={part.key} source={part.source} complete={part.complete&&!props.isStreaming} stateKey={`${currentSessionId??context.id}:${props.streamFadeKey??'message'}:${part.key}`} renderMarkdown={(text,id)=><PlainMarkdown {...props} content={text} streamFadeKey={`${props.streamFadeKey??'rich'}:${id}`}/>} renderMedia={node=><RichMedia node={node}/>}/>)}
 </div>:<PlainMarkdown {...props}/>;
 return workingDir!==undefined||currentSessionId!==undefined?<ChatSessionFileProvider value={{workingDir:workingDir??context.cwd,sessionId:currentSessionId??context.id}}>{body}</ChatSessionFileProvider>:body;
}
function RichMedia({node}:{node:RichNode}){
 const {t}=useTranslation(),context=useChatFileNavigation(),source=richMediaSource(node)??'',label=String(node.props.alt??node.props.label??node.props.caption??node.kind);
 const youtube=node.kind==='video'?youtubeVideo(source):null;
 const [url,setURL]=useState(''),[error,setError]=useState('');
 useEffect(()=>{
  if(node.kind==='image'||youtube)return;
  let alive=true;setURL('');setError('');
  if(/^https?:\/\//i.test(source)){
   try{const url=new URL(source);if(url.username||url.password)throw Error(t('chat.richMediaAddress'));setURL(url.href);}catch{setError(t('chat.richMediaAddress'));}
   return;
  }
  if(!context.id){setError(t('chat.fileLinkUnavailable'));return;}
  void chatCall<ChatFilePreviewData>('file.preview',{id:context.id,path:source}).then(file=>{
   if(!alive)return;if(file.kind!==node.kind)throw Error(t('chat.richMediaType'));setURL(file.url);
  }).catch(error=>{if(alive)setError(String(error instanceof Error?error.message:error));});
  return()=>{alive=false;};
 },[source,node.kind,context.id,youtube?.id,t]);
 if(node.kind==='image')return <ChatFileImage src={source} alt={label}/>;
 if(youtube)return <iframe src={youtube.embed} title={label} loading="lazy" allow="fullscreen; picture-in-picture" allowFullScreen referrerPolicy="strict-origin-when-cross-origin" className="aspect-video w-full border-0"/>;
 if(error)return <p role="alert">{error}</p>;
 if(!url)return <p role="status">{t('chat.loading')}</p>;
 // preload=none avoids downloading arbitrary media merely by scrolling past it.
 return node.kind==='audio'?<audio controls preload="none" crossOrigin="anonymous" src={url} aria-label={label} className="w-full" onError={()=>setError(t('chat.richMediaFailed'))}/>:<video controls playsInline preload="none" crossOrigin="anonymous" src={url} aria-label={label} className="max-h-[70vh] w-full" onError={()=>setError(t('chat.richMediaFailed'))}/>;
}
