/** The scheduler uses a private loopback bridge owned by this Host. It never
 * stores model credentials or owns a second set of Chat session files. */
export function createScheduleDispatcher({baseUrl,key}){
 const endpoint=new URL(baseUrl);if(endpoint.protocol!=='http:'||endpoint.hostname!=='127.0.0.1'||key.length<16)throw Error('Invalid private scheduler bridge');
 const post=async(route,body,signal)=>{
  const response=await fetch(baseUrl+'/'+route,{method:'POST',headers:{Authorization:'Bearer '+key,'Content-Type':'application/json'},body:JSON.stringify(body),signal});
  if(!response.ok){const text=await response.text();let detail;try{detail=JSON.parse(text);}catch{}const error=Error(String(detail?.error??text).slice(0,2048)||'Automation request failed');if(detail?.code==='target_unavailable')error.code=detail.code;throw error;}
  return response;
 };
 const dispatch=async(schedule,ctx)=>{
  let id;
  try{
   if(schedule.executionMode==='script'||schedule.preRunHook?.command){
    const mode=schedule.executionMode==='script'?'script':'hook';
    const result=await(await post('command',{schedule,mode,runId:ctx.runId,firedAt:ctx.firedAt},ctx.signal)).json();
    if(result.preRunHook)await ctx.onPreRunHookCompleted?.(result.preRunHook);
    if(result.error)throw Error(result.error);
    if(mode==='script'){if(result.value.sessionId)await ctx.onSessionBound?.(result.value.sessionId);return result.value;}
    const hook=result.value;await ctx.onPreRunHookCompleted?.(hook);
    if(hook.aborted||ctx.signal.aborted)throw new DOMException('aborted','AbortError');
    if(hook.decision==='skip')return {sessionId:'',skipped:true,resultText:hook.stdout||'Pre-run check skipped this run.'};
    if(hook.decision!=='run')throw Error(hook.error||hook.stderr||'Pre-run check failed: '+hook.status);
   }
   let prepared=await(await post('prepare',{schedule,runId:ctx.runId},ctx.signal)).json();
   if(prepared.deferred){
    // A bound conversation can finish its current turn without starting an
    // extra agent for this queued prompt. Cindy releases its execution slot.
    ctx.onQueueWaitStart?.();let claimed=false;
    try{await post('wait',{id:prepared.sessionId},ctx.signal);claimed=ctx.endQueueWait?.(true)??true;}
    finally{if(!claimed)ctx.endQueueWait?.(false);}
    if(!claimed)return {sessionId:prepared.sessionId,deferred:true,deferRetryMs:10000};
    prepared=await(await post('prepare',{schedule,runId:ctx.runId},ctx.signal)).json();
    if(prepared.deferred)return {sessionId:prepared.sessionId,deferred:true,deferRetryMs:10000};
   }
   id=prepared.sessionId;await ctx.onSessionBound?.(id);
   const policy=schedule.silentWhenIdle?'\n\nThis is a scheduled run that stays quiet by default. If the result needs user attention, call schedule_notify before finishing.':'\n\nThis is a scheduled run. If checks found no actionable work, you may call schedule_silence; failures remain visible.';
   const response=await post('execute',{id,runId:ctx.runId,prompt:schedule.prompt+policy},ctx.signal);
   const reader=response.body.getReader(),decoder=new TextDecoder();let buffer='',final;
   try{for(;;){const {done,value}=await reader.read();if(done)break;buffer+=decoder.decode(value,{stream:true});if(buffer.length>2*1024*1024)throw Error('Automation event exceeded its display limit');
    for(;;){const line=buffer.indexOf('\n');if(line<0)break;const message=JSON.parse(buffer.slice(0,line));buffer=buffer.slice(line+1);
     if(message.started)ctx.onTurnActive?.(id);
     if(message.progress||message.events?.length)ctx.onProgress?.();
     for(const item of message.events??[]){const event=item.data;if(event.type==='settled')final=event;}
     if(message.terminal)final=message.terminal;
    }
   }}finally{await reader.cancel();}
   if(!final)throw Error('Automation ended without a confirmed result.');
   if(final.status==='stopped')throw new DOMException('aborted','AbortError');
   if(final.status==='failed'||final.error)throw Error(final.error||'Automation failed.');
   const answer=final.state?.messages?.findLast(message=>message.role==='assistant');
   const text=typeof answer?.content==='string'?answer.content:(answer?.content??[]).filter(part=>part.type==='text').map(part=>part.text??'').join('\n');
   return {sessionId:id,resultText:text.slice(0,16000)};
  }catch(error){
   if(id&&ctx.signal.aborted){try{await post('stop',{id,runId:ctx.runId},AbortSignal.timeout(5000));}catch(stopError){throw new Error('Automation cancellation could not be confirmed: '+stopError.message);}}
   throw error;
  }
 };
 dispatch.notify=async(schedule,run)=>{await post('notify',{schedule,run},AbortSignal.timeout(30000));};
 return dispatch;
}
