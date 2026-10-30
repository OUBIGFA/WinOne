// WinOne persistence/HTTP seams for Cindy's original PR extraction, gh token
// source and status cache. Tokens never leave this worker or enter task rows.
import {randomUUID} from 'node:crypto';
import {desc,eq} from 'drizzle-orm';
import {sessionPrRefs} from '../cindy/tasks/db/schema.ts';
import {extractPrRefs} from '../cindy/tasks/git/prRefExtractor.ts';
import {PrStatusService} from '../cindy/tasks/git/prStatusService.ts';
import {getSharedGhCliTokenSource} from '../cindy/tasks/git/ghCliTokenSource.ts';

export function createOneBotPrService(store,{readToken,fetchPr}={}){
 const statuses=new PrStatusService({readToken:readToken??(()=>getSharedGhCliTokenSource().readTokenDetailed()),fetchPr:fetchPr??fetchGitHubPr});
 function refs(sessionId){return store.drizzle.select().from(sessionPrRefs).where(eq(sessionPrRefs.sessionId,sessionId)).orderBy(desc(sessionPrRefs.lastSeenAt)).all();}
 return {
  refs,
  record(sessionId,urls){
   const parsed=extractPrRefs(urls.join('\n')),now=Date.now();
   for(const ref of parsed)store.drizzle.insert(sessionPrRefs).values({id:randomUUID(),sessionId,...ref,firstSeenAt:now,lastSeenAt:now}).onConflictDoUpdate({target:[sessionPrRefs.sessionId,sessionPrRefs.owner,sessionPrRefs.repo,sessionPrRefs.prNumber],set:{lastSeenAt:now}}).run();
   return {recorded:parsed.length};
  },
  async statuses(sessionId){return statuses.getStatuses(refs(sessionId).slice(0,3));},
  close(){statuses.invalidate();},
 };
}

async function fetchGitHubPr(token,{owner,repo,prNumber}){
 const headers={Authorization:'Bearer '+token,Accept:'application/vnd.github+json','X-GitHub-Api-Version':'2022-11-28'};
 const response=await fetch(`https://api.github.com/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/pulls/${prNumber}`,{headers,signal:AbortSignal.timeout(15000)});
 if(!response.ok)throw Object.assign(Error('PR query failed'),{status:response.status});
 const pr=await response.json();
 if(!pr||!['open','closed'].includes(pr.state)||typeof pr.title!=='string')throw Error('Invalid PR response');
 let unresolved=null;
 // Match Cindy's bounded review-thread count. GraphQL failure does not erase
 // a successfully read REST state and is represented explicitly as unknown.
 try{
  const response=await fetch('https://api.github.com/graphql',{method:'POST',headers:{...headers,'Content-Type':'application/json'},body:JSON.stringify({query:'query($owner:String!,$repo:String!,$number:Int!){repository(owner:$owner,name:$repo){pullRequest(number:$number){reviewThreads(first:100){nodes{isResolved}}}}}',variables:{owner,repo,number:prNumber}}),signal:AbortSignal.timeout(10000)});
  if(response.ok){const data=await response.json();const threads=data.data?.repository?.pullRequest?.reviewThreads?.nodes;if(Array.isArray(threads))unresolved=threads.filter(thread=>!thread.isResolved).length;}
 }catch{/* Optional thread detail remains unknown; the original status service owns retries. */}
 return {...pr,branch:pr.head?.ref??'',unresolved_count:unresolved};
}
