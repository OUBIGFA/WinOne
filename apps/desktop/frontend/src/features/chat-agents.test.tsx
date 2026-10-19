// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {ChatAgents} from './chat-agents';
import {ChatDesktopSurface} from './chat-desktop-kit';
const call=vi.hoisted(()=>vi.fn());vi.mock('../bridge',()=>({chatCall:call}));
afterEach(()=>{cleanup();vi.resetAllMocks();});
const agents=[{name:'scout',status:'completed',task:'Inspect',model:'child-model'}];
const page={entries:[{id:3,model:[{role:'assistant',content:'Child reply'}]}],cursor:{before:3}};
async function open(){const user=userEvent.setup();render(<ChatDesktopSurface><ChatAgents conversationId="parent" agents={agents} busy={false} paused={false}/></ChatDesktopSurface>);await user.click(screen.getByRole('button',{name:'子代理 · 1'}));await user.click(screen.getByRole('button',{name:'查看消息 · scout'}));return user;}
it('reads child history, pages earlier messages and sends a follow-up without altering the parent draft',async()=>{
 call.mockImplementation(async(_method,params)=>params.action==='read'?params.agentCursor?{entries:[{id:1,model:[{role:'user',content:'Earlier child task'}]}]}:page:{accepted:true});
 const user=await open();await screen.findByText('Child reply');
 expect(call).toHaveBeenCalledWith('agents',{id:'parent',action:'read',name:'scout'});
 await user.click(screen.getByRole('button',{name:'加载更早消息'}));await screen.findByText('Earlier child task');expect(screen.getByText('Child reply')).toBeTruthy();
 fireEvent.change(screen.getByRole('textbox',{name:'发给子代理的消息'}),{target:{value:'Continue the inspection'}});
 await user.click(screen.getByRole('button',{name:'发送后续消息'}));
 expect(call).toHaveBeenCalledWith('agents',{id:'parent',action:'follow_up',name:'scout',text:'Continue the inspection'});
 await waitFor(()=>expect(screen.getByRole('textbox',{name:'发给子代理的消息'})).toHaveProperty('value',''));
 expect(call.mock.calls.some(([,params])=>params.action==='prompt'||params.action==='draft.save')).toBe(false);
});
it('retains the child draft after a failed follow-up and offers retry for failed history',async()=>{
 let failRead=true;call.mockImplementation(async(_method,params)=>{if(params.action==='read'){if(failRead)throw Error('History offline');return page;}throw Error('Send offline');});
 const user=await open();await screen.findByText('History offline');failRead=false;
 await user.click(screen.getByRole('button',{name:'刷新消息'}));await screen.findByText('Child reply');
 fireEvent.change(screen.getByRole('textbox',{name:'发给子代理的消息'}),{target:{value:'Keep this child draft'}});
 await user.click(screen.getByRole('button',{name:'发送后续消息'}));await screen.findByText('Send offline');
 expect(screen.getByRole('textbox',{name:'发给子代理的消息'})).toHaveProperty('value','Keep this child draft');
});
it('keeps child mutations and closed-harness reads disabled while paused',async()=>{
 render(<ChatDesktopSurface><ChatAgents conversationId="parent" agents={agents} busy={false} paused/></ChatDesktopSurface>);
 await userEvent.click(screen.getByRole('button',{name:'子代理 · 1'}));
 expect(screen.getByRole('button',{name:'查看消息 · scout'})).toHaveProperty('disabled',true);expect(call).not.toHaveBeenCalled();
});
it('refreshes committed follow-up results even when the child completed between snapshots',async()=>{
 let reads=0;call.mockImplementation(async()=>({entries:[{id:++reads,model:[{role:'assistant',content:reads===1?'First result':'Follow-up result'}]}]}));
 const view=render(<ChatDesktopSurface><ChatAgents conversationId="parent" agents={[{...agents[0],finishedAt:1}]} busy={false} paused={false}/></ChatDesktopSurface>);
 const user=userEvent.setup();await user.click(screen.getByRole('button',{name:'子代理 · 1'}));await user.click(screen.getByRole('button',{name:'查看消息 · scout'}));await screen.findByText('First result');
 // A fast follow-up can finish before the next Host snapshot; status remains
 // completed, so its new commit timestamp must trigger a history refresh.
 view.rerender(<ChatDesktopSurface><ChatAgents conversationId="parent" agents={[{...agents[0],finishedAt:2}]} busy={false} paused={false}/></ChatDesktopSurface>);
 await screen.findByText('Follow-up result');
});
