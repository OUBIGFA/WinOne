import {expect,it,vi} from 'vitest';
import {createChatReadQueue,scheduleChatCall} from './chat-read-queue';
it('admits a burst without exceeding the native read budget',async()=>{
 const read=createChatReadQueue(3);let active=0,peak=0;
 const values=await Promise.all(Array.from({length:90},(_,i)=>read('file:'+i,async()=>{active++;peak=Math.max(peak,active);await Promise.resolve();active--;return i;})));
 expect(peak).toBe(3);expect(values).toHaveLength(90);expect(values[89]).toBe(89);
});
it('coalesces identical in-flight reads and releases failures before retry',async()=>{
 const read=createChatReadQueue(1),operation=vi.fn(async()=>42);
 expect(await Promise.all([read('same',operation),read('same',operation)])).toEqual([42,42]);expect(operation).toHaveBeenCalledTimes(1);
 await expect(read('bad',async()=>{throw Error('failed');})).rejects.toThrow('failed');
 await expect(read('bad',async()=>7)).resolves.toBe(7);
});
it('does not serialize stop or file writes behind background reads',async()=>{
 const stop=vi.fn(async()=>'stopped'),save=vi.fn(async()=>'saved');
 const first=scheduleChatCall('stop','{}',stop),second=scheduleChatCall('file.save','{}',save);
 expect(stop).toHaveBeenCalledTimes(1);expect(save).toHaveBeenCalledTimes(1);expect(await Promise.all([first,second])).toEqual(['stopped','saved']);
});
it('bounds pending work rather than creating unlimited native jobs',async()=>{
 const read=createChatReadQueue(1,1);let finish!:(value:number)=>void;
 const a=read('a',()=>new Promise<number>(resolve=>{finish=resolve;}));const b=read('b',async()=>2);
 await expect(read('c',async()=>3)).rejects.toThrow('队列已满');
 finish(1);expect(await Promise.all([a,b])).toEqual([1,2]);
});
