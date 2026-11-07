// The caller owns redirects and response disposal. Reuse Cindy's actual
// single-hop SSRF gate: DNS checks, pinned addresses and the final authorization
// check must all apply to the socket used by this request (not a second lookup).
import {fetchSingleHopWithSsrFGuard,SsrFBlockedError} from '../../cindy/ghost/packages/browser-control-runtime/src/shim/ssrf-runtime.ts';
import {DownloadError} from '../../cindy/ghost/apps/desktop/src/main/downloader/types.ts';

export function guardedOutboundFetch(url: string, init: RequestInit = {}, beforeDispatch: () => void | Promise<void> = () => {}) {
 const parsed=new URL(url);
 if(parsed.username || parsed.password) throw new Error('Credentials in a plugin URL are not allowed');
 return fetchSingleHopWithSsrFGuard({url, init, signal:init.signal??undefined, requireHttps:true, beforeDispatch});
}

// A security refusal is not a transient network outage. Preserve its typed
// terminal result at the downloader boundary instead of retrying a blocked hop.
export async function guardedDownloadFetch(url:string,init:RequestInit={},beforeDispatch:()=>void|Promise<void>=()=>{}){
 try{return await guardedOutboundFetch(url,init,beforeDispatch);}
 catch(error){if(error instanceof SsrFBlockedError)throw new DownloadError('URL_POLICY','Download network policy refused this URL');throw error;}
}

// Fetch-shaped consumer adapter for real OAuth/secret-check code. Unlike the
// lower-level single-hop API, consuming or cancelling this body also releases
// the pinned dispatcher, so those callers cannot leak an outbound connection.
export async function guardedResponseFetch(url: string, init: RequestInit = {}, beforeDispatch: () => void | Promise<void> = () => {}): Promise<Response> {
 const {response, release}=await guardedOutboundFetch(url,init,beforeDispatch);
 if(!response.body){await release();return response;}
 const reader=response.body.getReader();
 const body=new ReadableStream<Uint8Array>({
  async pull(controller){
   try {const part=await reader.read();if(part.done){controller.close();await release();}else controller.enqueue(part.value);}
   catch(error){controller.error(error);await release();}
  },
  async cancel(reason){try{await reader.cancel(reason);}finally{await release();}},
 });
 return new Response(body,{status:response.status,statusText:response.statusText,headers:response.headers});
}
