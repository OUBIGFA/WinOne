// @vitest-environment jsdom
// Cindy ChatInput/UserMessageEditBox regression paths, through WinOne's public
// controls. Deferred Host receipts distinguish click-time input from next draft.
import {afterEach,expect,it,vi} from 'vitest';
import {act,cleanup,fireEvent,render,screen,waitFor,within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {ChatPage} from './chat';
import {ChatMessageEdit} from './chat-message-edit';
const host=vi.hoisted(()=>({call:vi.fn(),ai:vi.fn()}));
vi.mock('../bridge',()=>({chatCall:host.call,captureCall:host.ai,setFileDropTarget:vi.fn(async()=>{})}));
afterEach(()=>{cleanup();vi.resetAllMocks();localStorage.clear();});
const conversation={id:'chat',title:'Send audit',directory:'C:/fixture',sessionDirectory:'C:/fixture/history',model:{provider:'fixture',model:'model'},permission:'ask',createdAt:'2026-10-18',updatedAt:'2026-10-18'};
function backend(running=false){
 host.ai.mockResolvedValue({providers:[{id:'fixture',name:'Fixture',models:['model']}],tasks:{chat:conversation.model}});
 host.call.mockImplementation(async(method:string)=>{
  if(method==='catalog')return {projects:[],conversations:[conversation]};
  if(method==='open')return {messages:[{id:'user',turnId:'first',role:'user',content:'Earlier question'},{id:'answer',turnId:'first',role:'assistant',content:'Earlier answer'}],streaming:running};
  if(method==='draft.get')return {text:''};
  if(method==='poll')return {events:[],cursor:0,approvals:[],running,status:running?'running':'completed'};
  return {};
 });
}
async function open(){render(<ChatPage requestedConversationId='chat'/>);await screen.findByText((_,element)=>element?.tagName==='P'&&element.textContent==='Earlier answer');return screen.getByRole('textbox',{name:'消息'}) as HTMLTextAreaElement;}
it('keeps Send in Cindy main slot and Stop in its secondary slot while a draft is present',async()=>{
 backend(true);const input=await open();fireEvent.change(input,{target:{value:'Correction'}});
 const toolbar=document.querySelector('.chat-composer-toolbar')!;
 const send=within(toolbar as HTMLElement).getByRole('button',{name:'发送'}),stop=within(toolbar as HTMLElement).getByRole('button',{name:'停止'});
 expect(stop.compareDocumentPosition(send)&Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
 expect(toolbar.querySelectorAll('button')[toolbar.querySelectorAll('button').length-1]).toBe(send);
});
it('clears only the submitted draft before acknowledgement and lets the user type the next one',async()=>{
 backend();const original=host.call.getMockImplementation()!;let accept!:(value:unknown)=>void;
 host.call.mockImplementation((method,params)=>method==='prompt'?new Promise(resolve=>{accept=resolve;}):original(method,params));
 const input=await open();fireEvent.change(input,{target:{value:'Click-time input'}});await userEvent.click(screen.getByRole('button',{name:'发送'}));
 await waitFor(()=>expect(host.call.mock.calls.filter(([method])=>method==='prompt')).toHaveLength(1));
 expect(input.value).toBe('');expect(input.disabled).toBe(false);expect(screen.getAllByText('Click-time input')).toHaveLength(1);
 fireEvent.change(input,{target:{value:'Next draft'}});
 expect(host.call.mock.calls.some(([method,params])=>method==='draft.save'&&params.draft.text==='Next draft')).toBe(false);
 await act(async()=>accept({accepted:true}));
 expect(input.value).toBe('Next draft');await waitFor(()=>expect(host.call).toHaveBeenCalledWith('draft.save',{id:'chat',draft:{text:'Next draft',attachments:[]}}));
 expect(host.call.mock.calls.filter(([method])=>method==='prompt')).toHaveLength(1);
});
it('restores a rejected submission before newer text instead of losing either draft',async()=>{
 backend();const original=host.call.getMockImplementation()!;let reject!:(error:Error)=>void;
 host.call.mockImplementation((method,params)=>method==='prompt'?new Promise((_,fail)=>{reject=fail;}):original(method,params));
 const input=await open();fireEvent.change(input,{target:{value:'First input'}});await userEvent.click(screen.getByRole('button',{name:'发送'}));
 await waitFor(()=>expect(reject).toBeTypeOf('function'));expect(input.disabled).toBe(false);fireEvent.change(input,{target:{value:'Newer input'}});
 await act(async()=>reject(Error('发送未受理')));await screen.findByText('发送未受理');
 expect(input.value).toBe('First input\n\nNewer input');
 await waitFor(()=>expect(host.call).toHaveBeenCalledWith('draft.save',{id:'chat',draft:{text:'First input\n\nNewer input',attachments:[]}}));
});
it('lets an explicit upward wheel cancel following even within one pixel of the bottom',async()=>{
 backend(true);const original=host.call.getMockImplementation()!;let next=false;
 host.call.mockImplementation((method,params)=>method==='poll'&&next?Promise.resolve({events:params.cursor<1?[{sequence:1,data:{type:'delta',kind:'text_delta',turnId:'first',text:' next',timestamp:1}}]:[],cursor:1,approvals:[],running:true,status:'running'}):original(method,params));
 await open();const viewport=document.querySelector('.chat-transcript-viewport') as HTMLDivElement;
 Object.defineProperties(viewport,{clientHeight:{value:100},scrollHeight:{value:1000}});viewport.scrollTop=900;fireEvent.scroll(viewport);
 fireEvent.wheel(viewport,{deltaX:0,deltaY:-1});viewport.scrollTop=899;fireEvent.scroll(viewport);next=true;
 await waitFor(()=>expect(host.call.mock.calls.filter(([method])=>method==='poll').length).toBeGreaterThan(1),{timeout:2500});
 expect(viewport.scrollTop).toBe(899);
});
it('does not block edited resend on a delayed review preview and prevents synchronous double submit',async()=>{
 host.call.mockImplementation(()=>new Promise(()=>{}));let complete!:()=>void;const commit=vi.fn(()=>new Promise<void>(resolve=>{complete=resolve;})),cancel=vi.fn();
 render(<ChatMessageEdit id='chat' entry='user' text='Edited input' hasAttachments={false} onCancel={cancel} onCommit={commit}/>);
 const send=screen.getByRole('button',{name:'保存并重新发送'});expect(send).toHaveProperty('disabled',false);
 fireEvent.click(send);fireEvent.click(send);expect(commit).toHaveBeenCalledTimes(1);
 await act(async()=>complete());expect(cancel).toHaveBeenCalledTimes(1);
});
