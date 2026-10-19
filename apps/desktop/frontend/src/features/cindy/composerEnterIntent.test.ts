import {expect,it} from 'vitest';
import {resolveComposerEnterIntent} from './composerEnterIntent';

const enter={key:'Enter',shiftKey:false,altKey:false,ctrlKey:false,metaKey:false,repeat:false,isComposing:false};
it('keeps Cindy Enter/steer and modifier-enter semantics on Windows and macOS',()=>{
 expect(resolveComposerEnterIntent(enter,'enter',{turnRunning:true,platform:'win32'})).toBe('queue');
 expect(resolveComposerEnterIntent({...enter,ctrlKey:true},'enter',{turnRunning:true,platform:'win32'})).toBe('steer');
 expect(resolveComposerEnterIntent({...enter,metaKey:true},'enter',{turnRunning:true,platform:'darwin'})).toBe('steer');
 expect(resolveComposerEnterIntent({...enter,ctrlKey:true},'enter',{turnRunning:false,platform:'win32'})).toBe('queue');
 expect(resolveComposerEnterIntent(enter,'modifier-enter',{turnRunning:true,platform:'win32'})).toBe('native');
 expect(resolveComposerEnterIntent({...enter,ctrlKey:true},'modifier-enter',{turnRunning:true,platform:'win32'})).toBe('queue');
});
it('leaves IME and newlines alone and never repeats a submission',()=>{
 for(const modifier of ['shiftKey','altKey'])expect(resolveComposerEnterIntent({...enter,[modifier]:true},'enter',{turnRunning:true,platform:'win32'})).toBeNull();
 expect(resolveComposerEnterIntent({...enter,isComposing:true},'enter',{turnRunning:true,platform:'win32'})).toBe('native');
 expect(resolveComposerEnterIntent({...enter,repeat:true},'enter',{turnRunning:true,platform:'win32'})).toBe('ignore');
});
