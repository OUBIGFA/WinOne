// FileBodyView's real PlaintextEditor and floating document search/actions.
// WinOne owns the revision-checked save and preserves its source-view command.
import {useEffect,useRef,useState,type RefObject} from 'react';
import {CodeXml,Eye,Search,Save} from 'lucide-react';
import {useTranslation} from 'react-i18next';
import {FileEditor,type FileEditorHandle} from './FileEditor';
import {DocSearchBar} from './DocSearchBar';
import {ChromeIconButton} from '../panel-ui/ChromeIconButton';
export function FileDocument({id,path,value,markdown,rendered,readOnly,busy,dirty,line,editorRef,onChange,onSave,onToggleMode}:{id:string;path:string;value:string;markdown:boolean;rendered:boolean;readOnly?:boolean;busy:boolean;dirty:boolean;line?:number;editorRef:RefObject<FileEditorHandle|null>;onChange:(text:string)=>void;onSave:()=>Promise<boolean>;onToggleMode:()=>void}){
 const {t}=useTranslation(),[searchOpen,setSearchOpen]=useState(false),[query,setQuery]=useState(''),[total,setTotal]=useState(0),[active,setActive]=useState(0),searchInput=useRef<HTMLInputElement>(null);
 const ext=path.split('.').pop()?.toLowerCase()??'';
 const aliases:Record<string,string>={ts:'typescript',tsx:'tsx',js:'javascript',jsx:'jsx',py:'python',rs:'rust',yml:'yaml',cs:'csharp',ps1:'powershell',sh:'shell',c:'c',h:'cpp',htm:'html'};
 const language=markdown?(rendered?'markdown':'markdown-source'):(aliases[ext]??(/^(json|yaml|html|css|cpp|java|go|sql|xml|php|toml|lua|rb|swift)$/.test(ext)?ext:undefined));
 useEffect(()=>{if(searchOpen)searchInput.current?.focus();else editorRef.current?.search.clear();},[searchOpen,editorRef]);
 useEffect(()=>{if(!searchOpen)return;setTotal(editorRef.current?.search.findAll(query)??0);setActive(0);if(query)editorRef.current?.search.setActive(0);},[query,value,searchOpen,editorRef]);
 function move(direction:number){if(!total)return;const next=(active+direction+total)%total;setActive(next);editorRef.current?.search.setActive(next);}
 return <div className="cindy-file-document" onKeyDown={event=>{
  if((event.ctrlKey||event.metaKey)&&event.key.toLowerCase()==='f'){
   event.preventDefault();event.stopPropagation();setSearchOpen(true);
  }
 }}>
  <div className="cindy-file-floating-actions">{searchOpen?<DocSearchBar ref={searchInput} query={query} total={total} activeIndex={active} onChange={setQuery} onNext={()=>move(1)} onPrev={()=>move(-1)} onClose={()=>setSearchOpen(false)}/>:<>
   {dirty&&<span className="text-12 text-[var(--text-secondary)]">{t(busy?'chat.savingFile':'chat.unsavedFile')}</span>}
   <ChromeIconButton aria-label={t('chat.fileSearch')} onClick={()=>setSearchOpen(true)}><Search size={14}/></ChromeIconButton>
   {markdown&&<ChromeIconButton aria-label={t(rendered?'chat.fileSource':'chat.fileRendered')} onClick={onToggleMode}>{rendered?<CodeXml size={14}/>:<Eye size={14}/>}</ChromeIconButton>}
   {!readOnly&&dirty&&<ChromeIconButton aria-label={t('chat.saveFile')} disabled={busy} onClick={()=>void onSave()}><Save size={14}/></ChromeIconButton>}
  </>}</div>
  <FileEditor key={path+':'+language} ref={editorRef} id={id} path={path} value={value} language={language} readOnly={readOnly} line={line} onChange={onChange}/>
 </div>;
}
