// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,render,screen,waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {ModuleInstallation} from './module-installation';
const api=vi.hoisted(()=>({moduleCall:vi.fn(),pickModulePackage:vi.fn()}));vi.mock('../bridge',()=>api);
afterEach(()=>{cleanup();vi.clearAllMocks()});
const ready={id:'owned',state:'ready',fileName:'tool.winone-module',metadata:{module:{id:'example.tool',name:{'zh-CN':'示例工具','en-US':'Example tool'}},version:'1.0.0'},publisher:'example',keyFingerprint:'a'.repeat(64),manifestSha256:'b'.repeat(64),payloadBytes:1024,canInstall:true};
it('shows the verified source before explicitly confirming installation',async()=>{
 api.pickModulePackage.mockResolvedValue('C:\\fixture\\tool.winone-module');api.moduleCall.mockImplementation(async(method:string)=>method==='installPackage'?{state:'completed'}:ready);
 const changed=vi.fn();const user=userEvent.setup();render(<ModuleInstallation disabled={false} onChanged={changed}/>);
 await user.click(screen.getByRole('button',{name:'安装模块'}));await screen.findByText('示例工具');
 expect(screen.getByText('example')).toBeTruthy();expect(screen.getByText('1.0.0')).toBeTruthy();expect(api.moduleCall.mock.calls.some(([method])=>method==='installPackage')).toBe(false);
 await user.click(screen.getByRole('button',{name:'确认安装'}));await waitFor(()=>expect(changed).toHaveBeenCalledOnce());
 expect(api.moduleCall).toHaveBeenCalledWith('installPackage',{id:'owned'});
});
it('cancels a preview without installing and blocks incompatible packages',async()=>{
 api.pickModulePackage.mockResolvedValue('C:\\fixture\\tool.winone-module');api.moduleCall.mockResolvedValue({...ready,canInstall:false,error:'模块不兼容'});
 const user=userEvent.setup();render(<ModuleInstallation disabled={false} onChanged={()=>{}}/>);
 await user.click(screen.getByRole('button',{name:'安装模块'}));await screen.findByText('模块不兼容');
 expect(screen.getByRole('button',{name:'确认安装'}).hasAttribute('disabled')).toBe(true);
 await user.click(screen.getByRole('button',{name:'取消'}));await waitFor(()=>expect(api.moduleCall).toHaveBeenCalledWith('packagePreview.cancel',{id:'owned'}));
 expect(api.moduleCall.mock.calls.some(([method])=>method==='installPackage')).toBe(false);
});
it('lets a failed package check retry the selected file before confirmation',async()=>{
 api.pickModulePackage.mockResolvedValue('C:\\fixture\\tool.winone-module');
 api.moduleCall.mockResolvedValueOnce({...ready,state:'failed',canInstall:false,error:'模块包检查已取消或超时，请重试'}).mockResolvedValueOnce(ready);
 const user=userEvent.setup();render(<ModuleInstallation disabled={false} onChanged={()=>{}}/>);
 await user.click(screen.getByRole('button',{name:'安装模块'}));await screen.findByText('模块包检查已取消或超时，请重试');
 expect(screen.getByRole('button',{name:'确认安装'}).hasAttribute('disabled')).toBe(true);
 await user.click(screen.getByRole('button',{name:'重新检查'}));await waitFor(()=>expect(screen.getByRole('button',{name:'确认安装'}).hasAttribute('disabled')).toBe(false));
 expect(api.pickModulePackage).toHaveBeenCalledOnce();expect(screen.queryByText('模块包检查已取消或超时，请重试')).toBeNull();
});
it('ignores a file selection that returns after its page closed',async()=>{
 let selected!:(value:string)=>void;api.pickModulePackage.mockReturnValue(new Promise<string>(resolve=>{selected=resolve}));
 const user=userEvent.setup();const view=render(<ModuleInstallation disabled={false} onChanged={()=>{}}/>);
 await user.click(screen.getByRole('button',{name:'安装模块'}));view.unmount();selected('C:\\fixture\\tool.winone-module');
 await waitFor(()=>expect(api.pickModulePackage).toHaveBeenCalledOnce());expect(api.moduleCall).not.toHaveBeenCalled();
});
it('shows the version change and confirms an update through the shared dialog',async()=>{
 api.pickModulePackage.mockResolvedValue('C:\\fixture\\update.winone-module');
 api.moduleCall.mockImplementation(async(method:string)=>method==='updatePackage'?{state:'completed',warning:'旧文件暂时被占用'}:{...ready,operation:'update',previousVersion:'1.0.0',metadata:{...ready.metadata,version:'1.1.0'},wasEnabled:true});
 const changed=vi.fn(),user=userEvent.setup();render(<ModuleInstallation disabled={false} onChanged={changed}/>);
 await user.click(screen.getByRole('button',{name:'安装模块'}));await screen.findByText('1.0.0 → 1.1.0');
 await user.click(screen.getByRole('button',{name:'确认更新'}));await waitFor(()=>expect(changed).toHaveBeenCalledWith({operation:'update',state:'completed',warning:'旧文件暂时被占用'}));
 expect(api.moduleCall).toHaveBeenCalledWith('updatePackage',{id:'owned'});await waitFor(()=>expect(screen.queryByRole('dialog')).toBeNull());
});
it('keeps a rollback error visible and refreshes the installed state',async()=>{
 api.pickModulePackage.mockResolvedValue('C:\\fixture\\update.winone-module');
 api.moduleCall.mockImplementation(async(method:string)=>{if(method==='updatePackage')throw Error('更新失败，已恢复旧版本');return {...ready,operation:'update',previousVersion:'0.9.0'}});
 const changed=vi.fn(),user=userEvent.setup();render(<ModuleInstallation disabled={false} onChanged={changed}/>);
 await user.click(screen.getByRole('button',{name:'安装模块'}));await screen.findByRole('button',{name:'确认更新'});
 await user.click(screen.getByRole('button',{name:'确认更新'}));await screen.findByText(/更新失败，已恢复旧版本/);
 expect(screen.getByRole('button',{name:'确认更新'}).hasAttribute('disabled')).toBe(true);
 expect(changed).toHaveBeenCalledWith({operation:'update',state:'failed'});
});
it('shows the configuration format and affected fields before confirmation',async()=>{
 api.pickModulePackage.mockResolvedValue('C:\\fixture\\update.winone-module');
 api.moduleCall.mockResolvedValue({...ready,operation:'update',previousVersion:'0.9.0',migrationFrom:1,migrationTo:2,migrationPaths:['/label','/oldName']});
 const user=userEvent.setup();render(<ModuleInstallation disabled={false} onChanged={()=>{}}/>);
 await user.click(screen.getByRole('button',{name:'安装模块'}));
 await screen.findByText('配置格式将从版本 1 转为 2；失败时恢复原配置。');
 await user.click(screen.getByText('涉及的配置字段'));expect(screen.getByText('/label')).toBeTruthy();expect(screen.getByText('/oldName')).toBeTruthy();
 expect(screen.getByRole('button',{name:'确认更新'}).hasAttribute('disabled')).toBe(false);
});
