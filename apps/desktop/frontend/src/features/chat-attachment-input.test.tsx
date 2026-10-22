// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {act,cleanup,createEvent,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {ChatPage} from './chat';
import {ChatComposer} from './chat-composer';
const host=vi.hoisted(()=>({call:vi.fn(),ai:vi.fn(),clipboard:vi.fn(),inspect:vi.fn()}));
vi.mock('../bridge',()=>({chatCall:host.call,captureCall:host.ai,readClipboardFiles:host.clipboard,inspectDroppedPaths:host.inspect,setFileDropTarget:vi.fn(async()=>{}),pickChatFiles:vi.fn(async()=>[])}));
const conversation={id:'chat',title:'File input',directory:'C:/fixture',sessionDirectory:'C:/fixture/history',model:{provider:'fixture',model:'model'},permission:'ask'};
const file=new File(['notes'],'notes.txt',{type:'text/plain'});
function transfer(files:File[]=[],text=''){return {types:files.length?['Files','text/plain']:['text/plain'],items:files.map(file=>({kind:'file',type:file.type,getAsFile:()=>file})),files,getData:(type:string)=>type==='text/plain'?text:''};}
function setup(fresh=false){
 let created=!fresh;
 host.ai.mockResolvedValue({providers:[{id:'fixture',name:'Fixture',models:['model']}],tasks:{chat:conversation.model}});
 host.clipboard.mockResolvedValue([]);host.inspect.mockImplementation(async paths=>paths.map((path:string)=>({path,kind:path.endsWith('folder')?'directory':'file'})));
 host.call.mockImplementation(async(method,params)=>{
  if(method==='catalog')return {projects:[],conversations:created?[conversation]:[]};
  if(method==='conversation.new'){created=true;return conversation;}
  if(method==='open')return {messages:[],streaming:false};
  if(method==='draft.get')return {text:''};
  if(method==='poll')return {events:[],cursor:0,approvals:[],running:false,status:'completed'};
  if(method==='attachments.upload')return {url:'http://127.0.0.1/upload'};
  if(method==='attachments.add')return {attachments:params.paths.map((path:string)=>({path:'C:/imported/'+path.split('/').pop(),name:path.split('/').pop(),size:5,mimeType:'text/plain'}))};
  return {};
 });
 vi.spyOn(globalThis,'fetch').mockImplementation(async()=>({ok:true,json:async()=>({attachment:{path:'C:/imported/notes.txt',name:'notes.txt',size:5,mimeType:'text/plain'}})}) as Response);
}
async function open(fresh=false){setup(fresh);render(<ChatPage {...(!fresh?{requestedConversationId:'chat'}:{})}/>);if(!fresh)await waitFor(()=>expect(host.call.mock.calls.some(([method])=>method==='draft.get')).toBe(true));await waitFor(()=>{expect(screen.getByRole('textbox',{name:'消息'})).toHaveProperty('disabled',false);expect(document.querySelector('[data-slot=cindy-model]')?.getAttribute('title')).toContain('Fixture');});return screen.getByRole('textbox',{name:'消息'});}
beforeEach(()=>{vi.spyOn(URL,'createObjectURL').mockReturnValue('blob:fixture');vi.spyOn(URL,'revokeObjectURL').mockImplementation(()=>{});});
afterEach(()=>{cleanup();delete window.go;vi.restoreAllMocks();vi.clearAllMocks();localStorage.clear();});
it('leaves ordinary text paste to the editor and does not inspect the native clipboard',async()=>{
 const input=await open();const event=createEvent.paste(input,{clipboardData:transfer([],'plain text')});fireEvent(input,event);expect(event.defaultPrevented).toBe(false);expect(host.clipboard).not.toHaveBeenCalled();
});
it('uploads pasted files once and does not put accompanying clipboard text into the draft',async()=>{
 const input=await open();fireEvent.change(input,{target:{value:'keep draft'}});const event=createEvent.paste(input,{clipboardData:transfer([file],'duplicate text')});fireEvent(input,event);
 await screen.findByRole('button',{name:'移除附件 · notes.txt'});expect(event.defaultPrevented).toBe(true);expect(input).toHaveProperty('value','keep draft');expect(fetch).toHaveBeenCalledTimes(1);expect(vi.mocked(fetch).mock.calls[0][1]?.body).toBe(file);
 await waitFor(()=>expect(host.call).toHaveBeenCalledWith('draft.save',{id:'chat',draft:{text:'keep draft',attachments:[{path:'C:/imported/notes.txt',name:'notes.txt',size:5,mimeType:'text/plain'}]}}));
});
it('prefers Explorer paths and maps folders to references without uploading duplicate DOM files',async()=>{
 const input=await open();window.go={main:{App:{}}} as typeof window.go;host.clipboard.mockResolvedValue(['C:/source/notes.txt','C:/source/folder']);
 fireEvent.paste(input,{clipboardData:transfer([file])});await screen.findByRole('button',{name:'移除附件 · notes.txt'});expect(input).toHaveProperty('value','@C:/source/folder');expect(fetch).not.toHaveBeenCalled();expect(host.call).toHaveBeenCalledWith('attachments.add',{id:'chat',paths:['C:/source/notes.txt']});
});
it('accepts native-only clipboard files when WebView supplies no DOM items',async()=>{
 const input=await open();window.go={main:{App:{}}} as typeof window.go;host.clipboard.mockResolvedValue(['C:/source/notes.txt']);fireEvent.paste(input,{clipboardData:transfer()});await screen.findByRole('button',{name:'移除附件 · notes.txt'});
});
it('drops files on the whole composer and keeps the text after a failed import',async()=>{
 const input=await open();fireEvent.change(input,{target:{value:'draft'}});vi.mocked(fetch).mockResolvedValueOnce({ok:false,text:async()=> '附件导入失败'} as Response);
 fireEvent.drop(document.querySelector('.chat-composer-toolbar')!,{dataTransfer:transfer([file])});await screen.findByText('附件导入失败');expect(input).toHaveProperty('value','draft');expect(document.querySelectorAll('[data-slot=cindy-attachment]')).toHaveLength(0);
});
it('locks file mutation while paused without inserting a file or its accompanying text',()=>{
 const imports=vi.fn();render(<ChatComposer text='draft' onText={vi.fn()} onSend={vi.fn()} onStop={vi.fn()} running={false} busy={false} paused providers={[]} model={{provider:'',model:''}} permission='ask' onModel={vi.fn()} onPermission={vi.fn()} onImportFiles={imports} onImportPaths={vi.fn()}/>);
 const input=screen.getByRole('textbox',{name:'消息'}),event=createEvent.paste(input,{clipboardData:transfer([file],'text')});fireEvent(input,event);fireEvent.drop(input,{dataTransfer:transfer([file])});expect(event.defaultPrevented).toBe(true);expect(imports).not.toHaveBeenCalled();
});
it('retains new-chat browser files, imports them before sending and releases their object URLs',async()=>{
 const input=await open(true);fireEvent.paste(input,{clipboardData:transfer([file])});await screen.findByRole('button',{name:'移除附件 · notes.txt'});expect(fetch).not.toHaveBeenCalled();
 fireEvent.click(screen.getByRole('button',{name:'发送'}));await waitFor(()=>expect(host.call.mock.calls.some(([method])=>method==='prompt')).toBe(true));
 const prompt=host.call.mock.calls.find(([method])=>method==='prompt')![1];expect(prompt.attachments).toEqual(['C:/imported/notes.txt']);expect(fetch).toHaveBeenCalledTimes(1);expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:fixture');
 expect(host.call.mock.calls.filter(([method])=>method==='draft.save').every(([,params])=>params.draft.attachments.every((f:{path:string})=>!f.path.startsWith('blob:')))).toBe(true);
});
it('preserves a failed first-send file and retries bytes instead of sending a blob URL',async()=>{
 const input=await open(true);fireEvent.paste(input,{clipboardData:transfer([file])});await screen.findByRole('button',{name:'移除附件 · notes.txt'});
 vi.mocked(fetch).mockRejectedValueOnce(Error('附件上传失败'));fireEvent.click(screen.getByRole('button',{name:'发送'}));await screen.findByText('附件上传失败');await screen.findByRole('button',{name:'移除附件 · notes.txt'});
 expect(host.call.mock.calls.some(([method])=>method==='prompt')).toBe(false);expect(host.call.mock.calls.filter(([method])=>method==='draft.save').every(([,p])=>p.draft.attachments.every((f:{path:string})=>!f.path.startsWith('blob:')))).toBe(true);
 await waitFor(()=>expect(screen.getByRole('button',{name:'发送'})).toHaveProperty('disabled',false));fireEvent.click(screen.getByRole('button',{name:'发送'}));await waitFor(()=>expect(host.call.mock.calls.some(([method])=>method==='prompt')).toBe(true));expect(host.call.mock.calls.find(([method])=>method==='prompt')![1].attachments).toEqual(['C:/imported/notes.txt']);
});
it('removing a new-chat attachment releases its bytes without creating a conversation',async()=>{
 const input=await open(true);fireEvent.paste(input,{clipboardData:transfer([file])});const remove=await screen.findByRole('button',{name:'移除附件 · notes.txt'});fireEvent.click(remove);expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:fixture');expect(host.call.mock.calls.some(([method])=>method==='conversation.new')).toBe(false);
});
