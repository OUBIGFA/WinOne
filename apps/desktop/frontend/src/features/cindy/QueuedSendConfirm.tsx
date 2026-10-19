// Cindy confirm-dialog-provider presentation (779b5237, Apache-2.0), composed
// with the shared Base UI dialog. The parent owns the immutable send intent.
import {MessageCircleMore} from 'lucide-react';
import {Dialog,DialogContent,DialogTitle,DialogDescription,DialogFooter} from '../../components/ui/dialog';
import {Button} from '../../ui/button';
import {useTranslation} from '../../i18n';

export function QueuedSendConfirm({open,text,onCancel,onConfirm,onSteer}:{open:boolean;text:string;onCancel:()=>void;onConfirm:()=>void;onSteer:()=>void}){
 const {t}=useTranslation();
 return <Dialog open={open} onOpenChange={value=>{if(!value)onCancel();}}><DialogContent className="chat-send-confirm">
  <DialogTitle className="flex items-center gap-2"><MessageCircleMore size={18}/>{t('chat.confirmFollowup')}</DialogTitle>
  <DialogDescription>{t('chat.confirmFollowupHint')}</DialogDescription>
  {text&&<div className="chat-send-preview">{text}</div>}
  <DialogFooter><Button autoFocus variant="ghost" onClick={onCancel}>{t('chat.keepEditing')}</Button><Button onClick={onConfirm}>{t('chat.queueSend')}</Button><Button variant="primary" onClick={onSteer}>{t('chat.steerSend')}</Button></DialogFooter>
 </DialogContent></Dialog>;
}
