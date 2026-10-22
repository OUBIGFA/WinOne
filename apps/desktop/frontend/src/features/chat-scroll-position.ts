// The composer overlays the transcript: the visible bottom is its top edge,
// not the viewport's physical bottom. Measuring must never move the reader.
export function chatScrollMetrics(viewport:HTMLDivElement){
 const bounds=viewport.getBoundingClientRect();
 const main=viewport.closest('.chat-main'),footer=main?.querySelector<HTMLElement>('.chat-input-area');
 const spacer=viewport.querySelector<HTMLElement>('.chat-transcript-end-space');
 const origin=bounds.top;
 // Include the existing 32px fade so the last line is fully legible, not faded.
 const bottom=footer&&bounds.height>0?Math.max(origin,Math.min(bounds.bottom,footer.getBoundingClientRect().top-32)):origin+viewport.clientHeight;
 const usableHeight=Math.max(0,bottom-origin);
 const bottomTop=spacer&&footer&&bounds.height>0?Math.max(0,viewport.scrollTop+spacer.getBoundingClientRect().top-bottom):Math.max(0,viewport.scrollHeight-viewport.clientHeight);
 return {main,footer,spacer,origin,bottom,usableHeight,bottomTop,padding:Math.max(0,viewport.clientHeight-usableHeight)};
}

export function scrollChatToBottom(viewport:HTMLDivElement,behavior:ScrollBehavior='auto'){
 const {spacer,bottomTop,padding}=chatScrollMetrics(viewport);
 // Reconcile the spacer only for an explicit jump or live follow. Resizing
 // alone must not shorten the scroll extent and clamp the current position.
 if(spacer)spacer.style.height=padding+'px';
 // Short threads must not scroll below their reconciled content extent.
 const top=Math.min(bottomTop,Math.max(0,viewport.scrollHeight-viewport.clientHeight));
 if(behavior==='auto')viewport.scrollTop=top;
 else viewport.scrollTo({top,behavior});
}
