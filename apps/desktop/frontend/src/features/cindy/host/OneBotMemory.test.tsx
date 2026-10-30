// @vitest-environment jsdom
import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import {act, cleanup, fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import {createRef} from 'react';
import {i18n} from '../../../i18n';
import {BotMemorySettings} from '../bots/BotMemorySettings';
import {ConfirmDialogProvider} from '../panel-ui/confirm-dialog-provider';
import './onebotLocales';
import {BOT_MEMORY_CHANGED} from '../shared/botMemory';

const bridge=vi.hoisted(()=>({call:vi.fn()}));
vi.mock('../../../bridge',()=>({chatCall:bridge.call}));
afterEach(cleanup);
beforeEach(async()=>{vi.clearAllMocks();await i18n.changeLanguage('en-US');});

it('saves the real memory editor through the Bot-scoped Host call before leaving',async()=>{
 let memory={filename:'user_preferences.md',type:'user',title:'Reading preference',body:'Original text',updatedAt:'2026-10-26T08:00:00.000Z'};
 bridge.call.mockImplementation(async(method:string,params:Record<string,unknown>)=>{
  if(params.botId!=='reader')throw Error('wrong Bot ownership');
  if(method==='onebot.memory.list')return [{...memory,preview:memory.body}];
  if(method==='onebot.memory.read')return memory;
  if(method==='onebot.memory.update'){
   expect(params.expectedUpdatedAt).toBe('2026-10-26T08:00:00.000Z');
   memory={...memory,body:String(params.body),title:String(params.title),updatedAt:'2026-10-26T09:00:00.000Z'};
   return memory;
  }
  throw Error('Unexpected '+method);
 });
 const back=createRef<(()=>Promise<boolean>)|null>(),leave=createRef<(()=>Promise<boolean>)|null>();
 render(<ConfirmDialogProvider><BotMemorySettings botId="reader" botName="Reader" memoryEnabled onMemoryEnabledChange={()=>{}} aboutMe={{value:'',onChange:()=>{},onBlur:()=>{}}} backRef={back} leaveRef={leave}/></ConfirmDialogProvider>);
 fireEvent.click(await screen.findByText('Reading preference'));
 fireEvent.click(await screen.findByRole('button',{name:'Edit'}));
 fireEvent.change(screen.getByRole('textbox',{name:'Content'}),{target:{value:'Save before leaving'}});
 await act(async()=>{expect(await leave.current?.()).toBe(true);});
 await waitFor(()=>expect(memory.body).toBe('Save before leaving'));
 expect(bridge.call).toHaveBeenCalledWith('onebot.memory.update',expect.objectContaining({botId:'reader',filename:'user_preferences.md',body:'Save before leaving'}));
});

// Keep the upstream editor and dialogs real: these checks cover the WinOne
// boundary, including the error envelope preserved by the native Host.
function renderMemory() {
 const back=createRef<(()=>Promise<boolean>)|null>(),leave=createRef<(()=>Promise<boolean>)|null>();
 render(<ConfirmDialogProvider><BotMemorySettings botId="reader" botName="Reader" memoryEnabled onMemoryEnabledChange={()=>{}} aboutMe={{value:'',onChange:()=>{},onBlur:()=>{}}} backRef={back} leaveRef={leave}/></ConfirmDialogProvider>);
 return {back,leave};
}
const initial={filename:'user_preferences.md',type:'user',title:'Reading preference',body:'Original text',updatedAt:'2026-10-26T08:00:00.000Z'};

it('searches through the scoped Host and deletes only after confirming the current version',async()=>{
 let deleted=false;
 bridge.call.mockImplementation(async(method:string,params:Record<string,unknown>)=>{
  expect(params.botId).toBe('reader');
  if(method==='onebot.memory.list')return deleted||params.query==='missing'?[]:[{...initial,preview:initial.body}];
  if(method==='onebot.memory.read')return initial;
  if(method==='onebot.memory.delete'){expect(params.expectedUpdatedAt).toBe(initial.updatedAt);deleted=true;return null;}
  throw Error('Unexpected '+method);
 });
 renderMemory();
 const search=await screen.findByRole('textbox',{name:'Search memories'});
 fireEvent.change(search,{target:{value:'missing'}});
 await screen.findByText('No matching memories');
 fireEvent.change(search,{target:{value:''}});
 fireEvent.click(await screen.findByText(initial.title));
 fireEvent.click(await screen.findByRole('button',{name:'Delete'}));
 const dialog=await screen.findByRole('alertdialog');
 expect(deleted).toBe(false);
 fireEvent.click(within(dialog).getByRole('button',{name:'Delete'}));
 await screen.findByText('No memories yet');
 expect(deleted).toBe(true);
});

it.each(['Use latest','Keep my changes'])('keeps a concurrent Host edit until the user chooses %s',async(choice)=>{
 let memory={...initial},updates=0;
 bridge.call.mockImplementation(async(method:string,params:Record<string,unknown>)=>{
  if(method==='onebot.memory.list')return [{...memory,preview:memory.body}];
  if(method==='onebot.memory.read')return {...memory};
  if(method==='onebot.memory.update'){
   updates++;
   if(updates===1){memory={...memory,body:'Latest from OneBot',updatedAt:'2026-10-26T09:00:00.000Z'};throw Error(`[PRECONDITION_FAILED] ${BOT_MEMORY_CHANGED}`);}
   expect(params.expectedUpdatedAt).toBe('2026-10-26T09:00:00.000Z');
   memory={...memory,body:String(params.body),updatedAt:'2026-10-26T10:00:00.000Z'};
   return {...memory};
  }
  throw Error('Unexpected '+method);
 });
 renderMemory();
 fireEvent.click(await screen.findByText(initial.title));
 fireEvent.click(await screen.findByRole('button',{name:'Edit'}));
 fireEvent.change(screen.getByRole('textbox',{name:'Content'}),{target:{value:'My edited text'}});
 fireEvent.click(screen.getByRole('button',{name:'Done'}));
 await screen.findByText('Reader just updated this memory');
 expect(memory.body).toBe('Latest from OneBot');
 expect((screen.getByRole('textbox',{name:'Content'}) as HTMLTextAreaElement).value).toBe('My edited text');
 fireEvent.click(screen.getByRole('button',{name:choice}));
 await waitFor(()=>expect(memory.body).toBe(choice==='Use latest'?'Latest from OneBot':'My edited text'));
 await waitFor(()=>expect(screen.queryByText('Reader just updated this memory')).toBeNull());
 expect(updates).toBe(choice==='Use latest'?1:2);
});

it('keeps the draft and the page when saving fails and the user continues editing',async()=>{
 bridge.call.mockImplementation(async(method:string)=>{
  if(method==='onebot.memory.list')return [{...initial,preview:initial.body}];
  if(method==='onebot.memory.read')return initial;
  if(method==='onebot.memory.update')throw Error('[IO_ERROR] Unable to write');
  throw Error('Unexpected '+method);
 });
 const {leave}=renderMemory();
 fireEvent.click(await screen.findByText(initial.title));
 fireEvent.click(await screen.findByRole('button',{name:'Edit'}));
 fireEvent.change(screen.getByRole('textbox',{name:'Content'}),{target:{value:'Unwritten draft'}});
 let leaving:Promise<boolean>|undefined;
 act(()=>{leaving=leave.current?.();});
 const dialog=await screen.findByRole('alertdialog');
 fireEvent.click(within(dialog).getByRole('button',{name:'Keep Editing'}));
 await act(async()=>{expect(await leaving).toBe(false);});
 expect((screen.getByRole('textbox',{name:'Content'}) as HTMLTextAreaElement).value).toBe('Unwritten draft');
 expect(screen.getByText('Not saved')).toBeTruthy();
});
