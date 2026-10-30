// The private Host bridge already authenticates this worker. Lifecycle never
// accepts a renderer-provided process ID or path.
export function createOneBotLifecycleClient({baseUrl,key,im}){
 async function control(action,params){
  const response=await fetch(baseUrl+'/lifecycle',{method:'POST',headers:{Authorization:'Bearer '+key,'Content-Type':'application/json'},body:JSON.stringify({action,...params}),signal:AbortSignal.timeout(25000)});
  if(!response.ok)throw Error((await response.text()).slice(0,2048));
  const result=await response.json();if(result.ok!==true)throw Error('OneBot 会话控制未完成');
 }
 return {closeSession:id=>control('close',{sessionId:id}),restartRuntime:id=>control('restart',{sessionId:id}),
  deleteSessions:(botId,sessionIds,keepTaskHistory)=>control('delete-sessions',{botId,sessionIds,keepTaskHistory}),
  recycleHome:botId=>control('recycle-home',{botId}),
  onPaused:id=>im()?.pauseBot(id),onResumed:id=>im()?.resumeBot(id),
  onBeforeDelete:id=>im()?.pauseBot(id),onDeleted:id=>im()?.removeBot(id)};
}
