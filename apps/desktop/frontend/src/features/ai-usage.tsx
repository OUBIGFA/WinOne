import {Fragment,useCallback,useEffect,useRef,useState} from "react";
import {RefreshCw,ChevronLeft,ChevronRight,Download,Eye,EyeOff} from "lucide-react";
import {captureCall} from "../bridge";
import {useTranslation} from "../i18n";
import {Input} from "../components/ui/input";
import {Button} from "../ui/button";
import {Notice} from "../ui/notice";
import {Checkbox} from "../components/ui/checkbox";
import {Tabs,TabsList,TabsTrigger,TabsContent} from "../components/ui/tabs";
import {CaptureOptionSelect} from "../ui/capture-option-select";
import {AIProviderIcon} from "./ai-provider-icon";
import {AIModelIcon} from "./ai-model-icon";
import {UsageChart} from "./ai-usage-chart";
import {UsagePeriod} from "./ai-usage-period";
import {UsageHeatmap} from "./ai-usage-heatmap";
import {UsageBreakdowns} from "./ai-usage-breakdowns";
import {UsageSessions} from "./ai-usage-sessions";
import {QuotaCurve,type QuotaLine} from "./ai-usage-quota-curve";
import {UsageContext} from "./ai-usage-context";
import {normalizeUsagePage,type Totals,type UsageFilter,type UsagePage,type UsageRow} from "./ai-usage-types";
import type {AIProvider,MagpieSettings} from "./ai-configuration";
import "./ai-usage.css";

const initialFilter:UsageFilter={callerKey:"",period:"7d",provider:"",source:"",failed:false,offset:0,day:"",model:"",account:"",purpose:"",query:""};
// Read on entry or explicit changes; inactive tabs own no refresh timer and
// late replies cannot replace a newer filter's result.
export function AIUsage({active,providers,preferences,onPreferencesChange}:{active:boolean;providers:AIProvider[];preferences?:MagpieSettings;onPreferencesChange?:(patch:Partial<MagpieSettings>)=>void}){
 const {t}=useTranslation();const [tab,setTab]=useState("overview"),[filter,setFilter]=useState(initialFilter);
 const [revision,setRevision]=useState(0),[every,setEvery]=useState("0"),[loading,setLoading]=useState(false),[exporting,setExporting]=useState(false),[error,setError]=useState(""),[exported,setExported]=useState("");
 const [readQuota]=useState(createQuotaReader);const [page,setPage]=useState<UsagePage>();const ledger=tab==="overview"||tab==="requests";
 useEffect(()=>{
  if(!active||!ledger)return;
  let current=true;setLoading(true);setError("");
  void captureCall<UsagePage>("ai.usage.get",filter).then(value=>{if(current)setPage(normalizeUsagePage(value));}).catch(error=>{if(current)setError(String(error));}).finally(()=>{if(current)setLoading(false);});
  return()=>{current=false;};
 },[active,ledger,filter,revision]);
 useEffect(()=>{if(!active||every==="0")return;const timer=setInterval(()=>{if(document.visibilityState!=="hidden")setRevision(value=>value+1);},Number(every)*1000);return()=>clearInterval(timer);},[active,every]);
 const change=useCallback((patch:Partial<UsageFilter>)=>setFilter(value=>({...value,...patch,offset:0})),[]);
 const requestOrigin=useRef<UsageFilter|undefined>(undefined);
 // A chart click is a temporary drill-down. Leaving Requests restores its
 // prior filters, including explicit edits made before that click.
 const leaveRequests=useCallback((next:string)=>{if(next!=="requests"&&requestOrigin.current){setFilter(requestOrigin.current);requestOrigin.current=undefined;}setTab(next);},[]);
 useEffect(()=>{if(!active&&requestOrigin.current){setFilter(requestOrigin.current);requestOrigin.current=undefined;}},[active]);
 const pick=useCallback((dimension:string,id:string)=>{requestOrigin.current??=filter;if(dimension==="modelAt"){const at=id.indexOf("/");change({provider:id.slice(0,at),model:id.slice(at+1)});}else change(dimension==="agent"?{source:id}:{[dimension]:id});setTab("requests");},[change,filter]);
 const pickDay=useCallback((day:string)=>change({day:filter.day===day?"":day}),[change,filter.day]);
 const modelNames=new Map(providers.flatMap(p=>p.models.map(model=>[model,p.modelNames?.[model]||model] as const)));
 const named=page?{...page,by:Object.fromEntries(Object.entries(page.by??{}).map(([kind,rows])=>[kind,rows.map(row=>({...row,name:kind==="provider"||kind==="account"?providers.find(p=>p.id===row.id)?.name||row.id:kind==="agent"?t(row.id==="gateway"?"usage.gateway":"usage.winone"):modelNames.get(row.id)||row.name||row.id}))]))}:undefined;
 async function exportCSV(){setExporting(true);setError("");setExported("");try{const value=await captureCall<{state:string;path?:string;rows?:number}>("ai.usage.export",filter);if(value.state==="cancelled")return;if(value.state!=="saved"||!value.path||!Number.isInteger(value.rows)||value.rows!<0)throw Error(t("usage.exportFailed"));setExported(t("usage.exportSaved",{path:value.path,count:value.rows}));}catch(error){setError(String(error));}finally{setExporting(false);}}
 return <section className="settings-stack ai-usage" aria-label={t("usage.title")}>
  <p className="settings-help">{t("usage.hint")}</p>
  <Tabs value={tab} onValueChange={value=>leaveRequests(String(value))}>
   <TabsList variant="line" aria-label={t("usage.views")}>{["overview","requests","context","sessions"].map(value=><TabsTrigger value={value} key={value}>{t("usage."+value)}</TabsTrigger>)}</TabsList>
   <div className="usage-controls"><UsagePeriod value={filter.period} onChange={period=>change({period,day:""})}/><CaptureOptionSelect stacked label={t("usage.autoRefresh")} value={every} disabled={false} items={[{value:"0",label:t("usage.manual")},...['15','30','60'].map(value=>({value,label:t("usage.everySeconds",{seconds:value})}))]} onChange={setEvery}/><Button icon={RefreshCw} busy={loading} disabled={loading} onClick={()=>setRevision(value=>value+1)}>{t("usage.refresh")}</Button></div>
   {ledger&&<>
    <div className="grid min-w-0 grid-cols-1 gap-4 sm:grid-cols-3"><CaptureOptionSelect stacked label={t("usage.provider")} value={filter.provider} disabled={false} items={[{value:"",label:t("usage.allProviders")},...providers.map(p=>({value:p.id,label:p.name,icon:<AIProviderIcon icon={p.avatar||p.icon}/>}))]} onChange={provider=>change({provider})}/><CaptureOptionSelect stacked label={t("usage.source")} value={filter.source} disabled={false} items={[{value:"",label:t("usage.allSources")},{value:"winone",label:t("usage.winone")},{value:"gateway",label:t("usage.gateway")}]} onChange={source=>change({source})}/><label className="flex flex-col gap-2 text-sm"><span>{t("usage.search")}</span><Input type="search" value={filter.query} onChange={event=>change({query:event.target.value})}/></label></div>
    <div className="flex flex-wrap items-center justify-between gap-3"><label className="check-row"><Checkbox checked={filter.failed} onCheckedChange={failed=>change({failed})}/>{t("usage.failedOnly")}</label>{Object.entries(filter).some(([key,value])=>!["period","offset"].includes(key)&&!!value)&&<Button onClick={()=>setFilter({...initialFilter,period:filter.period})}>{t("usage.clearFilters")}</Button>}</div>
    {(filter.day||filter.model)&&<p className="settings-help">{filter.day} {modelNames.get(filter.model)||filter.model}</p>}
    {loading&&<p role="status">{t("usage.loading")}</p>}
    {exported&&<Notice tone="info">{exported}</Notice>}{error&&<Notice actions={<Button onClick={()=>setRevision(value=>value+1)}>{t("common.retry")}</Button>}>{error}</Notice>}
    {named&&<>
     {named.priceWarning&&<Notice tone="info">{t("usage.priceWarning")}</Notice>}
     <UsageTotals sum={named.sum}/>
     <UsageChart page={named} onPick={pick} onDay={pickDay}/>
     <TabsContent value="overview" className="settings-stack"><UsageBreakdowns page={named} onPick={pick}/><UsageAllowances providers={providers} active={active&&tab==="overview"} revision={revision} readQuota={readQuota} preferences={preferences} onPreferencesChange={onPreferencesChange}/>{!named.rows.length&&<p className="settings-empty">{t("usage.empty")}</p>}</TabsContent>
     <TabsContent value="requests" className="settings-stack">{named.heatmap&&<UsageHeatmap data={named.heatmap}/>}<div className="grid gap-4 sm:grid-cols-3"><CaptureOptionSelect stacked label={t("usage.account")} value={filter.account} disabled={false} items={[{value:"",label:t("usage.allAccounts")},...(named.accounts??[]).map(row=>({value:row.account,label:providers.find(p=>p.id===row.account)?.name||row.name||row.id}))]} onChange={account=>change({account})}/><CaptureOptionSelect stacked label={t("usage.callerKey")} value={filter.callerKey??""} disabled={false} items={[{value:"",label:t("usage.allCallerKeys")},{value:"none",label:t("usage.noCallerKey")},...(named.callerKeys??[]).map(key=>({value:key.id,label:key.callerKeyName||key.id}))]} onChange={callerKey=>change({callerKey})}/><CaptureOptionSelect stacked label={t("usage.purpose")} value={filter.purpose} disabled={false} items={[{value:"",label:t("usage.allPurposes")},...(named.purposes??[]).map(value=>({value,label:value}))]} onChange={purpose=>change({purpose})}/><Button icon={Download} busy={exporting} disabled={exporting||!named.total} onClick={()=>void exportCSV()}>{t("usage.export")}</Button></div><UsageRequests rows={named.rows} providers={providers}/><UsagePager total={named.total} offset={named.offset} count={named.rows.length} loading={loading} onChange={offset=>setFilter(value=>({...value,offset}))}/></TabsContent>
    </>}
   </>}
   <TabsContent value="sessions"><UsageSessions mode="sessions" active={active&&tab==="sessions"} period={filter.period} revision={revision} providers={providers}/></TabsContent>
   <TabsContent value="context"><UsageContext active={active&&tab==="context"} period={filter.period} revision={revision} providers={providers}/></TabsContent>
  </Tabs>
 </section>;
}

export function UsageTotals({sum}:{sum:Totals}){const {t,i18n}=useTranslation(),num=(value:number)=>new Intl.NumberFormat(i18n.language).format(value||0),cash=(value:number)=>new Intl.NumberFormat(i18n.language,{style:"currency",currency:"USD",maximumFractionDigits:6}).format(value||0);const prompt=sum.input+sum.cache_read+sum.cache_write;return <><dl className="usage-totals">{[{label:t("usage.calls"),value:num(sum.calls),detail:t("usage.errors")+" · "+num(sum.errors)},{label:t("usage.input"),value:num(sum.input),detail:t("usage.cache")+" · "+num(sum.cache_read+sum.cache_write)},{label:t("usage.output"),value:num(sum.output)},{label:t("usage.cost"),value:cash(sum.cost)},...(sum.credited?[{label:t("usage.credits"),value:num(sum.credits??0)}]:[]),{label:t("usage.speed"),value:sum.decode_ms?num(Math.round((sum.decode_out||0)*1000/sum.decode_ms))+" Token/s":"—"},{label:t("usage.ttft"),value:sum.timed?((sum.ttft_ms||0)/sum.timed/1000).toFixed(2)+" s":"—"},{label:t("usage.hitRate"),value:prompt?Math.round(sum.cache_read/prompt*100)+"%":"—"}].map(item=><div key={item.label}><dt>{item.label}</dt><dd>{item.value}</dd>{item.detail&&<dd className="usage-detail">{item.detail}</dd>}</div>)}</dl>{sum.unpriced>0&&<p className="settings-help">{t("usage.unpriced",{count:sum.unpriced})}</p>}</>}

export function UsagePager({total,offset,count,loading=false,onChange}:{total:number;offset:number;count:number;loading?:boolean;onChange:(offset:number)=>void}){const {t}=useTranslation();if(!total)return null;return <div className="flex flex-wrap items-center justify-between gap-3"><p className="text-sm text-muted-foreground">{t("usage.page",{from:Math.min(offset+1,total),to:Math.min(offset+count,total),total})}</p><div className="flex gap-2"><Button icon={ChevronLeft} disabled={loading||offset===0} onClick={()=>onChange(Math.max(0,offset-15))}>{t("usage.previous")}</Button><Button icon={ChevronRight} disabled={loading||offset+15>=total} onClick={()=>onChange(offset+15)}>{t("usage.next")}</Button></div></div>}

function UsageRequests({rows,providers}:{rows:UsageRow[];providers:AIProvider[]}){const {t,i18n}=useTranslation();const [open,setOpen]=useState<string[]>([]);if(!rows.length)return <p className="settings-empty">{t("usage.empty")}</p>;const num=(n:number)=>new Intl.NumberFormat(i18n.language).format(n||0);return <div className="usage-table-scroll"><table className="usage-table"><thead><tr>{["time","model","provider","tokenTotal","cost","elapsed","ttft","speed","status"].map(key=><th key={key}>{t("usage."+key)}</th>)}</tr></thead><tbody>{rows.map((row,index)=>{const id=row.t+index,provider=providers.find(p=>p.id===row.provider),failed=row.status>=400||!!row.err,expanded=open.includes(id);const speed=row.speed?Math.round(row.speed):0;return <Fragment key={id}><tr><td data-label={t("usage.time")}><button type="button" aria-expanded={expanded} onClick={()=>setOpen(value=>expanded?value.filter(key=>key!==id):[...value,id])}>{new Date(row.t).toLocaleString(i18n.language)}</button></td><td data-label={t("usage.model")}><span className="flex items-center gap-2"><AIModelIcon model={row.model}/>{provider?.modelNames?.[row.model]||row.model}</span></td><td data-label={t("usage.provider")}>{provider?.name||row.provider}<small>{t(row.agent==="gateway"?"usage.gateway":"usage.winone")}</small></td><td data-label={t("usage.tokenTotal")}>{num(row.in)} / {num(row.out)}<small>{t("usage.cache")} {num((row.cache_read||0)+(row.cache_write||0))}</small></td><td data-label={t("usage.cost")}>{row.priced?'$'+row.cost.toFixed(6):t("usage.unknownPrice")}{row.credits!=null&&<small>{num(row.credits)} {t("usage.credits")}</small>}</td><td data-label={t("usage.elapsed")}>{row.ms>0?(row.ms/1000).toFixed(2)+' s':'—'}</td><td data-label={t("usage.ttft")}>{row.ttft_ms?(row.ttft_ms/1000).toFixed(2)+" s":"?"}</td><td data-label={t("usage.speed")}>{speed?num(speed)+' Token/s':'—'}</td><td data-label={t("usage.status")} className={failed?'text-destructive':''}>{row.rejected?t("usage.rejected"):t(failed?"usage.failed":"usage.success")} {row.status||''}</td></tr>{expanded&&<tr><td colSpan={9}><dl className="usage-row-details">{Object.entries({[t("usage.callerKey")]:row.callerKeyName||row.callerKeyId||t("usage.noCallerKey"),[t("usage.requested")]:row.req,[t("usage.served")]:row.served,[t("usage.endpoint")]:row.ep,[t("usage.reasoning")]:row.reasoning,[t("usage.effort")]:row.effort,[t("usage.cacheRead")]:row.cache_read,[t("usage.cacheWrite")]:row.cache_write,[t("usage.cacheHour")]:row.cache_write_1h,[t("usage.ttft")]:row.ttft_ms?row.ttft_ms+' ms':undefined,[t("usage.stopReason")]:row.stop,[t("usage.error")]:row.err}).map(([key,value])=><div key={key}><dt>{key}</dt><dd>{value??'—'}</dd></div>)}</dl></td></tr>}</Fragment>})}</tbody></table></div>}

type BalancePart={label?:string;text:string;percent?:number;error?:string};
interface Allowance {provider:string;name:string;user?:string;plan?:string;windows:{name:string;used:number;unlimited?:boolean;resetsAt?:string;display?:string;amount?:number;limit?:number;unit?:string}[];balance?:string;balanceSplit?:BalancePart[];balanceParts?:BalancePart[];error?:string;seeAt?:string;readAt?:string;asOf?:string;history?:QuotaLine[]}

// A ledger redraw and StrictMode can overlap the same quota read. Share only
// the in-flight operation; explicit refresh always obtains a new vendor reading
// after it finishes, and rejected reads are never cached as successful quotas.
function createQuotaReader(){
 const pending=new Map<string,Promise<{quotas:Allowance[]}>>();
 return (provider:AIProvider)=>{
  const key=JSON.stringify([provider.id,provider.agent,provider.agentBackend]);
  const existing=pending.get(key);if(existing)return existing;
  const request=captureCall<{quotas:Allowance[]}>("ai.accounts.quota",{provider:provider.id,agent:provider.agent});
  const tracked=request.then(value=>{pending.delete(key);return value;},error=>{pending.delete(key);throw error;});
  pending.set(key,tracked);return tracked;
 };
}
function UsageAllowances({providers,active,revision,readQuota,preferences,onPreferencesChange}:{providers:AIProvider[];active:boolean;revision:number;readQuota:ReturnType<typeof createQuotaReader>;preferences?:MagpieSettings;onPreferencesChange?:(patch:Partial<MagpieSettings>)=>void}){const {t,i18n}=useTranslation();const [data,setData]=useState<{provider:AIProvider;quotas:Allowance[];error?:string}[]>([]),[loading,setLoading]=useState(false),[masked,setMasked]=useState(preferences?.hideAccounts??true),[left,setLeft]=useState(preferences?.quotaLeft??true),[range,setRange]=useState("cycle");useEffect(()=>{if(preferences){setMasked(!!preferences.hideAccounts);setLeft(!!preferences.quotaLeft);}},[preferences?.hideAccounts,preferences?.quotaLeft]);const configured=providers.filter(p=>p.authMode==="account"&&p.agent||p.authMode==="api-key"&&p.hasKey);const key=JSON.stringify(configured.map(p=>[p.id,p.agent,p.baseUrl,p.keys]));useEffect(()=>{if(!active)return;let current=true;setLoading(true);void Promise.all(configured.map(async provider=>{try{const value=await readQuota(provider);return {provider,quotas:value.quotas};}catch(error){return {provider,quotas:[],error:String(error)};}})).then(value=>{if(current)setData(value);}).finally(()=>{if(current)setLoading(false);});return()=>{current=false;};},[active,key,revision,readQuota]);return <section className="settings-group"><div className="flex flex-wrap items-center justify-between gap-3"><h3 className="settings-group-title">{t("usage.allowances")}</h3><div className="flex gap-2"><Button icon={masked?EyeOff:Eye} onClick={()=>{setMasked(!masked);onPreferencesChange?.({hideAccounts:!masked});}}>{t(masked?"usage.showAccounts":"usage.hideAccounts")}</Button><Button onClick={()=>{setLeft(!left);onPreferencesChange?.({quotaLeft:!left});}}>{t(left?"usage.showUsed":"usage.showLeft")}</Button></div></div><CaptureOptionSelect stacked label={t("usage.quotaTrend")} value={range} disabled={false} onChange={setRange} items={[{value:"off",label:t("usage.trendOff")},{value:"2d",label:t("usage.twoDays")},{value:"cycle",label:t("usage.cycle")}]} />{loading&&<p role="status">{t("usage.quotaLoading")}</p>}{!configured.length&&<p className="settings-help">{t("usage.noAccounts")}</p>}<div className="usage-group-grid">{data.map(({provider,quotas,error})=><section key={provider.id} className="usage-allowance"><h4 className="flex items-center gap-2"><AIProviderIcon icon={provider.avatar||provider.icon}/>{provider.name}</h4>{error&&<Notice>{error}</Notice>}{!error&&!quotas.length&&<p className="settings-help">{t("usage.quotaUnavailable")}</p>}{quotas.map((quota,index)=><div key={quota.user||index}><p className="usage-detail">{masked?t("usage.maskedAccount",{number:index+1}):quota.user||provider.name} · {quota.plan}</p>{(quota.error||quota.asOf)&&<AllowanceMessage quota={quota}/>}{quota.balance&&<p>{t("usage.balance")} · {quota.balance}</p>}{[quota.balanceParts,quota.balanceSplit].map((parts,index)=>parts?.length?<BalanceDetails key={index} parts={parts}/>:null)}{quota.windows?.map(window=><div className="usage-quota-window" key={window.name}><div><span>{window.name}</span><span>{window.unlimited?t("usage.unlimited"):(left?Math.max(0,100-window.used):window.used).toFixed(1)+'% '+t(left?"usage.remaining":"usage.used")}</span></div>{!window.unlimited&&<progress max={100} value={Math.max(0,Math.min(100,left?100-window.used:window.used))}/>}<p className="usage-detail">{window.display}{window.resetsAt&&' · '+t("usage.resetsAt",{time:new Date(window.resetsAt).toLocaleString(i18n.language)})}</p></div>)}<QuotaCurve quota={quota} range={range}/>{(quota.readAt||quota.asOf)&&<p className="usage-detail">{t("usage.readAt",{time:new Date(quota.asOf||quota.readAt!).toLocaleString(i18n.language)})}</p>}</div>)}</section>)}</div></section>}

// A vendor that does not expose balances is not a failed/zero reading. Keep
// its console action, but never turn an arbitrary non-web URL into navigation.
function AllowanceMessage({quota}:{quota:Allowance}){
 const {t,i18n}=useTranslation();let consoleURL:string|undefined;
 try{const url=new URL(quota.seeAt||'');if(url.protocol==='https:'&&!url.username&&!url.password)consoleURL=url.href;}catch{/* No console was supplied. */}
 const unavailable=quota.error?.startsWith('OpenCode tells no API key its balance');
 const message=quota.asOf?t('usage.quotaStale',{time:new Date(quota.asOf).toLocaleString(i18n.language)}):t(unavailable?'usage.openCodeNoBalance':'usage.quotaUnavailable');
 return <div className="settings-stack-compact">{unavailable?<p className="settings-help">{message}</p>:<Notice>{message}</Notice>}{consoleURL&&<a className="text-sm underline underline-offset-4" href={consoleURL} target="_blank" rel="noopener noreferrer">{t('usage.openConsole')}</a>}</div>;
}

// Preserve the upstream amount, zero, component error and optional percentage.
// Only the two upstream split labels are translated; named vendor fields stay intact.
function BalanceDetails({parts}:{parts:BalancePart[]}){
 const {t}=useTranslation();
 return <dl className="grid min-w-0 gap-2 text-sm">{parts.map((part,index)=><div key={index} className="min-w-0"><div className="flex flex-wrap justify-between gap-2"><dt>{part.label==='Granted'?t('usage.granted'):part.label==='Topped up'?t('usage.toppedUp'):part.label||t('usage.balance')}</dt><dd>{part.text}</dd></div>{part.error?<p className="settings-help">{t('usage.quotaUnavailable')}</p>:part.percent!==undefined?<progress className="w-full" max={100} value={Math.max(0,Math.min(100,part.percent))}/>:null}</div>)}</dl>;
}
