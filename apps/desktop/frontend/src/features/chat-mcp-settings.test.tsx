// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {ChatSettings} from './chat-settings';
const call=vi.hoisted(()=>vi.fn());
vi.mock('../bridge',()=>({chatCall:call,pickDirectory:vi.fn(),setFileDropTarget:vi.fn(async()=>{})}));
afterEach(()=>{cleanup();vi.resetAllMocks();});
const settings={workspaceDirectory:'C:\\work',skillDirectories:[],pluginDirectories:[],automations:[],mcpServers:[{name:'fixture',enabled:true,url:'http://127.0.0.1:12345',headers:{'X-Fixture':'saved'},toolExposure:{read:'direct'}}]};
async function open(){
 // The automation connector section on the MCP tab loads its own mappings.
 call.mockImplementation(async(method,params)=>method==='settings.get'?settings:method==='automations.connections.get'?{settings:{methods:{}},servers:[]}:params.settings);
 render(<ChatSettings open onOpenChange={()=>{}} providers={[]} defaultModel={{provider:'',model:''}} onSaved={()=>{}} onConversation={()=>{}}/>);
 const user=userEvent.setup();await user.click(await screen.findByRole('tab',{name:'MCP'}));
 await user.click(screen.getByText('高级'));return user;
}
it('saves MCP environment, headers and per-tool access overrides together',async()=>{
 const user=await open();
 fireEvent.change(screen.getByRole('textbox',{name:'环境变量（JSON 对象）'}),{target:{value:'{"FIXTURE":"local"}'}});
 fireEvent.change(screen.getByRole('textbox',{name:'请求头（JSON 对象）'}),{target:{value:'{"X-Fixture":"edited"}'}});
 fireEvent.change(screen.getByRole('textbox',{name:'单个工具调用方式（JSON）'}),{target:{value:'{"read":"deferred","remove":"hidden"}'}});
 await user.click(screen.getByRole('button',{name:'保存设置'}));
 await waitFor(()=>expect(call).toHaveBeenCalledWith('settings.save',{settings:{...settings,mcpServers:[{...settings.mcpServers[0],env:{FIXTURE:'local'},headers:{'X-Fixture':'edited'},toolExposure:{read:'deferred',remove:'hidden'}}]}}));
});
it('preserves invalid JSON across tabs and disables save until corrected',async()=>{
 const user=await open();const input=screen.getByRole('textbox',{name:'单个工具调用方式（JSON）'});
 fireEvent.change(input,{target:{value:'{"read":'}});
 expect(screen.getByRole('button',{name:'保存设置'})).toHaveProperty('disabled',true);
 expect(screen.getByRole('alert').textContent).toContain('JSON');
 await user.click(screen.getByRole('tab',{name:'常规'}));await user.click(screen.getByRole('tab',{name:'MCP'}));
 // Details may be remounted when switching tabs, but its draft must survive.
 const advanced=screen.getByText('高级');if(!advanced.closest('details')?.open)await user.click(advanced);
 expect(screen.getByRole('textbox',{name:'单个工具调用方式（JSON）'})).toHaveProperty('value','{"read":');
 fireEvent.change(screen.getByRole('textbox',{name:'单个工具调用方式（JSON）'}),{target:{value:'{"read":"invalid"}'}});
 expect(screen.getByRole('button',{name:'保存设置'})).toHaveProperty('disabled',true);
 fireEvent.change(screen.getByRole('textbox',{name:'单个工具调用方式（JSON）'}),{target:{value:'{"read":"hidden"}'}});
 expect(screen.queryByRole('alert')).toBeNull();await user.click(screen.getByRole('button',{name:'保存设置'}));
 expect(call).toHaveBeenLastCalledWith('settings.save',expect.objectContaining({settings:expect.objectContaining({mcpServers:[expect.objectContaining({toolExposure:{read:'hidden'}})]})}));
});
it('resets invalid drafts and keeps the saved map when removing another server',async()=>{
 const user=await open();await user.click(screen.getByRole('button',{name:'添加 MCP 服务'}));
 const advanced=screen.getAllByText('高级');await user.click(advanced[1]);
 const fields=screen.getAllByRole('textbox',{name:'单个工具调用方式（JSON）'});
 fireEvent.change(fields[1],{target:{value:'{"broken":'}});
 await user.click(screen.getByRole('button',{name:'移除 ·'}));
 expect(screen.queryByRole('alert')).toBeNull();
 fireEvent.change(screen.getByRole('textbox',{name:'单个工具调用方式（JSON）'}),{target:{value:'{"broken":'}});
 await user.click(screen.getByRole('button',{name:'重置'}));
 expect(screen.queryByRole('alert')).toBeNull();
 expect(JSON.parse((screen.getByRole('textbox',{name:'单个工具调用方式（JSON）'}) as HTMLTextAreaElement).value)).toEqual({read:'direct'});
 expect(screen.getByRole('button',{name:'保存设置'})).toHaveProperty('disabled',true);
});
