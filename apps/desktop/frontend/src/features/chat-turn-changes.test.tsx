// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,render,screen,waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {ChatTurnChanges} from './chat-turn-changes';
const bridge=vi.hoisted(()=>({call:vi.fn()}));
vi.mock('../bridge',()=>({chatCall:bridge.call}));
afterEach(()=>{cleanup();vi.restoreAllMocks();});

const patch='diff --git a/report.md b/report.md\n--- a/report.md\n+++ b/report.md\n@@ -1 +1 @@\n-old\n+new\n';
const review={snapshot:{isReversible:true,workspaceState:'applied',root:'C:/work',revision:'rev-1',files:[{path:'report.md',status:'M'},{path:'extra.md',status:'A'}]},patches:{'report.md':patch,'extra.md':patch}};

it('undoes a turn through review.turn.apply and offers reapply afterwards',async()=>{
 bridge.call.mockImplementation(async(method)=>method==='review.turn'?review:{...review.snapshot,workspaceState:'undone'});
 const onReview=vi.fn();
 render(<ChatTurnChanges id="chat-1" turnId="turn-1" version="v1" onReview={onReview}/>);
 await screen.findByRole('button',{name:'撤销本轮修改'});
 await userEvent.click(screen.getByRole('button',{name:'撤销本轮修改'}));
 expect(bridge.call).toHaveBeenCalledWith('review.turn.apply',{sessionId:'chat-1',turnId:'turn-1',mutation:{revision:'rev-1',action:'undo'}});
 await screen.findByRole('button',{name:'重新应用'});
});

it('reapplies an undone turn through review.turn.apply',async()=>{
 bridge.call.mockImplementation(async(method)=>method==='review.turn'?{...review,snapshot:{...review.snapshot,workspaceState:'undone'}}:{...review.snapshot,workspaceState:'applied'});
 render(<ChatTurnChanges id="chat-1" turnId="turn-1" version="v1" onReview={()=>{}}/>);
 await userEvent.click(await screen.findByRole('button',{name:'重新应用'}));
 expect(bridge.call).toHaveBeenCalledWith('review.turn.apply',{sessionId:'chat-1',turnId:'turn-1',mutation:{revision:'rev-1',action:'reapply'}});
 await waitFor(()=>expect(screen.getByRole('button',{name:'撤销本轮修改'})).toBeTruthy());
});

it('keeps a conflicted turn visible but refuses to overwrite the later edit',async()=>{
 bridge.call.mockImplementation(async()=>({...review,snapshot:{...review.snapshot,workspaceState:'conflict'}}));
 render(<ChatTurnChanges id="chat-1" turnId="turn-1" version="v1" onReview={()=>{}}/>);
 const button=await screen.findByRole('button',{name:'撤销本轮修改'});
 expect(button).toHaveProperty('disabled',true);
 expect(screen.getByRole('status',{name:undefined})).toBeTruthy();
 expect(screen.getByText('文件在本轮后已被修改。请先核对当前内容。')).toBeTruthy();
});

it('reports a partial capture instead of offering an undo that could lose work',async()=>{
 bridge.call.mockImplementation(async()=>({...review,snapshot:{...review.snapshot,isReversible:false}}));
 render(<ChatTurnChanges id="chat-1" turnId="turn-1" version="v1" onReview={()=>{}}/>);
 await screen.findByText('修改了 2 个文件');
 expect(screen.queryByRole('button',{name:'撤销本轮修改'})).toBeNull();
 expect(screen.getByText('部分文件没有完整的修改前后记录，无法撤销整轮修改。')).toBeTruthy();
});

it('opens a file diff for the clicked row and never shows a turn without files',async()=>{
 bridge.call.mockImplementation(async()=>review);
 const onReview=vi.fn();
 const {container}=render(<ChatTurnChanges id="chat-1" turnId="turn-1" version="v1" onReview={onReview}/>);
 const rows=await screen.findAllByRole('button',{name:/report\.md/});
 await userEvent.click(rows[0]);
 expect(onReview).toHaveBeenCalledWith('report.md');
 expect(container.querySelectorAll('.chat-turn-changes').length).toBe(1);
 bridge.call.mockImplementation(async()=>({...review,snapshot:{...review.snapshot,files:[]}}));
 cleanup();
 render(<ChatTurnChanges id="chat-1" turnId="turn-1" version="v1" onReview={onReview}/>);
 await waitFor(()=>expect(bridge.call).toHaveBeenCalledWith('review.turn',{sessionId:'chat-1',turnId:'turn-1'}));
});
