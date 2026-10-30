// @vitest-environment jsdom
import {act,cleanup,renderHook,waitFor} from '@testing-library/react';
import {afterEach,expect,it,vi} from 'vitest';
import {useChatNickname} from './interconnect-chat-preferences';
afterEach(cleanup);

it('persists a nickname across remounts, remains editable and never stores the room',async()=>{
 let stored={nickname:''};
 const native=vi.fn(async(method:string,params?:any)=>{if(method==='preferences.save')stored={...params};return {...stored};}) as any;
 const errors=vi.fn();
 const first=renderHook(()=>useChatNickname(native,errors));
 await waitFor(()=>expect(first.result.current.ready).toBe(true));
 act(()=>first.result.current.setNickname('  小明  '));
 await act(()=>first.result.current.save());
 expect(stored).toEqual({nickname:'小明'});
 first.unmount();
 const next=renderHook(()=>useChatNickname(native,errors));
 await waitFor(()=>expect(next.result.current.nickname).toBe('小明'));
 act(()=>next.result.current.setNickname('Alice'));
 await act(()=>next.result.current.save());
 expect(stored).toEqual({nickname:'Alice'});
 expect(errors).not.toHaveBeenCalled();
});

it('serializes blur and join saves and retains the saved default on failure',async()=>{
 let stored={nickname:'原昵称'},fail=true;
 const native=vi.fn(async(method:string,params?:any)=>{
  if(method==='preferences.save'){if(fail)throw new Error('disk full');stored={...params};}
  return {...stored};
 }) as any;
 const {result}=renderHook(()=>useChatNickname(native,vi.fn()));
 await waitFor(()=>expect(result.current.ready).toBe(true));
 await expect(result.current.save('草稿')).rejects.toThrow('disk full');
 expect(stored.nickname).toBe('原昵称');
 fail=false;
 await Promise.all([result.current.save('新昵称'),result.current.save('最终昵称'),result.current.save('最终昵称')]);
 expect(stored.nickname).toBe('最终昵称');
 expect(native.mock.calls.filter(([method]:[string])=>method==='preferences.save')).toHaveLength(3);
});

it('does not overwrite a default when reading fails and supports retry',async()=>{
 const error=vi.fn(),native=vi.fn().mockRejectedValueOnce(new Error('read failed')).mockResolvedValue({nickname:'恢复昵称'}) as any;
 const {result}=renderHook(()=>useChatNickname(native,error));
 await waitFor(()=>expect(error).toHaveBeenCalledWith('Error: read failed'));
 await expect(result.current.save('')).rejects.toThrow('尚未读取');
 expect(native).toHaveBeenCalledTimes(1);
 await act(()=>result.current.load());
 expect(result.current.nickname).toBe('恢复昵称');
});
