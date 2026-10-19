import {afterEach,expect,it,vi} from 'vitest';
import {restorePanelTabs} from './chat-panel-state';

const saved=new Map<string,string>();
vi.stubGlobal('localStorage',{getItem:(key:string)=>saved.get(key)??null,setItem:(key:string,value:string)=>saved.set(key,value)});
afterEach(()=>saved.clear());

it('keeps files and resources as fixed views rather than document tabs',()=>{
 const state=restorePanelTabs('new');
 expect(state.tabs.map(tab=>tab.kind)).toEqual(['home']);
 expect(state.section).toBe('tabs');expect(state.libraryTab).toBe('files');
});
it('migrates old file/resource tabs without losing document order, pins or selection',()=>{
 localStorage.setItem('winone.chat.panel.v1:chat',JSON.stringify({tabs:[{id:'file:a',kind:'file',path:'a.md',pinned:true},{id:'files',kind:'files',path:'C:/fixture/docs'},{id:'resources',kind:'resources'},{id:'terminal:1',kind:'terminal'}],selected:'resources'}));
 const state=restorePanelTabs('chat');
 expect(state.tabs.map(tab=>tab.id)).toEqual(['file:a','terminal:1']);
 expect(state.tabs[0].pinned).toBe(true);expect(state.section).toBe('library');expect(state.libraryTab).toBe('resources');expect(state.directory).toBe('C:/fixture/docs');
});
it('restores both levels independently and keeps an intentionally empty tab strip',()=>{
 localStorage.setItem('winone.chat.panel.v1:chat',JSON.stringify({tabs:[],selected:'',section:'library',libraryTab:'resources'}));
 const state=restorePanelTabs('chat');expect(state.tabs).toEqual([]);expect(state.section).toBe('library');expect(state.libraryTab).toBe('resources');
});
