// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {act,cleanup,renderHook,waitFor} from '@testing-library/react';
import {joinWorkspace,useWorkspaceTree} from './cindy/file-browser/host';
const native=vi.hoisted(()=>({call:vi.fn()}));
vi.mock('../bridge',()=>({chatCall:native.call}));
afterEach(()=>{cleanup();vi.resetAllMocks();vi.unstubAllGlobals();});

it('keeps an absolute selected file intact when the tree root changes',()=>{
 expect(joinWorkspace('C:/project','C:/other/note.md')).toBe('C:/other/note.md');
 expect(joinWorkspace('C:/project','src/main.ts')).toBe('C:/project/src/main.ts');
});

it('refreshes expanded folders without losing their open state',async()=>{
 let fresh=false;
 native.call.mockImplementation(async(_method,{path})=>({url:'http://fixture/'+encodeURIComponent(path)}));
 vi.stubGlobal('fetch',vi.fn(async(url:string)=>({ok:true,json:async()=>{
  const path=decodeURIComponent(url.slice('http://fixture/'.length));
  return {directory:path||'C:/project',entries:path.endsWith('/src')
   ?[{name:fresh?'new.ts':'old.ts',path:'C:/project/src/'+(fresh?'new.ts':'old.ts'),directory:false,size:12}]
   :[{name:'src',path:'C:/project/src',directory:true,size:0}]};
 }})));
 const {result}=renderHook(()=>useWorkspaceTree('session'));
 await waitFor(()=>expect(result.current.tree.initialLoading).toBe(false));
 act(()=>result.current.tree.toggleFolder('src'));
 await waitFor(()=>expect(result.current.tree.entries.get('src')?.[0].name).toBe('old.ts'));
 fresh=true;act(()=>result.current.refresh());
 await waitFor(()=>expect(result.current.tree.entries.get('src')?.[0].name).toBe('new.ts'));
 expect(result.current.tree.expanded.has('src')).toBe(true);
});
