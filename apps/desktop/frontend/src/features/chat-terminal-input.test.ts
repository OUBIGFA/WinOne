import {expect,it} from 'vitest';
import {createTerminalInput} from './chat-terminal-input';
it('buffers early input and serializes later chunks without losing or reordering keystrokes',async()=>{
 const sent:string[]=[];let finish!:()=>void;
 const queue=createTerminalInput(async text=>{sent.push(text);if(sent.length===1)await new Promise<void>(resolve=>{finish=resolve;});},error=>{throw error;});
 queue.enqueue('Write-');queue.enqueue('Output ');await Promise.resolve();expect(sent).toEqual([]);
 queue.attach();await Promise.resolve();expect(sent).toEqual(['Write-Output ']);
 queue.enqueue('proof');queue.enqueue('\r');await Promise.resolve();expect(sent).toHaveLength(1);
 finish();await new Promise(resolve=>setTimeout(resolve,0));expect(sent).toEqual(['Write-Output ','proof\r']);
 queue.close();queue.enqueue('late');await Promise.resolve();expect(sent).toHaveLength(2);
});
it('surfaces a failed write without replaying a possibly partly consumed command',async()=>{
 const sent:string[]=[],errors:unknown[]=[];
 const queue=createTerminalInput(async text=>{sent.push(text);throw Error('pipe closed');},error=>errors.push(error));
 queue.attach();queue.enqueue('command\r');await new Promise(resolve=>setTimeout(resolve,0));queue.enqueue('next\r');await Promise.resolve();
 expect(sent).toEqual(['command\r']);expect(String(errors[0])).toContain('pipe closed');
});
