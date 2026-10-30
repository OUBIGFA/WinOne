// This trusted document displays only privacy records. The same authenticated
// routing process owns the masker; no provider credential enters a URL.
// WinOne owns the Privacy settings form, so the page's "turn on" and "rules"
// links open that form instead of saving Magpie's own preferences directly.
function publish() {
 if(winOnePluginParent) parent.postMessage({type:'winone-plugin-height',height:Math.ceil(document.body.getBoundingClientRect().height)},winOnePluginParent);
}
window.winOnePublishMasking=publish;
function openSettingsAt(event) {
 event?.preventDefault?.();
 if(winOnePluginParent) parent.postMessage({type:'winone-masking-settings'},winOnePluginParent);
}
window.winOneMaskKinds=kinds=>{
 if(winOnePluginParent) parent.postMessage({type:'winone-masking-actions',kinds},winOnePluginParent);
};
function maskTurnOn(event) { openSettingsAt(event); }
window.winOneMaskingError=error=>{
 status(error.message,'err',0);
 const retry=el('button','text',t('Try again'));retry.dataset.t='';retry.dataset.en='Try again';
 retry.onclick=()=>{status('');void maskLoad(true)?.then(()=>{drawMaskCard();drawMaskList();});};
 document.getElementById('status').append(retry);
};
window.winOnePluginTheme=value=>{
 if(value.language){setLocale(value.language.startsWith('zh')?'zh':'en');document.documentElement.lang=value.language;}
 if(value.theme){document.documentElement.dataset.theme=value.theme;document.documentElement.classList.toggle('dark',value.theme==='dark');}
 translateStatic();
 if(state.settings)renderMaskTry(state.settings);
 maskDrawn='';maskDrawAll();publish();
};
(async()=>{
 try {
  window.winOnePluginTheme({language:window.bootPrefs?.lang,theme:window.bootPrefs?.theme});
  state=await api('state');load.done=true;renderMaskTry(state.settings);
  const page=document.getElementById('view-masking');
  page.hidden=false;magpie.start();translateStatic();
  await magpie.call('masking','open');
  if(params.get('record'))magpie.call('masking','focus',params.get('record'));
  new ResizeObserver(publish).observe(document.body);publish();
 }catch(error){window.winOneMaskingError(error);}
})();
