import {useState} from "react";
import {Download,Eye,RefreshCw} from "lucide-react";
import {captureCall} from "../bridge";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {Notice} from "../ui/notice";

type ArchivePart={body:string;headers:Record<string,string[]>;size?:number;truncated?:boolean;omitted?:boolean};
type ArchivePreview={id?:string;bytes:number;large?:boolean;request?:ArchivePart;response?:ArchivePart};

// Bodies load only after the user asks. The server keeps Magpie's preview
// limits; a native export streams the complete file without a UI payload.
export function RequestArchive({archive}:{archive:string}){
 const {t}=useTranslation(),[record,setRecord]=useState<ArchivePreview>(),[busy,setBusy]=useState(false),[error,setError]=useState(''),[saved,setSaved]=useState(''),[failedAction,setFailedAction]=useState<'read'|'save'>('read');
 async function read(){setBusy(true);setError('');try{setRecord(await captureCall<ArchivePreview>('ai.archive.read',{archive}));}catch(error){setFailedAction('read');setError(String(error));}finally{setBusy(false);}}
 async function save(){setBusy(true);setError('');setSaved('');try{const receipt=await captureCall<{state:string;path?:string}>('ai.archive.export',{archive});if(receipt.state==='saved'&&receipt.path)setSaved(receipt.path);else if(receipt.state!=='cancelled')throw Error(t('usage.archiveUnconfirmed'));}catch(error){setFailedAction('save');setError(String(error));}finally{setBusy(false);}}
 return <section className="mt-4 min-w-0 space-y-3">
  <div className="flex flex-wrap gap-2"><Button icon={Eye} disabled={busy} onClick={()=>void read()}>{t('usage.archiveView')}</Button><Button icon={Download} disabled={busy} onClick={()=>void save()}>{t('usage.archiveExport')}</Button></div>
  {busy&&<p role="status">{t('usage.archiveLoading')}</p>}
  {error&&<Notice actions={<Button icon={RefreshCw} disabled={busy} onClick={()=>void (failedAction==='save'?save():read())}>{t('common.retry')}</Button>}>{error}</Notice>}
  {saved&&<p role="status" className="break-all">{saved}</p>}
  {record?.large&&<p className="settings-help">{t('usage.archiveLarge')}</p>}
  {record&&(['request','response'] as const).map(key=>{const part=record[key];if(!part)return null;return <div key={key} className="min-w-0 space-y-2"><h4 className="font-medium">{t('usage.archive'+key)}</h4>
   {part.truncated&&<p className="settings-help">{t('usage.archiveTruncated')}</p>}
   {part.omitted?<p className="settings-help">{t('usage.archiveLarge')}</p>:<pre className="max-h-80 overflow-y-auto whitespace-pre-wrap text-sm [overflow-wrap:anywhere]">{part.body}</pre>}
   {Object.keys(part.headers).length>0&&<div><p>{t('usage.archiveHeaders')}</p><pre className="max-h-60 overflow-y-auto whitespace-pre-wrap text-sm [overflow-wrap:anywhere]">{JSON.stringify(part.headers,null,2)}</pre></div>}
  </div>;})}
 </section>;
}
