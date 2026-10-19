import {afterEach,expect,it,vi} from "vitest";
import {createAccountReader} from "./ai-account-reader";
const call=vi.hoisted(()=>vi.fn());vi.mock("../bridge",()=>({captureCall:call}));
afterEach(()=>{vi.resetAllMocks();vi.useRealTimers();});
it("shares catalog discovery and retries after a failure",async()=>{
 const reader=createAccountReader();call.mockRejectedValueOnce(Error("offline"));
 await expect(reader.catalog()).rejects.toThrow("offline");
 let finish!:(value:unknown)=>void;call.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve;}));
 const first=reader.catalog(),second=reader.catalog();expect(second).toBe(first);
 finish({agents:[{id:"codex",name:"Codex"}]});await first;
 expect(await reader.catalog()).toEqual({agents:[{id:"codex",name:"Codex"}]});
 expect(call).toHaveBeenCalledTimes(2);
});
it("rediscovers installed account types after a plugin mutation",async()=>{
 const reader=createAccountReader();
 call.mockResolvedValueOnce({agents:[]}).mockResolvedValueOnce({agents:[{id:"thirdparty",name:"Third party",backend:"plugin"}]});
 await reader.catalog();reader.invalidateCatalog();
 expect((await reader.catalog()).agents[0].id).toBe("thirdparty");
 expect(call).toHaveBeenCalledTimes(2);
});
it.each([false,true])("replaces an invalidated pending catalog, including failure=%s",async fail=>{
 const reader=createAccountReader();let finish!:(value:unknown)=>void,reject!:(error:Error)=>void;
 call.mockImplementationOnce(()=>new Promise((resolve,no)=>{finish=resolve;reject=no;}));
 const old=reader.catalog();reader.invalidateCatalog();
 const latest={agents:[{id:"installed",name:"Installed",backend:"plugin"}]};
 call.mockResolvedValueOnce(latest);const current=reader.catalog();await current;
 if(fail)reject(Error("obsolete discovery"));else finish({agents:[]});
 expect(await old).toEqual(latest);
 expect(await reader.catalog()).toEqual(latest);expect(call).toHaveBeenCalledTimes(2);
});
it("expires summaries and keeps provider and backend identities separate",async()=>{
 vi.useFakeTimers();const reader=createAccountReader();let sequence=0;
 call.mockImplementation(async()=>({accounts:[{agent:"cline",user:String(++sequence)}]}));
 const first={provider:"a",agent:"cline",draft:{agent:"cline",agentBackend:"plugin"}};
 expect((await reader.read(first)).accounts?.[0].user).toBe("1");
 expect((await reader.read({...first,provider:"b"})).accounts?.[0].user).toBe("2");
 expect((await reader.read({...first,draft:{agent:"cline",agentBackend:"builtin"}})).accounts?.[0].user).toBe("3");
 expect((await reader.read(first)).accounts?.[0].user).toBe("1");
 await vi.advanceTimersByTimeAsync(30_001);
 expect((await reader.read(first)).accounts?.[0].user).toBe("4");
});
it("retries failed reads and does not restore an invalidated in-flight summary",async()=>{
 const reader=createAccountReader(),request={agent:"codex",provider:"a"};
 call.mockRejectedValueOnce(Error("unavailable"));await expect(reader.read(request)).rejects.toThrow("unavailable");
 let finish!:(value:unknown)=>void;call.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve;}));
 const pending=reader.read(request);reader.invalidate();
 call.mockResolvedValue({accounts:[{agent:"codex",user:"new"}]});
 expect((await reader.read(request)).accounts?.[0].user).toBe("new");
 finish({accounts:[{agent:"codex",user:"old"}]});await pending;
 expect((await reader.read(request)).accounts?.[0].user).toBe("new");
});
