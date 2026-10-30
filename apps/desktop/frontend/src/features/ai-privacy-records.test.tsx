// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {act,cleanup,render,screen} from '@testing-library/react';
import {AIPrivacyRecords} from './ai-privacy-records';
const extensions={url:'http://127.0.0.1:4310/session/fixture/plugins.html',pages:[],panels:[]};
afterEach(()=>{cleanup();vi.restoreAllMocks();vi.unstubAllGlobals();});

it('opens the privacy document with an exact record ID, not the plugin manager',()=>{
 const record='1792954853787295701';
 const view=render(<AIPrivacyRecords extensions={extensions} active record={record}/>);
 const frame=screen.getByTitle('脱敏记录') as HTMLIFrameElement;
 expect(frame.src).toBe('http://127.0.0.1:4310/session/fixture/masking.html?record='+record);
 view.rerender(<AIPrivacyRecords extensions={extensions} active={false} record={record}/>);
 expect(screen.queryByTitle('脱敏记录')).toBeNull();
});

it('accepts height and settings navigation only from its own authenticated frame',()=>{
 const scroll=vi.fn();vi.stubGlobal('matchMedia',()=>({matches:true}));
 render(<><AIPrivacyRecords extensions={extensions} active/><section><button>Privacy settings</button></section></>);
 screen.getByRole('button',{name:'Privacy settings'}).parentElement!.scrollIntoView=scroll;
 const frame=screen.getByTitle('脱敏记录') as HTMLIFrameElement;
 const post=(origin:string,source:Window|null,data:unknown)=>act(()=>window.dispatchEvent(new MessageEvent('message',{origin,source,data})));
 post('http://untrusted.invalid',frame.contentWindow,{type:'winone-plugin-height',height:900});
 post('http://127.0.0.1:4310',window,{type:'winone-plugin-height',height:900});
 expect(frame.style.height).toBe('320px');
 post('http://127.0.0.1:4310',frame.contentWindow,{type:'winone-plugin-height',height:712.2});
 expect(frame.style.height).toBe('713px');
 post('http://127.0.0.1:4310',frame.contentWindow,{type:'winone-plugin-height',height:Infinity});
 expect(frame.style.height).toBe('713px');
 post('http://untrusted.invalid',frame.contentWindow,{type:'winone-masking-settings'});expect(scroll).not.toHaveBeenCalled();
 post('http://127.0.0.1:4310',frame.contentWindow,{type:'winone-masking-settings'});
 expect(scroll).toHaveBeenCalledTimes(1);expect(document.activeElement).toBe(screen.getByRole('button',{name:'Privacy settings'}));
});

it('stages masking kinds in the parent draft, but never while disabled',()=>{
 const onMaskKinds=vi.fn();
 const view=render(<AIPrivacyRecords extensions={extensions} active onMaskKinds={onMaskKinds}/>);
 const frame=screen.getByTitle('脱敏记录') as HTMLIFrameElement;
 const send=(kinds:unknown,origin='http://127.0.0.1:4310')=>act(()=>window.dispatchEvent(new MessageEvent('message',{origin,source:frame.contentWindow,data:{type:'winone-masking-actions',kinds}})));
 send(['API_KEY'],'https://untrusted.invalid');send([42]);expect(onMaskKinds).not.toHaveBeenCalled();
 send(['API_KEY','CUSTOM']);expect(onMaskKinds).toHaveBeenCalledExactlyOnceWith(['API_KEY','CUSTOM']);
 view.rerender(<AIPrivacyRecords extensions={extensions} active disabled onMaskKinds={onMaskKinds}/>);
 send(['PASSWORD']);expect(onMaskKinds).toHaveBeenCalledTimes(1);
});
