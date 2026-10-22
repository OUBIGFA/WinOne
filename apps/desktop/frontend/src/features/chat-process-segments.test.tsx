// @vitest-environment jsdom
import {afterEach,expect,it} from 'vitest';
import {cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {projectChatProcess} from './chat-process';
import {WorkProcess} from './cindy/WorkProcess';
import {WorkGroupBlock,type WorkGroupChild} from './cindy/WorkGroupBlock';
import type {ChatMessage,ProcessOperation} from './chat-state';
import {RetryStatus} from './cindy/RetryStatus';
import {__test_internals as expansion} from './cindy/useExpandedBlockMemory';
afterEach(()=>{cleanup();expansion.reset();});

it.each([false,true])('does not collapse or remount work across retry attempts (expanded=%s)',async expanded=>{
 const messages:ChatMessage[]=[{id:'u',turnId:'u',role:'user',content:'Check files'},...Array.from({length:7},(_,i):ChatMessage=>({role:'assistant',turnId:'u',timestamp:100+i,content:[{type:'thinking',thinking:`Inspect file ${i}`}]}))];
 const turn:ProcessOperation={id:'turn',turnId:'u',kind:'turn',status:'running',startedAt:90};
 const group=(operations:ProcessOperation[])=>{const item=projectChatProcess(messages,[],true,[turn,...operations]).find(item=>item.kind==='response');if(!item||item.kind!=='response')throw Error('missing group');return item;};
 const view=render(<WorkProcess group={group([])} conversationId="retry" onCopy={()=>{}}/>);
 const header=view.container.querySelector<HTMLButtonElement>('[data-slot=cindy-work]')!;
 if(expanded)fireEvent.click(header);
 const preview=view.container.querySelector('[data-live-work-preview]');
 for(let attempt=1;attempt<=3;attempt++){
  messages.push({role:'assistant',turnId:'u',timestamp:200+attempt,content:'',stopReason:'error',errorMessage:'HTTP 503'});
  const operation:ProcessOperation={id:'retry-'+attempt,turnId:'u',kind:'retry',status:'running',startedAt:Date.now(),delayMs:1000,attempt,maxAttempts:3};
  const response=group([operation]);
  expect(response.startedAt).toBe(90);
  view.rerender(<><WorkProcess group={response} conversationId="retry" onCopy={()=>{}}/><RetryStatus operation={operation} onStop={()=>{}}/></>);
  expect(view.container.querySelector('[data-slot=cindy-work]')).toBe(header);
  expect(expansion.isExpanded('retry:u:work-100:0')).toBe(expanded);
  if(!expanded)expect(view.container.querySelector('[data-live-work-preview]')).toBe(preview);
  expect(view.container.querySelector('.chat-retry-status')?.closest('.chat-work-process')).toBeNull();
  expect(view.container.querySelectorAll('[data-slot=cindy-work]')).toHaveLength(1);
 }
});

it('uses Cindy latest-five preview and keeps manual folding responsive while streaming',async()=>{
 const children=(count:number):WorkGroupChild[]=>Array.from({length:count},(_,i)=>({kind:'tools',key:String(i),toolCalls:[{clientId:String(i),content:'',toolName:'read',toolInput:{path:`file-${i}.txt`}}],resultMap:new Map(),settledIds:new Set([String(i)])}));
 const {container,rerender}=render(<WorkGroupBlock blockId="live-preview" isStreaming childItems={children(8)}/>);
 const header=container.querySelector('[data-slot="cindy-work"]') as HTMLButtonElement;
 expect(header.disabled).toBe(false);
 expect(container.querySelectorAll('[data-live-work-preview] [data-live-work-activity="tool"]')).toHaveLength(5);
 fireEvent.click(header);
 await waitFor(()=>expect(container.querySelectorAll('.process-body [data-live-work-activity="tool"]')).toHaveLength(8));
 fireEvent.click(header);
 expect(container.querySelector('[data-live-work-preview]')).not.toBeNull();
 rerender(<WorkGroupBlock blockId="live-preview" isStreaming childItems={children(9)}/>);
 expect(container.querySelectorAll('[data-live-work-preview] [data-live-work-activity="tool"]')).toHaveLength(5);
 expect(expansion.isExpanded('live-preview')).toBe(false);
});

it('does not offer a misleading group toggle when all live activities already fit the preview',()=>{
 const {container}=render(<WorkGroupBlock blockId="small-live" isStreaming childItems={[{kind:'thinking',key:'one',message:{clientId:'one',content:'Inspect the project'}}]}/>);
 expect((container.querySelector('[data-slot="cindy-work"]') as HTMLButtonElement).disabled).toBe(true);
 expect(container.querySelector('[data-live-work-preview]')).not.toBeNull();
});

it.each([true,false])('keeps a delivered answer between separate work groups (running=%s)',running=>{
 const items=projectChatProcess([
  {id:'u',role:'user',content:'Build and then verify'},
  {id:'work1',role:'assistant',timestamp:1,content:[{type:'thinking',thinking:'Inspect first'}]},
  {id:'delivery',role:'assistant',timestamp:2,stopReason:'stop',content:'First deliverable'},
  {id:'work2',role:'assistant',timestamp:3,content:[{type:'thinking',thinking:'Verify later'}]},
  {id:'done',role:'assistant',timestamp:4,content:'Final result'},
 ],[],running);
 const group=items.find(item=>item.kind==='response');if(!group||group.kind!=='response')throw Error('response missing');
 const {container}=render(<WorkProcess group={group} conversationId="test" onCopy={()=>{}}/>);
 expect(screen.getByText('First deliverable').closest('.desktop-conversation-message')).toBeTruthy();
 expect(screen.getByText('First deliverable').closest('.chat-work-process')).toBeNull();
 expect(container.querySelectorAll('[data-slot="cindy-work"]')).toHaveLength(2);
});
