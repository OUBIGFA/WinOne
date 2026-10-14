// Embed the pinned Magpie routing page unchanged. Only the WinOne document
// boundary (outer navigation, language/theme and available height) is adapted.
document.documentElement.classList.add("winone-routing");
function applyWinOneTheme(value){
 if(value.language){setLocale(value.language.startsWith("zh")?"zh":"en");document.documentElement.lang=value.language;translateStatic();}
 if(value.theme){document.documentElement.classList.toggle("dark",value.theme==="dark");document.documentElement.dataset.theme=value.theme;document.documentElement.style.colorScheme=value.theme;}
}
let winOneParentOrigin="",winOneLastHeight=0,winOneMeasureFrame=0;
function publishRoutingHeight(){cancelAnimationFrame(winOneMeasureFrame);winOneMeasureFrame=requestAnimationFrame(()=>{const view=document.getElementById("view-routing");if(!view||!winOneParentOrigin)return;const height=Math.ceil(view.getBoundingClientRect().height);if(height>0&&height!==winOneLastHeight){winOneLastHeight=height;parent.postMessage({type:"winone-routing-height",height},winOneParentOrigin);}});}
window.addEventListener("message",event=>{if(event.source!==parent)return;if(event.data?.type==="winone-routing-theme"){winOneParentOrigin=event.origin;applyWinOneTheme(event.data);winOneLastHeight=0;publishRoutingHeight();}});
const embedding=document.createElement("style");embedding.textContent=`
html.winone-routing,html.winone-routing body{height:auto!important;min-height:0!important;overflow:visible!important}
html.winone-routing header.top,html.winone-routing footer{display:none!important}
html.winone-routing .view{flex:none;top:0!important;height:auto!important;max-height:none!important;overflow:visible!important;scrollbar-gutter:auto}
`;document.head.append(embedding);
// Measure content, not the iframe's assigned height, to avoid a feedback loop.
const winOneRoutingView=document.getElementById("view-routing");
if(winOneRoutingView)new ResizeObserver(publishRoutingHeight).observe(winOneRoutingView);
