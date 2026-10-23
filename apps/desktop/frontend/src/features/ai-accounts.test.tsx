// @vitest-environment jsdom
import {afterEach,expect,it,vi} from "vitest";
import {act,cleanup,render,screen,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {AIAccounts} from "./ai-accounts";
const api=vi.hoisted(()=>({captureCall:vi.fn()}));vi.mock("../bridge",()=>api);
afterEach(()=>{cleanup();vi.clearAllMocks();});
const catalog={agents:[{id:'codex',name:'ChatGPT / Codex'}]};
it('checks in only the chosen saved account and reports the actual result',async()=>{
 api.captureCall.mockImplementation(async(method:string)=>{
  if(method==='ai.accounts.list')return {accounts:[{agent:'qoder',user:'fixture'}],checkinAvailable:true};
  if(method==='ai.accounts.checkin')return {checkins:[{user:'fixture',outcome:'claimed',credit:12,asked:true}]};
  throw Error(method);
 });
 const user=userEvent.setup();render(<AIAccounts agent="qoder" provider="saved-qoder" catalog={[{id:'qoder',name:'Qoder'}]} disabled={false} onBusy={()=>{}}/>);
 await user.click(await screen.findByRole('button',{name:'立即签到'}));
 expect(api.captureCall).toHaveBeenCalledWith('ai.accounts.checkin',{agent:'qoder',provider:'saved-qoder'});
 await screen.findByText(/签到成功/);await screen.findByText(/12/);
});
it('shares the global account masking choice without exposing the identity in a tooltip',async()=>{
 api.captureCall.mockResolvedValue({accounts:[{agent:'codex',user:'private@example.invalid'}],methods:[]});
 const props={agent:'codex',catalog:catalog.agents,disabled:false,onBusy:()=>{}};
 const view=render(<AIAccounts {...props} hideAccounts/>);
 await screen.findByText('账号 1');expect(screen.queryByText('private@example.invalid')).toBeNull();
 expect(view.container.querySelector('[title="private@example.invalid"]')).toBeNull();
 view.rerender(<AIAccounts {...props} hideAccounts={false}/>);await screen.findByText('private@example.invalid');
});
it('continues an explicit plugin login once after methods load, never on ordinary navigation',async()=>{
 let resolveList!:(value:any)=>void;const handled=vi.fn();
 api.captureCall.mockImplementation(async(method:string)=>{
  if(method==='ai.accounts.list')return new Promise(resolve=>{resolveList=resolve;});
  return {id:'requested-job',status:{agent:'web-fixture',state:'input',url:'',prompt:{type:'password',key:'api-key',message:'Fixture key'}}};
 });
 const props={agent:'web-fixture',provider:'plugin-service',catalog:[{id:'web-fixture',name:'Web fixture'}],disabled:false,onBusy:()=>{}};
 const view=render(<AIAccounts {...props} signInRequest="click-1" onSignInRequestHandled={handled}/>);
 expect(api.captureCall.mock.calls.some(([method])=>method==='ai.accounts.signin.start')).toBe(false);
 await act(async()=>resolveList({accounts:[],methods:[{type:'api',label:'Key'}]}));
 await screen.findByLabelText('Fixture key');expect(handled).toHaveBeenCalledOnce();
 view.rerender(<AIAccounts {...props} signInRequest="click-1" onSignInRequestHandled={handled}/>);
 expect(api.captureCall.mock.calls.filter(([method])=>method==='ai.accounts.signin.start')).toHaveLength(1);
 expect(api.captureCall).toHaveBeenCalledWith('ai.accounts.signin.start',{agent:'web-fixture',provider:'plugin-service',method:0});
});

it('uses the plugin login method and keeps authorization keys in a password input',async()=>{
 let submitted:string|undefined;let selectedMethod:number|undefined;
 const status={agent:'cline',state:'input',url:'',prompt:{type:'password',key:'api-key',message:'Cline API key',placeholder:'cr_…'}};
 api.captureCall.mockImplementation(async(method:string,payload:any)=>{
  if(method==='ai.accounts.list')return {accounts:[],methods:[{type:'oauth',label:'浏览器登录'},{type:'api',label:'使用 API 密钥'}]};
  if(method==='ai.accounts.signin.start'){selectedMethod=payload.method;return {id:'plugin-login',status};}
  if(method==='ai.accounts.signin.get')return {id:'plugin-login',status};
  if(method==='ai.accounts.signin.submit'){submitted=payload.callback;return {};}
  if(method==='ai.accounts.signin.cancel')return {};
  throw Error(method);
 });
 const user=userEvent.setup();render(<AIAccounts agent="cline" provider="cline-service" catalog={[{id:'cline',name:'Cline'}]} disabled={false} onBusy={()=>{}}/>);
 await user.click(await screen.findByRole('combobox',{name:'登录方式'}));
 await user.click(await screen.findByRole('option',{name:'使用 API 密钥'}));
 await user.click(screen.getByRole('button',{name:'登录'}));
 const input=await screen.findByLabelText('Cline API key');expect(input.getAttribute('type')).toBe('password');
 expect(screen.queryByText('请在浏览器中完成登录')).toBeNull();
 await user.type(input,'fixture-authorization-key');await user.click(screen.getByRole('button',{name:'继续'}));
 expect(selectedMethod).toBe(1);expect(submitted).toBe('fixture-authorization-key');
});
it('signs in again to the same connection without creating another account or model selector',async()=>{
 let completed=false;const calls:string[]=[];const starts:unknown[]=[];
 api.captureCall.mockImplementation(async(method:string,payload:unknown)=>{calls.push(method);switch(method){
  case 'ai.accounts.catalog':return catalog;
  case 'ai.accounts.list':return {accounts:completed?[{agent:'codex',user:'test@example.invalid'}]:[]};
  case 'ai.accounts.models':return {models:[{id:'model-a',name:'Model A'}]};
  case 'ai.accounts.signin.start':starts.push(payload);return {id:'login-1',status:{agent:'codex',state:'waiting',url:'https://example.invalid/login'}};
  case 'ai.accounts.signin.get':completed=true;return {id:'login-1',status:{agent:'codex',state:'done',url:'https://example.invalid/login'}};
  case 'ai.accounts.signin.open':return {};
  default:throw Error(method);
 }});
 render(<AIAccounts agent="codex" provider="work-account" disabled={false} onBusy={()=>{}}/>);
 const login=await screen.findByRole('button',{name:'登录'});await waitFor(()=>expect(login.hasAttribute('disabled')).toBe(false));
 await userEvent.click(login);await screen.findByText('test@example.invalid');expect(screen.queryByRole('combobox',{name:'账号可用模型'})).toBeNull();expect(calls).not.toContain('ai.accounts.models');
 expect(calls.filter(method=>method==='ai.accounts.signin.start')).toHaveLength(1);
 const relogin=await screen.findByRole('button',{name:'重新登录'});
 await waitFor(()=>expect(relogin.hasAttribute('disabled')).toBe(false));await userEvent.click(relogin);
 expect(starts).toEqual([{agent:'codex',provider:'work-account'},{agent:'codex',provider:'work-account'}]);
 expect(screen.queryByRole('combobox')).toBeNull();
});
it('cancels the pending login when its settings panel closes',async()=>{
 const canceled:string[]=[];
 api.captureCall.mockImplementation(async(method:string,payload:{id?:string})=>{
  if(method==='ai.accounts.catalog')return catalog;if(method==='ai.accounts.list')return {accounts:[]};
  if(method==='ai.accounts.signin.start')return {id:'owned-login',status:{agent:'codex',state:'waiting',url:''}};
  if(method==='ai.accounts.signin.cancel'){canceled.push(payload.id!);return {}};
  return {id:'owned-login',status:{agent:'codex',state:'waiting',url:''}};
 });
 const {unmount}=render(<AIAccounts agent="codex" disabled={false} onBusy={()=>{}}/>);
 const login=await screen.findByRole('button',{name:'登录'});await waitFor(()=>expect(login.hasAttribute('disabled')).toBe(false));
 await userEvent.click(login);await screen.findByText('正在等待账号授权…');unmount();
 await waitFor(()=>expect(canceled).toContain('owned-login'));
});

it('shows and copies a device code for entry on the provider website',async()=>{
 const status={agent:'cline',state:'waiting',url:'https://example.invalid/device',code:'HFPM-MCLY'};
 api.captureCall.mockImplementation(async(method:string)=>method==='ai.accounts.list'?{accounts:[]}:method==='ai.accounts.signin.open'?{}:{id:'device-job',status});
 const user=userEvent.setup();const copy=vi.spyOn(navigator.clipboard,'writeText').mockResolvedValue();
 render(<AIAccounts agent="cline" provider="cline-account" catalog={[{id:'cline',name:'Cline'}]} disabled={false} onBusy={()=>{}}/>);
 await user.click(await screen.findByRole('button',{name:'登录'}));
 const code=await screen.findByLabelText('设备授权码');expect((code as HTMLInputElement).readOnly).toBe(true);
 await user.click(screen.getByRole('button',{name:'复制设备码'}));expect(copy).toHaveBeenCalledWith('HFPM-MCLY');
 expect(screen.getByText('在登录页面输入或确认此设备码，完成后会自动更新登录状态。')).toBeTruthy();
 expect(screen.queryByRole('button',{name:'提交'})).toBeNull();
});

it('does not reopen a plugin CLI authorization page, but keeps explicit reopening available',async()=>{
 const status={agent:'grok',state:'waiting',url:'https://example.invalid/device',opened:true};
 api.captureCall.mockImplementation(async(method:string)=>method==='ai.accounts.list'?{accounts:[]}:{id:'opened-job',status});
 const user=userEvent.setup();render(<AIAccounts agent="grok" provider="grok-account" catalog={[{id:'grok',name:'Grok'}]} disabled={false} onBusy={()=>{}}/>);
 await user.click(await screen.findByRole('button',{name:'登录'}));
 await waitFor(()=>expect(api.captureCall).toHaveBeenCalledWith('ai.accounts.signin.get',{id:'opened-job'}));
 expect(api.captureCall.mock.calls.some(([method])=>method==='ai.accounts.signin.open')).toBe(false);
 expect((screen.getByLabelText('登录链接') as HTMLInputElement).value).toBe(status.url);
 await user.click(screen.getByRole('button',{name:'打开登录页面'}));
 expect(api.captureCall).toHaveBeenCalledWith('ai.accounts.signin.open',{id:'opened-job'});
});

it('retains rejected authorization input and submits with Enter',async()=>{
 let rejected=false;const status={agent:'claude',state:'waiting',url:'',pasteCode:true};
 api.captureCall.mockImplementation(async(method:string)=>{
  if(method==='ai.accounts.list')return {accounts:[]};
  if(method==='ai.accounts.signin.submit')rejected=true;
  return {id:'paste-job',status,...(rejected?{inputError:'验证码不正确'}:{})};
 });
 const user=userEvent.setup();render(<AIAccounts agent="claude" provider="claude-account" catalog={[{id:'claude',name:'Claude Code'}]} disabled={false} onBusy={()=>{}}/>);
 await user.click(await screen.findByRole('button',{name:'登录'}));
 const input=await screen.findByLabelText('授权码');await user.type(input,'bad-code{Enter}');
 await screen.findByText('验证码不正确');expect((input as HTMLInputElement).value).toBe('bad-code');
 expect(api.captureCall).toHaveBeenCalledWith('ai.accounts.signin.submit',{id:'paste-job',callback:'bad-code'});
});

it('keeps the sign-in alive when the busy callback changes identity',async()=>{
 const canceled:string[]=[];const status={agent:'cline',state:'waiting',url:'',code:'ABCD-EFGH'};
 api.captureCall.mockImplementation(async(method:string,payload:any)=>{
  if(method==='ai.accounts.list')return {accounts:[]};
  if(method==='ai.accounts.signin.cancel')canceled.push(payload.id);
  return {id:'stable-job',status};
 });
 const user=userEvent.setup();const props={agent:'cline',provider:'connection',catalog:[{id:'cline',name:'Cline'}],disabled:false};
 const view=render(<AIAccounts {...props} onBusy={()=>{}}/>);await user.click(await screen.findByRole('button',{name:'登录'}));await screen.findByLabelText('设备授权码');
 view.rerender(<AIAccounts {...props} onBusy={()=>{}}/>);expect(canceled).toEqual([]);
 view.unmount();await waitFor(()=>expect(canceled).toContain('stable-job'));
});

it('keeps a waiting login recoverable when automatic browser opening fails',async()=>{
 const status={agent:'factory',state:'waiting',url:'https://example.invalid/device',code:'ABCD-EFGH'};
 api.captureCall.mockImplementation(async(method:string)=>{
  if(method==='ai.accounts.list')return {accounts:[]};
  if(method==='ai.accounts.signin.open')throw Error('browser unavailable');
  return {id:'browser-job',status};
 });
 const user=userEvent.setup();render(<AIAccounts agent="factory" provider="factory-account" catalog={[{id:'factory',name:'Factory'}]} disabled={false} onBusy={()=>{}}/>);
 await user.click(await screen.findByRole('button',{name:'登录'}));
 await screen.findByText('无法自动打开登录页面。可重试打开，或复制链接到浏览器继续。');
 expect(screen.getByRole('button',{name:'复制登录链接'})).toBeTruthy();
 expect(api.captureCall.mock.calls.some(([method])=>method==='ai.accounts.signin.cancel')).toBe(false);
});

it('blocks repeated submissions until validation and clears secrets on completion',async()=>{
 let pending=false,finished=false,submissions=0;
 const state=()=>({id:'key-job',status:{agent:'cline',state:finished?'done':'input',url:'',prompt:{type:'password',key:'api-key',message:'Cline API key'}},inputPending:pending});
 api.captureCall.mockImplementation(async(method:string)=>{
  if(method==='ai.accounts.list')return {accounts:[]};
  if(method==='ai.accounts.signin.submit'){submissions++;pending=true;}
  return state();
 });
 const user=userEvent.setup();render(<AIAccounts agent="cline" catalog={[{id:'cline',name:'Cline'}]} disabled={false} onBusy={()=>{}}/>);
 await user.click(await screen.findByRole('button',{name:'登录'}));
 const input=await screen.findByLabelText('Cline API key');await user.type(input,'private-fixture{Enter}{Enter}');
 expect(submissions).toBe(1);expect((input as HTMLInputElement).disabled).toBe(true);
 finished=true;await screen.findByText('登录成功，可获取模型列表。');expect(screen.queryByDisplayValue('private-fixture')).toBeNull();
});

it('cancels a login that starts after the panel has closed',async()=>{
 let reply!:(value:any)=>void;
 api.captureCall.mockImplementation(async(method:string)=>{
  if(method==='ai.accounts.list')return {accounts:[]};
  if(method==='ai.accounts.signin.start')return new Promise(resolve=>{reply=resolve;});
  return {};
 });
 const user=userEvent.setup();const view=render(<AIAccounts agent="cline" catalog={[{id:'cline',name:'Cline'}]} disabled={false} onBusy={()=>{}}/>);
 await user.click(await screen.findByRole('button',{name:'登录'}));view.unmount();
 await act(async()=>{reply({id:'late-job',status:{agent:'cline',state:'waiting',url:''}});});
 await waitFor(()=>expect(api.captureCall).toHaveBeenCalledWith('ai.accounts.signin.cancel',{id:'late-job'}));
});

it('shows the selected preliminary option hint and submits its value',async()=>{
 const status={agent:'zcode',state:'input',url:'',prompt:{type:'select',key:'site',message:'选择地区',options:[{label:'国际',value:'global',hint:'Use the global endpoint'},{label:'中国',value:'cn',hint:'使用中国区地址'}]}};
 api.captureCall.mockImplementation(async(method:string)=>method==='ai.accounts.list'?{accounts:[]}:{id:'prompt-job',status});
 const user=userEvent.setup();render(<AIAccounts agent="zcode" catalog={[{id:'zcode',name:'ZCode'}]} disabled={false} onBusy={()=>{}}/>);
 await user.click(await screen.findByRole('button',{name:'登录'}));await user.click(await screen.findByRole('combobox',{name:'选择地区'}));
 await user.click(await screen.findByRole('option',{name:'中国'}));expect(screen.getByText('使用中国区地址')).toBeTruthy();
 await user.click(screen.getByRole('button',{name:'继续'}));expect(api.captureCall).toHaveBeenCalledWith('ai.accounts.signin.submit',{id:'prompt-job',callback:'cn'});
});

it('clears the previous login when switching account connections',async()=>{
 api.captureCall.mockImplementation(async(method:string)=>method==='ai.accounts.list'?{accounts:[]}:{id:'first-job',status:{agent:'cline',state:'waiting',url:'',code:'ABCD-EFGH'}});
 const user=userEvent.setup();const props={agent:'cline',catalog:[{id:'cline',name:'Cline'}],disabled:false,onBusy:()=>{}};
 const view=render(<AIAccounts {...props} provider="first"/>);await user.click(await screen.findByRole('button',{name:'登录'}));await screen.findByLabelText('设备授权码');
 view.rerender(<AIAccounts {...props} provider="second"/>);
 expect(screen.queryByLabelText('设备授权码')).toBeNull();expect(api.captureCall).toHaveBeenCalledWith('ai.accounts.signin.cancel',{id:'first-job'});
});
