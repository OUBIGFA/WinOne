// Measure actual upstream and Host renderers: no source-text assertions.
import {createRequire} from 'node:module';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {execFileSync} from 'node:child_process';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [base,folder]=process.argv.slice(2),output=resolve(folder);
assert.equal(new URL(base).hostname,'127.0.0.1');
execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,...(process.env.WINONE_BROWSER_EXE?{executablePath:process.env.WINONE_BROWSER_EXE}:{channel:'msedge'})});
const results=[],failures=[];
try{
 const page=await browser.newPage({viewport:{width:1000,height:900},reducedMotion:'reduce'}),errors=[];
 page.on('pageerror',error=>errors.push(error.message));
 await page.route('**/rendering-layout-fixture',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><head><meta charset="utf-8"></head><body><div id="root"></div><script type="module">
 import React from '/node_modules/.vite/deps/react.js';import ReactDOM from '/node_modules/.vite/deps/react-dom_client.js';import ReactDOMMain from '/node_modules/.vite/deps/react-dom.js';
 import '/src/styles/shadcn.css';import '/src/style.css';import '/src/features/chat.css';import '/src/features/cindy/theme.css';import '/src/features/chat-side-panel.css';
 import {ChatMessageView} from '/src/features/chat-messages.tsx';import {ChatDesktopSurface} from '/src/features/chat-desktop-kit.tsx';import {ChatReviewView} from '/src/features/chat-review.tsx';import {setApplicationLanguage} from '/src/i18n.ts';
 import {AIResponse,ConversationMessage} from '/node_modules/.vite/deps/chatgpt-desktop-kit.js';
 const e=React.createElement,root=ReactDOM.createRoot(document.getElementById('root'));window.copied='';Object.defineProperty(navigator,'clipboard',{value:{writeText:async text=>{window.copied=text;}}});
 window.go={main:{App:{Chat:async(method,payload)=>JSON.stringify(method==='review.snapshot'?{root:'C:/isolated',branch:'main',scope:'working',base:'',revision:'r1',files:window.reviewFiles||[]}:method==='review.diff'?window.reviewPatch||'':[])}}};
 window.renderFixture=async value=>{await setApplicationLanguage(value.language||'en-US');document.documentElement.classList.toggle('dark',!!value.dark);document.getElementById('root').style.width=value.width+'px';window.reviewFiles=value.files;window.reviewPatch=value.patch;ReactDOMMain.flushSync(()=>root.render(e(ChatDesktopSurface,null,value.review?e('div',{style:{height:'700px'}},e(ChatReviewView,{key:JSON.stringify(value),sessionId:'isolated',onOpenFile:()=>{}})):e('div',{className:'client-thread client-thread--chat'},e('section',{id:'host'},e(ChatMessageView,{key:JSON.stringify(value),message:{role:value.role||'assistant',content:value.text},canFork:false,onCopy:()=>{},onFork:()=>{}})),e('section',{id:'reference'},e(ConversationMessage,{role:'assistant'},e(AIResponse,{content:value.text,animate:false,responseId:'reference'})))))));};
 </script></body></html>`}));
 await page.goto(new URL('/rendering-layout-fixture',base).href);await page.waitForFunction(()=>typeof window.renderFixture==='function');
 const settle=()=>page.evaluate(async()=>{await document.fonts.ready;await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));});
 const long='LongToken0123456789'.repeat(28),code=`const longValue = "${long}";`;
 const rich=`# Main heading\n\nParagraph **strong** _emphasis_ and ~~deleted~~.\n\n## Second heading\n\n> Quoted paragraph.\n\n- First item\n- Second item\n  - Nested item\n\n1. Ordered item\n\n- [x] Done\n\nInline \`${long}\`\n\n[${long}](https://example.test/)\n\n\`\`\`typescript\n${code}\n\`\`\`\n\n| First | Second | Third |\n| --- | --- | --- |\n| ${long} | **All content** | ${long} |\n\n---\n\n![Sample](data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aHfoAAAAASUVORK5CYII=)`;
 for(const width of [320,360,720])for(const dark of [false,true])for(const role of ['assistant','user']){
  const id=`${width}-${dark}-${role}`;await page.evaluate(value=>window.renderFixture(value),{width,dark,role,text:rich});await settle();
  const expand=page.getByRole('button',{name:'Show full message'});if(await expand.count())await expand.click();
  const metrics=await page.evaluate(()=>{
   const host=document.querySelector('#host'),markdown=host.querySelector('.chat-markdown');
   const overflow=[host,...host.querySelectorAll('*')].filter(el=>el.clientWidth&&el.scrollWidth>el.clientWidth+1&&!el.closest('[aria-hidden="true"]')).map(el=>({tag:el.tagName,class:el.className,client:el.clientWidth,scroll:el.scrollWidth}));
   const properties=['fontFamily','fontSize','fontWeight','lineHeight','marginTop','marginBottom','paddingInlineStart'];
   const sample=el=>Object.fromEntries(properties.map(p=>[p,getComputedStyle(el)[p]]));
   return {overflow,typography:['p','h1','h2','li','blockquote','code'].map(selector=>({selector,host:sample(markdown.querySelector(selector)),reference:sample(document.querySelector('#reference .desktop-ai-markdown').querySelector(selector))})),text:markdown.textContent};
  });
  results.push({id,...metrics});if(metrics.overflow.length)failures.push({id,overflow:metrics.overflow});
  // Cindy 69661422 chatChrome / MarkdownRenderer now own the reading scale.
  // Keep the kit comparison for font family and validate the requested sizes.
  if(role==='assistant')for(const entry of metrics.typography)try{
   assert.equal(entry.host.fontFamily,entry.reference.fontFamily);
   assert.equal(entry.host.fontSize,{p:'15px',h1:'20px',h2:'18px',li:'15px',blockquote:'15px',code:'14px'}[entry.selector]);
   if(entry.selector==='p')assert.equal(entry.host.lineHeight,'24px');
   if(entry.selector==='h1'||entry.selector==='h2')assert.equal(entry.host.fontWeight,'500');
  }catch{failures.push({id,typography:entry});}
  assert(metrics.text.includes(code));
  await page.locator('#host .chat-user-text .desktop-markdown-copy, #host .chat-answer-content > .chat-markdown .desktop-markdown-copy').click();assert.equal(await page.evaluate(()=>window.copied),code+'\n');
 }
 for(const width of [320,720]){
  await page.evaluate(value=>window.renderFixture(value),{width,review:true});await page.getByText('No file changes').waitFor();await settle();
  const state=await page.evaluate(()=>{const el=document.querySelector('.chat-review-diff'),label=[...el.querySelectorAll('*')].find(el=>el.textContent==='No file changes'&&el.children.length===0),a=el.getBoundingClientRect(),b=label.getBoundingClientRect();return {left:b.left-a.left,right:a.right-b.right,top:b.top-a.top,overflow:[...document.querySelectorAll('.chat-review,.chat-review *')].filter(el=>el.clientWidth&&el.scrollWidth>el.clientWidth+1).map(el=>({tag:el.tagName,class:el.className,client:el.clientWidth,scroll:el.scrollWidth}))};});
  results.push({id:'review-empty-'+width,...state});if(state.left<20||state.right<20||state.top<20||state.overflow.length)failures.push({id:'review-empty-'+width,...state});
 }
 assert.deepEqual(errors,[]);await page.screenshot({path:join(output,'review-empty.png')});
 await writeFile(join(output,'rendering-layout.json'),JSON.stringify({results,failures},null,2));
 assert.deepEqual(failures,[]);console.log(`Chat rendering: ${results.length} layout cases, Cindy typography, no horizontal overflow, complete code copied.`);
}finally{await browser.close();}
