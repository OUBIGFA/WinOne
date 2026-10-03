import {expect,it,vi} from "vitest";
import {startCaptureLaunchReceiver,type CaptureLaunchRequest} from "./capture-launch";

it("reads queued startup work and acknowledges only after the view accepts it",async()=>{
  const queue:CaptureLaunchRequest[]=[{id:"1",draftId:"a".repeat(32)},{id:"2",draftId:"b".repeat(32)}];
  let notify=()=>{};
  let release!:(value:boolean)=>void;
  const first=new Promise<boolean>(resolve=>{release=resolve;});
  const received:string[]=[];
  const receiver=startCaptureLaunchReceiver({read:async()=>queue[0]??null,acknowledge:async id=>{if(queue[0]?.id!==id)return false;queue.shift();notify();return true;},subscribe:listener=>{notify=listener;return ()=>{};}},async request=>{received.push(request.id);return request.id==="1"?first:true;},error=>{throw error;});
  await vi.waitFor(()=>expect(received).toEqual(["1"]));
  notify();notify();
  expect(queue).toHaveLength(2);
  release(true);
  await vi.waitFor(()=>expect(received).toEqual(["1","2"]));
  await vi.waitFor(()=>expect(queue).toHaveLength(0));
  receiver.stop();
});

it("leaves busy work pending and retries when requested",async()=>{
  const request={id:"1",draftId:"a".repeat(32)};
  let busy=true,remaining:CaptureLaunchRequest|null=request;
  const receiver=startCaptureLaunchReceiver({read:async()=>remaining,acknowledge:async()=>{remaining=null;return true;},subscribe:()=>()=>{}},async()=>!busy,()=>{});
  await new Promise(resolve=>setTimeout(resolve,0));
  expect(remaining).toEqual(request);
  busy=false;receiver.retry();
  await vi.waitFor(()=>expect(remaining).toBeNull());
  receiver.stop();
});

it("does not consume a request after the receiving view has unmounted",async()=>{
  let release!:(value:boolean)=>void;
  const accepted=new Promise<boolean>(resolve=>{release=resolve;});
  const acknowledge=vi.fn(async()=>true);
  const handler=vi.fn(async()=>accepted);
  const receiver=startCaptureLaunchReceiver({read:async()=>({id:"1",draftId:"a".repeat(32)}),acknowledge,subscribe:()=>()=>{}},handler,()=>{});
  await vi.waitFor(()=>expect(handler).toHaveBeenCalledOnce());
  receiver.stop();release(true);
  await new Promise(resolve=>setTimeout(resolve,0));
  expect(acknowledge).not.toHaveBeenCalled();
});
