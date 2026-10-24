// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {act,fireEvent,within,cleanup,render,screen,waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {ChatPage} from './chat';
const native=vi.hoisted(()=>({call:vi.fn(),ai:vi.fn()}));
vi.mock('../bridge',()=>({inspectDroppedPaths:async(paths:string[])=>paths.map(path=>({path,kind:'file'})),chatCall:native.call,captureCall:native.ai,setFileDropTarget:vi.fn(async()=>{})}));
afterEach(()=>{cleanup();vi.resetAllMocks();localStorage.clear();});
const conversation={id:'history',title:'Existing history',directory:'C:/fixture',sessionDirectory:'C:/fixture/history',model:{provider:'local',model:'model'},permission:'ask',createdAt:'2026-10-11',updatedAt:'2026-10-11'};
// Streaming text may be split into Cindy fade spans. Assert the complete
// visible paragraph rather than requiring a single text node.
const findReply=(text:string)=>screen.findByText((_,element)=>element?.tagName==='P'&&element.textContent===text);
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
it('edits the queued input in the composer, preserves its attachments and restores the next draft',async()=>{
 backend(true);const original=native.call.getMockImplementation()!;
 let queued={id:'queued',requestId:'queued',role:'user',content:'Original queue',pending:true,queued:true,attachments:[{path:'C:/fixture/a.txt',name:'a.txt',size:2,mimeType:'text/plain'}]};
 native.call.mockImplementation(async(method,params)=>{
  if(method==='open'||method==='state')return {messages:[queued],streaming:true};
  if(method==='draft.get')return {text:'My next draft'};
  if(method==='queue'){
   if(params.action==='update')queued={...queued,content:params.text};
   return {messages:[queued],streaming:true};
  }
  return original(method,params);
 });
 render(<ChatPage requestedConversationId='history'/>);
 const row=await screen.findByText('Original queue',{exact:false});fireEvent.mouseEnter(row.closest('[role=listitem]')!);
 await userEvent.click(screen.getByRole('button',{name:'编辑第 1 条排队消息'}));
 const input=screen.getByRole('textbox',{name:'消息'});
 await waitFor(()=>expect(input).toHaveProperty('value','Original queue'));
 expect(native.call).toHaveBeenCalledWith('queue',expect.objectContaining({action:'lock',entry:'queued',locked:true}));
 fireEvent.change(input,{target:{value:'Changed queue'}});
 await userEvent.click(screen.getByRole('button',{name:'保存排队消息'}));
 await waitFor(()=>expect(input).toHaveProperty('value','My next draft'));
 expect(native.call).toHaveBeenCalledWith('queue',expect.objectContaining({action:'update',text:'Changed queue',attachments:['C:/fixture/a.txt']}));
 expect(native.call.mock.calls.filter(([method])=>method==='prompt')).toHaveLength(0);
});
it('submits each mid-turn message to Pi without overwriting an earlier follow-up',async()=>{
 backend(true);render(<ChatPage requestedConversationId='history'/>);await findReply('Saved answer');
 const user=userEvent.setup(),input=screen.getByRole('textbox',{name:'消息'});
 for(const text of ['First follow-up','Second follow-up']){input.focus();await user.type(input,text,{skipClick:true});await user.keyboard('{Enter}');await user.click(await screen.findByRole('button',{name:'排队发送'}));await waitFor(()=>expect((input as HTMLTextAreaElement).value).toBe(''));}
 expect(native.call.mock.calls.filter(([method])=>method==='prompt').map(([,params])=>params.text)).toEqual(['First follow-up','Second follow-up']);
});
it('keeps stop available while a visible send button submits a mid-turn message',async()=>{
 backend(true);render(<ChatPage requestedConversationId='history'/>);await findReply('Saved answer');
 const user=userEvent.setup(),input=screen.getByRole('textbox',{name:'消息'});input.focus();await user.type(input,'Button follow-up',{skipClick:true});
 expect(screen.getByRole('button',{name:'停止'})).toBeTruthy();await user.click(screen.getByRole('button',{name:'发送'}));
 expect(native.call.mock.calls.some(([method])=>method==='prompt')).toBe(false);
 await user.click(await screen.findByRole('button',{name:'排队发送'}));
 await waitFor(()=>expect(native.call).toHaveBeenCalledWith('prompt',expect.objectContaining({text:'Button follow-up'})));
 expect(screen.getByRole('button',{name:'停止'})).toBeTruthy();
});
it('keeps the draft when mid-stream sending is cancelled',async()=>{
 backend(true);render(<ChatPage requestedConversationId="history"/>);await findReply('Saved answer');
 const user=userEvent.setup(),input=screen.getByRole('textbox',{name:'消息'});input.focus();await user.type(input,'Keep this draft',{skipClick:true});await user.keyboard('{Enter}');
 await user.click(await screen.findByRole('button',{name:'继续编辑'}));expect((input as HTMLTextAreaElement).value).toBe('Keep this draft');
 expect(native.call.mock.calls.some(([method])=>method==='prompt')).toBe(false);
});
it('gives repeated identical follow-ups distinct receipts after acknowledgement',async()=>{
 backend(true);render(<ChatPage requestedConversationId="history"/>);await findReply('Saved answer');
 const user=userEvent.setup(),input=screen.getByRole('textbox',{name:'消息'});
 for(let i=0;i<2;i++){input.focus();await user.type(input,'Continue',{skipClick:true});await user.keyboard('{Enter}');await user.click(await screen.findByRole('button',{name:'排队发送'}));await waitFor(()=>expect((input as HTMLTextAreaElement).value).toBe(''));}
 const ids=native.call.mock.calls.filter(([method])=>method==='prompt').map(([,value])=>value.requestId);expect(new Set(ids).size).toBe(2);
 expect(screen.getAllByText('Continue')).toHaveLength(2);
});
it('keeps the reader position when upward intent arrives before a send acknowledgement',async()=>{
 backend(true);const original=native.call.getMockImplementation()!;let acknowledge!:(value:unknown)=>void,receipt='',accepted=false;
 native.call.mockImplementation((method,params)=>{
  if(method==='prompt'){receipt=params.requestId;return new Promise(resolve=>{acknowledge=resolve;});}
  if(method==='poll'&&accepted)return Promise.resolve({events:params.cursor<1?[{sequence:1,data:{type:'pi_event',eventType:'message_end',message:{role:'assistant',turnId:receipt,timestamp:12345,content:'Latest queued reply'}}}]:[],cursor:1,approvals:[],running:true,status:'running'});
  return original(method,params);
 });
 render(<ChatPage requestedConversationId="history"/>);await findReply('Saved answer');
 const viewport=document.querySelector('.chat-transcript-viewport') as HTMLDivElement;Object.defineProperties(viewport,{clientHeight:{value:100},scrollHeight:{value:1000}});
 const user=userEvent.setup(),input=screen.getByRole('textbox',{name:'消息'});input.focus();await user.type(input,'Show the next answer',{skipClick:true});await user.keyboard('{Enter}');await user.click(await screen.findByRole('button',{name:'排队发送'}));
 fireEvent.wheel(viewport,{deltaX:0,deltaY:-40});viewport.scrollTop=0;fireEvent.scroll(viewport);
 await act(async()=>{accepted=true;acknowledge({accepted:true});});await findReply('Latest queued reply');
 expect(viewport.scrollTop).toBe(0);
});
it('offers direct insertion alongside queue and preserves the delivery mode through the Host',async()=>{
 backend(true);render(<ChatPage requestedConversationId='history'/>);await findReply('Saved answer');
 const user=userEvent.setup(),input=screen.getByRole('textbox',{name:'消息'});
 input.focus();await user.type(input,'Correct this turn',{skipClick:true});await user.keyboard('{Enter}');
 await screen.findByRole('dialog');
 expect(screen.getByRole('button',{name:'继续编辑'})).toBeTruthy();expect(screen.getByRole('button',{name:'排队发送'})).toBeTruthy();
 await user.click(screen.getByRole('button',{name:'直接插入会话'}));
 await waitFor(()=>expect(native.call).toHaveBeenCalledWith('prompt',expect.objectContaining({text:'Correct this turn',deliveryMode:'steer',requestId:expect.any(String)})));
 expect(native.call.mock.calls.some(([method])=>method==='stop')).toBe(false);
});
it('uses Cindy Ctrl+Enter for direct insertion without opening a queue confirmation',async()=>{
 backend(true);render(<ChatPage requestedConversationId='history'/>);await findReply('Saved answer');
 const user=userEvent.setup(),input=screen.getByRole('textbox',{name:'消息'});input.focus();await user.type(input,'Immediate correction',{skipClick:true});expect(document.activeElement).toBe(input);await user.keyboard('{Control>}{Enter}{/Control}');
 await waitFor(()=>expect(native.call).toHaveBeenCalledWith('prompt',expect.objectContaining({text:'Immediate correction',deliveryMode:'steer'})));
 expect(screen.queryByRole('dialog')).toBeNull();
});
it('retains the complete draft after a rejected insertion',async()=>{
 backend(true);const original=native.call.getMockImplementation()!;native.call.mockImplementation((method,params)=>method==='prompt'?Promise.reject(Error('插话暂不可用')):original(method,params));
 render(<ChatPage requestedConversationId='history'/>);await findReply('Saved answer');
 const user=userEvent.setup(),input=screen.getByRole('textbox',{name:'消息'});input.focus();await user.type(input,'Keep my correction',{skipClick:true});await user.keyboard('{Enter}');await user.click(await screen.findByRole('button',{name:'直接插入会话'}));
 await screen.findByText('插话暂不可用');expect((input as HTMLTextAreaElement).value).toBe('Keep my correction');
 expect(screen.queryByText('Keep my correction',{selector:'.desktop-conversation-message p'})).toBeNull();
});
it('coalesces rapid send clicks and keeps the input pending until its receipt is observed',async()=>{
 backend(false);const original=native.call.getMockImplementation()!;let accept!:(value:unknown)=>void;
 native.call.mockImplementation((method,params)=>method==='prompt'?new Promise(resolve=>{accept=resolve;}):original(method,params));
 render(<ChatPage requestedConversationId='history'/>);await findReply('Saved answer');
 const input=screen.getByRole('textbox',{name:'消息'});fireEvent.change(input,{target:{value:'Send once'}});
 const send=screen.getByRole('button',{name:'发送'});fireEvent.click(send);fireEvent.click(send);
 await waitFor(()=>expect(native.call.mock.calls.filter(([method])=>method==='prompt')).toHaveLength(1));
 expect(input).toHaveProperty('disabled',false);expect(input).toHaveProperty('value','');await act(async()=>accept({accepted:true}));
 expect(screen.getAllByText('Send once')).toHaveLength(1);expect(screen.getByRole('button',{name:'停止'})).toBeTruthy();
});
it('never sends an IME confirmation, key repeat or newline as a new message',async()=>{
 backend(false);render(<ChatPage requestedConversationId='history'/>);await findReply('Saved answer');const user=userEvent.setup(),input=screen.getByRole('textbox',{name:'消息'});
 fireEvent.change(input,{target:{value:'1. First item'}});input.focus();
 fireEvent.keyDown(input,{key:'Enter',isComposing:true});fireEvent.keyDown(input,{key:'Enter',keyCode:229});fireEvent.keyDown(input,{key:'Enter',repeat:true});
 await user.keyboard('{Shift>}{Enter}{/Shift}');expect((input as HTMLTextAreaElement).value).toContain('2. ');
 expect(native.call.mock.calls.some(([method])=>method==='prompt')).toBe(false);
});
it('keeps file-only messages insertable while the current turn is running',async()=>{
 backend(true);const original=native.call.getMockImplementation()!,file={path:'C:/fixture/attachments/brief.txt',name:'brief.txt',size:4,mimeType:'text/plain'};
 native.call.mockImplementation((method,params)=>method==='attachments.add'?Promise.resolve({attachments:[file]}):original(method,params));
 render(<ChatPage requestedConversationId='history'/>);await findReply('Saved answer');
 await act(async()=>window.dispatchEvent(new CustomEvent('winone:chat-files-drop',{detail:{paths:[file.path]}})));
 await screen.findByText('brief.txt');const input=screen.getByRole('textbox',{name:'消息'});input.focus();await userEvent.keyboard('{Control>}{Enter}{/Control}');
 await waitFor(()=>expect(native.call).toHaveBeenCalledWith('prompt',expect.objectContaining({text:'',attachments:[file.path],deliveryMode:'steer'})));
 expect(screen.getByRole('button',{name:'停止'})).toBeTruthy();
});
it('reuses the same receipt after an ambiguous send failure instead of submitting a duplicate',async()=>{
 backend(false);const original=native.call.getMockImplementation()!;let attempts=0;
 native.call.mockImplementation((method,params)=>method==='prompt'?(++attempts===1?Promise.reject(Error('Acknowledgement lost')):Promise.resolve({accepted:true,replayed:true})):original(method,params));
 render(<ChatPage requestedConversationId='history'/>);await findReply('Saved answer');fireEvent.change(screen.getByRole('textbox',{name:'消息'}),{target:{value:'One intended request'}});
 await userEvent.click(screen.getByRole('button',{name:'发送'}));await screen.findByText('Acknowledgement lost');
 await userEvent.click(screen.getByRole('button',{name:'发送'}));await waitFor(()=>expect(attempts).toBe(2));
 const calls=native.call.mock.calls.filter(([method])=>method==='prompt');expect(calls[0][1].requestId).toBe(calls[1][1].requestId);
});
it('keeps retry disabled across stale idle polls, then displays the replacement branch in order',async()=>{
 backend(false);const original=native.call.getMockImplementation()!;let requestId='',ready=false;
 native.call.mockImplementation((method,params)=>{
  if(method==='retry'){requestId=params.requestId;return Promise.resolve({accepted:true});}
  if(method==='poll'&&ready)return Promise.resolve({events:params.cursor<1?[{sequence:1,data:{type:'branch_reset',state:{messages:[{id:'replacement',requestId,turnId:requestId,role:'user',content:'Saved original prompt'}],hasOlder:false,streaming:true}}},{sequence:2,data:{type:'pi_event',eventType:'message_end',message:{id:'new-answer',role:'assistant',turnId:requestId,content:'Replacement answer'}}}]:[],cursor:2,approvals:[],running:false,status:'completed'});
  return original(method,params);
 });
 render(<ChatPage requestedConversationId='history'/>);await findReply('Saved answer');const retry=within(document.querySelector('.desktop-conversation-message--assistant')!).getByRole('button',{name:'重试'});
 await userEvent.click(retry);await waitFor(()=>expect(requestId).not.toBe(''));expect(retry).toHaveProperty('disabled',true);
 await waitFor(()=>expect(native.call.mock.calls.filter(([method])=>method==='poll').length).toBeGreaterThan(1),{timeout:2500});fireEvent.click(retry);
 expect(native.call.mock.calls.filter(([method])=>method==='retry')).toHaveLength(1);
 ready=true;await waitFor(()=>expect(screen.getByText('Replacement answer')).toBeTruthy(),{timeout:2500});expect(screen.queryByText('Saved answer')).toBeNull();
 const rows=[...document.querySelectorAll('.desktop-conversation-message')];expect(rows.map(row=>row.textContent).join('|')).toMatch(/Saved original prompt.*Replacement answer/);
 expect(native.call.mock.calls.some(([method])=>method==='state')).toBe(false);
});
it('retries a persisted response after reopening without requiring a prompt from this mount',async()=>{
 backend(false);render(<ChatPage requestedConversationId='history'/>);await findReply('Saved answer');
 await userEvent.click(within(document.querySelector('.desktop-conversation-message--assistant')!).getByRole('button',{name:'重试'}));
 await waitFor(()=>expect(native.call).toHaveBeenCalledWith('retry',expect.objectContaining({id:'history',entry:'user-entry',requestId:expect.any(String)})));
 expect(native.call.mock.calls.filter(([method])=>method==='prompt')).toHaveLength(0);
 // Ordered branch-reset events own the live transcript. An extra state fetch
 // can arrive behind deltas and restore the abandoned branch or erase a partial.
 expect(native.call.mock.calls.filter(([method])=>method==='state')).toHaveLength(0);
});
