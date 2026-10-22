// @vitest-environment jsdom
import {act,cleanup,fireEvent,render,screen,within} from '@testing-library/react';
import {useRef} from 'react';
import {afterEach,expect,it,vi} from 'vitest';
import {ChatPlan,ChatPlanReminder} from './chat-plan';
import {WorkProcess} from './cindy/WorkProcess';
import type {ChatWorkflow} from './chat-state';
import {__test_internals} from './cindy/useExpandedBlockMemory';
afterEach(()=>{cleanup();vi.unstubAllGlobals();__test_internals.reset();});
const workflow:ChatWorkflow={items:[{step:'Read configuration',status:'completed'},{step:'Check the result',status:'in_progress'}],explanation:'Keep the saved values'};

it('reuses the inline card, retaining the reader’s disclosure state when the plan completes',()=>{
 const view=render(<ChatPlan workflow={workflow}/>);
 const plan=screen.getByRole('region',{name:'计划'}),button=within(plan).getByRole('button');
 expect(within(plan).getByText('Read configuration')).toBeTruthy();expect(within(plan).getByText('Keep the saved values')).toBeTruthy();
 fireEvent.click(button);expect(button.getAttribute('aria-expanded')).toBe('false');
 view.rerender(<ChatPlan workflow={{items:workflow.items.map(item=>({...item,status:'completed'}))}}/>);
 expect(button.getAttribute('aria-expanded')).toBe('false');
 fireEvent.click(button);expect(within(plan).getByText('Read configuration')).toBeTruthy();
 expect(plan.querySelector('.animate-spinner')).toBeNull();
});

it('publishes one stable inline plan from committed update_plan calls, not their raw JSON',()=>{
 const call=(id:string,items:ChatWorkflow['items'])=>({kind:'tool' as const,key:id,tool:{id,name:'update_plan',args:{plan:items},running:false,result:JSON.stringify({items})}});
 const view=render(<WorkProcess conversationId="c" onCopy={()=>{}} group={{kind:'response',key:'turn',messages:[],running:true,hasTools:true,entries:[call('first',workflow.items)]}}/>);
 const anchor=screen.getByRole('region',{name:'计划'});
 view.rerender(<WorkProcess conversationId="c" onCopy={()=>{}} group={{kind:'response',key:'turn',messages:[],running:true,hasTools:true,entries:[call('first',workflow.items),call('last',workflow.items.map(item=>({...item,status:'completed'})))]}}/>);
 expect(screen.getAllByRole('region',{name:'计划'})).toHaveLength(1);expect(screen.getByRole('region',{name:'计划'})).toBe(anchor);
 expect(anchor.getAttribute('data-chat-plan-key')).toBe('c:turn:first');
 expect(screen.queryByRole('button',{name:/update_plan/})).toBeNull();expect(within(anchor).getByRole('button').textContent).toContain('2/2');
});

it('shows the reminder only off-screen; completed steps do not end a running plan',()=>{
 let notify:(entries:{isIntersecting:boolean}[])=>void=()=>{};
 vi.stubGlobal('IntersectionObserver',class{constructor(callback:typeof notify){notify=callback;}observe(){notify([{isIntersecting:true}]);}disconnect(){}});
 function Fixture({active=true,items=workflow.items}:{active?:boolean;items?:ChatWorkflow['items']}){const viewport=useRef<HTMLDivElement>(null);return <><div ref={viewport}><ChatPlan anchor="live-plan" workflow={{items}}/></div><ChatPlanReminder anchor="live-plan" workflow={{items}} viewport={viewport} active={active} animated={active}/></>;}
 const view=render(<Fixture/>);expect(screen.queryByRole('button',{name:/步骤/})).toBeNull();
 act(()=>notify([{isIntersecting:false}]));const pill=screen.getByRole('button',{name:/步骤/});
 fireEvent.click(pill);expect(pill.getAttribute('aria-expanded')).toBe('true');
 fireEvent.pointerDown(document.body);expect(pill.getAttribute('aria-expanded')).toBe('false');
 view.rerender(<Fixture items={workflow.items.map(item=>({...item,status:'completed'}))}/>);
 act(()=>notify([{isIntersecting:false}]));expect(screen.getByRole('button',{name:/步骤/})).toBeTruthy();
 view.rerender(<Fixture active={false}/>);expect(screen.queryByRole('button',{name:/步骤/})).toBeNull();
});
