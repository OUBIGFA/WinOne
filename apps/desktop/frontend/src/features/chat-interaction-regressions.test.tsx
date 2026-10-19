// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {act,cleanup,render,screen,waitFor,within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {ChatPage} from './chat';
import {ChatComposer} from './chat-composer';

const native=vi.hoisted(()=>({call:vi.fn(),ai:vi.fn()}));
vi.mock('../bridge',()=>({chatCall:native.call,captureCall:native.ai,setFileDropTarget:vi.fn(async()=>{}),pickChatFiles:vi.fn(),pickDirectory:vi.fn()}));
afterEach(()=>{cleanup();vi.restoreAllMocks();vi.resetAllMocks();localStorage.clear();});
const model={provider:'service',model:'model'};
const conversation={id:'conversation',title:'A conversation',directory:'C:\\project',sessionDirectory:'C:\\history',permission:'ask',model,createdAt:'2026-10-13',updatedAt:'2026-10-13'};
const messages=[{id:'user-1',role:'user',content:'First question'},{id:'answer-1',role:'assistant',content:'First answer'},{id:'user-2',role:'user',content:'Second question'},{id:'answer-2',role:'assistant',content:'Second answer'}];
function backend(){
 native.ai.mockResolvedValue({providers:[{id:'service',name:'Service',models:['model']}],tasks:{chat:model}});
 native.call.mockImplementation(async(method,params)=>{
  if(method==='catalog')return {projects:[],conversations:[conversation]};
  if(method==='open'||method==='state')return {messages,hasOlder:true,before:'user-1'};
  if(method==='draft.get')return {text:''};
  if(method==='poll')return {events:[],cursor:0,approvals:[],running:false,status:'idle',error:'Request aborted'};
  if(method==='settings.get')return {workspaceDirectory:'C:\\work',skillDirectories:[],pluginDirectories:[],mcpServers:[],automations:[]};
  if(method==='settings.save')return params.settings;
  if(method==='thinking'||method==='fast')return {...conversation,...(method==='thinking'?{thinkingLevel:params.thinkingLevel}:{fastMode:params.fastMode})};
  return {};
 });
}
async function open(){backend();render(<ChatPage/>);await userEvent.click(await screen.findByRole('button',{name:'A conversation'}));await screen.findByText('First answer');}

it('keeps a dismissed transport error closed across repeated idle polls',async()=>{
 await open();const alert=await screen.findByRole('alert');await userEvent.click(within(alert).getByRole('button',{name:'关闭'}));
 expect(screen.queryByText('Request aborted')).toBeNull();
 await waitFor(()=>expect(native.call.mock.calls.filter(call=>call[0]==='poll').length).toBeGreaterThan(1),{timeout:3000});
 expect(screen.queryByText('Request aborted')).toBeNull();
});

it('saves chat settings then closes the dialog without a transcript notice',async()=>{
 await open();await userEvent.click(screen.getByRole('button',{name:'Chat 设置'}));
 const input=await screen.findByRole('textbox',{name:'默认工作目录'});input.focus();await userEvent.type(input,'-updated',{skipClick:true});
 await userEvent.click(screen.getByRole('button',{name:'保存设置'}));
 await waitFor(()=>expect(screen.queryByRole('dialog')).toBeNull());expect(screen.queryByText('设置已保存')).toBeNull();
});

it('retries the selected user message rather than always retrying the last user',async()=>{
 await open();const message=screen.getByText('First question').closest('.desktop-conversation-message--user');
 expect(message).toBeTruthy();await userEvent.click(within(message as HTMLElement).getByRole('button',{name:'重试'}));
 await waitFor(()=>expect(native.call).toHaveBeenCalledWith('retry',expect.objectContaining({id:'conversation',entry:'user-1'})));
});

it('returns the new-chat composer to its initial height on each new-chat action',async()=>{
 backend();localStorage.setItem('winone.chat.composer-height.v1','620');const {container}=render(<ChatPage/>);
 const input=await screen.findByRole('textbox',{name:'消息'});expect(parseFloat((input.closest('.chat-composer') as HTMLElement).style.height)).toBe(220);
 const original=input.closest('[data-chat-input-root]');await userEvent.click(screen.getByRole('button',{name:'新对话'}));
 await waitFor(()=>expect(container.querySelector('[data-chat-input-root]')).not.toBe(original));
 expect(screen.getAllByRole('separator',{name:'调整输入框高度'})).toHaveLength(1);
});

it('shows the submitted user message before creating and opening a new conversation finishes',async()=>{
 backend();const fallback=native.call.getMockImplementation()!;let create!:(value:unknown)=>void,accept!:(value:unknown)=>void;
 native.call.mockImplementation((method,params)=>method==='conversation.new'?new Promise(resolve=>{create=resolve;}):method==='prompt'?new Promise(resolve=>{accept=resolve;}):fallback(method,params));
 render(<ChatPage/>);const input=await screen.findByRole('textbox',{name:'消息'});input.focus();await userEvent.type(input,'Immediate user bubble',{skipClick:true});await userEvent.keyboard('{Enter}');
 expect(screen.getByText('Immediate user bubble').closest('.desktop-conversation-message--user')).toBeTruthy();
 await act(async()=>create({...conversation,id:'new-conversation'}));
 await waitFor(()=>expect(native.call).toHaveBeenCalledWith('prompt',expect.objectContaining({text:'Immediate user bubble'})));
 expect(screen.getAllByText('Immediate user bubble').filter(node=>node.closest('.desktop-conversation-message--user'))).toHaveLength(1);
 await act(async()=>accept({accepted:true}));
 expect(screen.getAllByText('Immediate user bubble').filter(node=>node.closest('.desktop-conversation-message--user'))).toHaveLength(1);
});

it('loads history only on request and then moves the viewport to the new earlier messages',async()=>{
 await open();const fallback=native.call.getMockImplementation()!;
 expect(native.call.mock.calls.filter(([method,params])=>method==='state'&&params?.before)).toHaveLength(0);
 native.call.mockImplementation((method,params)=>method==='state'&&params.before?Promise.resolve({messages:[{id:'older-user',role:'user',content:'Earlier question'},{id:'older-answer',role:'assistant',content:'Earlier answer'}],hasOlder:false}):fallback(method,params));
 const viewport=document.querySelector('.chat-transcript-viewport') as HTMLElement;
 Object.defineProperty(viewport,'scrollHeight',{configurable:true,get:()=>screen.queryByText('Earlier question')?1500:1000});viewport.scrollTop=150;
 await userEvent.click(screen.getByRole('button',{name:'加载更早消息'}));await screen.findByText('Earlier question');
 expect(viewport.scrollTop).toBe(0);expect(screen.queryByText('更早消息已加载，向上滚动查看')).toBeNull();
});

it('opens the model menu at the selected model and clears a stale search',async()=>{
 const intoView=vi.fn();HTMLElement.prototype.scrollIntoView=intoView;
 const models=Array.from({length:80},(_,i)=>'model-'+i);
 render(<ChatComposer text='' onText={()=>{}} onSend={()=>{}} onStop={()=>{}} running={false} busy={false} providers={[{id:'service',name:'Service',models,authMode:'api-key',hasKey:false}]} model={{provider:'service',model:'model-60'}} permission='ask' onModel={()=>{}} onPermission={()=>{}}/>);
 await userEvent.click(screen.getByRole('button',{name:'模型'}));
 await waitFor(()=>expect(intoView.mock.instances.some(node=>(node as HTMLElement).getAttribute('aria-selected')==='true')).toBe(true));
 const search=screen.getByRole('textbox',{name:'搜索模型'});await userEvent.type(search,'model-2');await userEvent.keyboard('{Escape}');
 await userEvent.click(screen.getByRole('button',{name:'模型'}));await waitFor(()=>expect((screen.getByRole('textbox',{name:'搜索模型'}) as HTMLInputElement).value).toBe(''));
 expect(screen.getByRole('option',{name:'model-60'}).getAttribute('aria-selected')).toBe('true');
});

it('invalidates a pending history request when leaving and reopening the same conversation',async()=>{
 await open();const fallback=native.call.getMockImplementation()!;let finish!:(value:unknown)=>void;
 native.call.mockImplementation((method,params)=>method==='state'&&params.before?new Promise(resolve=>{finish=resolve;}):fallback(method,params));
 await userEvent.click(screen.getByRole('button',{name:'加载更早消息'}));await screen.findByRole('button',{name:'正在加载更早消息'});
 await userEvent.click(screen.getByRole('button',{name:'新对话'}));await userEvent.click(screen.getByRole('button',{name:'A conversation'}));await screen.findByText('First answer');
 expect(screen.getByRole('button',{name:'加载更早消息'}).hasAttribute('disabled')).toBe(false);
 await act(async()=>finish({messages:[{id:'stale',role:'assistant',content:'Stale historical result'}],hasOlder:false}));
 expect(screen.queryByText('Stale historical result')).toBeNull();expect(screen.getByRole('button',{name:'加载更早消息'})).toBeTruthy();
});

it('changes the thinking level and fast mode from the composer',async()=>{
 await open();
 // Base UI menus open on keyboard focus in jsdom; clicking the pill does not
 // reliably dispatch the trigger interaction the menu listens for.
 const openThinkingMenu=async()=>{const trigger=await screen.findByRole('button',{name:/思考强度/});trigger.focus();await userEvent.keyboard('{ArrowDown}');};
 await openThinkingMenu();
 await userEvent.click(await screen.findByRole('menuitem',{name:'高强度推理'}));
 await waitFor(()=>expect(native.call).toHaveBeenCalledWith('thinking',{id:'conversation',thinkingLevel:'high'}));
 await openThinkingMenu();
 await userEvent.click(await screen.findByText('快速模式'));
 await waitFor(()=>expect(native.call).toHaveBeenCalledWith('fast',{id:'conversation',fastMode:true}));
});

it('compacts the conversation from the composer button',async()=>{
 await open();
 await userEvent.click(await screen.findByRole('button',{name:'压缩上下文'}));
 await waitFor(()=>expect(native.call).toHaveBeenCalledWith('compact',{id:'conversation'}));
});

it('renders the context usage reported by the durable state',async()=>{
 backend();const fallback=native.call.getMockImplementation()!;
 native.call.mockImplementation((method,params)=>method==='open'?Promise.resolve({messages,hasOlder:true,before:'user-1',contextUsage:{tokens:25000,contextWindow:100000,percent:25},usage:{input:1000,output:200,cacheRead:0,cacheWrite:0,totalTokens:1200,cost:0,costKnown:false}}):fallback(method,params));
 render(<ChatPage/>);await userEvent.click(await screen.findByRole('button',{name:'A conversation'}));await screen.findByText('First answer');
 const chip=await screen.findByRole('button',{name:'上下文用量 · 25%'});
 await userEvent.click(chip);
 expect(await screen.findByText('费用未知')).toBeTruthy();
 expect(screen.getByText((_,node)=>node?.tagName==='DD'&&node.textContent==='25.0k / 100.0k')).toBeTruthy();
});

it('toggles the completion sound from the composer',async()=>{
 await open();
 const toggle=await screen.findByRole('button',{name:'关闭完成提示音'});
 expect(toggle.getAttribute('aria-pressed')).toBe('true');
 await userEvent.click(toggle);
 expect((await screen.findByRole('button',{name:'开启完成提示音'})).getAttribute('aria-pressed')).toBe('false');
 expect(localStorage.getItem('winone.chat.sound.v1')).toBe('false');
});
