// @vitest-environment jsdom
import {cleanup,render,screen,waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {afterEach,expect,it} from 'vitest';
import {ChatPlan} from './chat-plan';
afterEach(cleanup);
it('shows the current step, expands the committed steps and keeps completed plans collapsed',async()=>{
 const workflow={items:[{step:'Read configuration',status:'completed' as const},{step:'Check the result',status:'in_progress' as const}],explanation:'Keep the saved values'};
 const view=render(<ChatPlan workflow={workflow}/>);const user=userEvent.setup();
 expect(screen.getByText('Check the result')).toBeTruthy();
 expect(screen.queryByText('Read configuration')).toBeNull();
 await user.click(screen.getByRole('button',{name:/计划/}));
 expect(await screen.findByText('Read configuration')).toBeTruthy();expect(screen.getByText('Keep the saved values')).toBeTruthy();
 view.rerender(<ChatPlan workflow={{items:workflow.items.map(item=>({...item,status:'completed'}))}}/>);
 await waitFor(()=>expect(screen.queryByText('Read configuration')).toBeNull());
 expect(screen.getByRole('button',{name:/计划/}).getAttribute('aria-expanded')).toBe('false');
});
