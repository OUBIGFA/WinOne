// @vitest-environment jsdom
import {cleanup,render,screen,within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {afterEach,expect,it,vi} from 'vitest';
import {ChatComposer} from './chat-composer';
afterEach(cleanup);
const base={text:'Keep my draft',onText:vi.fn(),onSend:vi.fn(),onStop:vi.fn(),running:false,busy:false,providers:[{id:'p',name:'Provider',authMode:'api-key' as const,hasKey:true,models:['m']}],model:{provider:'p',model:'m'},permission:'ask' as const,onModel:vi.fn(),onPermission:vi.fn()};
it('shows task ownership and resumes paused execution without sending the draft',async()=>{
 const onResume=vi.fn(),onSend=vi.fn(),user=userEvent.setup();
 render(<ChatComposer {...base} onSend={onSend} paused onResume={onResume} taskGraph={{scheduling:'paused',total:2,tasks:[{id:1,kind:'root',conversationId:1,background:false,state:'waiting'},{id:2,kind:'child',conversationId:1,owner:1,background:true,state:'ready'}]}}/>);
 expect(screen.getByRole('button',{name:'恢复任务'})).toBeTruthy();
 const input=screen.getByRole('textbox',{name:'消息'});await user.click(input);await user.keyboard('{Enter}');expect(onSend).not.toHaveBeenCalled();
 await user.click(screen.getByRole('button',{name:'任务 · 2'}));
 const tree=screen.getByRole('list',{name:'任务'});expect(within(tree).getAllByRole('listitem')).toHaveLength(2);
 expect(within(within(tree).getAllByRole('listitem')[0]).getByRole('list')).toBeTruthy();
 expect(screen.getByText('后台')).toBeTruthy();
 expect(screen.getAllByText('等待恢复')).toHaveLength(2);expect(screen.queryByText('执行中')).toBeNull();
 await user.click(screen.getByRole('button',{name:'恢复任务'}));expect(onResume).toHaveBeenCalledOnce();expect(onSend).not.toHaveBeenCalled();
 expect(input).toHaveProperty('value','Keep my draft');
});
it('offers pause while a run is active and keeps stop separate',async()=>{
 const onPause=vi.fn(),onStop=vi.fn();render(<ChatComposer {...base} running onPause={onPause} onStop={onStop}/>);
 await userEvent.click(screen.getByRole('button',{name:'暂停任务'}));expect(onPause).toHaveBeenCalledOnce();expect(onStop).not.toHaveBeenCalled();
});
