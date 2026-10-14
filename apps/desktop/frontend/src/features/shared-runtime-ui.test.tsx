// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,render,screen,waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {ModuleInstallation} from './module-installation';
import {ModuleRemoval} from './module-removal';
import {ModulesPage} from './modules';
const api=vi.hoisted(()=>({moduleCall:vi.fn(),pickModulePackage:vi.fn()}));
vi.mock('../bridge',()=>api);
afterEach(()=>{cleanup();vi.resetAllMocks()});
const identity={kind:'runtime',id:'example.shared',version:'1.0.0',manifestSha256:'c'.repeat(64)};
const metadata={kind:'runtime',version:'1.0.0',runtime:{id:'example.shared',name:{'zh-CN':'共享引擎','en-US':'Shared engine'}}};

it('previews a runtime without module activation text and installs through the existing confirmation',async()=>{
 api.pickModulePackage.mockResolvedValue('C:\\fixture\\shared.winone-runtime');
 api.moduleCall.mockImplementation(async(method:string)=>method==='installPackage'?{state:'completed',kind:'runtime'}:{id:'shared-install',state:'ready',operation:'install',metadata,fileName:'shared.winone-runtime',canInstall:true,publisher:'example'});
 const changed=vi.fn(),user=userEvent.setup();render(<ModuleInstallation disabled={false} onChanged={changed}/>);
 await user.click(screen.getByRole('button',{name:'安装模块'}));await screen.findByText('共享引擎');
 expect(screen.queryByText('安装后保持关闭，可在工具管理中开启。')).toBeNull();
 await user.click(screen.getByRole('button',{name:'确认安装'}));await waitFor(()=>expect(changed).toHaveBeenCalledWith({operation:'install',state:'completed',kind:'runtime'}));
 expect(api.moduleCall).toHaveBeenCalledWith('installPackage',{id:'shared-install'});
});

it('shows consumers and keeps used runtimes separate from module toggles',async()=>{
 api.moduleCall.mockResolvedValue({revision:0,modules:[],packages:{runtimes:[{...identity,name:metadata.runtime.name,requiredBy:[{kind:'module',id:'example.consumer',version:'2.0.0',manifestSha256:'d'.repeat(64)}],canRemove:false}],dependencies:{}}});
 render(<ModulesPage onChanged={()=>{}}/>);await screen.findByText('共享引擎');
 expect(screen.getByText(/example.consumer/)).toBeTruthy();
 expect(screen.queryByRole('switch')).toBeNull();
 expect(screen.getByRole('button',{name:'卸载依赖'}).hasAttribute('disabled')).toBe(true);
});

it('keeps module controls available when shared inventory cannot be read',async()=>{
 api.moduleCall.mockResolvedValue({revision:0,modules:[],packages:{error:'共享依赖登记已损坏',runtimes:null,dependencies:null}});
 render(<ModulesPage onChanged={()=>{}}/>);
 await screen.findByText('共享依赖登记已损坏');
 expect(screen.getByRole('button',{name:'安装模块'})).toBeTruthy();
});

it('does not describe an unreadable dependency as unused',async()=>{
 api.moduleCall.mockResolvedValue({revision:0,modules:[],packages:{runtimes:[{...identity,name:metadata.runtime.name,requiredBy:[],canRemove:false,error:'引用信息无法校验'}],dependencies:{}}});
 render(<ModulesPage onChanged={()=>{}}/>);await screen.findByText('引用信息无法校验');
 expect(screen.queryByText('当前没有模块使用')).toBeNull();
 expect(screen.getByRole('button',{name:'卸载依赖'}).hasAttribute('disabled')).toBe(true);
 expect(screen.getByRole('button',{name:'卸载依赖'}).getAttribute('title')).not.toBe('仍被其他模块使用');
});

it('keeps the selected runtime version in the removal preview and confirmation',async()=>{
 api.moduleCall.mockImplementation(async(method:string)=>method==='packageRemoval.preview'?{id:'shared-remove',metadata}:{state:'completed'});
 const changed=vi.fn(),user=userEvent.setup();render(<ModuleRemoval id={identity.id} kind="runtime" manifestSha256={identity.manifestSha256} disabled={false} onChanged={changed}/>);
 await user.click(screen.getByRole('button',{name:'卸载依赖'}));await screen.findByText('共享引擎 · 1.0.0');
 expect(api.moduleCall).toHaveBeenCalledWith('packageRemoval.preview',{id:identity.id,kind:'runtime',manifestSha256:identity.manifestSha256});
 await user.click(screen.getByRole('button',{name:'确认卸载'}));await waitFor(()=>expect(changed).toHaveBeenCalledOnce());
 expect(api.moduleCall).toHaveBeenCalledWith('removePackage',{id:'shared-remove'});
});
