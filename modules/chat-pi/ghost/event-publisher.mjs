// Ordered metadata delivery through the existing private Host bridge. No
// polling and no transcript per token; only start/end boundaries are sent.
export function createGhostEventPublisher(bridge){
 let tail=Promise.resolve(),closed=false;
 const send=input=>{
  if(!bridge||closed)return;
  tail=tail.then(async()=>{
   const response=await fetch(bridge.baseUrl+'/ghost-tool',{method:'POST',headers:{Authorization:'Bearer '+bridge.key,'Content-Type':'application/json'},body:JSON.stringify({method:'agent-native-event',params:input}),signal:AbortSignal.timeout(5000)});
   if(!response.ok)throw Error('Plugin metadata delivery failed');
  }).catch(()=>{}); // Observers must never prevent the original Chat run.
 };
 return {send,async close(){send({kind:'closed'});closed=true;await tail;}};
}
