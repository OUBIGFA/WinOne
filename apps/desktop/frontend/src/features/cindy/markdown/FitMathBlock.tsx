import {useLayoutEffect,useRef,type ReactNode} from 'react';

/** Display math is atomic, unlike prose/code. Fit its intrinsic box instead of
 * clipping symbols or making a horizontal scroller; zoom also scales layout
 * height. Source, selection, copy-as-image and the accessible MathML stay intact.
 */
export function FitMathBlock({children}:{children:ReactNode}){
 const frame=useRef<HTMLDivElement>(null),content=useRef<HTMLDivElement>(null);
 useLayoutEffect(()=>{
  const parent=frame.current,node=content.current;if(!parent||!node)return;
  let alive=true,pending=0;
  const fit=()=>{pending=0;if(!alive||parent.clientWidth<=0||node.offsetWidth<=0)return;const scale=String(Math.min(1,Math.max(1,parent.clientWidth-1)/node.offsetWidth));if(node.style.zoom!==scale)node.style.zoom=scale;};
  // Writing zoom inside a ResizeObserver delivery changes both observed boxes
  // and triggers the browser's undelivered-notifications loop warning.
  const schedule=()=>{if(alive&&!pending)pending=requestAnimationFrame(fit);};
  fit();const observer=typeof ResizeObserver==='undefined'?null:new ResizeObserver(schedule);observer?.observe(parent);observer?.observe(node);
  void document.fonts?.ready.then(schedule);
  return()=>{alive=false;observer?.disconnect();cancelAnimationFrame(pending);};
 },[children]);
 return <div ref={frame} className="chat-math-fit"><div ref={content} className="chat-math-intrinsic">{children}</div></div>;
}
