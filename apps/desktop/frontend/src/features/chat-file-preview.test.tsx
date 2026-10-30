// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {act,cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {ChatFilePreview} from './chat-file-preview';
import {invalidateChatFileStats} from './chat-file-access';
const io=vi.hoisted(()=>({call:vi.fn(),fetch:vi.fn()}));
vi.mock('../bridge',()=>({chatCall:io.call}));
// I/O lifecycle is tested independently of CodeMirror geometry. Pointer and
// retained-selection coverage runs the real editor in the browser workbench.
vi.mock('./cindy/editor/FileDocument',()=>({FileDocument:({value,onChange}:any)=><textarea aria-label="document" value={value} onChange={e=>onChange(e.target.value)}/>}));
let content:string,modified:number,missing:boolean;
const props={id:'watch',path:'C:/fixture/note.txt',onDirty:vi.fn(),onHandle:vi.fn()};
const metadata=()=>({path:props.path,name:'note.txt',kind:'text',size:content.length,modified,revision:String(modified),url:'http://localhost/note.txt',mimeType:'text/plain'});
beforeEach(()=>{
 content='initial';modified=1;missing=false;invalidateChatFileStats();
 io.call.mockImplementation(async(method:string)=>{
  if(method==='file.preview')return metadata();
  if(method==='file.stat')return {files:[{requested:props.path,...(missing?{missing:true}:{file:{path:props.path,name:'note.txt',size:content.length,modified}})}]};
 });
 io.fetch.mockImplementation(async()=>({ok:true,text:async()=>content}));vi.stubGlobal('fetch',io.fetch);
});
afterEach(()=>{cleanup();vi.unstubAllGlobals();vi.clearAllMocks();});
const focus=()=>act(()=>{window.dispatchEvent(new Event('focus'));});
const value=()=> (screen.getByRole('textbox',{name:'document'}) as HTMLTextAreaElement).value;
it('refreshes a changed file without replacing its editor and does not refetch unchanged content',async()=>{
 render(<ChatFilePreview {...props}/>);const node=await screen.findByRole('textbox',{name:'document'});
 content='external update';modified++;
 focus();await waitFor(()=>expect(value()).toBe(content));expect(screen.getByRole('textbox',{name:'document'})).toBe(node);
 const reads=io.fetch.mock.calls.length;focus();await waitFor(()=>expect(io.call).toHaveBeenCalledWith('file.stat',expect.anything()));
 expect(io.fetch).toHaveBeenCalledTimes(reads);expect(props.onDirty).toHaveBeenLastCalledWith(false);
 expect(io.call.mock.calls.some(([method])=>method==='file.save')).toBe(false);
});
it('protects dirty drafts and discards a refresh whose response arrives after editing',async()=>{
 render(<ChatFilePreview {...props}/>);const node=await screen.findByRole('textbox',{name:'document'});
 let release!:(value:any)=>void;
 io.fetch.mockImplementationOnce(()=>new Promise(resolve=>{release=resolve;}));
 content='disk changed';modified++;focus();await waitFor(()=>expect(release).toBeTypeOf('function'));
 fireEvent.change(node,{target:{value:'unsaved draft'}});
 await act(async()=>{release({ok:true,text:async()=>content});});
 expect(value()).toBe('unsaved draft');expect(props.onDirty).toHaveBeenLastCalledWith(true);
 const reads=io.fetch.mock.calls.length;focus();expect(io.fetch).toHaveBeenCalledTimes(reads);
});
it('pauses hidden tabs and catches up when reactivated',async()=>{
 const view=render(<ChatFilePreview {...props} active={false}/>);await screen.findByRole('textbox',{name:'document'});
 content='while hidden';modified++;focus();expect(value()).toBe('initial');
 expect(io.call.mock.calls.some(([method])=>method==='file.stat')).toBe(false);
 view.rerender(<ChatFilePreview {...props} active/>);await waitFor(()=>expect(value()).toBe(content));
});
it('retains readable content after disappearance and clears the warning on recovery',async()=>{
 render(<ChatFilePreview {...props}/>);await screen.findByRole('textbox',{name:'document'});
 missing=true;focus();await screen.findByRole('alert');expect(value()).toBe('initial');
 missing=false;focus();await waitFor(()=>expect(screen.queryByRole('alert')).toBeNull());expect(value()).toBe('initial');
});
