import {useState,type ReactNode} from 'react';
import {ChevronRight,CodeXml,Copy,Eye,FileText} from 'lucide-react';
import {Button} from '../ui/button';
import {useTranslation} from '../i18n';
import {ChatMarkdown} from './chat-markdown';
import {ChatFileImage,ChatFileNavigation,useChatFileNavigation} from './chat-file-navigation';
import {resolveToolFilePath} from './cindy/localPathResolver';
import {MarkdownDiffBlock} from './cindy/markdown/MarkdownDiffBlock';
import {toolFileContent} from './chat-tool-content';

/** Cindy's file-vs-command split, using its actual Markdown/Diff renderers.
 * The operation snapshot is never replaced by today's file. A separate explicit
 * action opens the current file in the complete workbench editor.
 */
export function ChatToolFilePreview({file,name,input,result,isError,running,raw,onOpenCurrent}:{file:string;name:string;input:Record<string,unknown>;result:unknown;isError?:boolean;running:boolean;raw:ReactNode;onOpenCurrent:()=>void}){
 const {t}=useTranslation(),navigation=useChatFileNavigation(),[source,setSource]=useState(false),[error,setError]=useState('');
 const snapshot=toolFileContent(name,input,result),path=resolveToolFilePath(file,navigation.cwd),markdown=/\.(md|markdown|mdown|mkd)$/i.test(file),image=/\.(png|jpe?g|gif|webp|bmp|svg)$/i.test(file)&&name.toLowerCase()==='read';
 const label=snapshot?t('chat.toolSnapshot_'+snapshot.source):t('chat.toolOutput');
 async function copy(){try{await navigator.clipboard.writeText(snapshot?.text??'');setError('');}catch{setError(t('chat.copyFailed'));}}
 return <div className="chat-tool-file-preview">
  <div className="chat-tool-file-actions"><span>{label}{isError?' · '+t('chat.failed'):running?' · '+t('chat.working'):''}{name.toLowerCase()==='read'&&(input.offset||input.limit)?' · '+t('chat.toolReadExcerpt'):''}</span>
   {snapshot?.kind==='document'&&markdown&&<Button variant="ghost" size="sm" icon={source?Eye:CodeXml} onClick={()=>setSource(value=>!value)}>{t(source?'chat.fileRendered':'chat.fileSource')}</Button>}
   {snapshot&&<Button variant="ghost" size="icon" icon={Copy} aria-label={t('chat.copy')} onClick={()=>void copy()}/>}
   {navigation.onOpen&&<Button variant="ghost" size="sm" icon={FileText} onClick={()=>{navigation.onOpen?.(path);onOpenCurrent();}}>{t('chat.openCurrentFile')}</Button>}
  </div>
  {error&&<p role="alert">{error}</p>}
  <ChatFileNavigation cwd={path.replace(/[\\/][^\\/]+$/,'')}>
   <div className="chat-tool-document">
    {image&&!isError?<ChatFileImage src={path} alt={file}/>:snapshot?.kind==='diff'?<MarkdownDiffBlock raw={snapshot.text}/>:snapshot?.kind==='document'?(markdown&&!source&&!isError?<ChatMarkdown text={snapshot.text}/>:<pre>{snapshot.text}</pre>):null}
   </div>
  </ChatFileNavigation>
  <details className="chat-tool-raw" open={!snapshot&&!image||undefined}><summary><ChevronRight size={14}/>{t('chat.rawToolData')}</summary>{raw}</details>
 </div>;
}
