// @vitest-environment jsdom
import {afterAll,afterEach,beforeAll,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen,waitFor,within} from '@testing-library/react';
import {useEffect,useState} from 'react';
import {ChatSidePanel} from './chat-side-panel';
import {ChatDesktopSurface} from './chat-desktop-kit';
import type {ChatFileHandle} from './chat-file-preview';

// Mock the OS/content boundary, not tab controls: real Cindy tab, menu, focus
// and confirmation components are exercised, including retained editor drafts.
vi.mock('../bridge',()=>({chatCall:vi.fn(async()=>({}))}));
vi.mock('./chat-file-preview',()=>({ChatFilePreview:({path,onDirty,onHandle}:{path:string;onDirty:(value:boolean)=>void;onHandle:(handle:ChatFileHandle)=>void})=>{
 const [text,setText]=useState('');useEffect(()=>{onHandle({dirty:()=>!!text,save:async()=>{onDirty(false);return true;}});},[text]);
 return <textarea aria-label={'Edit '+path} value={text} onChange={event=>{setText(event.target.value);onDirty(true);}}/>;
}}));
vi.mock('./cindy/file-browser/FileBrowser',()=>({FileBrowser:({active,onOpenFile}:{active:boolean;onOpenFile:(path:string)=>void})=><div data-testid="file-browser" data-active={active}><button onClick={()=>onOpenFile('C:/fixture/note.md')}>Open note</button></div>}));
vi.mock('./chat-resources',()=>({ChatResourcesPanel:({active}:{active:boolean})=><div data-testid="resource-browser" data-active={active}>Resource inventory</div>}));
vi.mock('./chat-browser',()=>({ChatBrowserView:({active,tabId}:{active:boolean;tabId:string})=><div data-testid={tabId} data-active={active}>Browser content</div>}));
vi.mock('./chat-terminal',()=>({ChatTerminalView:({active,tabId}:{active:boolean;tabId:string})=><div data-testid={tabId} data-active={active}>Terminal content</div>}));
vi.mock('./chat-review',()=>({ChatReviewView:()=> <div>Review content</div>}));
vi.mock('./cindy/editor/MermaidLightboxHost',()=>({MermaidLightboxHost:()=>null}));
vi.mock('./cindy/editor/MermaidSourceEditor',()=>({MermaidSourceEditorHost:()=>null}));
const scrollToDescriptor=Object.getOwnPropertyDescriptor(HTMLElement.prototype,'scrollTo');
beforeAll(()=>Object.defineProperty(HTMLElement.prototype,'scrollTo',{configurable:true,value:vi.fn()}));
afterAll(()=>{if(scrollToDescriptor)Object.defineProperty(HTMLElement.prototype,'scrollTo',scrollToDescriptor);else Reflect.deleteProperty(HTMLElement.prototype,'scrollTo');});
afterEach(()=>{cleanup();localStorage.clear();vi.clearAllMocks();});
const defaults={id:'fixture',initialTab:'home' as const,onClose:vi.fn(),onReference:vi.fn(),onComment:vi.fn(async()=>{}),onReviewComment:vi.fn(),commentMarkerNumbers:[],onDirtyChange:vi.fn(),onDock:vi.fn(),dock:'right' as const};
const open=()=>render(<ChatDesktopSurface><ChatSidePanel {...defaults}/></ChatDesktopSurface>);

it('exposes only the two fixed library subviews without an add/close-tab control',async()=>{
 open();const sections=screen.getByRole('tablist',{name:'右侧栏分区'});expect(within(sections).getAllByRole('tab')).toHaveLength(2);
 fireEvent.click(within(sections).getByRole('tab',{name:'文件与资源'}));await screen.findByTestId('file-browser');
 const library=screen.getByRole('tabpanel',{name:'文件与资源'});expect(within(library).getAllByRole('tab').map(tab=>tab.textContent)).toEqual(['文件','资源']);
 expect(within(library).queryByRole('button',{name:/添加标签|关闭标签/})).toBeNull();
 fireEvent.click(within(library).getByRole('tab',{name:'资源'}));expect(screen.getByTestId('resource-browser').dataset.active).toBe('true');expect(screen.getByTestId('file-browser').dataset.active).toBe('false');
 expect(JSON.parse(localStorage.getItem('winone.chat.panel.v1:fixture')!).tabs.map((tab:{kind:string})=>tab.kind)).toEqual(['home']);
});
it('keeps drafts and native tab state across major-tab switches and protects a dirty tab on close',async()=>{
 localStorage.setItem('winone.chat.panel.v1:fixture',JSON.stringify({tabs:[{id:'file:note',kind:'file',path:'note.md',title:'note.md'},{id:'browser:1',kind:'browser',url:'about:blank'},{id:'terminal:1',kind:'terminal'}],selected:'file:note',section:'tabs',libraryTab:'files'}));
 open();const editor=await screen.findByRole('textbox',{name:'Edit note.md'});fireEvent.change(editor,{target:{value:'Unsaved work'}});
 await waitFor(()=>expect(defaults.onDirtyChange).toHaveBeenLastCalledWith(true));
 fireEvent.click(screen.getByRole('tab',{name:'文件与资源'}));fireEvent.click(screen.getByRole('tab',{name:'标签页'}));
 expect(screen.getByRole('textbox',{name:'Edit note.md'})).toBe(editor);expect((editor as HTMLTextAreaElement).value).toBe('Unsaved work');
 // Ctrl+W affects only the selected dynamic tab, never a fixed library view.
 fireEvent.keyDown(editor,{key:'w',ctrlKey:true});expect(await screen.findByRole('alertdialog')).toBeTruthy();fireEvent.click(screen.getByRole('button',{name:'取消'}));await waitFor(()=>expect(screen.queryByRole('alertdialog')).toBeNull());expect((editor as HTMLTextAreaElement).value).toBe('Unsaved work');
 fireEvent.click(screen.getByRole('tab',{name:'浏览器'}));await waitFor(()=>expect(screen.getByTestId('browser:1').dataset.active).toBe('true'));
 fireEvent.click(screen.getByRole('tab',{name:'文件与资源'}));expect(screen.getByTestId('browser:1').dataset.active).toBe('false');
 fireEvent.keyDown(screen.getByRole('tab',{name:'文件'}),{key:'w',ctrlKey:true});expect(screen.queryByRole('alertdialog')).toBeNull();
 fireEvent.click(screen.getByRole('tab',{name:'标签页'}));expect(screen.getByTestId('browser:1').dataset.active).toBe('true');expect(screen.getByTestId('terminal:1').dataset.active).toBe('false');
});
it('routes library file opens into the existing document tab area and restores that tab on reopen',async()=>{
 const first=open();fireEvent.click(screen.getByRole('tab',{name:'文件与资源'}));fireEvent.click(await screen.findByRole('button',{name:'Open note'}));
 await screen.findByRole('textbox',{name:'Edit C:/fixture/note.md'});expect(screen.getByRole('tab',{name:'标签页'}).getAttribute('aria-selected')).toBe('true');
 first.unmount();open();expect(await screen.findByRole('textbox',{name:'Edit C:/fixture/note.md'})).toBeTruthy();
});
