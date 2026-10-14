/** Policy tools target only the authenticated conversation's current turn.
 * Host and Cindy both verify the accepted request before changing policy. */
export function automationPolicyTools({baseUrl,key}){
 return ['silence','notify'].map(action=>({name:'schedule_'+action,label:action==='silence'?'Silence this automation run':'Report this automation run',description:action==='silence'?'For the currently running automation only: keep this run quiet when checks found no actionable change. Failures still report.':'For the currently running automation only: request a completion notification because a result needs the user’s attention.',parameters:{type:'object',properties:{},additionalProperties:false},execute:async(_id,_args,signal)=>{
  const response=await fetch(baseUrl+'/scheduler/policy',{method:'POST',headers:{Authorization:'Bearer '+key,'Content-Type':'application/json'},body:JSON.stringify({action}),signal});
  if(!response.ok)throw Error(await response.text());const value=await response.json();if(!value.applied)throw Error('No active automation policy was changed.');
  return {content:[{type:'text',text:JSON.stringify(value)}],details:value};
 }}));
}
