// @vitest-environment jsdom
import {afterEach,expect,it} from 'vitest';
import {cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {projectChatProcess} from './chat-process';
import {WorkProcess} from './cindy/WorkProcess';
import {WorkGroupBlock,type WorkGroupChild} from './cindy/WorkGroupBlock';
import {__test_internals as expansion} from './cindy/useExpandedBlockMemory';
afterEach(()=>{cleanup();expansion.reset();});

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
