// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {within,cleanup,render,screen,waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {ChatPage} from './chat';
const native=vi.hoisted(()=>({call:vi.fn(),ai:vi.fn()}));
vi.mock('../bridge',()=>({chatCall:native.call,captureCall:native.ai,setFileDropTarget:vi.fn(async()=>{})}));
afterEach(()=>{cleanup();vi.resetAllMocks();localStorage.clear();});
const conversation={id:'history',title:'Existing history',directory:'C:/fixture',sessionDirectory:'C:/fixture/history',model:{provider:'local',model:'model'},permission:'ask',createdAt:'2026-10-11',updatedAt:'2026-10-11'};
function backend(running:boolean){
 native.ai.mockResolvedValue({providers:[{id:'local',name:'Local',models:['model']}],tasks:{chat:conversation.model}});
 native.call.mockImplementation(async(method:string)=>{
  if(method==='catalog')return {projects:[],conversations:[conversation]};
  if(method==='open'||method==='state')return {messages:[{id:'user-entry',role:'user',content:'Saved original prompt'},{id:'answer',role:'assistant',content:'Saved answer'}],streaming:running};
  if(method==='draft.get')return {text:''};
  if(method==='poll')return {events:[],cursor:0,approvals:[],running,status:running?'running':'completed'};
  if(method==='prompt'||method==='retry')return {accepted:true};
  return {};
 });
}
it('submits each mid-turn message to Pi without overwriting an earlier follow-up',async()=>{
 backend(true);render(<ChatPage requestedConversationId='history'/>);await screen.findByText('Saved answer');
 const user=userEvent.setup(),input=screen.getByRole('textbox',{name:'消息'});
 for(const text of ['First follow-up','Second follow-up']){input.focus();await user.type(input,text,{skipClick:true});await user.keyboard('{Enter}');await waitFor(()=>expect((input as HTMLTextAreaElement).value).toBe(''));}
 expect(native.call.mock.calls.filter(([method])=>method==='prompt').map(([,params])=>params.text)).toEqual(['First follow-up','Second follow-up']);
});
it('keeps stop available while a visible send button submits a mid-turn message',async()=>{
 backend(true);render(<ChatPage requestedConversationId='history'/>);await screen.findByText('Saved answer');
 const user=userEvent.setup(),input=screen.getByRole('textbox',{name:'消息'});input.focus();await user.type(input,'Button follow-up',{skipClick:true});
 expect(screen.getByRole('button',{name:'停止'})).toBeTruthy();await user.click(screen.getByRole('button',{name:'发送'}));
 await waitFor(()=>expect(native.call).toHaveBeenCalledWith('prompt',expect.objectContaining({text:'Button follow-up'})));
 expect(screen.getByRole('button',{name:'停止'})).toBeTruthy();
});
it('retries a persisted response after reopening without requiring a prompt from this mount',async()=>{
 backend(false);render(<ChatPage requestedConversationId='history'/>);await screen.findByText('Saved answer');
 await userEvent.click(within(document.querySelector('.desktop-conversation-message--assistant')!).getByRole('button',{name:'重试'}));
 await waitFor(()=>expect(native.call).toHaveBeenCalledWith('retry',expect.objectContaining({id:'history',entry:'user-entry',requestId:expect.any(String)})));
 expect(native.call.mock.calls.filter(([method])=>method==='prompt')).toHaveLength(0);
});
