// @vitest-environment jsdom
// Public streaming transitions: neither saving a Pi entry nor the next model
// request may remount an existing Cindy work group or inject status into input.
import {afterEach,expect,it} from 'vitest';
import {cleanup,fireEvent,render,screen} from '@testing-library/react';
import {projectChatProcess,type ProcessResponse} from './chat-process';
import type {ChatMessage} from './chat-state';
import {WorkProcess} from './cindy/WorkProcess';
import {ChatMessageView} from './chat-messages';
import {__test_internals as expansion} from './cindy/useExpandedBlockMemory';
afterEach(()=>{cleanup();expansion.reset();});
const calls=Array.from({length:6},(_,index)=>({type:'toolCall',id:`call-${index}`,name:'powershell',arguments:{command:`Write-Output ${index}`}}));
const user:ChatMessage={id:'user',turnId:'turn',role:'user',content:'Inspect'};
const work:ChatMessage={role:'assistant',turnId:'turn',timestamp:100,content:[{type:'thinking',thinking:'Inspecting'},...calls]};
function group(messages:ChatMessage[]){return projectChatProcess([user,...messages],[],true).find(item=>item.kind==='response') as ProcessResponse;}
const process=(messages:ChatMessage[])=><WorkProcess group={group(messages)} conversationId='stable' onCopy={()=>{}}/>;

it('retains the same work disclosure and manual expansion when a streamed message is saved',()=>{
 const view=render(process([work]));
 const header=view.container.querySelector('[data-slot=cindy-work]') as HTMLButtonElement;
 fireEvent.click(header);expect(view.container.querySelector('[data-live-work-preview]')).toBeNull();
 view.rerender(process([{...work,id:'saved-entry'}]));
 expect(view.container.querySelector('[data-slot=cindy-work]')).toBe(header);
 expect(view.container.querySelector('[data-live-work-preview]')).toBeNull();
 expect(view.container.querySelectorAll('.process-body [data-live-work-activity=tool]')).toHaveLength(6);
});
it('does not close and reopen the latest-five preview while the next request starts thinking',()=>{
 const view=render(process([work]));const header=view.container.querySelector('[data-slot=cindy-work]');
 for(const content of [[],[{type:'thinking',thinking:'Next step'}], [{type:'thinking',thinking:'Next step'},...calls.map(call=>({...call,id:call.id+'-next'}))]]){
  view.rerender(process([work,{role:'assistant',turnId:'turn',timestamp:200,content}]));
  expect(view.container.querySelector('[data-slot=cindy-work]')).toBe(header);
  expect(view.container.querySelector('[data-live-work-preview]')).not.toBeNull();
 }
});
it.each(['queue','steer'] as const)('never places %s metadata inside the user message',deliveryMode=>{
 const message:ChatMessage={...user,pending:true,deliveryMode,content:'Only my own words'};
 const view=render(<ChatMessageView message={message} onCopy={()=>{}} onFork={()=>{}} canFork={false}/>);
 expect(screen.getByText('Only my own words')).toBeTruthy();
 expect(view.container.querySelector('.desktop-conversation-message [role=status]')).toBeNull();
 expect(view.container.textContent).not.toMatch(/等待排队处理|等待插入/);
});
it.each([true,false])('uses the current Cindy work surface for thinking-only messages (running=%s)',running=>{
 const view=render(<ChatMessageView message={{role:'assistant',timestamp:1,content:[{type:'thinking',thinking:'Real thinking'}]}} running={running} onCopy={()=>{}} onFork={()=>{}} canFork={false}/>);
 expect(view.container.querySelector('[data-slot=cindy-work]')).not.toBeNull();
 expect(view.container.querySelector('[data-slot=cindy-thinking]')).toBeNull();
});
it('does not render a timed but still empty thinking block',()=>{
 const view=render(<ChatMessageView message={{role:'assistant',content:[{type:'thinking',thinkingStartedAt:1}]}} running onCopy={()=>{}} onFork={()=>{}} canFork={false}/>);
 expect(view.container.childElementCount).toBe(0);
});
it('defers provider errors while Cindy/Pi is still recovering the same turn',()=>{
 const view=render(<ChatMessageView message={{role:'assistant',content:[],errorMessage:'The socket connection was closed unexpectedly.'}} running onCopy={()=>{}} onFork={()=>{}} canFork={false}/>);
 expect(view.container.querySelector('[role=alert]')).toBeNull();
 view.rerender(<ChatMessageView message={{role:'assistant',content:[],errorMessage:'The socket connection was closed unexpectedly.'}} onCopy={()=>{}} onFork={()=>{}} canFork={false}/>);
 expect(view.container.querySelector('[role=alert]')).not.toBeNull();
});
