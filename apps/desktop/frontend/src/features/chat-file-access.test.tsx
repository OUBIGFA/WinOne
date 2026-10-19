// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,render,screen,waitFor} from '@testing-library/react';
import {inspectChatPaths,invalidateChatFileStats} from './chat-file-access';
import {ChatFileNavigation,ChatInlineCode} from './chat-file-navigation';
const native=vi.hoisted(()=>({call:vi.fn()}));vi.mock('../bridge',()=>({chatCall:native.call}));
afterEach(()=>{cleanup();invalidateChatFileStats();vi.resetAllMocks();});
it('coalesces sibling metadata checks into bounded batches without creating previews',async()=>{
 native.call.mockImplementation(async(method,{paths})=>{expect(method).toBe('file.stat');return {files:paths.map((path:string)=>({requested:path,file:{path,name:path,directory:false,size:4,modified:0}}))};});
 const paths=Array.from({length:150},(_,i)=>`file-${i}.md`);
 const [a,b]=await Promise.all([inspectChatPaths('burst',paths),inspectChatPaths('burst',paths)]);
 expect(a).toHaveLength(150);expect(b).toEqual(a);expect(native.call).toHaveBeenCalledTimes(3);
 expect(Math.max(...native.call.mock.calls.map(([,p])=>p.paths.length))).toBeLessThanOrEqual(64);
});
it('does not stat ordinary identifiers after resolving them against an absolute CWD',async()=>{
 render(<ChatFileNavigation id="identifiers" cwd="C:/project"><ChatInlineCode>userId</ChatInlineCode><ChatInlineCode>getValue()</ChatInlineCode></ChatFileNavigation>);
 await Promise.resolve();expect(native.call).not.toHaveBeenCalled();expect(screen.getByText('userId').closest('a')).toBeNull();
});
it('recognizes a directory path as a folder instead of a failing file preview',async()=>{
 native.call.mockImplementation(async(_method,{paths})=>({files:paths.map((path:string)=>({requested:path,file:{path:'C:/project/docs',name:'docs',directory:true,size:0,modified:0}}))}));
 const open=vi.fn();render(<ChatFileNavigation id="directory" cwd="C:/project" onDirectory={open}><ChatInlineCode>docs/</ChatInlineCode></ChatFileNavigation>);
 await waitFor(()=>expect(screen.getByText('docs/').closest('a')).not.toBeNull());screen.getByText('docs/').click();expect(open).toHaveBeenCalledWith(expect.stringMatching(/docs\/?$/));
});
