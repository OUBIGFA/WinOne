// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,fireEvent,render,screen,within} from '@testing-library/react';
import {AgentActionRow} from './cindy/AgentActionRow';
import {WorkProcess} from './cindy/WorkProcess';
import {toolFileContent,toolResultText} from './chat-tool-content';
import {__test_internals} from './cindy/useExpandedBlockMemory';

afterEach(()=>{cleanup();__test_internals.reset();vi.restoreAllMocks();});
const markdown='# 标准文档\n\n这是 **加粗内容**，不是 JSON。\n\n| 名称 | 说明 |\n| --- | --- |\n| WinOne | 正常表格 |\n\n```ts\nconst path = "C:\\\\notes";\n```\n\n- 保留换行\n- 保留列表';

it('renders the real live read payload as Markdown rather than a serialized transport envelope',async()=>{
 const result={content:[{type:'text',text:markdown}],details:{truncation:false}};
 render(<WorkProcess conversationId="preview" onCopy={()=>{}} group={{kind:'response',key:'turn',messages:[],running:true,hasTools:true,entries:[{kind:'tool',key:'read',tool:{id:'read',name:'read',args:{path:'README.md'},running:false,result}}]}}/>);
 fireEvent.click(screen.getByRole('button',{name:/README.md/}));
 const dialog=await screen.findByRole('dialog');
 expect(within(dialog).getByRole('heading',{name:'标准文档'})).toBeTruthy();
 expect(within(dialog).getByRole('table')).toBeTruthy();
 expect(within(dialog).getByRole('list').textContent).toContain('保留换行');
 expect(dialog.querySelector('.chat-tool-document')?.textContent).not.toContain('"type": "text"');
 fireEvent.click(within(dialog).getByRole('button',{name:'查看源码'}));
 expect(dialog.querySelector('.chat-tool-document pre')?.textContent).toBe(markdown);
 fireEvent.click(within(dialog).getByRole('button',{name:'查看预览'}));
 expect(within(dialog).getByRole('heading',{name:'标准文档'})).toBeTruthy();
});

it.each(['Read','Write'])('renders %s history as a document and keeps raw data secondary',async name=>{
 const input=name==='Write'?{path:'readme.markdown',content:markdown}:{path:'readme.markdown',offset:10,limit:20};
 render(<AgentActionRow message={{clientId:name,content:'',toolName:name,toolInput:input}} status="done" toolResult={name==='Read'?markdown:'Successfully wrote the file.'}/>);
 fireEvent.click(screen.getByRole('button',{name:/readme.markdown/}));
 const dialog=await screen.findByRole('dialog');
 expect(within(dialog).getByRole('heading',{name:'标准文档'})).toBeTruthy();
 const raw=dialog.querySelector<HTMLDetailsElement>('.chat-tool-raw');expect(raw?.open).toBe(false);
 fireEvent.click(within(dialog).getByText('原始工具数据'));
 expect(raw?.textContent).toContain('readme.markdown');
});

it('retains edit segments and never interprets JSON document strings as envelopes',()=>{
 const json='{"content":[{"type":"text","text":"literal\\nsequence"}]}';
 expect(toolResultText(json)).toBe(json);
 expect(toolFileContent('read',{},json)).toEqual({kind:'document',source:'read',text:json});
 expect(toolFileContent('edit',{edits:[{oldText:'# Before',newText:'# After'},{oldText:'old',newText:'new'}]},undefined)?.text).toBe('@@ 1 @@\n-# Before\n+# After\n@@ 2 @@\n-old\n+new');
});
