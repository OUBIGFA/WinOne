// @vitest-environment jsdom
// A OneBot teammate's chat on WinOne's Chat surface: Cindy's companion rows,
// private-chat reply quotes and the time/unread timeline.
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,renderHook,screen} from '@testing-library/react';
import {i18n} from '../i18n';
import './cindy/host/onebotLocales';
import {ChatMessageView} from './chat-messages';
import {botTranscriptRows,useBotTimeline,BotTimelineMarkers} from './chat-bot-mode';
import {projectChatProcess} from './chat-process';
import type {ChatMessage} from './chat-state';
import {MemoryRouter} from 'react-router-dom';
import {BotChatQuoteProvider,type BotChatQuoteValue} from './cindy/bots/botChatQuoteContext';

beforeEach(async()=>{await i18n.changeLanguage('en-US');});
afterEach(()=>cleanup());

const hidden:ChatMessage={id:'welcome-entry',requestId:'welcome',turnId:'welcome',role:'user',content:'Host greeting instruction',hiddenInput:true,timestamp:1_000};
const opening:ChatMessage={id:'opening',turnId:'welcome',role:'assistant',content:'Hello, I am Reader.',timestamp:2_000};
const owner:ChatMessage={id:'owner-entry',requestId:'input-1',turnId:'input-1',role:'user',content:'Summarise the report',timestamp:3_000,attachments:[{name:'chart.png',path:'C:/a/chart.png',size:1,mimeType:'image/png'},{name:'data.csv',path:'C:/a/data.csv',size:1,mimeType:'text/csv'}]};
const narration:ChatMessage={id:'narration',turnId:'input-1',role:'assistant',content:'Reading it now.',stopReason:'toolUse',timestamp:4_000};
const reply:ChatMessage={id:'reply',turnId:'input-1',role:'assistant',content:'The report shows growth.',timestamp:900_000};

it('publishes the rows Cindy reads: owner inputs by receipt id and replies with the input they answer',()=>{
 expect(botTranscriptRows([hidden,opening,owner,narration,reply])).toEqual([
  {clientId:'opening',role:'assistant',content:'Hello, I am Reader.',createdAt:2_000,replyToClientId:'welcome'},
  {clientId:'input-1',role:'user',content:'Summarise the report',createdAt:3_000,images:[{originalName:'chart.png'}],files:[{name:'data.csv'}]},
  {clientId:'reply',role:'assistant',content:'The report shows growth.',createdAt:900_000,replyToClientId:'input-1'},
 ]);
});

it('groups the timeline by five minutes and marks the first reply after the read position',()=>{
 const items=projectChatProcess([hidden,opening,owner,reply],[],false);
 const {result}=renderHook(()=>useBotTimeline(items,true,2_500));
 expect([...result.current.groups.keys()]).toEqual(['response:welcome','response:input-1']);
 expect(result.current.unreadKey).toBe('response:input-1');
 expect(result.current.latestReplyAt).toBe(900_000);
 render(<BotTimelineMarkers timestamp={900_000} unread/>);
 expect(screen.getByRole('separator',{name:'NEW'})).toBeTruthy();
});

function quoteContext(overrides:Partial<BotChatQuoteValue>={}):BotChatQuoteValue{
 const rows=botTranscriptRows([owner,reply]);
 return {sessionId:'chat',names:{owner:'You',companion:'Reader'},getMessage:id=>rows.find(row=>row.clientId===id),
  isFirstReplyFor:(assistant,target)=>assistant==='reply'&&target==='input-1',showReplyTargets:false,quote:vi.fn(),locate:vi.fn(),...overrides};
}
// A teammate's chat always renders inside the OneBot page router.
const avatar=<span data-testid="avatar">R</span>;

it('quotes a teammate reply from its Reply action and shows the face beside the bubble',()=>{
 const context=quoteContext();
 const view=render(<MemoryRouter><BotChatQuoteProvider value={context}><ChatMessageView conversationId="chat" message={reply} onCopy={()=>{}} onFork={()=>{}} canFork bot={{avatar,clientId:'reply',replyToClientId:'input-1'}}/></BotChatQuoteProvider></MemoryRouter>);
 expect(view.container.querySelector('.desktop-conversation-message--bot .chat-message-bubble')?.textContent).toContain('The report shows growth.');
 expect(screen.getByTestId('avatar')).toBeTruthy();
 // Cindy's companion bar has no branch action.
 expect(screen.queryByRole('button',{name:i18n.t('chat.branchFrom')})).toBeNull();
 fireEvent.click(screen.getByRole('button',{name:'Reply'}));
 expect(context.quote).toHaveBeenCalledWith({clientId:'reply',role:'assistant',excerpt:'The report shows growth.'});
});

it('shows the owner quote inside the bubble and the answered message only when that preference is on',()=>{
 const quoted:ChatMessage={...owner,requestId:'input-2',content:'And the costs?',replyQuote:{clientId:'reply',role:'assistant',excerpt:'Snapshot'}};
 const context=quoteContext();
 const user=render(<MemoryRouter><BotChatQuoteProvider value={context}><ChatMessageView conversationId="chat" message={quoted} onCopy={()=>{}} onFork={()=>{}} canFork={false} bot={{avatar,clientId:'input-2'}}/></BotChatQuoteProvider></MemoryRouter>);
 // The original is loaded, so the live text replaces the send-time snapshot.
 expect(user.container.querySelector('.chat-message-bubble')?.textContent).toContain('The report shows growth.');
 fireEvent.click(screen.getByText('The report shows growth.'));
 expect(context.locate).toHaveBeenCalledWith('reply');
 cleanup();
 const hiddenTarget=render(<MemoryRouter><BotChatQuoteProvider value={context}><ChatMessageView conversationId="chat" message={reply} onCopy={()=>{}} onFork={()=>{}} canFork={false} bot={{avatar,clientId:'reply',replyToClientId:'input-1'}}/></BotChatQuoteProvider></MemoryRouter>);
 expect(hiddenTarget.container.querySelector('[data-bot-reply-target]')).toBeNull();
 cleanup();
 const shown=render(<MemoryRouter><BotChatQuoteProvider value={quoteContext({showReplyTargets:true})}><ChatMessageView conversationId="chat" message={reply} onCopy={()=>{}} onFork={()=>{}} canFork={false} bot={{avatar,clientId:'reply',replyToClientId:'input-1'}}/></BotChatQuoteProvider></MemoryRouter>);
 expect(shown.container.querySelector('[data-bot-reply-target="input-1"]')?.textContent).toContain('Summarise the report');
});
