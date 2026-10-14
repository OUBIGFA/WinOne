// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {copyText} from './clipboard';

afterEach(()=>{vi.unstubAllGlobals();vi.restoreAllMocks();document.body.innerHTML='';});
it('propagates a native clipboard rejection without silently retrying another API',async()=>{
 const error=new Error('Clipboard permission denied'),fallback=vi.fn(()=>true);
 vi.stubGlobal('navigator',{clipboard:{writeText:vi.fn().mockRejectedValue(error)}});
 Object.defineProperty(document,'execCommand',{configurable:true,value:fallback});
 await expect(copyText('A local message')).rejects.toBe(error);expect(fallback).not.toHaveBeenCalled();
});
it('uses the local legacy clipboard only when the native API is absent and restores focus',async()=>{
 vi.stubGlobal('navigator',{});const input=document.createElement('input');document.body.append(input);input.focus();
 Object.defineProperty(document,'execCommand',{configurable:true,value:vi.fn(()=>true)});
 await copyText('Local text');expect(document.activeElement).toBe(input);expect(document.querySelector('textarea')).toBeNull();
});
it('reports a failed legacy clipboard write and cleans up its temporary element',async()=>{
 vi.stubGlobal('navigator',{});Object.defineProperty(document,'execCommand',{configurable:true,value:vi.fn(()=>false)});
 await expect(copyText('Local text')).rejects.toThrow('Clipboard unavailable');expect(document.querySelector('textarea')).toBeNull();
});
