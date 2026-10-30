// One Cindy renderer serves conversation text and the file viewer.
import {MarkdownRenderer} from './chat-rich-answer';
import type {MarkdownRendererProps} from './cindy/markdown/MarkdownRenderer';
import './cindy/markdown/upstream.css';
import './cindy/panel-ui/locales';
import './cindy/panel-ui/theme.css';
import './chat-markdown.css';
export function ChatMarkdown({text,onCopy:_,...props}:{text:string;onCopy?:(text:string)=>void}&Omit<MarkdownRendererProps,'content'>){
 return <div className="chat-markdown"><MarkdownRenderer content={text} renderRichAnswers {...props}/></div>;
}
