// Cindy's complete review UI talks to this instance-scoped transport. Paths,
// Git roots, revision guards and writes stay in the existing WinOne Host.
import {createContext,useContext,useEffect,useState,type ReactNode} from 'react';
import {chatCall} from '../../../bridge';
import {parseGitDiff} from './diffParser';
import {buildCappedDiffData} from './shared/gitReviewCapped';
import type {FileDiff,FileStatus,ReviewScope,ReviewData,ReviewDirtySummary,ReviewCommitListData,ReviewCommitDiffData,ReviewBranchDiffData,ReviewFileDiffRequest,ReviewFileDiffData,ReviewImagePreviewRequest,ReviewImagePreviewData,ReviewMarkdownPreviewRequest,ReviewMarkdownPreviewData,ReviewFileTarget,ReviewHunkOperationRequest,ReviewStageOperationResult,ReviewStageAction,ReviewCommitRequest,ReviewCommitResult,ReviewPushResult,ReviewPushConfirmForce,ReviewDiffSummaryEntry,ReviewBranchBaseCandidate,GitInProgressState} from './shared/gitReviewWire';
import type {TurnChangeSetDetail} from './shared/turnChangeSet';

export type SnapshotFile={path:string;oldPath?:string;status:string;untracked?:boolean;additions:number;deletions:number;size:number;binary?:boolean};
export type Snapshot={root:string;branch:string;scope:string;base:string;revision:string;files:SnapshotFile[];source?:'session';isReversible?:boolean;workspaceState?:string};
type Metadata={repository:boolean;root:string;branch:string;headOid:string;detached:boolean;unborn:boolean;upstream:string;ahead:number;behind:number;candidates:ReviewBranchBaseCandidate[];inProgress:GitInProgressState[]};
type ReadParams={sessionId:string;ignoreWhitespace?:boolean};
type FileParams=ReadParams&ReviewFileDiffRequest;
type TargetsParams=ReadParams&{targets:ReviewFileTarget[]};
type Preview={present:boolean;url?:string;size?:number;mime?:string;root?:string};
export type ChatMessage={role:string;toolName?:string;toolInput?:unknown};

let activities=new Map<string,{isRunning:boolean;paths:string[]}>();
const listeners=new Set<()=>void>();
export const makerChatStore={
 getRunningSnapshot:()=>activities,
 subscribeAll:(listener:()=>void)=>{listeners.add(listener);return()=>{listeners.delete(listener);};},
 subscribe:(_sessionId:string,listener:()=>void)=>makerChatStore.subscribeAll(listener),
 getSnapshot:(sessionId:string)=>({messages:(activities.get(sessionId)?.paths??[]).map(path=>({role:'tool_use',toolName:'write',toolInput:{path}}))}),
};
export function setReviewActivity(sessionId:string,isRunning:boolean,paths:string[]){
 const old=activities.get(sessionId);if(old?.isRunning===isRunning&&JSON.stringify(old.paths)===JSON.stringify(paths))return;
 activities=new Map(activities).set(sessionId,{isRunning,paths});listeners.forEach(listener=>listener());
}
export const createLogger=(name:string)=>({warn:(...args:unknown[])=>console.warn(name,...args),error:(...args:unknown[])=>console.error(name,...args)});
const feedbackEvent='winone:review-feedback';
export const toast={success:(message:string)=>window.dispatchEvent(new CustomEvent(feedbackEvent,{detail:{message,error:false}})),error:(message:string)=>window.dispatchEvent(new CustomEvent(feedbackEvent,{detail:{message,error:true}}))};
export function ReviewFeedback(){const [value,setValue]=useState<{message:string;error:boolean}>();useEffect(()=>{const receive=(event:Event)=>setValue((event as CustomEvent).detail);window.addEventListener(feedbackEvent,receive);return()=>window.removeEventListener(feedbackEvent,receive);},[]);return value?<div role={value.error?'alert':'status'} className="shrink-0 border-t border-[var(--border-default)] px-3 py-2 text-12 text-[var(--text-secondary)]">{value.message}</div>:null;}
export const isReviewRemoteOversizeError=(error:unknown)=>String(error).includes('GIT_REVIEW_REMOTE_OVERSIZE');

function sourceScope(source:string){return source==='unstaged'?'working':source==='turn'?'working':source;}
function statusOf(file:SnapshotFile):FileDiff['status']{return file.untracked?'untracked':({A:'added',D:'deleted',M:'modified',R:'renamed',C:'copied',T:'typechange'} as const)[file.status?.[0] as 'A']??'unknown';}
function pathJoin(root:string,path:string){return /^(?:[A-Za-z]:[\\/]|[\\/]{2}|\/)/.test(path)?path:root.replace(/[\\/]$/,'')+'/'+path;}
const emptyAhead={ahead:0,behind:0,upstream:null,stale:true};
function scopeOf(sessionId:string,snapshot:Snapshot,metadata?:Metadata):ReviewScope{return {sessionId,workdir:snapshot.root,worktreePath:snapshot.root,workingDir:snapshot.root,repoRoot:snapshot.source==='session'?null:snapshot.root,branch:metadata?.branch||snapshot.branch||null,headOid:metadata?.headOid||null,isDetached:metadata?.detached??false,isUnborn:metadata?.unborn??false,source:'workingDir',aheadBehind:metadata?{ahead:metadata.ahead,behind:metadata.behind,upstream:metadata.upstream||null,stale:true}:emptyAhead,disabledReason:snapshot.source==='session'?'non-git':null,disabledMessage:null,resolutionChain:[{source:'saved-session',path:snapshot.root,ok:true}]};}

function summaryEntry(file:SnapshotFile,snapshot:Snapshot,source:FileDiff['source']):ReviewDiffSummaryEntry{return {id:source+':'+file.path,source,path:file.path,oldPath:file.oldPath??null,status:statusOf(file),additions:file.additions??0,deletions:file.deletions??0,changedLines:(file.additions??0)+(file.deletions??0),changedBytes:0,isBinary:!!file.binary,isSubmodule:false,revision:snapshot.revision};}
function placeholder(entry:ReviewDiffSummaryEntry):FileDiff{return {...entry,kind:entry.isBinary?'binary':'text',size:null,isTooLarge:false,mode:{old:null,new:null},index:{oldOid:null,newOid:null},rawHeader:'',rawPatch:'',hunks:[],error:null};}
async function textAt(url:string){const response=await fetch(url);if(!response.ok)throw Error((await response.text()).trim()||`HTTP ${response.status}`);return response.text();}
async function mapLimit<T,R>(items:T[],action:(item:T)=>Promise<R>):Promise<R[]>{const result=new Array<R>(items.length);let next=0;await Promise.all(Array.from({length:Math.min(4,items.length)},async()=>{while(next<items.length){const index=next++;result[index]=await action(items[index]);}}));return result;}

export function createReviewHost(sessionId:string,options:{turnId?:string;initial?:Snapshot;onOpenFile:(path:string,line?:number)=>void}){
 const snapshots=new Map<string,Snapshot>();let metadata:Metadata|undefined;
 if(options.initial)snapshots.set('working:',options.initial);
 const call=<T,>(method:string,params:Record<string,unknown>={})=>chatCall<T>('review.'+method,{sessionId,...params});
 const snapshot=async(scope='working',base='',turnId?:string)=>{const value=await call<Snapshot>('snapshot',{scope,base,turnId});snapshots.set(scope+':'+base+':'+(turnId??''),value);return value;};
 const meta=async()=>{metadata=await call<Metadata>('metadata');return metadata;};
 async function readFile(s:Snapshot,file:SnapshotFile,source:FileDiff['source'],ignoreWhitespace=false,turnId?:string):Promise<FileDiff>{
  const entry=summaryEntry(file,s,source);if(file.binary)return {...placeholder(entry),turnId};
  const result=await call<{url:string}>('diff.preview',{scope:s.scope||sourceScope(source),base:s.base||'',path:file.path,revision:s.revision,turnId,ignoreWhitespace});
  const patch=await textAt(result.url);
  const diff=parseGitDiff(patch,{source,pathHint:file.path,oldPathHint:file.oldPath,isUntracked:file.untracked});
  return {...diff,id:entry.id,revision:s.revision,turnId,size:new TextEncoder().encode(patch).length};
 }
 async function bucket(s:Snapshot,source:FileDiff['source'],ignoreWhitespace=false){
  const capped=buildCappedDiffData(s.files.map(file=>summaryEntry(file,s,source)));
  if(capped)return {diffs:[] as FileDiff[],capped};
  const diffs=await mapLimit(s.files,file=>readFile(s,file,source,ignoreWhitespace));
  const loaded=buildCappedDiffData(diffs.map(diff=>({...summaryEntry(s.files.find(file=>file.path===diff.path)!,s,source),changedBytes:diff.size??0})));
  return {diffs:loaded?[]:diffs,capped:loaded};
 }
 async function get(params:ReadParams):Promise<ReviewData>{
  const [working,staged,info]=await Promise.all([snapshot(),snapshot('staged'),meta()]);
  const scope=scopeOf(sessionId,working,info);
  if(working.source==='session')return {scope,status:null,diffs:{staged:[],unstaged:[]},summary:{sessionId,disabledReason:'non-git',disabledMessage:null,totalFiles:working.files.length,stagedFiles:0,unstagedFiles:0,untrackedFiles:0,unmergedFiles:0,dirty:false}};
  const [left,right]=await Promise.all([bucket(working,'unstaged',params.ignoreWhitespace),bucket(staged,'staged',params.ignoreWhitespace)]);
  const byPath=new Map<string,FileStatus>();
  for(const [s,source]of [[working,'unstaged'],[staged,'staged']] as const)for(const file of s.files){const entry=byPath.get(file.path)||{path:file.path,oldPath:file.oldPath??null,indexStatus:null,worktreeStatus:null,isUntracked:!!file.untracked,isUnmerged:file.status==='U',isSubmodule:false,sources:[],rawXY:'  '};const status=statusOf(file);if(source==='staged')entry.indexStatus=status==='untracked'||status==='unknown'?null:status;else entry.worktreeStatus=status==='added'||status==='copied'||status==='unknown'?'modified':status;entry.sources.push(source);byPath.set(file.path,entry);}
  const files=[...byPath.values()],untrackedCount=files.filter(file=>file.isUntracked).length,unmergedCount=files.filter(file=>file.isUnmerged).length;
  const summary:ReviewDirtySummary={sessionId,disabledReason:null,disabledMessage:null,totalFiles:files.length,stagedFiles:staged.files.length,unstagedFiles:working.files.length,untrackedFiles:untrackedCount,unmergedFiles:unmergedCount,dirty:files.length>0};
  return {scope,status:{scope,files,stagedCount:staged.files.length,unstagedCount:working.files.length,untrackedCount,unmergedCount,inProgress:info.inProgress??[],writeDisabledReasons:unmergedCount?['unmerged']:[],dirty:summary.dirty},diffs:{staged:right.diffs,unstaged:left.diffs,capped:{staged:right.capped,unstaged:left.capped}},summary};
 }
 async function commits(_params:ReadParams&{baseRef?:string|null}):Promise<ReviewCommitListData>{const [s,info,rows]=await Promise.all([snapshot(),meta(),call<{id:string;title:string;date:string}[]>('commits')]);return {scope:scopeOf(sessionId,s,info),baseRef:_params.baseRef??null,baseOid:info.candidates?.find(ref=>ref.refName===_params.baseRef)?.oid??null,headOid:info.headOid||null,commits:rows.map(row=>({oid:row.id,shortOid:row.id.slice(0,8),title:row.title,authorTime:Date.parse(row.date)/1000})),warning:null};}
 async function fileDiff(params:FileParams):Promise<ReviewFileDiffData>{const base=params.commitOid||params.branchBaseRef||'',s=await snapshot(sourceScope(params.source),base,params.source==='turn'?options.turnId:undefined),file=s.files.find(file=>file.path===params.path);return {scope:scopeOf(sessionId,s,metadata),diff:file?await readFile(s,file,params.source,params.ignoreWhitespace,params.source==='turn'?options.turnId:undefined):null};}
 async function preview(params:ReadParams&ReviewImagePreviewRequest,side:string):Promise<Preview>{const diff=params.diff,base=params.commitOid||params.branchBaseRef||'';return call<Preview>('file.preview',{scope:sourceScope(diff.source),base,path:diff.path,side,revision:diff.revision,turnId:diff.turnId});}
 async function imagePreview(params:ReadParams&ReviewImagePreviewRequest):Promise<ReviewImagePreviewData>{const [old,next]=await Promise.all(['old','new'].map(async side=>{try{const value=await preview(params,side);return {present:value.present,oid:null,mime:value.mime??null,size:value.size??null,dataUrl:value.url,error:null};}catch(error){return {present:true,oid:null,mime:null,size:null,error:String(error)};}}));return {diffId:params.diff.id,old,new:next,maxBytes:16*1024*1024};}
 async function markdownPreview(params:ReadParams&ReviewMarkdownPreviewRequest):Promise<ReviewMarkdownPreviewData>{const value=await preview(params,'new'),maxBytes=3*1024*1024;return {diffId:params.diff.id,content:value.url&&(value.size??0)<=maxBytes?await textAt(value.url):null,size:value.size??null,baseDir:pathJoin(value.root??'',params.diff.path).replace(/[\\/][^\\/]+$/,''),maxBytes,reason:!value.present?'deleted':(value.size??0)>maxBytes?'too-large':null,error:null};}
 async function mutate(action:ReviewStageAction,params:TargetsParams|ReviewHunkOperationRequest):Promise<ReviewStageOperationResult>{
  const hunk='diff'in params,targets=hunk?[{path:params.diff.path,oldPath:params.diff.oldPath,source:params.diff.source==='staged'?'staged':'unstaged',revision:params.diff.revision}]:params.targets;
  const revisions=new Set(targets.map(target=>target.revision).filter(Boolean));
  if(revisions.size!==1)throw Error('请刷新差异后重试');
  await call('mutate',{mutation:{action:action==='discard'?'revert':action,paths:targets.flatMap(target=>target.oldPath&&target.oldPath!==target.path?[target.path,target.oldPath]:[target.path]),hunk:hunk?params.hunkIndex:undefined,revision:[...revisions][0]}});
  return {data:await get({sessionId}),operation:{action,succeeded:targets.map(target=>target.path),failed:[],partial:false}};
 }
 const gitReview={get,summary:async(params:ReadParams)=>(await get(params)).summary,commits,fileDiff,imagePreview,markdownPreview,
  commitDiff:async(params:ReadParams&{oid:string}):Promise<ReviewCommitDiffData>=>{const [s,info]=await Promise.all([snapshot('commit',params.oid),meta()]),value=await bucket(s,'commit',params.ignoreWhitespace);return {scope:scopeOf(sessionId,s,info),commitOid:params.oid,diffs:value.diffs,capped:value.capped};},
  branchDiff:async(params:ReadParams&{baseRef?:string|null}):Promise<ReviewBranchDiffData>=>{const info=await meta(),base=params.baseRef||info.candidates.find(ref=>ref.isDefaultBranch)?.refName||info.candidates[0]?.refName;if(!base){const s=await snapshot();return {scope:scopeOf(sessionId,s,info),baseRef:null,baseOid:null,headOid:info.headOid||null,mergeBaseOid:null,candidates:info.candidates,diffs:[],capped:null,warning:{code:'no-base-candidates',message:'没有可比较的分支'}};}const s=await snapshot('branch',base),value=await bucket(s,'branch',params.ignoreWhitespace);return {scope:scopeOf(sessionId,s,info),baseRef:base,baseOid:info.candidates.find(ref=>ref.refName===base)?.oid??null,headOid:info.headOid,mergeBaseOid:null,candidates:info.candidates,diffs:value.diffs,capped:value.capped,warning:null};},
  stageFile:(params:TargetsParams)=>mutate('stage',params),stageAll:(params:TargetsParams)=>mutate('stage',params),unstageFile:(params:TargetsParams)=>mutate('unstage',params),unstageAll:(params:TargetsParams)=>mutate('unstage',params),stageHunk:(params:ReviewHunkOperationRequest)=>mutate('stage',params),unstageHunk:(params:ReviewHunkOperationRequest)=>mutate('unstage',params),discardFile:(params:TargetsParams)=>mutate('discard',params),discardAll:(params:TargetsParams)=>mutate('discard',params),discardHunk:(params:ReviewHunkOperationRequest)=>mutate('discard',params),
  openFile:async(params:{sessionId:string;path:string})=>{const root=metadata?.root||options.initial?.root||'';options.onOpenFile(pathJoin(root,params.path));},
  commit:async(params:ReviewCommitRequest):Promise<ReviewCommitResult>=>{await call('mutate',{mutation:{action:'commit',message:params.message,includeUnstaged:params.includeUnstaged}});const data=await get({sessionId});return {data,commitOid:data.scope.headOid??'',stdout:'',stderr:''};},
  push:async(params:ReadParams&{confirmForce?:ReviewPushConfirmForce}):Promise<ReviewPushResult>=>{const result=await call<Omit<Extract<ReviewPushResult,{kind:'pushed'}>,'data'>>('push',{confirmForce:params.confirmForce});const data=await get({sessionId});return {...result,data};},
 };
 const maker={getTurnChangeSets:async(_sessionId:string,ids:string[]):Promise<TurnChangeSetDetail[]>=>mapLimit(ids,async id=>{const turn=id==='recorded-session'?undefined:id,s=await snapshot('working','',turn),diffs=await mapLimit(s.files,file=>readFile(s,file,'turn',false,turn));return {id,sessionId,anchorClientId:id,provider:'pi',providerTurnId:id,cwd:s.root,state:s.isReversible?'complete':'partial',workspaceState:s.workspaceState==='undone'?'undone':'applied',isReversible:!!s.isReversible,incompleteReasons:[],createdAt:0,completedAt:0,files:diffs.map(diff=>({id:diff.id,path:diff.path,oldPath:diff.oldPath,status:diff.status,additions:diff.additions,deletions:diff.deletions})),fileCount:diffs.length,additions:diffs.reduce((sum,diff)=>sum+diff.additions,0),deletions:diffs.reduce((sum,diff)=>sum+diff.deletions,0),diffs};})};
 return {sessionId,platform:'win32' as const,gitReview,maker};
}
export type ReviewHost=ReturnType<typeof createReviewHost>;
const ReviewContext=createContext<ReviewHost|null>(null);
export function ReviewHostProvider({host,children}:{host:ReviewHost;children:ReactNode}){return <ReviewContext.Provider value={host}>{children}</ReviewContext.Provider>;}
export function useReviewHost(){const host=useContext(ReviewContext);if(!host)throw Error('Review host is unavailable');return host;}
