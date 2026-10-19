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
 fireEvent.click(screen.getByRole('button',{name:'返回底部'}));expect(viewport.scrollTop).toBe(1000);
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
