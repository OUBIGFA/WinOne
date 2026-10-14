// @vitest-environment jsdom
import {afterEach,expect,it} from 'vitest';
import {cleanup,render,screen} from '@testing-library/react';
import {projectChatProcess} from './chat-process';
import {WorkProcess} from './cindy/WorkProcess';
afterEach(cleanup);

it('keeps every work segment fully open while a turn alternates between text and tools',()=>{
 const messages=[{id:'u-live',role:'user',content:'Build'},
  {id:'first',role:'assistant',timestamp:1,content:[{type:'thinking',thinking:'First complete working detail'}]},
  {id:'delivery',role:'assistant',timestamp:2,stopReason:'stop',content:'Progress update'},
  {id:'second',role:'assistant',timestamp:3,content:[{type:'thinking',thinking:'Second complete working detail'}]}];
 const group=projectChatProcess(messages,[],true).find(item=>item.kind==='response');
 if(!group||group.kind!=='response')throw Error('response missing');
 const {container,rerender}=render(<WorkProcess group={group} conversationId="live-test" onCopy={()=>{}}/>);
 expect(Array.from(container.querySelectorAll('[data-slot="cindy-work"]')).every(node=>node.getAttribute('aria-expanded')==='true')).toBe(true);
 expect(container.querySelectorAll('.chat-expanded-thinking')).toHaveLength(2);
 const next=projectChatProcess([...messages,{id:'answer',role:'assistant',timestamp:4,content:'Still working'}],[],true).find(item=>item.kind==='response');
 if(!next||next.kind!=='response')throw Error('response missing');
 rerender(<WorkProcess group={next} conversationId="live-test" onCopy={()=>{}}/>);
 expect(container.querySelectorAll('.chat-expanded-thinking')).toHaveLength(2);
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
 expect(screen.getByText('First deliverable').closest('.chat-work-commentary')).toBeTruthy();
 expect(container.querySelectorAll('[data-slot="cindy-work"]')).toHaveLength(2);
});
