// Unmodified package controls without WinOne styles: a computed-style oracle
// catches accidental overrides that importing the correct component cannot.
import {createRoot} from 'react-dom/client';
import {DesktopProvider,Tabs,PageBar,WorkbenchPage,ConversationMessage,AIResponse} from 'chatgpt-desktop-kit';
import 'chatgpt-desktop-kit/components.css';
import {BrowserPanel} from './chat-pane-reference';
import '../src/features/chat-upstream-pane.css';
const dark=new URLSearchParams(location.search).get('theme')==='dark';
document.documentElement.classList.toggle('dark',dark);
createRoot(document.getElementById('root')!).render(<DesktopProvider theme={dark?'dark':'light'}><div className="desktop-client"><BrowserPanel full={false} onFull={()=>{}} onClose={()=>{}} onInfo={()=>{}}/></div><div style={{fontSize:16}}><ConversationMessage role="user" label="User"><div>Sample message</div></ConversationMessage><ConversationMessage role="assistant" label="Assistant"><AIResponse responseId="reference" content={'## Heading\n\nParagraph.\n\n```typescript\nconst answer = 42;\n```'}/></ConversationMessage></div></DesktopProvider>);
