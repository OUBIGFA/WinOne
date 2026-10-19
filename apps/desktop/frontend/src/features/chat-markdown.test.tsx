// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,render,screen} from '@testing-library/react';
import {ChatMarkdown} from './chat-markdown';
const bridge=vi.hoisted(()=>({call:vi.fn()}));
vi.mock('../bridge',()=>({chatCall:bridge.call}));
afterEach(()=>{cleanup();vi.restoreAllMocks();vi.unstubAllGlobals();});

it('renders Chinese punctuation emphasis and preserves single-tilde ranges',()=>{
 const {container}=render(<ChatMarkdown text={'**小标题：**正文\n\n4~6 天后再等 4~6 天。'}/>);
 expect(container.querySelector('strong')?.textContent).toBe('小标题：');
 expect(container.querySelector('del')).toBeNull();
});

it('renders inline and display formulas while keeping fenced code literal',()=>{
 const {container}=render(<ChatMarkdown text={'公式 \\(x^2\\)\n\n$$\n\\frac{1}{2}\n$$\n\n```text\n$x^2$\n```'}/>);
 expect(container.querySelectorAll('.katex')).toHaveLength(2);
 expect(container.querySelector('pre code')?.textContent).toBe('$x^2$\n');
});

it('highlights fenced code and retains safe HTML images without executing HTML',()=>{
 const {container}=render(<ChatMarkdown text={'```typescript\nconst answer = 42;\n```\n\n<img src="https://example.test/diagram.png" alt="Diagram" />\n\n<script>alert(1)</script>'}/>);
 expect(container.querySelector('code .hljs-keyword')?.textContent).toBe('const');
 expect(screen.getByRole('img',{name:'Diagram'}).getAttribute('src')).toBe('https://example.test/diagram.png');
 expect(container.querySelector('script')).toBeNull();
});

it('flushes the complete answer as soon as streaming finishes',()=>{
 const {container,rerender}=render(<ChatMarkdown text="**Ready" isStreaming streamFadeKey="final-flush"/>);
 rerender(<ChatMarkdown text="**Ready.** The last tokens are present." isStreaming={false} streamFadeKey="final-flush"/>);
 expect(container.querySelector('strong')?.textContent).toBe('Ready.');
 expect(container.textContent).toContain('The last tokens are present.');
 expect(container.querySelector('.stream-word')).toBeNull();
});
// The real document editor needs layout and constructable stylesheets. Its
// default preview, local images, save and search are verified in the browser
// by build/test-chat-cindy-workbench.mjs instead of a simulated DOM editor.
