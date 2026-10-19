// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {ChatGeneratedFiles} from './chat-generated-files';
import {invalidateChatFileStats} from './chat-file-access';
import {ChatMarkdown} from './chat-markdown';
const bridge=vi.hoisted(()=>({call:vi.fn()}));
vi.mock('../bridge',()=>({chatCall:bridge.call}));
afterEach(()=>{cleanup();invalidateChatFileStats();vi.restoreAllMocks();});

it('opens only existing generated files and preserves copy-path and reveal actions',async()=>{
 bridge.call.mockImplementation(async(method,params)=>{if(method==='file.stat')return {files:params.paths.map((path:string)=>path==='missing'?{requested:path,missing:true}:{requested:path,file:{path:'C:/work/report.md',name:'report.md',directory:false,size:9,modified:0}})};});
 const open=vi.fn();render(<ChatGeneratedFiles id="chat" paths={['report.md','report.md','missing']} onOpen={open}/>);
 const chip=await screen.findByRole('button',{name:'report.md'});expect(screen.queryByRole('button',{name:'missing'})).toBeNull();
 await userEvent.click(chip);expect(open).toHaveBeenCalledWith('C:/work/report.md');
 expect(screen.queryByRole('menu')).toBeNull();
 open.mockClear();fireEvent.contextMenu(chip,{button:2,clientX:30,clientY:50});
 await screen.findByRole('menu');expect(open).not.toHaveBeenCalled();
 await userEvent.click(screen.getByRole('menuitem',{name:'在资源管理器中显示'}));
 expect(bridge.call).toHaveBeenCalledWith('file.reveal',{id:'chat',path:'C:/work/report.md'});
 expect(bridge.call.mock.calls.filter(([method])=>method==='file.stat')).toHaveLength(1);
 expect(bridge.call.mock.calls.some(([method])=>method==='file.preview')).toBe(false);
 expect(screen.queryByText(/暂时无法读取/)).toBeNull();
});

it('copies complete fenced code through the local clipboard and reports failure locally',async()=>{
 const write=vi.fn().mockResolvedValue(undefined);Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:write}});
 render(<ChatMarkdown text={'```text\n中文代码\nsecond line\n```'}/>);
 await userEvent.click(screen.getByRole('button',{name:'复制代码'}));expect(write).toHaveBeenCalledWith('中文代码\nsecond line\n');
 write.mockRejectedValueOnce(Error('denied'));await userEvent.click(screen.getByRole('button',{name:'已复制'}));
 await waitFor(()=>expect(screen.getByRole('button',{name:'复制失败'})).toBeTruthy());
});
