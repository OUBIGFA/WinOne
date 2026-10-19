// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {act,cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {ChatReviewView} from './chat-review';
import {ReviewFileTreeSidebar} from './cindy/review/ReviewTabBody';
import {parseGitDiff} from './cindy/review/diffParser';
import {i18n} from '../i18n';
const bridge=vi.hoisted(()=>({call:vi.fn()}));
vi.mock('../bridge',()=>({chatCall:bridge.call}));
const patch='--- a/note.txt\n+++ b/note.txt\n@@ -1 +1 @@\n-old\n+new\n';
const metadata={repository:true,root:'C:/chat',branch:'main',headOid:'1234567890123456789012345678901234567890',detached:false,unborn:false,upstream:'',ahead:0,behind:0,candidates:[],inProgress:[]};
function fixture(recorded=false){bridge.call.mockImplementation(async(method:string,params:any)=>{
 if(method==='review.snapshot')return {root:'C:/chat',branch:'main',source:recorded?'session':undefined,scope:params.scope||'working',base:'',revision:'one',files:params.scope==='staged'?[]:[{path:recorded?'D:/output/note.txt':'note.txt',status:'M',additions:1,deletions:1,size:12}]};
 if(method==='review.metadata')return metadata;
 if(method==='review.commits'||method==='review.branches')return [];
 if(method==='review.diff.preview')return {url:'https://review.test/note.diff'};
 if(method==='review.mutate')return {};
 throw Error('Unexpected review request: '+method);
});}
beforeEach(async()=>{localStorage.clear();await i18n.changeLanguage('en-US');fixture();vi.stubGlobal('fetch',vi.fn(async()=>new Response(patch,{headers:{'Content-Type':'text/plain'}})));});
afterEach(()=>{cleanup();vi.clearAllMocks();vi.unstubAllGlobals();});

it('shows recorded changes and opens their real files without Git mutation controls',async()=>{
 fixture(true);const open=vi.fn();render(<ChatReviewView sessionId="ordinary" onOpenFile={open}/>);
 await screen.findByText('new');
 expect(screen.queryByRole('button',{name:'Stage'})).toBeNull();
 expect(screen.queryByRole('button',{name:'Push'})).toBeNull();
 expect(screen.getByText(/latest recorded file operation/i)).toBeTruthy();
 fireEvent.click(screen.getAllByRole('button',{name:'Open file'})[0]);
 expect(open).toHaveBeenCalledWith('D:/output/note.txt',undefined);
});

it('filters the Cindy file tree without keeping a nonmatching navigation target',()=>{
 const diff=parseGitDiff(patch,{source:'unstaged',pathHint:'note.txt'}),select=vi.fn();
 render(<ReviewFileTreeSidebar diffs={[diff]} activeFileId={diff.id} onSelectFile={select}/>);
 fireEvent.change(screen.getByRole('textbox',{name:'Filter files'}),{target:{value:'absent'}});
 expect(screen.queryByText('note.txt')).toBeNull();expect(screen.getByText('No matching files')).toBeTruthy();expect(select).not.toHaveBeenCalled();
});

it('cannot stage a file until its displayed diff finishes loading',async()=>{
 let resolveDiff!:(response:Response)=>void;
 vi.stubGlobal('fetch',vi.fn(()=>new Promise<Response>(resolve=>{resolveDiff=resolve;})));
 render(<ChatReviewView sessionId="loading" onOpenFile={()=>{}}/>);
 await waitFor(()=>expect(bridge.call).toHaveBeenCalledWith('review.diff.preview',expect.objectContaining({path:'note.txt',revision:'one'})));
 expect(screen.queryAllByRole('button',{name:/^Stage/}).some(node=>!(node as HTMLButtonElement).disabled)).toBe(false);
 await act(async()=>resolveDiff(new Response(patch)));
 await screen.findByText('new');
 expect((screen.getByRole('button',{name:'Stage'}) as HTMLButtonElement).disabled).toBe(false);
});

it('requires confirmation before reverting a hunk and returns the displayed revision',async()=>{
 render(<ChatReviewView sessionId="hunk" onOpenFile={()=>{}}/>);
 await screen.findByText('new');fireEvent.click(screen.getByRole('button',{name:'Revert hunk'}));
 expect(bridge.call.mock.calls.some(([method])=>method==='review.mutate')).toBe(false);
 fireEvent.click(await screen.findByRole('button',{name:'Revert'}));
 await waitFor(()=>expect(bridge.call).toHaveBeenCalledWith('review.mutate',expect.objectContaining({mutation:{action:'revert',paths:['note.txt'],hunk:0,revision:'one'}})));
});
