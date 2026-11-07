// Exercise the production page shim with browser primitives, not a rewritten
// relay algorithm. Its own HTTP echo must never be delivered to the sender.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
const source=fs.readFileSync(new URL('./page-shim.js',import.meta.url),'utf8');
test('BroadcastChannel delivers once to siblings and never echoes to its sender', async()=>{
 const posts=[]; let stream;
 const sandbox={URL,EventTarget,MessageEvent,queueMicrotask,crypto:globalThis.crypto,
  location:{href:'http://127.0.0.1:1234/scope/__page_panel__',origin:'http://127.0.0.1:1234'},
  document:{currentScript:{src:'http://127.0.0.1:1234/scope/__page_shim__'}},
  MutationObserver:class{observe(){}},
  EventSource:class{constructor(){stream=this}close(){}},
  fetch:async(url,init)=>{posts.push(JSON.parse(init.body));return new Response(null,{status:204})},
 };
 vm.runInNewContext(source,sandbox);
 const sender=new sandbox.BroadcastChannel('fixture'),sibling=new sandbox.BroadcastChannel('fixture');
 let own=0;const received=[];sender.onmessage=()=>own++;sibling.onmessage=e=>received.push(e.data);
 sender.postMessage({question:42}); await new Promise(resolve=>setImmediate(resolve));
 assert.deepEqual(received,[{question:42}]);
 stream.onmessage({data:JSON.stringify(posts[0])});
 assert.equal(own,0,'own server echo reached its sender');
 assert.equal(received.length,1,'sibling got local and server duplicates');
 stream.onmessage({data:JSON.stringify({channel:'fixture',source:'another-page',data:{answer:42}})});
 assert.equal(own,1);assert.equal(received.length,2);
 sender.close();sibling.close();
});
