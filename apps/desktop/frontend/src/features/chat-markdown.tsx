// Markdown/AIResponse body adapted directly from chatgpt-desktop-kit 0.6.0.
// Preserve its markup and GFM parser, adding scoped local-file URL routing.
import ReactMarkdown,{defaultUrlTransform} from 'react-markdown';
import remarkGfm from 'remark-gfm';
import {ChatFileLink,ChatFileImage,ChatInlineCode} from './chat-file-navigation';
import {Children,isValidElement,useState,type ReactNode} from 'react';
import {useTranslation} from '../i18n';
import {copyText} from './clipboard';
import type {Components} from 'react-markdown';
import './chat-markdown.css';

function textContent(value:ReactNode):string{return Children.toArray(value).map(child=>isValidElement<{children?:ReactNode}>(child)?textContent(child.props.children):String(child)).join('');}
// Adapted directly from chatgpt-desktop-kit 0.6.0 CodeBlock (MIT). Preserve the
// upstream controls and local feedback while using the WebView clipboard path.
function ChatCodeBlock({children}:{children?:ReactNode}){
 const {t}=useTranslation(),[state,setState]=useState<'ready'|'copied'|'failed'>('ready');
 const code=Children.toArray(children).find(isValidElement<{className?:string}>);
 const language=isValidElement<{className?:string}>(code)?code.props.className?.match(/language-(\S+)/)?.[1]:undefined;
 return <div className="desktop-markdown-codeblock"><div className="desktop-markdown-codebar"><span>{language??t('chat.code')}</span><button type="button" className="desktop-markdown-copy" onClick={async()=>{try{await copyText(textContent(children));setState('copied');}catch{setState('failed');}}}>{t(state==='copied'?'chat.copied':state==='failed'?'chat.copyFailed':'chat.copyCode')}</button></div><pre>{children}</pre></div>;
}
export const chatMarkdownComponents:Components={a:({node:_,...props})=><ChatFileLink {...props}/>,img:({node:_,...props})=><ChatFileImage {...props}/>,code:({node:_,...props})=><ChatInlineCode {...props}/>,pre:ChatCodeBlock};

// One renderer and palette serve messages, work commentary and file previews.
// The upstream renderer retains its tables, fenced-code controls and selection.
export function ChatMarkdown({text}:{text:string;onCopy?:(text:string)=>void}){
 return <div className="desktop-ai-markdown chat-markdown select-text"><ReactMarkdown remarkPlugins={[remarkGfm]} components={chatMarkdownComponents} urlTransform={value=>/^file:\/\/|^[A-Za-z]:[\\/]/.test(value)?value:defaultUrlTransform(value)}>{text}</ReactMarkdown></div>;
}
