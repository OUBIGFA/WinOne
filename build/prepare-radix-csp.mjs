// Radix's scroll-lock singleton inserts a style element when a menu or dialog
// opens. Preserve the exact generated rules and reference counting under
// WinOne's style-src 'self' by adopting/removing a constructed stylesheet.
import {readFile,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
const directory=resolve(import.meta.dirname,'../apps/desktop/frontend/node_modules/react-style-singleton');
const info=JSON.parse(await readFile(join(directory,'package.json'),'utf8'));
if(info.version!=='2.2.3')throw Error('Review the scroll-lock CSP adapter for react-style-singleton '+info.version);
const marker='// WinOne constructable stylesheet adapter v1';
for(const name of ['es5','es2015','es2019']){
 const path=join(directory,'dist',name,'singleton.js');let text=await readFile(path,'utf8');
 if(text.includes(marker))continue;
 const patches=[
  ['function makeStyleTag() {',`function makeStyleTag() {\n    ${marker}\n    if (typeof CSSStyleSheet !== 'undefined' && typeof CSSStyleSheet.prototype.replaceSync === 'function' && 'adoptedStyleSheets' in document) return { winOneSheet: new CSSStyleSheet() };`],
  ['function injectStyles(tag, css) {',`function injectStyles(tag, css) {\n    if (tag.winOneSheet) { tag.winOneSheet.replaceSync(css); return; }`],
  ['function insertStyleTag(tag) {',`function insertStyleTag(tag) {\n    if (tag.winOneSheet) { document.adoptedStyleSheets = [...document.adoptedStyleSheets, tag.winOneSheet]; return; }`],
  ['if (!counter && stylesheet) {',`if (!counter && stylesheet) {\n                if (stylesheet.winOneSheet) document.adoptedStyleSheets = document.adoptedStyleSheets.filter(sheet => sheet !== stylesheet.winOneSheet);`],
 ];
 for(const [from,to]of patches){if(text.split(from).length!==2)throw Error('Scroll-lock lifecycle changed: '+name+' / '+from);text=text.replace(from,to);}
 await writeFile(path,text,'utf8');
}
