// @vitest-environment jsdom
import {render,screen,cleanup,waitFor,act,fireEvent} from '@testing-library/react';
import {afterEach,beforeEach,describe,it,expect,vi} from 'vitest';
import {AIThirdPartyPlugins} from './ai-third-party-plugins';
import '../i18n';
const {call,pick}=vi.hoisted(()=>({call:vi.fn(),pick:vi.fn()}));
vi.mock('../bridge',()=>({captureCall:call,pickDirectory:pick}));
afterEach(cleanup);
beforeEach(()=>{call.mockReset();pick.mockReset();call.mockResolvedValue({url:'http://127.0.0.1:12345/session/fixture/plugins.html'});});
describe('third-party plugin document boundary',()=>{
 it('sends native data-theme changes to the plugin document',async()=>{
  document.documentElement.dataset.theme='dark';
  try{
   render(<AIThirdPartyPlugins active disabled={false} onProvider={()=>{}}/>);
   const frame=await screen.findByTitle('插件') as HTMLIFrameElement,send=vi.spyOn(frame.contentWindow!,'postMessage');
   fireEvent.load(frame);expect(send).toHaveBeenLastCalledWith(expect.objectContaining({theme:'dark'}),'http://127.0.0.1:12345');
   await act(async()=>{document.documentElement.dataset.theme='light';});
   expect(send).toHaveBeenLastCalledWith(expect.objectContaining({theme:'light'}),'http://127.0.0.1:12345');
  }finally{delete document.documentElement.dataset.theme;}
 });
 it('keeps the plugin provider identity when opening account settings',async()=>{
  const provider=vi.fn();render(<AIThirdPartyPlugins active disabled={false} onProvider={provider}/>);
  const frame=await screen.findByTitle('插件') as HTMLIFrameElement;
  act(()=>window.dispatchEvent(new MessageEvent('message',{origin:'http://127.0.0.1:12345',source:frame.contentWindow,data:{type:'winone-plugin-native',id:'open-account',method:'provider',provider:'thirdparty',name:'Third party',plugin:'fixture-package'}})));
  await waitFor(()=>expect(provider).toHaveBeenCalledWith({id:'thirdparty',name:'Third party',backend:'plugin',plugin:'fixture-package'}));
 });
 it('carries an explicit sign-in through the authenticated native bridge',async()=>{
  const provider=vi.fn();render(<AIThirdPartyPlugins active disabled={false} onProvider={provider}/>);
  const frame=await screen.findByTitle('插件') as HTMLIFrameElement;
  act(()=>window.dispatchEvent(new MessageEvent('message',{origin:'http://127.0.0.1:12345',source:frame.contentWindow,data:{type:'winone-plugin-native',id:'signin',method:'provider',provider:'web-fixture',name:'Web',plugin:'fixture',signIn:true}})));
  await waitFor(()=>expect(provider).toHaveBeenCalledWith({id:'web-fixture',name:'Web',backend:'plugin',plugin:'fixture'},true));
 });
 it('replies to an in-flight native request when settings become locked',async()=>{
  const provider=vi.fn();render(<AIThirdPartyPlugins active disabled onProvider={provider}/>);
  const frame=await screen.findByTitle('插件') as HTMLIFrameElement,send=vi.spyOn(frame.contentWindow!,'postMessage');
  act(()=>window.dispatchEvent(new MessageEvent('message',{origin:'http://127.0.0.1:12345',source:frame.contentWindow,data:{type:'winone-plugin-native',id:'blocked',method:'provider',provider:'web-fixture'}})));
  await waitFor(()=>expect(send).toHaveBeenCalledWith(expect.objectContaining({id:'blocked',error:'另一个操作正在进行，请稍后重试。'}),'http://127.0.0.1:12345'));
  expect(provider).not.toHaveBeenCalled();
 });
 it('refreshes account discovery only for authenticated plugin changes',async()=>{
  const changed=vi.fn();render(<AIThirdPartyPlugins active disabled={false} onChanged={changed} onProvider={()=>{}}/>);
  const frame=await screen.findByTitle('插件') as HTMLIFrameElement;
  const data={type:'winone-plugin-changed'};
  act(()=>window.dispatchEvent(new MessageEvent('message',{origin:'http://untrusted.invalid',source:frame.contentWindow,data})));
  expect(changed).not.toHaveBeenCalled();
  act(()=>window.dispatchEvent(new MessageEvent('message',{origin:'http://127.0.0.1:12345',source:frame.contentWindow,data})));
  expect(changed).toHaveBeenCalledOnce();
 });
 it('opens only when visible and closes its viewer when leaving',async()=>{
  const view=render(<AIThirdPartyPlugins active={false} disabled={false} onProvider={()=>{}}/>);
  expect(call).not.toHaveBeenCalled();
  view.rerender(<AIThirdPartyPlugins active disabled={false} onProvider={()=>{}}/>);
  const frame=await screen.findByTitle('插件');
  expect(frame.getAttribute('src')).toContain('/plugins.html');
  expect(call).toHaveBeenCalledWith('ai.routing.plugins.open');
  view.unmount();expect(call).toHaveBeenCalledWith('ai.routing.plugins.close');
 });
 it('rejects another window and origin before invoking the native directory picker',async()=>{
  render(<AIThirdPartyPlugins active disabled={false} onProvider={()=>{}}/>);
  const frame=await screen.findByTitle('插件') as HTMLIFrameElement;
  const request={type:'winone-plugin-native',method:'choose',id:'request-1'};
  act(()=>window.dispatchEvent(new MessageEvent('message',{origin:'http://127.0.0.1:12345',source:window,data:request})));
  act(()=>window.dispatchEvent(new MessageEvent('message',{origin:'http://untrusted.invalid',source:frame.contentWindow,data:request})));
  expect(pick).not.toHaveBeenCalled();
  pick.mockResolvedValue('E:/fixture-plugin');
  act(()=>window.dispatchEvent(new MessageEvent('message',{origin:'http://127.0.0.1:12345',source:frame.contentWindow,data:request})));
  await waitFor(()=>expect(pick).toHaveBeenCalledOnce());
 });
 it('keeps plugin operations inaccessible while the surrounding form is locked',async()=>{
  render(<AIThirdPartyPlugins active disabled onProvider={()=>{}}/>);
  const frame=await screen.findByTitle('插件');expect(frame.hasAttribute('inert')).toBe(true);
 });
});
