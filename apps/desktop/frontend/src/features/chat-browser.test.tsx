// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {act,cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {ChatBrowserView} from './chat-browser';
import {i18n} from '../i18n';
const bridge=vi.hoisted(()=>({call:vi.fn()}));
vi.mock('../bridge',()=>({chatBrowserCall:bridge.call}));
afterEach(()=>{cleanup();vi.clearAllMocks();});
it('opens a native tab and navigates from the address bar without losing tab identity',async()=>{
 await i18n.changeLanguage('en-US');
 bridge.call.mockImplementation(async(method:string,value:any)=>({sessionId:value.sessionId,tabId:value.tabId,url:value.url??'about:blank',title:'Example',loading:false,canGoBack:false,canGoForward:false,visible:true,error:'',favicon:'',isAudible:false,muted:false,crashed:false}));
 render(<ChatBrowserView sessionId="chat-one" tabId="browser:test" url="about:blank"/>);
 const address=await screen.findByRole('textbox',{name:'Search or enter a URL'});
 fireEvent.change(address,{target:{value:'example.com'}});fireEvent.keyDown(address,{key:'Enter'});
 await waitFor(()=>expect(bridge.call).toHaveBeenCalledWith('navigate',expect.objectContaining({sessionId:'chat-one',tabId:'browser:test',url:'https://example.com'})));
 expect(screen.getByRole('button',{name:'Comment on page'})).toBeTruthy();
});
it('keeps a text selection reported during entry and adds the acknowledged screenshot',async()=>{
 await i18n.changeLanguage('en-US');const added=vi.fn().mockResolvedValue(undefined);
 const target={kind:'text',point:{x:10,y:20},viewport:{width:640,height:480},region:null,selectedText:'Selected words',immediate:false,targetTag:'p',targetLabel:'Selected words',targetRole:null,targetSelector:'p',targetPath:'html > body > p',nearbyText:'Selected words',themeVariant:'light',designBaseline:null,markerNumber:1};
 const emit=(kind:string,value:unknown)=>window.dispatchEvent(new CustomEvent('winone:chat-browser',{detail:{kind,sessionId:'chat-one',tabId:'browser:test',value}}));
 bridge.call.mockImplementation(async(method:string,value:any)=>{
  if(method==='comment'){
   if(value.channel==='browser-comment:enter-mode')emit('comment',{channel:'browser-comment:element-selected',payload:target});
   emit('comment',{channel:'browser-comment:command-result',payload:{requestId:value.payload?.requestId,command:value.channel,ok:true}});return null;
  }
  if(method==='capture'){emit('operation',{operationId:value.operationId,result:{screenshotPath:'C:/isolated/marker.png'}});return null;}
  return {sessionId:value.sessionId,tabId:value.tabId,url:'https://example.test',title:'Example',loading:false,canGoBack:false,canGoForward:false,visible:true,error:'',favicon:'',isAudible:false,muted:false,crashed:false};
 });
 render(<ChatBrowserView sessionId="chat-one" tabId="browser:test" url="https://example.test" onComment={added}/>);
 await act(async()=>fireEvent.click(await screen.findByRole('button',{name:'Comment on page'})));
 const editor=await screen.findByPlaceholderText('Describe the change you want');fireEvent.change(editor,{target:{value:'Fix this text'}});fireEvent.click(screen.getByRole('button',{name:'Add comment'}));
 await waitFor(()=>expect(added).toHaveBeenCalledWith(expect.objectContaining({markerNumber:1,comment:'Fix this text',screenshotPath:'C:/isolated/marker.png',target})));
 expect(await screen.findByRole('status')).toHaveProperty('textContent','Comment and screenshot added to the composer.');
});
