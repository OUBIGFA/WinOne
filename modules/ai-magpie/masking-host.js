// This trusted document displays only privacy records. The same authenticated
// routing process owns the masker; no token or account data enters a URL.
view = 'settings';
const setShown = 'privacy';
function fit() { publish(); }
function publish() {
 if(winOnePluginParent) parent.postMessage({type:'winone-plugin-height',height:Math.ceil(document.body.getBoundingClientRect().height)},winOnePluginParent);
}
function openMaskRecord(key) { maskOpenRecords({},key); }
window.winOneMaskingError=error=>{
 status(error.message,'err',0);
 const retry=el('button','text',t('Try again'));
 retry.onclick=()=>{status('');void maskLoad(true)?.then(drawMaskObs);};
 document.getElementById('status').append(retry);
};
window.winOnePluginTheme=value=>{
 if(value.language){setLocale(value.language.startsWith('zh')?'zh':'en');document.documentElement.lang=value.language;}
 if(value.theme){document.documentElement.dataset.theme=value.theme;document.documentElement.classList.toggle('dark',value.theme==='dark');}
 if(maskS){renderMaskObs(maskS);renderMaskTry(maskS);}
};
(async()=>{
 try {
  window.winOnePluginTheme({language:window.bootPrefs?.lang,theme:window.bootPrefs?.theme});
  state=await api('state');renderMaskObs(state.settings);renderMaskTry(state.settings);
  if(params.get('record'))maskOpenRecords({},params.get('record'));
  new ResizeObserver(publish).observe(document.body);publish();
 }catch(error){window.winOneMaskingError(error);}
})();
