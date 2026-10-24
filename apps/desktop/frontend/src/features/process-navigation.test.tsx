// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {act,cleanup,fireEvent,render,screen} from '@testing-library/react';
import {ProcessBody,ProcessCollapseAction,ProcessNavigationProvider} from './process-navigation';
afterEach(()=>{cleanup();vi.restoreAllMocks();vi.unstubAllGlobals();});
it('offers collapse as soon as the work is long, including at its natural bottom without scrolling',()=>{
 let height=300,bodyTop=150,bodyBottom=450,footerTop=506;
 const observers:Array<()=>void>=[];
 vi.stubGlobal('ResizeObserver',class{constructor(callback:()=>void){observers.push(callback);}observe(){}disconnect(){}});
 vi.spyOn(HTMLElement.prototype,'scrollHeight','get').mockImplementation(function(this:HTMLElement){return this.classList.contains('process-body')?height:600;});
 vi.spyOn(HTMLElement.prototype,'getBoundingClientRect').mockImplementation(function(this:HTMLElement){
  if(this.classList.contains('process-body'))return {top:bodyTop,bottom:bodyBottom,right:600,height} as DOMRect;
  if(this.classList.contains('chat-input-area'))return {top:footerTop} as DOMRect;
  return {top:100,bottom:700,right:620,height:600,left:0} as DOMRect;
 });
 const collapse=vi.fn();
 render(<main className="chat-main"><ProcessNavigationProvider><div className="chat-reading-pane"><div className="chat-transcript-viewport"><ProcessBody blockKey="work" onCollapse={collapse}>Work</ProcessBody></div><ProcessCollapseAction/></div><footer className="chat-input-area"/></ProcessNavigationProvider></main>);
 expect(screen.queryByRole('button',{name:'收起思考过程'})).toBeNull();
 // Streaming grows the body while the reader remains at its natural bottom.
 height=600;bodyTop=-100;bodyBottom=500;act(()=>observers.forEach(update=>update()));
 const button=screen.getByRole('button',{name:'收起思考过程'});
 expect(button.style.top).toBe('370px');
 fireEvent.click(button);expect(collapse).toHaveBeenCalledTimes(1);
 height=1000;bodyBottom=900;act(()=>observers.forEach(update=>update()));
 expect(button.style.top).toBe('570px');
 footerTop=340;act(()=>observers.forEach(update=>update()));
 expect(button.style.top).toBe('570px');
 bodyTop=750;bodyBottom=1350;act(()=>observers.forEach(update=>update()));
 expect(screen.queryByRole('button',{name:'收起思考过程'})).toBeNull();
});
