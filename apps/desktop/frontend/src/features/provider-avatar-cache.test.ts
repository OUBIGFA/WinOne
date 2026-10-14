import {beforeEach,expect,it,vi} from "vitest";
import {captureCall} from "../bridge";
import {loadProviderAvatar,storeProviderAvatar} from "./provider-avatar-cache";

vi.mock("../bridge",()=>({captureCall:vi.fn()}));
const call=vi.mocked(captureCall);
beforeEach(()=>{call.mockReset();});

it("transfers a maximum-sized picture without overflowing one RPC frame",async()=>{
 const bytes=new Uint8Array(1<<20);bytes.fill(200);const received=new Uint8Array(bytes.length);let offset=0;
 call.mockImplementation(async(method,args:any)=>{
  expect(method).toBe("ai.icons.store");expect(JSON.stringify(args).length).toBeLessThan(1<<20);
  expect(args.offset).toBe(offset);expect(args.upload).toBe(offset?"upload-receipt":"");
  const chunk=Uint8Array.from(atob(args.data),c=>c.charCodeAt(0));received.set(chunk,offset);offset+=chunk.length;
  return offset===bytes.length?{icon:"file:0123456789abcdef.png"}:{upload:"upload-receipt",offset};
 });
 expect(await storeProviderAvatar(bytes)).toBe("file:0123456789abcdef.png");
 expect(new Uint8Array(received)).toEqual(bytes);
});

it("reads one immutable avatar once for concurrent UI consumers",async()=>{
 call.mockResolvedValue({data:btoa("image fixture"),size:13,mimeType:"image/png"});
 const [a,b]=await Promise.all([loadProviderAvatar("file:1123456789abcdef.png"),loadProviderAvatar("file:1123456789abcdef.png")]);
 expect(a).toBe("data:image/png;base64,aW1hZ2UgZml4dHVyZQ==");expect(b).toBe(a);expect(call).toHaveBeenCalledTimes(1);
});

it("does not cache failed reads or accept a zero-length chunk",async()=>{
 call.mockResolvedValueOnce({data:"",size:10,mimeType:"image/png"});
 await expect(loadProviderAvatar("file:2123456789abcdef.png")).rejects.toThrow();
 call.mockResolvedValueOnce({data:btoa("ok"),size:2,mimeType:"image/png"});
 await expect(loadProviderAvatar("file:2123456789abcdef.png")).resolves.toBe("data:image/png;base64,b2s=");
});
