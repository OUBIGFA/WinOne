// @vitest-environment jsdom
import {cleanup,render,screen,fireEvent,waitFor} from '@testing-library/react';
import {afterEach,beforeEach,describe,it,expect,vi} from 'vitest';
import {useState} from 'react';
import {AIPlugins,type BuiltinPlugin} from './ai-plugins';
import type {MagpieSettings} from './ai-configuration';
import '../i18n';
const call=vi.hoisted(()=>vi.fn());
vi.mock('../bridge',()=>({captureCall:call}));
const plugins:BuiltinPlugin[]=[{id:'gateway',name:'Gateway',sub:'Gateway connections',on:true,chosen:true,essential:true,state:'active',inUse:0,config:{callsPage:20},schema:{properties:{callsPage:{type:'integer',title:'Recent calls at a time',enum:[10,20,50,100],default:20}},'x-order':['callsPage']}},{id:'search',name:'Web search',sub:'Search tools',on:true,chosen:true,essential:true,state:'active',inUse:1},{id:'library',name:'Library',sub:'Shared resources',on:true,chosen:true,essential:false,state:'active',inUse:0}];
function Harness(){const [settings,setSettings]=useState<MagpieSettings>({}),[invalid,setInvalid]=useState(false);return <><AIPlugins settings={settings} disabled={false} onValidityChange={setInvalid} onChange={patch=>setSettings({...settings,...patch})}/><output data-testid="settings">{JSON.stringify(settings)}</output><button disabled={invalid}>Save fixture</button></>;}
afterEach(()=>cleanup());
beforeEach(()=>{call.mockReset();call.mockResolvedValue({builtinPlugins:plugins,bundledPlugins:[{directory:'calc',package:'@magpie-community/tools-calc',name:'Calc',version:'0.1.0',kind:'tools',providers:[]}]});});
describe('Magpie plugins',()=>{
 it('keeps preset overrides and protects features in use',async()=>{render(<Harness/>);await screen.findByRole('radio',{name:'全部功能'});expect(screen.getByRole('switch',{name:'网页搜索'}).getAttribute('aria-disabled')).toBe('true');fireEvent.click(screen.getByRole('switch',{name:'资源库'}));expect(screen.getByTestId('settings').textContent).toContain('"library":{"enabled":false}');fireEvent.click(screen.getByRole('radio',{name:'常用功能'}));expect(screen.getByTestId('settings').textContent).toContain('"builtinPreset":"essentials"');expect(screen.getByRole('switch',{name:'资源库'}).getAttribute('aria-checked')).toBe('false');});
 it('persists bundled enable/options, rejects invalid JSON without losing text',async()=>{render(<Harness/>);await screen.findByRole('radio',{name:'全部功能'});fireEvent.click(screen.getByRole('tab',{name:'内置扩展'}));fireEvent.click(screen.getByRole('switch',{name:'Calc'}));expect(screen.getByTestId('settings').textContent).toContain('"calc":{"enabled":true}');fireEvent.click(screen.getAllByText('详细信息').at(-1)!);const field=await screen.findByRole('textbox',{name:'插件选项（JSON）'});fireEvent.change(field,{target:{value:'{"precision":'}});expect((field as HTMLTextAreaElement).value).toBe('{"precision":');await waitFor(()=>expect((screen.getByRole('button',{name:'Save fixture'}) as HTMLButtonElement).disabled).toBe(true));fireEvent.change(field,{target:{value:'{"precision":20}'}});expect(screen.getByTestId('settings').textContent).toContain('"precision":20');await waitFor(()=>expect((screen.getByRole('button',{name:'Save fixture'}) as HTMLButtonElement).disabled).toBe(false));});
 it('does not load plugins while another tab is active',()=>{render(<AIPlugins active={false} settings={{}} disabled={false} onChange={()=>{}}/>);expect(call).not.toHaveBeenCalled();});
});
