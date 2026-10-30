// @vitest-environment jsdom
// What a OneBot teammate learned from a reply (Cindy botLearning receipts):
// the open chat reads them from the Host and shows them under that reply.
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {act,cleanup,render,renderHook,screen,waitFor} from '@testing-library/react';
import {MemoryRouter} from 'react-router-dom';
import {i18n} from '../i18n';
import './cindy/host/onebotLocales';
import {ChatMessageView} from './chat-messages';
import {useBotLearningReceipts,withBotLearningReceipts} from './chat-bot-mode';
import type {ChatMessage} from './chat-state';

const bridge=vi.hoisted(()=>({call:vi.fn()}));
vi.mock('../bridge',async importOriginal=>({...await importOriginal<typeof import('../bridge')>(),chatCall:bridge.call}));

const owner:ChatMessage={id:'owner-entry',requestId:'input-1',turnId:'input-1',role:'user',content:'Use short reports from now on',timestamp:1_000};
const reply:ChatMessage={id:'reply',turnId:'input-1',role:'assistant',content:'Understood.',timestamp:2_000};
const remembered={kind:'memory',key:'user_report-format.md',title:'Report format',action:'created'};
let receipts:Record<string,unknown>;
let pending:Array<(value:unknown)=>void>;
const reads:unknown[]=[];

beforeEach(async()=>{
 await i18n.changeLanguage('en-US');
 receipts={reply:[remembered]};pending=[];reads.length=0;
 bridge.call.mockReset().mockImplementation(async(method:string,params:any)=>{
  if(method==='onebot.events')return params.cursor===Number.MAX_SAFE_INTEGER?{cursor:1,events:[],reset:true}:new Promise(resolve=>pending.push(resolve));
  if(method==='onebot.messages.learning'){reads.push(params);return {receipts:structuredClone(receipts)};}
  throw Error('Unexpected '+method);
 });
});
afterEach(async()=>{cleanup();await act(async()=>{for(const finish of pending.splice(0))finish({cursor:1,events:[],reset:false});});});

/** Deliver one Host change page to the open long poll. */
async function push(cursor:number,events:unknown[]){
 await waitFor(()=>expect(pending.length).toBeGreaterThan(0));
 await act(async()=>{pending.shift()!({cursor,events,reset:false});});
}

it('shows a reply\'s receipts under that reply only',()=>{
 const messages=withBotLearningReceipts([owner,reply],{reply:[remembered],'owner-entry':[remembered]});
 // Owner messages never carry receipts; unchanged rows keep their identity.
 expect(messages[0]).toBe(owner);
 expect(messages[1].agentMeta?.botLearning).toEqual([remembered]);
 const unchanged=[owner,reply];
 expect(withBotLearningReceipts(unchanged,{other:[remembered]})).toBe(unchanged);
 expect(withBotLearningReceipts(unchanged,null)).toBe(unchanged);
 render(<MemoryRouter><ChatMessageView conversationId="chat" message={messages[1]} onCopy={()=>{}} onFork={()=>{}} canFork bot={{avatar:null,clientId:'reply',replyToClientId:'input-1'}}/></MemoryRouter>);
 expect(screen.getByRole('button',{name:'Remembered: Report format'})).toBeTruthy();
});

it('reads this chat\'s receipts again only when the Host reports a change for it',async()=>{
 const {result}=renderHook(()=>useBotLearningReceipts('chat',true));
 await waitFor(()=>expect(result.current).toEqual({reply:[remembered]}));
 const initial=reads.length;
 expect(reads.every(params=>JSON.stringify(params)==='{"sessionId":"chat"}')).toBe(true);
 receipts={reply:[remembered,{kind:'skill',key:'short-reports',title:'Short reports',action:'created'}]};
 await push(2,[{type:'message-meta-changed',payload:{sessionId:'other',clientId:'x'}}]);
 expect(reads.length).toBe(initial);
 await push(3,[{type:'message-meta-changed',payload:{sessionId:'chat',clientId:'reply'}}]);
 await waitFor(()=>expect((result.current?.reply as unknown[])?.length).toBe(2));
 expect(reads.length).toBe(initial+1);
});

it('reads nothing outside a teammate chat',async()=>{
 const {result}=renderHook(()=>useBotLearningReceipts('chat',false));
 await act(async()=>{});
 expect(result.current).toBeNull();
 expect(bridge.call).not.toHaveBeenCalled();
});
