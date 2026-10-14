/** Copy locally in the WebView; never route clipboard actions through Chat RPC. */
export async function copyText(value:string):Promise<void>{
  // An available API rejecting a write is an actionable error. A legacy retry
  // would hide permission/focus failures from the caller's inline feedback.
  if (navigator.clipboard?.writeText) { await navigator.clipboard.writeText(value); return; }
  const focused=document.activeElement as HTMLElement|null;
  const area=document.createElement('textarea'); area.value=value; area.readOnly=true; area.style.position='fixed'; area.style.opacity='0';
  document.body.appendChild(area); area.select();
  try { if (!document.execCommand?.('copy')) throw new Error('Clipboard unavailable'); } finally { area.remove(); focused?.focus({preventScroll:true}); }
}
