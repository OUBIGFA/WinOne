// @vitest-environment jsdom
import {cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {afterEach,expect,it,vi} from 'vitest';
import userEvent from '@testing-library/user-event';
import {ChatMarkdown} from './chat-markdown';
import {ChatFileLink,ChatFileNavigation,ChatInlineCode} from './chat-file-navigation';
import {invalidateChatFileStats} from './chat-file-access';
const native=vi.hoisted(()=>({call:vi.fn(),media:vi.fn()}));
vi.mock('../bridge',()=>({chatCall:native.call,chatMediaCall:native.media}));
afterEach(()=>{cleanup();invalidateChatFileStats();vi.restoreAllMocks();vi.resetAllMocks();});
function setup(href='src/你好 file.ts:12:3',directory=false){
 const open=vi.fn(),folder=vi.fn(),browser=vi.fn(),copy=vi.fn(async()=>{});
 Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:copy}});
 native.call.mockImplementation(async(method,params)=>{
  if(method==='file.stat')return {files:params.paths.map((path:string)=>({requested:path,file:{path,name:'file.ts',directory,size:10,modified:0}}))};
  if(method==='file.preview')return {path:params.path,kind:'text',url:'http://127.0.0.1:1234/verified/document.html'};
  return {};
 });
 native.media.mockResolvedValue({success:true});
 render(<ChatFileNavigation id="session" cwd="C:/project" onOpen={open} onDirectory={folder} onBrowser={browser}><ChatFileLink href={href} directory={directory}>file</ChatFileLink></ChatFileNavigation>);
 return {open,folder,browser,copy,link:screen.getByRole('link',{name:'file'})};
}
it('opens a resolved file and line on left click without showing a menu',async()=>{
 const f=setup();await userEvent.click(f.link);await waitFor(()=>expect(f.open).toHaveBeenCalledWith('C:/project/src/你好 file.ts',12));
 expect(screen.queryByRole('menu')).toBeNull();expect(native.media).not.toHaveBeenCalled();
});
it.each([{ctrlKey:true},{shiftKey:true},{metaKey:true},{button:1}])('preserves the reading pane on separate activation %o',async modifiers=>{
 const f=setup();if(modifiers.button===1)fireEvent(f.link,new MouseEvent('auxclick',{bubbles:true,button:1}));else fireEvent.click(f.link,modifiers);
 await waitFor(()=>expect(native.call).toHaveBeenCalledWith('file.open',{id:'session',path:'C:/project/src/你好 file.ts'}));expect(f.open).not.toHaveBeenCalled();
});
it('shares the complete file menu, copies location and restores keyboard focus',async()=>{
 const f=setup();f.link.focus();fireEvent.keyDown(f.link,{key:'F10',shiftKey:true});
 await screen.findByRole('menu');
 for(const name of ['在侧栏中打开','使用系统应用打开','在资源管理器中显示','复制文件','复制路径','复制路径与行号'])expect(screen.getByRole('menuitem',{name})).toBeTruthy();
 await userEvent.click(screen.getByRole('menuitem',{name:'复制路径与行号'}));expect(f.copy).toHaveBeenCalledWith('src/你好 file.ts:12:3');
 await waitFor(()=>expect(document.activeElement).toBe(f.link));
 fireEvent.contextMenu(f.link,{clientX:50,clientY:60});await userEvent.click(await screen.findByRole('menuitem',{name:'复制文件'}));
 await waitFor(()=>expect(native.media).toHaveBeenCalledWith('copy-file',{id:'session',path:'C:/project/src/你好 file.ts'}));
});
it('keeps HTML page, browser and source actions separate',async()=>{
 const f=setup('report.html');fireEvent.contextMenu(f.link);
 await userEvent.click(await screen.findByRole('menuitem',{name:'在侧栏浏览器中打开'}));
 await waitFor(()=>expect(f.browser).toHaveBeenCalledWith('http://127.0.0.1:1234/verified/document.html'));
 fireEvent.contextMenu(f.link);await userEvent.click(await screen.findByRole('menuitem',{name:'查看源码'}));
 await waitFor(()=>expect(f.open).toHaveBeenCalledWith('C:/project/report.html',1));
 fireEvent.click(f.link,{shiftKey:true});await waitFor(()=>expect(native.call).toHaveBeenCalledWith('link.open',{url:'http://127.0.0.1:1234/verified/document.html'}));
});
it('uses the folder browser for directories and retains reveal/copy operations',async()=>{
 const f=setup('docs/',true);await userEvent.click(f.link);await waitFor(()=>expect(f.folder).toHaveBeenCalled());expect(f.open).not.toHaveBeenCalled();
 fireEvent.contextMenu(f.link);await userEvent.click(await screen.findByRole('menuitem',{name:'复制文件'}));
 await waitFor(()=>expect(native.media).toHaveBeenCalledWith('copy-file',{id:'session',path:expect.stringMatching(/docs\/$/)}));
});
it('does not navigate when stat reports a missing file or a denied path',async()=>{
 const f=setup();native.call.mockImplementation(async(_method,params)=>({files:params.paths.map((path:string)=>({requested:path,missing:true}))}));
 await userEvent.click(f.link);await screen.findByRole('alert');expect(f.open).not.toHaveBeenCalled();
});
it('never replaces an external URL or explicit absolute path with a same-named attachment',async()=>{
 const open=vi.fn();native.call.mockResolvedValue({});
 render(<ChatFileNavigation id="session" cwd="C:/project" files={[{name:'report.md',path:'C:/attachments/report.md'}]} onOpen={open}><ChatFileLink href="https://example.com/report.md">web</ChatFileLink></ChatFileNavigation>);
 await userEvent.click(screen.getByRole('link'));await waitFor(()=>expect(native.call).toHaveBeenCalledWith('link.open',{url:'https://example.com/report.md'}));expect(open).not.toHaveBeenCalled();
});
it('recognizes inline source coordinates and file URLs only after stat confirms existence',async()=>{
 native.call.mockImplementation(async(_method,params)=>({files:params.paths.map((path:string)=>({requested:path,file:{path,name:'file.ts',directory:false,size:10,modified:0}}))}));
 render(<ChatFileNavigation id="inline" cwd="C:/project"><ChatInlineCode>src/file.ts:12:3</ChatInlineCode><ChatInlineCode>file:///C:/project/%E4%B8%AD%E6%96%87.md</ChatInlineCode><ChatInlineCode>userId</ChatInlineCode></ChatFileNavigation>);
 await waitFor(()=>expect(screen.getAllByRole('button')).toHaveLength(2));expect(screen.getByText('userId').closest('a')).toBeNull();
});
it.each(['bare','inline','link','inline-spaces','link-spaces'])('renders a full Windows path through the shared Cindy chip (%s)',async format=>{
 const path=format.endsWith('spaces')?String.raw`C:\Program Files\WinOne\设计手册.md`:String.raw`D:\Data\Desktop\claude-code-frontend-design-中文.md`,open=vi.fn();
 native.call.mockImplementation(async(_method,params)=>({files:params.paths.map((path:string)=>({requested:path,file:{path,name:'claude-code-frontend-design-中文.md',directory:false,size:10,modified:0}}))}));
 const content=format==='bare'?`文件： ${path}`:format.startsWith('inline')?'`'+path+'`':`[设计手册](<${path}>)`;
 render(<ChatFileNavigation id="markdown" cwd="C:/project" onOpen={open}><ChatMarkdown text={content}/></ChatFileNavigation>);
 const chip=await screen.findByRole('button',{name:format.startsWith('link')?'设计手册':path});
 expect(chip.tagName).toBe('CODE');expect(chip.querySelector('svg')).toBeTruthy();
 await userEvent.click(chip);await waitFor(()=>expect(open).toHaveBeenCalledWith(path,undefined));
 expect(screen.queryByRole('menu')).toBeNull();
 fireEvent.contextMenu(chip);await screen.findByRole('menu');
 expect(screen.getByRole('menuitem',{name:'复制文件'})).toBeTruthy();
 expect(screen.getByRole('menuitem',{name:'复制路径'})).toBeTruthy();
});
