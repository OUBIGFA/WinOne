// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,render,screen,waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {ChatGeneratedFiles} from './chat-generated-files';
import {ChatMarkdown} from './chat-markdown';
const bridge=vi.hoisted(()=>({call:vi.fn()}));
vi.mock('../bridge',()=>({chatCall:bridge.call}));
afterEach(()=>{cleanup();vi.restoreAllMocks();});

it('opens only existing generated files and preserves copy-path and reveal actions',async()=>{
 bridge.call.mockImplementation(async(method,params)=>{if(method==='file.preview'){if(params.path==='missing')throw Error('missing');return {path:'C:/work/report.md',name:'report.md',kind:'text',size:9,revision:'one',url:'http://127.0.0.1/report.md'};}});
 const open=vi.fn();render(<ChatGeneratedFiles id="chat" paths={['report.md','report.md','missing']} onOpen={open}/>);
 const chip=await screen.findByRole('button',{name:'report.md'});expect(screen.queryByRole('button',{name:'missing'})).toBeNull();
 await userEvent.click(chip);expect(open).toHaveBeenCalledWith('C:/work/report.md');
 expect(bridge.call.mock.calls.filter(([method])=>method==='file.preview')).toHaveLength(2);
});

it('copies complete fenced code through the local clipboard and reports failure locally',async()=>{
 const write=vi.fn().mockResolvedValue(undefined);Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:write}});
 render(<ChatMarkdown text={'```text\n中文代码\nsecond line\n```'}/>);
 await userEvent.click(screen.getByRole('button',{name:'复制代码'}));expect(write).toHaveBeenCalledWith('中文代码\nsecond line\n');
 write.mockRejectedValueOnce(Error('denied'));await userEvent.click(screen.getByRole('button',{name:'已复制'}));
 await waitFor(()=>expect(screen.getByRole('button',{name:'复制失败'})).toBeTruthy());
});
