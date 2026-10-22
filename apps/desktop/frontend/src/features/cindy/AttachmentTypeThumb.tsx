// AttachmentTypeThumb adapted from Cindy a152497a (Apache-2.0).
// WinOne uses its scoped preview URL for image content; the Host revalidates
// metadata on every mount. Other formats retain the file-type tile.
import {useEffect,useState} from 'react';
import {FileText} from 'lucide-react';
import {chatCall} from '../../bridge';
import type {ChatAttachment} from '../chat-types';
import type {ChatFilePreviewData} from '../chat-file-preview';
export function AttachmentTypeThumb({id,file}:{id:string;file:ChatAttachment}){
 const [source,setSource]=useState<string>();
 useEffect(()=>{if(!id||!file.mimeType.startsWith('image/'))return;let alive=true;void chatCall<ChatFilePreviewData>('file.preview',{id,path:file.path}).then(result=>{if(alive&&result.kind==='image')setSource(result.url);}).catch(()=>{if(alive)setSource(undefined);});return()=>{alive=false;};},[id,file.path,file.mimeType]);
 return file.path.startsWith('blob:')&&file.mimeType.startsWith('image/')?<img src={file.path} alt="" draggable={false} className="cindy-attachment-image"/>:source?<img src={source} alt="" draggable={false} className="cindy-attachment-image"/>:<FileText size={24}/>;
}
