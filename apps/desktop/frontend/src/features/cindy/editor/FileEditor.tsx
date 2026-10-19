// WinOne host for Cindy PlaintextEditor. The actual editor, Markdown widgets,
// themes and history remain upstream; only file capability URLs and CSS scope
// cross this boundary.
import {createPortal} from 'react-dom';
import {forwardRef,useCallback,useImperativeHandle,useLayoutEffect,useRef,useState} from 'react';
import {chatCall} from '../../../bridge';
import {resolveLocalPath} from '../localPathResolver';
import {PlaintextEditor,type PlaintextEditorHandle} from './PlaintextEditor';
import compactStyles from './sidebar-editor.css?raw';
import {prepareMedia,registerPreviewMedia} from '../media/host';
import type {ChatFilePreviewData} from '../../chat-file-preview';
export type FileEditorHandle=PlaintextEditorHandle;
export const FileEditor=forwardRef<FileEditorHandle,{id:string;path:string;value:string;language?:string;readOnly?:boolean;line?:number;onChange:(value:string)=>void}>(function FileEditor({id,path,value,language,readOnly,line,onChange},ref){
 const container=useRef<HTMLDivElement>(null),editor=useRef<PlaintextEditorHandle>(null),[root,setRoot]=useState<ShadowRoot>();
 const images=useRef(new Map<string,Promise<string>>());
 useImperativeHandle(ref,()=>({
  getValue:()=>editor.current?.getValue()??value,
  setValue:text=>editor.current?.setValue(text),
  insertMarkdownTableAt:coords=>editor.current?.insertMarkdownTableAt(coords),
  getViewportTopLine:()=>editor.current?.getViewportTopLine()??null,
  scrollToLine:line=>editor.current?.scrollToLine(line),
  getSelectionText:()=>editor.current?.getSelectionText()??null,
  getSelectionLineRange:()=>editor.current?.getSelectionLineRange()??null,
  search:{findAll:query=>editor.current?.search.findAll(query)??0,setActive:index=>editor.current?.search.setActive(index),clear:()=>editor.current?.search.clear()},
 }),[value]);
 useLayoutEffect(()=>{const host=container.current!;const shadow=host.shadowRoot??host.attachShadow({mode:'open'});const sheet=new CSSStyleSheet();sheet.replaceSync(':host{display:flex;height:100%;min-height:0;min-width:0;flex:1} .select-text{user-select:text;display:flex;flex:1;flex-direction:column;min-width:0;min-height:0;overflow:hidden} .select-text *{user-select:text} button,input,textarea,select{font:inherit;color:inherit} *{box-sizing:border-box} .cm-editor{width:100%;height:100%;min-height:0} .cm-scroller{overflow:auto}');const compact=new CSSStyleSheet();compact.replaceSync(compactStyles);shadow.adoptedStyleSheets=[sheet,compact];setRoot(shadow);},[]);
 useLayoutEffect(()=>{if(editor.current&&editor.current.getValue()!==value)editor.current.setValue(value);},[value]);
 useLayoutEffect(()=>{if(line!==undefined)editor.current?.scrollToLine(line);},[line]);
 const resolveImage=useCallback((url:string)=>{
  if(/^(?:https?:|data:)/i.test(url))return prepareMedia(url,id).then(value=>value.url);
  let pending=images.current.get(url);if(pending)return pending;
  const carried=url.startsWith('xdt-file://')?new URL(url).searchParams.get('path'):null;
  const filePath=carried??resolveLocalPath(url,path.replace(/[\\/][^\\/]+$/,''));
  pending=chatCall<ChatFilePreviewData>('file.preview',{id,path:filePath}).then(file=>{if(file.kind!=='image')throw Error('The referenced file is not an image');registerPreviewMedia(id,file);return file.url;});
  images.current.set(url,pending);void pending.catch(()=>images.current.delete(url));return pending;
 },[id,path]);
 return <div ref={container} className="cindy-file-editor">{root&&createPortal(<PlaintextEditor ref={editor} initialValue={value} language={language} readOnly={readOnly} initialScrollLine={line} imageBaseDir={path.replace(/[\\/][^\\/]+$/,'')} imageResolver={resolveImage} onChange={onChange}/>,root)}</div>;
});
