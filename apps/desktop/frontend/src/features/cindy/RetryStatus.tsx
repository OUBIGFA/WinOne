// Cindy ErrorBanner/CCAgentSessionView (a44ac274), Apache-2.0,
// Copyright 2026 XD Inc. Adapt its independent composer-side retry notice to
// Pi's structured operation records. Never parse a model reply for status, and
// never key/remount the work process when attempts or countdowns change.
import {useEffect,useState} from 'react';
import {RotateCcw,Square} from 'lucide-react';
import {useTranslation} from '../../i18n';
import {Button} from '../../ui/button';
import type {ProcessOperation} from '../chat-state';

export function RetryStatus({operation,onStop,stopping=false}:{operation:ProcessOperation;onStop:()=>void;stopping?:boolean}){
 const {t}=useTranslation(),[now,setNow]=useState(Date.now);
 const until=operation.startedAt+(operation.delayMs??0);
 useEffect(()=>{setNow(Date.now());if(until<=Date.now())return;const timer=setInterval(()=>{const time=Date.now();setNow(time);if(time>=until)clearInterval(timer);},250);return()=>clearInterval(timer);},[until]);
 const seconds=Math.max(0,Math.ceil((until-now)/1000));
 return <div className="chat-retry-status" role="status">
  <RotateCcw size={15} aria-hidden/>
  <span>{t(seconds?'chat.retryCountdown':'chat.retryConnecting',{seconds})}{operation.attempt!==undefined&&<> · {operation.attempt}{operation.maxAttempts!==undefined?` / ${operation.maxAttempts}`:''}</>}</span>
  <Button variant="ghost" size="sm" icon={Square} disabled={stopping} onClick={onStop}>{t('chat.stop')}</Button>
 </div>;
}
