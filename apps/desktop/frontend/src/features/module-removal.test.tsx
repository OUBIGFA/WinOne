// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,render,screen,waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {ModuleRemoval} from './module-removal';

const api=vi.hoisted(()=>({moduleCall:vi.fn()}));
vi.mock('../bridge',()=>api);
afterEach(()=>{cleanup();vi.resetAllMocks()});
const preview={id:'remove-owned',metadata:{version:'1.0.0',module:{name:{'zh-CN':'示例工具','en-US':'Example tool'}}}};

it('shows the module before confirmation and cancel leaves it installed',async()=>{
 api.moduleCall.mockResolvedValue(preview);
 const changed=vi.fn(),user=userEvent.setup();render(<ModuleRemoval id="example.tool" disabled={false} onChanged={changed}/>);
 await user.click(screen.getByRole('button',{name:'卸载模块'}));await screen.findByText('示例工具 · 1.0.0');
 await user.click(screen.getByRole('button',{name:'取消'}));await waitFor(()=>expect(screen.queryByRole('dialog')).toBeNull());
 expect(api.moduleCall).toHaveBeenCalledWith('packagePreview.cancel',{id:'remove-owned'});
 expect(api.moduleCall.mock.calls.some(([method])=>method==='removePackage')).toBe(false);expect(changed).not.toHaveBeenCalled();
});

it('confirms once and preserves the file-reclamation warning',async()=>{
 api.moduleCall.mockImplementation(async(method:string)=>method==='packageRemoval.preview'?preview:{state:'completed',warning:'文件仍待回收'});
 const changed=vi.fn(),user=userEvent.setup();render(<ModuleRemoval id="example.tool" disabled={false} onChanged={changed}/>);
 await user.click(screen.getByRole('button',{name:'卸载模块'}));await screen.findByText('示例工具 · 1.0.0');
 await user.click(screen.getByRole('button',{name:'确认卸载'}));await waitFor(()=>expect(screen.queryByRole('dialog')).toBeNull());
 expect(api.moduleCall).toHaveBeenCalledWith('removePackage',{id:'remove-owned'});expect(changed).toHaveBeenCalledWith('文件仍待回收');
});

it('shows a failed removal and requires fresh confirmation before retrying',async()=>{
 api.moduleCall.mockImplementation(async(method:string)=>method==='packageRemoval.preview'?preview:method==='removePackage'?{state:'failed',error:'模块状态已变化'}:{});
 const changed=vi.fn(),user=userEvent.setup();render(<ModuleRemoval id="example.tool" disabled={false} onChanged={changed}/>);
 await user.click(screen.getByRole('button',{name:'卸载模块'}));await screen.findByText('示例工具 · 1.0.0');
 await user.click(screen.getByRole('button',{name:'确认卸载'}));await screen.findByText(/模块状态已变化/);
 expect(screen.getByRole('button',{name:'确认卸载'}).hasAttribute('disabled')).toBe(true);expect(changed).not.toHaveBeenCalled();
 await user.click(screen.getByRole('button',{name:'取消'}));await user.click(screen.getByRole('button',{name:'卸载模块'}));await screen.findByText('示例工具 · 1.0.0');
 expect(screen.getByRole('button',{name:'确认卸载'}).hasAttribute('disabled')).toBe(false);
});
