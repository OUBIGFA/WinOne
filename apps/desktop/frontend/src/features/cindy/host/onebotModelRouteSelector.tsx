// Cindy BotModelChainEditor uses the same local model catalog and picker as
// WinOne Chat/automations. Only Pi/local routes exist here; a full selection
// is emitted atomically so changing a provider cannot save a mixed route.
import {useEffect,useState,type ComponentProps} from 'react';
import {ChevronDown} from 'lucide-react';
import {useTranslation} from 'react-i18next';
import {Popover,PopoverContent,PopoverTrigger} from '../panel-ui/popover';
import {ModelSelectorContent} from './modelSelector';
import {useProviders} from './providers';
import {AIModelIcon} from '../../ai-model-icon';

type ContentProps=ComponentProps<typeof ModelSelectorContent>;
type Props=Pick<ContentProps,'vendorKey'|'modelId'|'effort'|'currentProviderId'|'onModelChange'|'onEffortChange'|'fastMode'|'onFastModeChange'|'onUnifiedSelect'|'onNavigateToProviders'|'unifiedAgents'> & {
 disabled?:boolean;ariaContext?:string;unknownModelLabel?:(model:string)=>string;
 deviceId?:string;agentDeviceId?:string|null;sshRemote?:boolean;
 agentIdentity?:unknown;triggerVariant?:string;popoverSide?:'top'|'bottom';
 excludeSubscriptionDirect?:boolean;excludeChatBridgedCodex?:boolean;configurationEnabled?:boolean;unifiedPanel?:boolean;
};
export function ModelRouteSelector(props:Props){
 const {t}=useTranslation(),{providers}=useProviders();
 const [open,setOpen]=useState(false);
 const provider=providers.find(p=>props.currentProviderId?p.id===props.currentProviderId:p.models.pi?.some(m=>m.id===props.modelId));
 const model=provider?.models.pi?.find(m=>m.id===props.modelId);
 const label=model?.name||(props.modelId?props.unknownModelLabel?.(props.modelId)??props.modelId:t('chat.model'));
 const unavailable=!!props.deviceId||!!props.agentDeviceId||props.sshRemote||props.vendorKey!=='pi';
 const disabled=props.disabled||unavailable;
 // A paused/archived editor ends this interaction. Re-enabling it must not
 // reopen a portal left over from the previous editable state.
 useEffect(()=>{if(disabled)setOpen(false);},[disabled]);
 // Effort values belong to the provider, not WinOne's old fixed level labels.
 const description=[provider?.name??props.currentProviderId,label,props.effort,props.fastMode?t('chat.fastMode'):''].filter(Boolean).join(' · ');
 return <Popover open={open&&!disabled} onOpenChange={value=>setOpen(!disabled&&value)}>
  <PopoverTrigger asChild><button type="button" disabled={disabled}
   aria-label={[props.ariaContext,description].filter(Boolean).join(': ')} title={description}
   className="inline-flex max-w-full min-w-0 items-center gap-1.5 rounded-lg border border-[var(--border-default)] px-2 py-1 text-12 text-[var(--text-primary)] disabled:opacity-50">
   <AIModelIcon model={props.modelId} name={model?.name}/><span className="truncate">{description}</span><ChevronDown size={13} className="shrink-0"/>
  </button></PopoverTrigger>
  <PopoverContent align="end" side={props.popoverSide??'bottom'} collisionPadding={16}
   className="cindy-chat-ui z-[10010] w-auto max-w-[calc(100vw-32px)] overflow-hidden p-0">
   <ModelSelectorContent {...props} fastModeConfigurable={['pi']} onDismiss={()=>setOpen(false)}
    onUnifiedSelect={props.onUnifiedSelect?selection=>{if(disabled)return false;return props.onUnifiedSelect!(selection);}:undefined}
    onEffortChange={effort=>{if(!disabled)props.onEffortChange(effort);}}
    onFastModeChange={fast=>{if(!disabled)props.onFastModeChange?.(fast);}}/>
  </PopoverContent>
 </Popover>;
}
