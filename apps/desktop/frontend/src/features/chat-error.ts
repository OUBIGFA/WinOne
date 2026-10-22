// ErrorBanner/networkError adaptation from Cindy a44ac274 (Apache-2.0).
// Classification only changes presentation; it never starts retries or changes
// input receipts. Unknown diagnostics remain available in the collapsed details.
import {isNetworkishErrorMessage} from './cindy/networkError';
export type ChatErrorKind='network'|'timeout'|'auth'|'rateLimit'|'cancelled'|'unsupported'|'unknown';
export function chatErrorKind(message:string):ChatErrorKind{
 if(/\b(?:401|403)\b|invalid[_ ]api[_ ]key|missing bearer|authentication (?:failed|required)|token (?:expired|revoked)/i.test(message))return 'auth';
 if(/\b429\b|rate[_ -]?limit|too many requests|quota (?:exceeded|exhausted)|insufficient_quota/i.test(message))return 'rateLimit';
 if(/ETIMEDOUT|\btimed?\s*out\b|\btimeout\b|超时/i.test(message))return 'timeout';
 if(isNetworkishErrorMessage(message)||/socket connection was closed unexpectedly|failed to fetch|network request failed|\bUND_ERR_SOCKET\b/i.test(message))return 'network';
 if(/\b(?:AbortError|aborted|cancelled|canceled)\b/i.test(message))return 'cancelled';
 if(/不支持的模块操作|\(unsupported\)|unsupported (?:module|chat) (?:operation|command)/i.test(message))return 'unsupported';
 return 'unknown';
}
export function chatErrorSummary(message:string,t:(key:string)=>string):string{
 // Keep translated product guidance in either language (including a useful
 // local prefix followed by a raw Host error). It is already actionable and
 // must not be buried behind generic diagnostics, especially save barriers.
 for(const key of ['saveFileBeforeNavigate','requestedConversationMissing','browserDraftBusy','sendCancelled','fileLinkMissing','fileLinkUnavailable','fileReadFailed','copyFailed','saveFailed','draftLoadFailed']){
  const hint=t('chat.'+key);
  if(hint&&hint!=='chat.'+key&&(message===hint||message.startsWith(hint+': ')))return hint;
 }
 const kind=chatErrorKind(message);
 // Product-authored Chinese instructions are already actionable. Do not replace
 // a specific path/save conflict with a less useful generic sentence.
 if(kind==='unknown'&&/[\u3400-\u9fff]/.test(message))return message;
 return t('chat.error_'+kind);
}
