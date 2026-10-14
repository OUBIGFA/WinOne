import {useEffect,useRef,useState} from "react";
import {captureCall} from "../bridge";
import {useTranslation} from "../i18n";
import {Button} from "../ui/button";
import {Notice} from "../ui/notice";
import {Input} from "../components/ui/input";
import {Field,FieldLabel} from "../components/ui/field";
import {CaptureOptionSelect} from "../ui/capture-option-select";
import type {AIAgent} from "./ai-configuration";
import {AIProviderIcon} from "./ai-provider-icon";
import type {AccountReader,AccountState} from "./ai-account-reader";
import {LogIn,RefreshCw,ExternalLink,Check,X,Copy} from "lucide-react";

type Account={agent:string;user:string;plan?:string;lapsed?:string;icon?:string};
type SignInMethod={type:"oauth"|"api";label:string;placeholder?:string};
type SignInPrompt={type:"text"|"select"|"password";key:string;message:string;placeholder?:string;options?:{label:string;value:string;hint?:string}[]};
type SignIn={id:string;status:{agent:string;url:string;code?:string;state:string;user?:string;error?:string;pasteCallback?:boolean;pasteCode?:boolean;pasteKey?:boolean;keysURL?:string;instructions?:string;prompt?:SignInPrompt};inputError?:string;inputPending?:boolean};
const running=(job:SignIn|undefined)=>!!job&&!['done','failed','canceled'].includes(job.status.state);

// A login belongs to this settings window; closing/unmounting cancels it. The
// vendor's browser page handles passwords, not a WinOne credential text field.
export function AIAccounts({agent,provider,draft,disabled,onBusy,onLoading,catalog,reader}:{onLoading?:(loading:boolean)=>void;reader?:AccountReader;catalog?:AIAgent[];agent:string;provider?:string;draft?:{agent:string;agentBackend?:string};disabled:boolean;onBusy:(busy:boolean)=>void}){
 const {t}=useTranslation();const [loadedAgents,setLoadedAgents]=useState<AIAgent[]>([]);
 // A supplied catalog belongs to the parent, including its initial empty
 // loading state. Fetching it again races the Host's account-operation gate.
 const agents=catalog??loadedAgents;
 const accountType=agents.find(value=>value.id===agent);
 const [accounts,setAccounts]=useState<Account[]>([]);
 const [methods,setMethods]=useState<SignInMethod[]>([]),[method,setMethod]=useState(-1);
 const [job,setJob]=useState<SignIn>();const [loading,setLoading]=useState(false);const [error,setError]=useState("");const [callback,setCallback]=useState("");
 const [action,setAction]=useState("");const [copied,setCopied]=useState("");const [choiceMade,setChoiceMade]=useState(false);
 const [starting,setStarting]=useState(false);
 const current=useRef<SignIn|undefined>(undefined);const opened=useRef("");const active=running(job);const loadSequence=useRef(0);const generation=useRef(0);const busyHandler=useRef(onBusy);
 const updateJob=(value:SignIn|undefined)=>{current.current=value;setJob(value);};
 // Account discovery is read-only; only an authorization in progress guards
 // navigation. The local login/refresh buttons still wait for their own read.
 useEffect(()=>{busyHandler.current=onBusy;onBusy(active||starting);},[active,starting,onBusy]);
 // Model discovery shares this provider's backend gate; it waits locally for
 // discovery without disabling the tab list or other providers.
 useEffect(()=>{onLoading?.(loading);return()=>onLoading?.(false);},[loading,onLoading]);
 async function loadAccounts(refresh=false){const sequence=++loadSequence.current;setLoading(true);setError("");try{
  if(catalog===undefined&&!loadedAgents.length){const value=await captureCall<{agents:AIAgent[]}>("ai.accounts.catalog",{});if(sequence!==loadSequence.current)return;setLoadedAgents(value.agents);}
  const request={agent,...(provider?{provider}:{}),...(draft?{draft}:{})};
  const value=await (reader?reader.read(request,refresh):captureCall<AccountState>("ai.accounts.list",request));if(sequence!==loadSequence.current)return;setAccounts(value.accounts??[]);const choices=value.methods??[];setMethods(choices);setMethod(current=>current>=0&&current<choices.length?current:Math.max(0,choices.findIndex(choice=>choice.type==="oauth")));
 }catch(e){if(sequence===loadSequence.current)setError(String(e));}finally{if(sequence===loadSequence.current)setLoading(false);}}
 // Connection identity owns authorization. A new onBusy callback does not;
 // late start replies are canceled even if the panel closed before they arrived.
 useEffect(()=>{generation.current++;updateJob(undefined);setAccounts([]);setMethods([]);setMethod(-1);setStarting(false);setLoading(false);setAction("");setCopied("");if(agent)void loadAccounts();return()=>{generation.current++;loadSequence.current++;const owned=current.current;current.current=undefined;if(running(owned))void captureCall("ai.accounts.signin.cancel",{id:owned!.id}).catch(()=>{});busyHandler.current(false);};},[agent,provider,draft?.agent,draft?.agentBackend]);
 useEffect(()=>{setCallback("");setChoiceMade(false);},[job?.id,job?.status.prompt?.key,active]);
 useEffect(()=>{if(!job||!active)return;let live=true;let timer:ReturnType<typeof setTimeout>;
  const poll=async()=>{try{const next=await captureCall<SignIn>("ai.accounts.signin.get",{id:job.id});if(!live)return;updateJob(next);
   const pageKey=`${next.id}:${next.status.url}`;
   if(running(next)&&next.status.url&&opened.current!==pageKey){opened.current=pageKey;try{await captureCall("ai.accounts.signin.open",{id:next.id});}catch{if(live)setError(t("ai.openLoginFailed"));}}
   if(next.status.state==='done'){void loadAccounts(true);return;}
   if(running(next))timer=setTimeout(()=>void poll(),400);
  }catch(e){if(live){setError(String(e));const latest=current.current??job;updateJob({...latest,status:{...latest.status,state:'failed'}});void captureCall("ai.accounts.signin.cancel",{id:job.id}).catch(()=>{});}}};
  timer=setTimeout(()=>void poll(),200);return()=>{live=false;clearTimeout(timer);};
 },[job?.id,active]);
 async function signIn(){const owner=generation.current;reader?.invalidate();setError("");setStarting(true);setLoading(true);setCallback("");setCopied("");try{const next=await captureCall<SignIn>("ai.accounts.signin.start",{agent,...(provider?{provider}:{}),...(draft?{draft}:{}),...(methods.length?{method}:{})});if(owner!==generation.current){void captureCall("ai.accounts.signin.cancel",{id:next.id}).catch(()=>{});return;}updateJob(next);}catch(e){if(owner===generation.current)setError(String(e));}finally{if(owner===generation.current){setLoading(false);setStarting(false);}}}
 async function cancel(){if(!job||action)return;const owner=generation.current;setAction("cancel");try{const next=await captureCall<SignIn>("ai.accounts.signin.cancel",{id:job.id});if(owner===generation.current)updateJob(next);}catch(e){if(owner===generation.current)setError(String(e));}finally{if(owner===generation.current)setAction("");}}
 async function submit(){if(!job||!canSubmit||inputBusy)return;const owner=generation.current;setAction("submit");setError("");try{const next=await captureCall<SignIn>("ai.accounts.signin.submit",{id:job.id,callback});if(owner===generation.current&&next?.id===job.id&&next.status)updateJob(next);}catch(e){if(owner===generation.current)setError(String(e));}finally{if(owner===generation.current)setAction("");}}
 async function copy(value:string,kind:string){setError("");try{await navigator.clipboard.writeText(value);setCopied(kind);}catch{setError(t("ai.copySignInFailed"));}}
 const prompt=job?.status.prompt;const inputBusy=!!job?.inputPending||!!action;
 const promptReady=prompt?.type==="text"|| (prompt?.type==="select"?choiceMade&&!!prompt.options?.some(option=>option.value===callback):!!callback.trim());
 const acceptsPaste=!!(job?.status.pasteCallback||job?.status.pasteCode||job?.status.pasteKey);
 const canSubmit=active&&(prompt?job?.status.state==="input"&&promptReady:job?.status.state==="waiting"&&acceptsPaste&&!!callback.trim());
 const hint=job?.status.state==='starting'?"ai.signInStarting":job?.status.state==='finishing'?"ai.signInFinishing":job?.status.code?"ai.deviceCodeHint":job?.status.pasteKey?"ai.authorizationKeyHint":job?.status.pasteCode?"ai.authorizationCodeHint":job?.status.pasteCallback?"ai.callbackHint":job?.status.url?"ai.signInBrowser":"ai.signInWaiting";
 return <section id="ai-account-connection" tabIndex={-1} className="settings-group ai-account-section outline-none" aria-label={t("ai.accountConnection")}>
  {/* The type was chosen when this connection was added. Keep it visible as
      identity; changing it here could pair another agent with existing models. */}
  <h3 className="settings-group-title flex items-center gap-2"><AIProviderIcon icon={accountType?.icon}/>{accountType?.name||agent}</h3>
  {error&&<Notice>{error}</Notice>}
  {methods.length>1&&<CaptureOptionSelect stacked label={t("ai.signInMethod")} value={method} items={methods.map((choice,index)=>({value:index,label:choice.label}))} disabled={disabled||active||loading} onChange={setMethod}/>}
  <div className="settings-account-summary">
   {accounts.length?<ul className="settings-stack-compact">{accounts.map(account=><li className="min-w-0" key={account.agent+account.user}><p className="break-all">{account.user}</p>{account.plan&&<p className="muted">{account.plan}</p>}{account.lapsed&&<p>{t("ai.signInAgain")}</p>}</li>)}</ul>:<p className="muted" role="status">{t(loading?"ai.loadingAccounts":error?"ai.accountStatusUnknown":"ai.noAccount")}</p>}
   <div className="settings-actions"><Button icon={LogIn} variant={accounts.length&&!accounts.some(account=>account.lapsed)?"secondary":"primary"} disabled={disabled||active||loading||!agent} onClick={()=>void signIn()}>{t(accounts.length?"ai.reauthenticate":"ai.signIn")}</Button><Button icon={RefreshCw} variant="ghost" disabled={disabled||active||loading} onClick={()=>void loadAccounts(true)}>{t("ai.refreshAccounts")}</Button></div>
  </div>
  {job&&<div className="settings-state-panel">
   {active&&!job.status.prompt&&<p role="status">{t(hint)}</p>}
   {job.status.state==='done'&&<p role="status">{t("ai.signedIn")}</p>}
   {job.status.state==='canceled'&&<p role="status">{t("ai.signInCanceled")}</p>}
   {job.status.state==='failed'&&!job.status.error&&<Notice>{t("ai.signInFailed")}</Notice>}
   {job.status.error&&<Notice>{job.status.error}</Notice>}
   {active&&job.status.instructions&&<p className="muted">{job.status.instructions}</p>}{job.inputError&&<Notice>{job.inputError}</Notice>}
   {active&&prompt&&<form onSubmit={event=>{event.preventDefault();void submit();}}><Field className="[&>button]:w-auto [&>button]:self-start">
    {prompt.type==="select"?<><CaptureOptionSelect stacked label={prompt.message} value={callback} items={(prompt.options??[]).map(option=>({value:option.value,label:option.label}))} disabled={inputBusy} onChange={value=>{setCallback(value);setChoiceMade(true);}}/>{prompt.options?.find(option=>option.value===callback)?.hint&&<p className="muted">{prompt.options.find(option=>option.value===callback)!.hint}</p>}</>:<><FieldLabel htmlFor="ai-signin-input">{prompt.message}</FieldLabel><Input id="ai-signin-input" type={prompt.type==="password"?"password":"text"} autoComplete="off" spellCheck={false} disabled={inputBusy} placeholder={prompt.placeholder} value={callback} onChange={event=>setCallback(event.target.value)}/></>}
    <Button type="submit" icon={Check} busy={inputBusy} disabled={!canSubmit}>{t("ai.continueSignIn")}</Button>
   </Field></form>}
   {active&&job.status.code&&<Field><FieldLabel htmlFor="ai-device-code">{t("ai.deviceCode")}</FieldLabel><div className="flex min-w-0 items-center gap-2"><Input id="ai-device-code" className="font-mono" readOnly value={job.status.code} onFocus={event=>event.currentTarget.select()}/><Button icon={copied==='device'?Check:Copy} aria-label={t("ai.copyDeviceCode")} title={t("ai.copyDeviceCode")} onClick={()=>void copy(job.status.code!,"device")}/></div></Field>}
   {active&&job.status.url&&<Field><FieldLabel htmlFor="ai-login-link">{t("ai.loginLink")}</FieldLabel><div className="flex min-w-0 items-center gap-2"><Input id="ai-login-link" readOnly value={job.status.url} onFocus={event=>event.currentTarget.select()}/><Button icon={copied==='link'?Check:Copy} aria-label={t("ai.copyLoginLink")} title={t("ai.copyLoginLink")} onClick={()=>void copy(job.status.url,"link")}/></div></Field>}
   {active&&!prompt&&(job.status.pasteCallback||job.status.pasteCode||job.status.pasteKey)&&<form onSubmit={event=>{event.preventDefault();void submit();}}><Field className="[&>button]:w-auto [&>button]:self-start"><FieldLabel htmlFor="ai-callback">{t(job.status.pasteKey?"ai.authorizationKey":job.status.pasteCode?"ai.authorizationCode":"ai.callback")}</FieldLabel><Input id="ai-callback" type={job.status.pasteKey?"password":"text"} autoComplete="off" spellCheck={false} disabled={inputBusy} value={callback} onChange={event=>setCallback(event.target.value)}/><Button type="submit" icon={Check} busy={inputBusy} disabled={!canSubmit}>{t("ai.submitCode")}</Button></Field></form>}
   {active&&<div className="settings-actions">{job.status.keysURL&&<Button icon={ExternalLink} onClick={()=>void captureCall("ai.accounts.signin.open",{id:job.id,keys:true}).catch(e=>setError(String(e)))}>{t("ai.openKeys")}</Button>}{job.status.url&&<Button icon={ExternalLink} onClick={()=>void captureCall("ai.accounts.signin.open",{id:job.id}).catch(()=>setError(t("ai.openLoginFailed")))}>{t("ai.openLogin")}</Button>}<Button icon={X} busy={action==='cancel'} onClick={()=>void cancel()}>{t("ai.cancelLogin")}</Button></div>}
  </div>}
  <p className="settings-help">{t("ai.accountScope")}</p>
 </section>;
}
