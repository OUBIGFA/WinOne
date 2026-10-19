// Mermaid generates trusted SVG under securityLevel=strict. Keep the same
// geometry and CSS, but mount styles through CSSOM instead of inline markup.
import {forwardRef,useLayoutEffect,useRef,type HTMLAttributes} from 'react';
import {parseFragment,type DefaultTreeAdapterMap} from 'parse5';
type ParsedNode=DefaultTreeAdapterMap['childNode'];
type ParsedElement=DefaultTreeAdapterMap['element'];
const sheets=new WeakMap<SVGElement,{root:DocumentOrShadowRoot;sheet:CSSStyleSheet}>();
export function releaseMermaidSvg(svg:SVGElement){const owned=sheets.get(svg);if(owned){owned.root.adoptedStyleSheets=owned.root.adoptedStyleSheets.filter(sheet=>sheet!==owned.sheet);sheets.delete(svg);}}
export function createMermaidSvg(markup:string,root:DocumentOrShadowRoot):SVGElement|null{return buildSvg(markup,root);}
export function createMermaidImageSvg(markup:string):SVGElement|null{return buildSvg(markup,null);}
function buildSvg(markup:string,root:DocumentOrShadowRoot|null):SVGElement|null{
 // The pure HTML parser never hands inline styles to the browser's parser.
 const source=parseFragment(markup).childNodes.find((node):node is ParsedElement=>'tagName' in node&&node.tagName==='svg');if(!source)return null;
 const rules:string[]=[];
 function clone(node:ParsedNode):Node|null{
  if(node.nodeName==='#text')return document.createTextNode((node as DefaultTreeAdapterMap['textNode']).value);
  if(!('tagName' in node)||node.tagName==='script')return null;
  if(node.tagName==='style'){
   const css=node.childNodes.map(child=>child.nodeName==='#text'?(child as DefaultTreeAdapterMap['textNode']).value:'').join('');
   if(root){rules.push(css);return null;}
   const style=document.createElementNS('http://www.w3.org/2000/svg','style');style.setAttribute('type','text/plain');style.textContent=css;return style;
  }
  const result=document.createElementNS(node.namespaceURI,node.tagName);
  for(const attribute of node.attrs){
   if(attribute.name.toLowerCase().startsWith('on'))continue;
   if(attribute.name==='style'&&'style' in result)(result as HTMLElement|SVGElement).style.cssText=attribute.value;
   else if(attribute.namespace)result.setAttributeNS(attribute.namespace,attribute.prefix?attribute.prefix+':'+attribute.name:attribute.name,attribute.value);
   else result.setAttribute(attribute.name,attribute.value);
  }
  for(const child of node.childNodes){const copy=clone(child);if(copy)result.appendChild(copy);}
  return result;
 }
 const svg=clone(source) as SVGElement;svg.dataset.winoneMermaid='';
 if(root&&rules.length){const sheet=new CSSStyleSheet();sheet.replaceSync(rules.join('\n'));root.adoptedStyleSheets=[...root.adoptedStyleSheets,sheet];sheets.set(svg,{root,sheet});}
 return svg;
}
export const MermaidSvg=forwardRef<HTMLDivElement,HTMLAttributes<HTMLDivElement>&{svg:string}>(function MermaidSvg({svg,...props},forwardedRef){
 const ref=useRef<HTMLDivElement|null>(null);
 useLayoutEffect(()=>{const element=ref.current!;const image=createMermaidSvg(svg,element.getRootNode() as Document|ShadowRoot);if(image)element.replaceChildren(image);return()=>{if(image)releaseMermaidSvg(image);};},[svg]);
 return <div {...props} ref={element=>{ref.current=element;if(typeof forwardedRef==='function')forwardedRef(element);else if(forwardedRef)forwardedRef.current=element;}}/>;
});
