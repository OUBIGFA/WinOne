// @vitest-environment jsdom
import {useState} from 'react';
import {afterEach,expect,it,vi} from 'vitest';
import {act,cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {AIProviderKeys} from './ai-provider-keys';
import type {AIProvider} from './ai-configuration';
const call=vi.hoisted(()=>vi.fn());
vi.mock('../bridge',()=>({captureCall:call}));
afterEach(()=>{cleanup();call.mockReset();});
const provider:AIProvider={id:'one',name:'Fixture',authMode:'api-key',hasKey:true,models:[],keys:[{id:'first',hasKey:true,enabled:true},{id:'second',hasKey:true,enabled:false}]};

it('repeatedly shows and hides each saved key without editing the form',async()=>{
 call.mockImplementation(async(_method,{id})=>({key:`fixture-${id}`}));
 const change=vi.fn(),edit=vi.fn();render(<AIProviderKeys provider={provider} edits={{}} disabled={false} onChange={change} onEdit={edit}/>);
 const user=userEvent.setup(),first=screen.getByLabelText('API 密钥 1') as HTMLInputElement;
 expect(call).not.toHaveBeenCalled();expect(first.type).toBe('password');expect(first.value).toBe('');
 for(let cycle=0;cycle<3;cycle++){
  await user.click(screen.getByRole('button',{name:'显示 API 密钥 1'}));
  await waitFor(()=>expect(first.value).toBe('fixture-first'));expect(first.type).toBe('text');
  await user.click(screen.getByRole('button',{name:'隐藏 API 密钥 1'}));
  expect(first.value).toBe('');expect(first.type).toBe('password');
 }
 await user.click(screen.getByRole('button',{name:'显示 API 密钥 2'}));
 await waitFor(()=>expect((screen.getByLabelText('API 密钥 2') as HTMLInputElement).value).toBe('fixture-second'));
 expect(first.value).toBe('');expect(change).not.toHaveBeenCalled();expect(edit).not.toHaveBeenCalled();
 expect(call).toHaveBeenCalledWith('ai.keys.reveal',{provider:'one',id:'second'});
});

it('toggles an unsaved replacement without requesting or resubmitting the stored key',async()=>{
 const edit=vi.fn();
 function Draft(){const [edits,setEdits]=useState<Record<string,string>>({first:'draft-secret'});return <AIProviderKeys provider={provider} edits={edits} disabled={false} onChange={()=>{}} onEdit={(id,value)=>{edit(id,value);setEdits(value?{[id]:value}:{});}}/>;}
 render(<Draft/>);const user=userEvent.setup(),input=screen.getByLabelText('API 密钥 1') as HTMLInputElement;
 for(let cycle=0;cycle<2;cycle++){
  await user.click(screen.getByRole('button',{name:'显示 API 密钥 1'}));expect(input.type).toBe('text');expect(input.value).toBe('draft-secret');
  await user.click(screen.getByRole('button',{name:'隐藏 API 密钥 1'}));expect(input.type).toBe('password');expect(input.value).toBe('draft-secret');
 }
 expect(call).not.toHaveBeenCalled();expect(edit).not.toHaveBeenCalled();
 await user.click(screen.getByRole('button',{name:'显示 API 密钥 1'}));
 fireEvent.change(input,{target:{value:'replacement'}});expect(edit).toHaveBeenLastCalledWith('first','replacement');expect(input.value).toBe('replacement');
});

it('keeps a failed reveal hidden and allows another attempt',async()=>{
 call.mockRejectedValueOnce(Error('读取失败，请重试')).mockResolvedValue({key:'fixture-first'});
 const edit=vi.fn();render(<AIProviderKeys provider={provider} edits={{}} disabled={false} onChange={vi.fn()} onEdit={edit}/>);const user=userEvent.setup();
 await user.click(screen.getByRole('button',{name:'显示 API 密钥 1'}));await screen.findByRole('alert');
 const input=screen.getByLabelText('API 密钥 1') as HTMLInputElement;expect(input.type).toBe('password');expect(input.value).toBe('');
 await user.click(screen.getByRole('button',{name:'显示 API 密钥 1'}));await waitFor(()=>expect(input.value).toBe('fixture-first'));expect(screen.queryByRole('alert')).toBeNull();expect(edit).not.toHaveBeenCalled();
});

it('clears revealed or pending saved keys when the saved configuration is reset',async()=>{
 let finish!:(value:{key:string})=>void;call.mockReturnValueOnce(new Promise(resolve=>{finish=resolve;})).mockResolvedValue({key:'current-secret'});
 const props={provider,edits:{},disabled:false,onChange:vi.fn(),onEdit:vi.fn()};
 const view=render(<AIProviderKeys {...props} resetToken={1}/>),user=userEvent.setup();
 await user.click(screen.getByRole('button',{name:'显示 API 密钥 1'}));
 view.rerender(<AIProviderKeys {...props} resetToken={2}/>);
 await act(async()=>finish({key:'stale-secret'}));
 const input=screen.getByLabelText('API 密钥 1') as HTMLInputElement;expect(input.value).toBe('');expect(input.type).toBe('password');
 await user.click(screen.getByRole('button',{name:'显示 API 密钥 1'}));await waitFor(()=>expect(input.value).toBe('current-secret'));
 view.rerender(<AIProviderKeys {...props} resetToken={3}/>);expect(input.value).toBe('');expect(input.type).toBe('password');
});

it('discards a late reveal when its key row is removed',async()=>{
 let finish!:(value:{key:string})=>void;call.mockReturnValue(new Promise(resolve=>{finish=resolve;}));
 const props={edits:{},disabled:false,onChange:vi.fn(),onEdit:vi.fn()};
 const view=render(<AIProviderKeys {...props} provider={provider}/>);const user=userEvent.setup();
 await user.click(screen.getByRole('button',{name:'显示 API 密钥 1'}));
 view.rerender(<AIProviderKeys {...props} provider={{...provider,keys:[provider.keys![1]]}}/>);
 await act(async()=>finish({key:'stale-secret'}));
 expect((screen.getByLabelText('API 密钥') as HTMLInputElement).value).toBe('');expect(screen.queryByDisplayValue('stale-secret')).toBeNull();
});
