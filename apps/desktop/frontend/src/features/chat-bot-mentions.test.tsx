// @vitest-environment jsdom
// Cindy's composer @teammate items on WinOne's textarea composer: the list,
// the keyboard choice, the wire links the Host validates, and the chips the
// sent message shows.
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen} from '@testing-library/react';
import {useState} from 'react';
import {i18n} from '../i18n';
import {ChatComposer} from './chat-composer';
import {ChatMarkdown} from './chat-markdown';
import {activeBotMentionQuery,serializeBotMentions} from './chat-bot-mentions';
import {readAgentInputReferences} from '../../../../../modules/chat-pi/cindy/tasks/core/agentInputProjection';

beforeEach(async()=>{await i18n.changeLanguage('en-US');});
afterEach(()=>cleanup());

const mentions=[{id:'reader',name:'Reader',description:'Reads reports'},{id:'reader-pro',name:'Reader Pro'},{id:'assistant',name:'小助手'}];

it('finds the @query at the caret and ignores addresses and finished words',()=>{
 expect(activeBotMentionQuery('Ask @Rea',8)).toEqual({start:4,query:'Rea'});
 expect(activeBotMentionQuery('请@小助',4)).toEqual({start:1,query:'小助'});
 expect(activeBotMentionQuery('mail a@b',8)).toBeNull();
 expect(activeBotMentionQuery('Ask @Reader now',15)).toBeNull();
});

it('sends each current teammate as its reference link with the range the Host accepts',()=>{
 const sent=serializeBotMentions('@Reader Pro and @Reader, then 请@小助手看看 (mail x@Reader.com, @Readers, @Nobody)',mentions);
 expect(sent.text).toBe('[Reader Pro](cindy://bot/reader-pro) and [Reader](cindy://bot/reader), then 请[小助手](cindy://bot/assistant)看看 (mail x@Reader.com, @Readers, @Nobody)');
 expect(sent.agentReferences.map(reference=>[reference.botId,sent.text.slice(reference.start,reference.end)])).toEqual([
  ['reader-pro','[Reader Pro](cindy://bot/reader-pro)'],['reader','[Reader](cindy://bot/reader)'],['assistant','[小助手](cindy://bot/assistant)'],
 ]);
 // The Host keeps exactly these references (Cindy readAgentInputReferences).
 expect(readAgentInputReferences(sent.agentReferences,sent.text)).toHaveLength(3);
 // Two teammates with one name cannot be told apart; that @Name stays text.
 expect(serializeBotMentions('@Reader',[...mentions,{id:'other',name:'Reader'}])).toEqual({text:'@Reader',agentReferences:[]});
 // Brackets in a name are escaped inside the link label.
 const odd=serializeBotMentions('@A[1] hi',[{id:'a',name:'A[1]'}]);
 expect(odd.text).toBe('[A\\[1\\]](cindy://bot/a) hi');
 expect(readAgentInputReferences(odd.agentReferences,odd.text)).toHaveLength(1);
});

function Composer({initial='',list=mentions}:{initial?:string;list?:typeof mentions}){
 const [text,setText]=useState(initial);
 return <ChatComposer text={text} onText={setText} onSend={vi.fn()} onStop={vi.fn()} running={false} busy={false} providers={[]} model={{provider:'',model:''}} permission='ask' onModel={vi.fn()} onPermission={vi.fn()} hideRuntimeControls mentions={list}/>;
}
function type(input:HTMLTextAreaElement,value:string){
 fireEvent.change(input,{target:{value,selectionStart:value.length,selectionEnd:value.length}});
}

it('lists matching teammates for @ and inserts the chosen name from the keyboard',async()=>{
 render(<Composer/>);
 const input=screen.getByRole('combobox',{name:'Message'}) as HTMLTextAreaElement;
 type(input,'Ask @rea');
 const list=screen.getByRole('listbox',{name:'Teammates'});
 expect(Array.from(list.querySelectorAll('[role=option]')).map(option=>option.textContent)).toEqual(['ReaderMessage or delegate','Reader ProMessage or delegate']);
 expect(input.getAttribute('aria-expanded')).toBe('true');
 fireEvent.keyDown(input,{key:'ArrowDown'});
 expect(screen.getAllByRole('option')[1].getAttribute('aria-selected')).toBe('true');
 fireEvent.keyDown(input,{key:'Enter'});
 expect(input.value).toBe('Ask @Reader Pro ');
 expect(screen.queryByRole('listbox')).toBeNull();
});

it('closes on Escape until the caret leaves that @, and shows nothing without matches',()=>{
 render(<Composer/>);
 const input=screen.getByRole('combobox',{name:'Message'}) as HTMLTextAreaElement;
 type(input,'@');
 expect(screen.getAllByRole('option')).toHaveLength(3);
 fireEvent.keyDown(input,{key:'Escape'});
 expect(screen.queryByRole('listbox')).toBeNull();
 type(input,'@R');
 expect(screen.queryByRole('listbox')).toBeNull();
 type(input,'@R @zz');
 expect(screen.queryByRole('listbox')).toBeNull();
 type(input,'@R @小');
 expect(screen.getAllByRole('option').map(option=>option.textContent)).toEqual(['小助手Message or delegate']);
 fireEvent.mouseDown(screen.getByRole('option'));
 fireEvent.click(screen.getByRole('option'));
 expect(input.value).toBe('@R @小助手 ');
});

it('keeps an ordinary chat composer without the teammate list',()=>{
 render(<Composer list={[]}/>);
 const input=screen.getByRole('textbox',{name:'Message'}) as HTMLTextAreaElement;
 type(input,'@rea');
 expect(screen.queryByRole('listbox')).toBeNull();
 expect(input.getAttribute('aria-expanded')).toBeNull();
});

it('shows a referenced teammate as a read-only chip in a message',()=>{
 const view=render(<ChatMarkdown text={'Please ask [Reader](cindy://bot/reader) to check.'}/>);
 const chip=view.container.querySelector('[data-inline-reference-chip]');
 expect(chip?.textContent).toBe('Reader');
 expect(chip?.getAttribute('role')).toBeNull();
 expect(view.container.querySelector('a')).toBeNull();
});
