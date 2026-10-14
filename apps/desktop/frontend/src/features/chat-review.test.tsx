// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen} from '@testing-library/react';
import {ChatReviewView} from './chat-review';
import {i18n} from '../i18n';
const bridge=vi.hoisted(()=>({call:vi.fn()}));
vi.mock('../bridge',()=>({chatCall:bridge.call}));
beforeEach(async()=>{await i18n.changeLanguage('en-US');});
afterEach(()=>{cleanup();vi.clearAllMocks();});

it('shows recorded file edits in ordinary folders without offering Git mutations',async()=>{
 const patch='--- a/note.txt\n+++ b/note.txt\n@@ -1 +1 @@\n-old\n+new\n';
 bridge.call.mockImplementation(async(method:string)=>method==='review.snapshot'?{root:'C:/chat',source:'session',scope:'working',revision:'one',files:[{path:'D:/output/note.txt',status:'M'}]}:method==='review.diff'?patch:[]);
 const open=vi.fn();render(<ChatReviewView sessionId="ordinary" onOpenFile={open}/>);
 await screen.findByText('+new');
 expect(screen.queryByRole('button',{name:'Stage'})).toBeNull();
 expect(screen.queryByRole('button',{name:'Push'})).toBeNull();
 expect(screen.getByText(/latest recorded file operation/i)).toBeTruthy();
 fireEvent.click(screen.getAllByRole('button',{name:'Open file'})[0]);
 expect(open).toHaveBeenCalledWith('D:/output/note.txt',undefined);
});
