// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {ChatMessageView} from './chat-messages';
import {ChatReviewView} from './chat-review';
import {ChatTurnChanges} from './chat-turn-changes';
import {i18n} from '../i18n';

// The native Git backend is the external boundary; render the actual kit controls.
const bridge=vi.hoisted(()=>({call:vi.fn()}));
vi.mock('../bridge',()=>({chatCall:bridge.call}));
const messageProps={canFork:false,onCopy:()=>{},onFork:()=>{}};
beforeEach(async()=>{await i18n.changeLanguage('en-US');});
afterEach(()=>{cleanup();vi.clearAllMocks();vi.unstubAllGlobals();});

it.each([
 {role:'user',content:'Please try again'},
 {role:'user',content:'',attachments:[{name:'notes.txt',path:'C:/isolated/notes.txt',size:12,mimeType:'text/plain'}]},
 {role:'assistant',content:'',errorMessage:'The service stopped'},
])('retries $role messages even when only attachments or errors remain',message=>{
 const retry=vi.fn();render(<ChatMessageView {...messageProps} message={message} onRetry={retry}/>);
 fireEvent.click(screen.getByRole('button',{name:'Retry'}));expect(retry).toHaveBeenCalledOnce();
});

it('does not create a second file-change surface inside assistant messages',()=>{
 render(<ChatMessageView {...messageProps} message={{role:'assistant',content:'Updated the file.'}}/>);
 expect(screen.queryByRole('button',{name:/View file changes|chat.reviewViewChanges/})).toBeNull();
});

it('shows the current turn file rows and opens the selected file diff',async()=>{
 const patch='--- a/report.md\n+++ b/report.md\n@@ -1 +1,2 @@\n-old\n+new\n+detail\n';
 bridge.call.mockImplementation(async(method:string)=>method==='review.turn'?{snapshot:{root:'C:/isolated',files:[{path:'report.md',status:'M'},{path:'extra.ts',status:'A'}]},patches:{'report.md':patch,'extra.ts':'--- /dev/null\n+++ b/extra.ts\n@@ -0,0 +1 @@\n+export {};\n'}}:[]);
 const open=vi.fn();render(<><ChatMessageView {...messageProps} conversationId="isolated" message={{role:'assistant',content:'Updated the files.'}}/><ChatTurnChanges id="isolated" turnId="turn-one" version="v1" onReview={open}/></>);
 const row=await screen.findByRole('button',{name:/report.md/});fireEvent.click(row);
 expect(open).toHaveBeenCalledWith('report.md');expect(screen.getByText('2 files changed')).toBeTruthy();
 expect(screen.getAllByText('+3').length).toBeGreaterThan(0);
});

it('routes a Markdown file reference and its line to the in-app preview',async()=>{
 bridge.call.mockImplementation(async(_method,{paths})=>({files:paths.map((path:string)=>({requested:path,file:{path,name:'example.ts',directory:false,size:10,modified:0}}))}));
 const open=vi.fn();render(<ChatMessageView {...messageProps} conversationId="isolated" message={{role:'assistant',content:'See [source](src/example.ts:42).'}} onOpenFile={open}/>);
 fireEvent.click(await screen.findByRole('button',{name:'source'}));await waitFor(()=>expect(open).toHaveBeenCalledWith('src/example.ts',42));
});

it('retains historical provider and model names when current services no longer contain them',()=>{
 render(<ChatMessageView {...messageProps} providers={[]} message={{role:'assistant',content:'Saved answer',provider:'removed',model:'retired',providerName:'Original service',modelName:'Historical model'}}/>);
 expect(screen.getByText('Original service · Historical model')).toBeTruthy();
});

it('keeps failed-answer diagnostics behind the same friendly error presentation',()=>{
 const diagnostic='The socket connection was closed unexpectedly.';
 render(<ChatMessageView {...messageProps} message={{role:'assistant',content:'',errorMessage:diagnostic}}/>);
 expect(screen.getByRole('alert').textContent).toContain('The connection was interrupted');
 expect(screen.getByText(diagnostic).closest('details')?.open).toBe(false);
});

it('does not report no changes when the Git snapshot could not be read',async()=>{
 bridge.call.mockImplementation(async(method:string)=>{if(method==='review.snapshot')throw Error('Git directory is unavailable');return [];});
 render(<ChatReviewView sessionId="isolated" onOpenFile={()=>{}}/>);
 expect(await screen.findByRole('alert')).toHaveProperty('textContent',expect.stringContaining('Git directory is unavailable'));
 await waitFor(()=>expect(screen.queryByText('No file changes')).toBeNull());
});

it('retains staging, hunk folding, open-file and copy-patch actions',async()=>{
 const patch='diff --git a/example.ts b/example.ts\n--- a/example.ts\n+++ b/example.ts\n@@ -1 +1 @@\n-old\n+new\n';
 bridge.call.mockImplementation(async(method:string)=>method==='review.snapshot'?{root:'C:/isolated',branch:'main',scope:'working',base:'',revision:'rev-1',files:[{path:'example.ts',status:'M'}]}:method==='review.diff.preview'?{url:'http://127.0.0.1/preview/example.ts'}:[]);
 // The Cindy review host reads patches through review.diff.preview and a
 // preview URL; serve the patch body over fetch instead of a direct reply.
 vi.stubGlobal('fetch',vi.fn(async()=>({ok:true,text:async()=>patch})));
 const open=vi.fn();render(<ChatReviewView sessionId="isolated" onOpenFile={open}/>);
 await screen.findByText('new');fireEvent.click(screen.getByRole('button',{name:'Collapse all diffs'}));expect(screen.queryByText('new')).toBeNull();
 fireEvent.click(await screen.findByRole('button',{name:'Expand all diffs'}));expect(screen.getByText('new')).toBeTruthy();
 fireEvent.click(screen.getAllByRole('button',{name:'Open file'})[0]);expect(open).toHaveBeenCalledWith('C:/isolated/example.ts',undefined);
 fireEvent.keyDown(screen.getByRole('button',{name:'View options'}),{key:'ArrowDown'});
 expect(await screen.findByRole('menuitem',{name:/copy git apply/i})).toBeTruthy();
 fireEvent.keyDown(document.body,{key:'Escape'});
 fireEvent.click(screen.getByRole('button',{name:'Stage'}));
 await waitFor(()=>expect(bridge.call).toHaveBeenCalledWith('review.mutate',{sessionId:'isolated',mutation:{action:'stage',paths:['example.ts'],revision:'rev-1'}}));
});
