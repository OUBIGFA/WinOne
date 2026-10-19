// Preserve Mermaid's rendering and styles while using CSSOM in WinOne's
// style-src 'self' WebView. Re-applied after npm ci; no app CSP is relaxed.
import {readFileSync,writeFileSync} from 'node:fs';
import {dirname,resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
const root=resolve(dirname(fileURLToPath(import.meta.url)),'..');
const modules=resolve(root,'apps/desktop/frontend/node_modules');
const metadata=JSON.parse(readFileSync(resolve(modules,'mermaid/package.json'),'utf8'));
if(metadata.version!=='11.14.0')throw Error('Revalidate the Mermaid CSP adapter for this package version');
const core=resolve(modules,'mermaid/dist/mermaid.core.mjs');
let text=readFileSync(core,'utf8');
const marker='// WinOne: use a constructable stylesheet during diagram measurement.';
if(!text.includes(marker)){
 const before='  style1.innerHTML = rules;\n  svg.insertBefore(style1, firstChild);';
 if(!text.includes(before))throw Error('Mermaid stylesheet construction changed');
 text=text.replace(before,`  ${marker}
  style1.type = "text/plain";
  style1.textContent = rules;
  svg.insertBefore(style1, firstChild);
  const winOneCspRoot = svg.getRootNode();
  const winOneCspSheet = new CSSStyleSheet();
  winOneCspSheet.replaceSync(rules);
  winOneCspRoot.adoptedStyleSheets = [...winOneCspRoot.adoptedStyleSheets, winOneCspSheet];
  const winOneCspObserver = new MutationObserver(() => {
    if (!svg.isConnected) {
      winOneCspRoot.adoptedStyleSheets = winOneCspRoot.adoptedStyleSheets.filter(sheet => sheet !== winOneCspSheet);
      winOneCspObserver.disconnect();
    }
  });
  winOneCspObserver.observe(svg.ownerDocument.documentElement, {childList: true, subtree: true});`);
 writeFileSync(core,text,'utf8');
}
const nodeSanitizer='// WinOne: sanitize the existing SVG rather than reparsing inline CSS.';
if(!text.includes(nodeSanitizer)){
 text=text.replace('const style1 = document.createElement("style");','const style1 = document.createElementNS("http://www.w3.org/2000/svg", "style");');
 text=text.replace('  style1.type = "text/plain";\n  style1.textContent = rules;\n  svg.insertBefore(style1, firstChild);','  style1.textContent = rules;');
 const before=`  if (isSandboxed) {
    const svgEl = root.select(enclosingDivID_selector + " svg").node();
    svgCode = putIntoIFrame(svgCode, svgEl);
  } else if (!isLooseSecurityLevel) {
    svgCode = DOMPurify.sanitize(svgCode, {
      ADD_TAGS: DOMPURIFY_TAGS,
      ADD_ATTR: DOMPURIFY_ATTR,
      HTML_INTEGRATION_POINTS: { foreignobject: true }
    });
  }`;
 if(!text.includes(before))throw Error('Mermaid sanitization changed; keep its full policy intact');
 text=text.replace(before,`  ${nodeSanitizer}
  if (!isSandboxed && !isLooseSecurityLevel) {
    DOMPurify.sanitize(svg, {
      IN_PLACE: true,
      ADD_TAGS: DOMPURIFY_TAGS,
      ADD_ATTR: DOMPURIFY_ATTR,
      HTML_INTEGRATION_POINTS: { foreignobject: true }
    });
    svgCode = cleanUpSvgCode(svg.outerHTML, isSandboxed, evaluate(config.arrowMarkerAbsolute));
  }
  // Styles are Mermaid-generated CSS. XML serialization preserves their text
  // without creating an active style element in the application document.
  const winOneStyleMarkup = new XMLSerializer().serializeToString(style1);
  svgCode = svgCode.replace(/(<svg\\b[^>]*>)/, match => match + winOneStyleMarkup);
  if (isSandboxed) {
    svgCode = putIntoIFrame(svgCode, svg);
  }`);
 writeFileSync(core,text,'utf8');
}
const attribute=resolve(modules,'d3-selection/src/selection/attr.js');text=readFileSync(attribute,'utf8');
if(!text.includes('// WinOne CSSOM style attributes')){
 for(const value of ['value','v']){
  const before=`this.setAttribute(name, ${value});`;
  if(!text.includes(before))throw Error('D3 attribute construction changed');
  text=text.replace(before,`if (name === "style" && this.style) this.style.cssText = ${value}; else this.setAttribute(name, ${value});`);
 }
 writeFileSync(attribute,'// WinOne CSSOM style attributes\n'+text,'utf8');
}
console.log('Mermaid CSSOM adapter ready.');
