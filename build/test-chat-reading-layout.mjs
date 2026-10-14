// Render the real React message component and measure the complete spacing.
// This complements the native result host's reading_spacing_test.go.
import fs from 'node:fs/promises';
import path from 'node:path';
import {createRequire} from 'node:module';
import {execFileSync} from 'node:child_process';
const require=createRequire(import.meta.url);
const {chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE);
const [baseURL,outputArg]=process.argv.slice(2);
if(!baseURL||!outputArg)throw Error('Usage: test-chat-reading-layout.mjs <local Vite URL> <ignored output directory>');
const url=new URL(baseURL);if(url.hostname!=='127.0.0.1')throw Error('Use the isolated local UI server');
const output=path.resolve(outputArg);execFileSync('git',['check-ignore','--quiet','--no-index',path.join(output,'layout.probe')]);await fs.mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true,...(process.env.WINONE_BROWSER_EXE?{executablePath:process.env.WINONE_BROWSER_EXE}:{channel:process.env.WINONE_LAYOUT_BROWSER||'msedge'})});
const results=[];
try{
 const page=await browser.newPage({viewport:{width:900,height:900},reducedMotion:'reduce'}),errors=[];
 page.on('pageerror',error=>errors.push(error.message));
 await page.route('**/reading-layout-fixture',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><head><meta charset="utf-8"></head><body><div id="root"></div><script type="module">
 import React from '/node_modules/.vite/deps/react.js';import ReactDOM from '/node_modules/.vite/deps/react-dom_client.js';import ReactDOMMain from '/node_modules/.vite/deps/react-dom.js';
 import '/src/styles/shadcn.css';import '/src/style.css';import '/src/features/chat.css';import '/src/features/cindy/theme.css';import {ChatMessageView} from '/src/features/chat-messages.tsx';import {ChatDesktopSurface} from '/src/features/chat-desktop-kit.tsx';import {setApplicationLanguage} from '/src/i18n.ts';
 const root=ReactDOM.createRoot(document.getElementById('root'));
 window.renderReading=async (value)=>{await setApplicationLanguage(value.language);document.documentElement.classList.toggle('dark',value.dark);document.getElementById('root').style.width=value.width+'px';ReactDOMMain.flushSync(()=>root.render(React.createElement(ChatDesktopSurface,null,React.createElement(ChatMessageView,{key:value.id,conversationId:value.id,message:{id:value.id,role:'assistant',content:[{type:'thinking',thinking:'Thinking marker.\\nAnother thought line.'},{type:'text',text:(value.heading?'## ':'')+'Answer marker.\\n\\nAnother paragraph.'}]},canFork:false,onCopy:()=>{},onFork:()=>{}}))));};
 </script></body></html>`}));
 await page.goto(new URL('/reading-layout-fixture',url).href);await page.waitForFunction(()=>typeof window.renderReading==='function');
 for(const language of ['zh-CN','en-US'])for(const dark of [false,true])for(const width of [360,720])for(const expanded of [false,true])for(const heading of [false,true]){
  const value={id:`${language}-${dark}-${width}-${expanded}-${heading}`,language,dark,width,expanded,heading};
  await page.evaluate(value=>window.renderReading(value),value);
  if(expanded){await page.locator('[data-slot="cindy-thinking"]').click();await page.locator('.chat-thinking-group .select-text p').waitFor({state:'visible'});}
  await page.evaluate(async()=>{await document.fonts.ready;await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));});
  const bounds=await page.evaluate(expanded=>{
   const thought=document.querySelector(expanded?'.chat-thinking-group .select-text p':'.chat-thinking-group [data-slot="cindy-thinking"]');
   const answer=document.querySelector('.chat-answer-content > .chat-markdown > :first-child');
   if(!thought||!answer)throw Error('Missing rendered reading boundary');
   const a=thought.getBoundingClientRect(),b=answer.getBoundingClientRect();return {gap:b.top-a.bottom,upper:{y:a.y,height:a.height},lower:{y:b.y,height:b.height}};
  },expanded);
  results.push({...value,...bounds});
  if(Math.abs(bounds.gap-16)>.6)throw Error(`${value.id}: actual gap ${bounds.gap}px; expected 16px`);
 }
 if(errors.length)throw Error(errors.join('\n'));
 await fs.writeFile(path.join(output,'reading-layout.json'),JSON.stringify(results,null,2));
 console.log(`React reading layout: ${results.length} cases, total gap 16px, no browser errors`);
}catch(error){await fs.writeFile(path.join(output,'reading-layout-failed.json'),JSON.stringify({error:error.message,results},null,2));throw error;}
finally{await browser.close();}
