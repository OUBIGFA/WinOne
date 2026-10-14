// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen} from '@testing-library/react';
import {ChatMessageView} from './chat-messages';

afterEach(()=>{cleanup();vi.restoreAllMocks();});
it('renders the same Markdown structure in user and assistant messages',()=>{
 const text='## Heading\n\n**Strong** and `inline`\n\n- One\n- Two';
 const props={canFork:false,onCopy:()=>{},onFork:()=>{}};
 const {container}=render(<><ChatMessageView {...props} message={{role:'user',content:text}}/><ChatMessageView {...props} message={{role:'assistant',content:text}}/></>);
 expect(screen.getAllByRole('heading',{name:'Heading',level:2})).toHaveLength(2);
 expect(container.querySelectorAll('.chat-user-text strong')).toHaveLength(1);
 expect(container.querySelectorAll('.chat-user-text li')).toHaveLength(2);
});
it('collapses long user text without truncating the copied message',()=>{
 vi.spyOn(HTMLElement.prototype,'scrollHeight','get').mockReturnValue(480);
 const text=Array.from({length:20},(_,i)=>`Line ${i+1}`).join('\n'),copy=vi.fn();
 render(<ChatMessageView message={{id:'long-user',role:'user',content:text}} canFork={false} onCopy={copy} onFork={()=>{}}/>);
 const toggle=screen.getByRole('button',{name:'展开全文'});
 expect(toggle.getAttribute('aria-expanded')).toBe('false');
 fireEvent.click(toggle);
 expect(screen.getByRole('button',{name:'收起内容'}).getAttribute('aria-expanded')).toBe('true');
 fireEvent.click(screen.getByRole('button',{name:'复制'}));expect(copy).toHaveBeenCalledWith(text);
});
