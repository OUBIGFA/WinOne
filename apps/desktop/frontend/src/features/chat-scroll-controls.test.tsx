// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {act,cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {useRef} from 'react';
import {ChatScrollControls} from './chat-scroll-controls';

let resize:()=>void;
beforeEach(()=>{
 vi.stubGlobal('ResizeObserver',class {constructor(callback:()=>void){resize=callback;}observe(){}disconnect(){}});
 vi.stubGlobal('requestAnimationFrame',(callback:FrameRequestCallback)=>setTimeout(()=>callback(0),0));
 vi.stubGlobal('cancelAnimationFrame',clearTimeout);vi.stubGlobal('matchMedia',()=>({matches:true}));
});
afterEach(()=>{cleanup();vi.unstubAllGlobals();});
function Fixture(){const viewport=useRef<HTMLDivElement>(null);return <><div ref={viewport} data-testid="viewport"><div>{[0,1,2].map(index=><div key={index} data-chat-turn={index}>Round {index}</div>)}</div></div><ChatScrollControls viewportRef={viewport} conversationId="chat"/></>;}

function OverlayFixture(){const viewport=useRef<HTMLDivElement>(null);return <main className="chat-main"><header className="chat-header"/><div ref={viewport} data-testid="viewport"><div><div data-chat-turn="one">First round</div><div data-chat-turn="two">Last round</div><div className="chat-transcript-end-space" style={{height:320}}/></div></div><footer className="chat-input-area" data-testid="footer"/><ChatScrollControls viewportRef={viewport} conversationId="overlay"/></main>;}

it('uses the current input edge for bottom jumps without moving the transcript on resize',async()=>{
 render(<OverlayFixture/>);const viewport=screen.getByTestId('viewport'),footer=screen.getByTestId('footer'),spacer=viewport.querySelector<HTMLElement>('.chat-transcript-end-space')!;
 let footerTop=620;
 Object.defineProperties(viewport,{clientHeight:{value:800},scrollHeight:{get:()=>1400+parseFloat(spacer.style.height)}});
 viewport.getBoundingClientRect=()=>({top:100,bottom:900,height:800} as DOMRect);
 footer.getBoundingClientRect=()=>({top:footerTop} as DOMRect);
 spacer.getBoundingClientRect=()=>({top:1500-viewport.scrollTop} as DOMRect);
 viewport.scrollTop=500;act(()=>resize());
 const bottom=()=>screen.getByRole('button',{name:'返回底部'});
 fireEvent.click(bottom());expect(viewport.scrollTop).toBe(912);expect(spacer.style.height).toBe('312px');fireEvent.scroll(viewport);
 await waitFor(()=>expect(bottom()).toHaveProperty('disabled',true));
 // Growing the overlay changes the logical bottom, not scrollTop or the
 // physical scroll extent. The button must become usable again.
 footerTop=380;act(()=>resize());expect(viewport.scrollTop).toBe(912);expect(spacer.style.height).toBe('312px');
 expect(bottom()).toHaveProperty('disabled',false);
 expect(screen.getByRole('navigation').style.top).toBe('130px');
 fireEvent.click(bottom());expect(viewport.scrollTop).toBe(1152);expect(spacer.style.height).toBe('552px');fireEvent.scroll(viewport);
 await waitFor(()=>expect(bottom()).toHaveProperty('disabled',true));
 // Shrinking at the old bottom must not clamp the reader backwards. A
 // deliberate bottom jump removes that old allowance and realigns content.
 footerTop=720;act(()=>resize());expect(viewport.scrollTop).toBe(1152);expect(spacer.style.height).toBe('552px');
 expect(bottom()).toHaveProperty('disabled',false);fireEvent.click(bottom());
 expect(viewport.scrollTop).toBe(812);expect(spacer.style.height).toBe('212px');
 footerTop=220;act(()=>resize());expect(screen.getByRole('navigation').parentElement?.classList.contains('chat-header')).toBe(true);
});

it('keeps navigation still when the composer has horizontal clearance',()=>{
 render(<OverlayFixture/>);const viewport=screen.getByTestId('viewport'),footer=screen.getByTestId('footer');
 Object.defineProperties(viewport,{clientHeight:{value:800},scrollHeight:{value:1600}});
 viewport.getBoundingClientRect=()=>({top:100,bottom:900,height:800,right:1200} as DOMRect);
 const editor=document.createElement('div');editor.className='chat-input-content';footer.append(editor);
 let top=700;footer.getBoundingClientRect=()=>({top} as DOMRect);
 editor.getBoundingClientRect=()=>({top,right:1100,left:100,bottom:900} as DOMRect);
 act(()=>resize());const original=screen.getByRole('navigation').style.top;
 top=350;act(()=>resize());expect(screen.getByRole('navigation').style.top).toBe(original);
 expect(screen.getByRole('navigation').parentElement?.classList.contains('chat-header')).toBe(false);
});

it('moves between user rounds and both transcript boundaries',async()=>{
 render(<Fixture/>);const viewport=screen.getByTestId('viewport');
 Object.defineProperties(viewport,{clientHeight:{value:200},scrollHeight:{value:1200}});
 viewport.getBoundingClientRect=()=>({top:100} as DOMRect);
 Array.from(viewport.querySelectorAll<HTMLElement>('[data-chat-turn]')).forEach((node,index)=>{node.getBoundingClientRect=()=>({top:112+index*400-viewport.scrollTop} as DOMRect);});
 Object.defineProperty(viewport,'scrollTo',{value:({top}:ScrollToOptions)=>{viewport.scrollTop=Math.min(1000,top??0);fireEvent.scroll(viewport);}});
 act(()=>resize());
 await waitFor(()=>expect(screen.getByRole('button',{name:'下一轮对话'})).toHaveProperty('disabled',false));
 expect(screen.getByRole('button',{name:'上一轮对话'})).toHaveProperty('disabled',true);
 fireEvent.click(screen.getByRole('button',{name:'下一轮对话'}));expect(viewport.scrollTop).toBe(400);
 await waitFor(()=>expect(screen.getByRole('button',{name:'上一轮对话'})).toHaveProperty('disabled',false));
 fireEvent.click(screen.getByRole('button',{name:'下一轮对话'}));expect(viewport.scrollTop).toBe(800);
 await waitFor(()=>expect(screen.getByRole('button',{name:'下一轮对话'})).toHaveProperty('disabled',true));
 fireEvent.click(screen.getByRole('button',{name:'上一轮对话'}));expect(viewport.scrollTop).toBe(400);
 fireEvent.click(screen.getByRole('button',{name:'返回底部'}));expect(viewport.scrollTop).toBe(1000);fireEvent.scroll(viewport);
 await waitFor(()=>expect(screen.getByRole('button',{name:'返回底部'})).toHaveProperty('disabled',true));
 fireEvent.click(screen.getByRole('button',{name:'返回顶部'}));expect(viewport.scrollTop).toBe(0);
});

it('skips the first round at the top inset and recognizes the last round at the scroll limit',async()=>{
 render(<Fixture/>);const viewport=screen.getByTestId('viewport');
 Object.defineProperties(viewport,{clientHeight:{value:800},scrollHeight:{value:1000}});
 viewport.getBoundingClientRect=()=>({top:100} as DOMRect);
 Array.from(viewport.querySelectorAll<HTMLElement>('[data-chat-turn]')).forEach((node,index)=>{node.getBoundingClientRect=()=>({top:148+index*400-viewport.scrollTop} as DOMRect);});
 Object.defineProperty(viewport,'scrollTo',{value:({top}:ScrollToOptions)=>{viewport.scrollTop=Math.min(200,top??0);fireEvent.scroll(viewport);}});
 act(()=>resize());await waitFor(()=>expect(screen.getByRole('button',{name:'下一轮对话'})).toHaveProperty('disabled',false));
 fireEvent.click(screen.getByRole('button',{name:'下一轮对话'}));expect(viewport.scrollTop).toBe(200);
 await waitFor(()=>expect(screen.getByRole('button',{name:'上一轮对话'})).toHaveProperty('disabled',false));
 expect(screen.getByRole('button',{name:'下一轮对话'})).toHaveProperty('disabled',true);
});
