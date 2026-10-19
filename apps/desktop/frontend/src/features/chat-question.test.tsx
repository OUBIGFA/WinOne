// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,render,screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {ChatQuestion} from './chat-question';
afterEach(cleanup);
const request={id:'q1',tool:'ask_user_question',args:{},kind:'question' as const,questions:[{question:'选择方向',options:['方案 A','方案 B']},{question:'保留哪些内容？'}]};
it('requires actual answers, supports choices and custom text, and submits once',async()=>{
 const answer=vi.fn(),cancel=vi.fn(),user=userEvent.setup();
 render(<ChatQuestion request={request} busy={false} onAnswer={answer} onCancel={cancel} onStop={()=>{}}/>);
 expect((screen.getByRole('button',{name:'提交答案'}) as HTMLButtonElement).disabled).toBe(true);
 expect(screen.getByRole('button',{name:'方案 A'}).getAttribute('aria-pressed')).toBe('false');
 await user.click(screen.getByRole('button',{name:'方案 B'}));
 await user.type(screen.getByRole('textbox',{name:'保留哪些内容？'}),'保留原配置');
 await user.click(screen.getByRole('button',{name:'提交答案'}));
 expect(answer).toHaveBeenCalledExactlyOnceWith(['方案 B','保留原配置']);expect(cancel).not.toHaveBeenCalled();
});
it('cancels without submitting a default answer and blocks edits while responding',async()=>{
 const answer=vi.fn(),cancel=vi.fn(),stop=vi.fn(),user=userEvent.setup();
 const view=render(<ChatQuestion request={request} busy={false} onAnswer={answer} onCancel={cancel} onStop={stop}/>);
 await user.click(screen.getByRole('button',{name:'取消提问'}));expect(cancel).toHaveBeenCalledOnce();expect(answer).not.toHaveBeenCalled();
 view.rerender(<ChatQuestion request={request} busy onAnswer={answer} onCancel={cancel} onStop={stop}/>);
 expect((screen.getByRole('textbox',{name:'选择方向'}) as HTMLTextAreaElement).disabled).toBe(true);
 expect((screen.getByRole('button',{name:'方案 A'}) as HTMLButtonElement).disabled).toBe(true);
});
