import {useLayoutEffect,useRef,useState} from 'react';
import {createPortal} from 'react-dom';
import CodeMirror,{EditorView,type ReactCodeMirrorProps} from '@uiw/react-codemirror';

/** CodeMirror uses constructable stylesheets inside a shadow root. This keeps
 * its generated syntax styles compatible with the desktop's strict style CSP
 * and isolates it from global page/aside/pre rules. No CSP exception is needed. */
export function ChatCodeEditor({line,...props}:ReactCodeMirrorProps & {line?:number}){
 const host=useRef<HTMLDivElement>(null),view=useRef<EditorView>(undefined),[root,setRoot]=useState<ShadowRoot>();
 useLayoutEffect(()=>{
  const node=host.current!;const shadow=node.shadowRoot??node.attachShadow({mode:'open'});
  const sheet=new CSSStyleSheet();sheet.replaceSync(':host{display:block;height:100%;min-height:0;overflow:hidden} .cm-theme,.cm-theme-light,.cm-theme-dark{height:100%} .cm-editor{height:100%;font:14px/1.6 var(--app-font-code-default,Consolas,monospace)} .cm-scroller{overflow:auto}');shadow.adoptedStyleSheets=[sheet];setRoot(shadow);
 },[]);
 function jump(editor:EditorView){if(!line)return;const target=editor.state.doc.line(Math.max(1,Math.min(line,editor.state.doc.lines)));editor.dispatch({selection:{anchor:target.from},effects:EditorView.scrollIntoView(target.from,{y:'center'})});}
 useLayoutEffect(()=>{if(view.current)jump(view.current);},[line]);
 return <div ref={host} className="chat-code-editor">{root&&createPortal(<CodeMirror {...props} root={root} height="100%" onCreateEditor={editor=>{view.current=editor;jump(editor);}}/>,root)}</div>;
}
