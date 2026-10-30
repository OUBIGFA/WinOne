// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {act,cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {i18n} from '../../../i18n';
import {BotProjectsSettings} from '../bots/BotProjectsSettings';
import './onebotLocales';

const bridge=vi.hoisted(()=>({call:vi.fn()}));
vi.mock('../../../bridge',()=>({chatCall:bridge.call}));
let rows:Array<{id:string;name:string;path:string;linked:boolean;available:boolean}>;
let finishPoll:((value:unknown)=>void)|undefined;
let failSave=false;
const label=(key:string)=>i18n.t(key);
beforeEach(async()=>{
 await i18n.changeLanguage('en-US');
 rows=[{id:'project-choice',name:'Registered project',path:'E:/isolated/project',linked:false,available:true}];
 finishPoll=undefined;failSave=false;
 bridge.call.mockReset().mockImplementation(async(method:string,params:any)=>{
  if(method==='onebot.events'){
   if(params.cursor===Number.MAX_SAFE_INTEGER)return {cursor:0,events:[],reset:true};
   return new Promise(resolve=>{finishPoll=resolve;});
  }
  if(params.botId!=='reader')throw Error('Wrong Bot');
  if(method==='onebot.projects.list')return structuredClone(rows);
  if(method==='onebot.projects.set'){
   if(failSave)throw Error('Unable to save');
   expect(params.id).toBe('project-choice');
   rows=rows.map(row=>({...row,linked:params.linked}));
   return structuredClone(rows);
  }
  throw Error('Unexpected '+method);
 });
});
afterEach(async()=>{
 cleanup();
 await act(async()=>{finishPoll?.({cursor:0,events:[],reset:false});});
});

it('uses the original selector to link and unlink the Host-provided project',async()=>{
 render(<BotProjectsSettings botId="reader"/>);
 const select=await screen.findByRole('combobox',{name:label('bots.projects.choose')});
 await waitFor(()=>expect(select.hasAttribute('disabled')).toBe(false));
 fireEvent.keyDown(select,{key:'ArrowDown'});
 fireEvent.click(await screen.findByRole('option',{name:'Registered project'}));
 fireEvent.click(screen.getByRole('button',{name:label('bots.projects.add')}));
 const remove=await screen.findByRole('button',{name:i18n.t('bots.projects.remove',{name:'Registered project'})});
 expect(rows[0].linked).toBe(true);
 fireEvent.click(remove);
 await screen.findByText(label('bots.projects.empty'));
 expect(rows[0].linked).toBe(false);
});

it('keeps a linked project visible when unlink fails and supports retry',async()=>{
 rows[0].linked=true;rows[0].available=false;
 render(<BotProjectsSettings botId="reader"/>);
 const remove=await screen.findByRole('button',{name:i18n.t('bots.projects.remove',{name:'Registered project'})});
 await screen.findByText(label('bots.projects.unavailable'));
 failSave=true;fireEvent.click(remove);
 await screen.findByRole('alert');
 expect(rows[0].linked).toBe(true);
 expect(screen.getByText('Registered project')).toBeTruthy();
 failSave=false;fireEvent.click(remove);
 await screen.findByText(label('bots.projects.empty'));
 expect(rows[0].linked).toBe(false);
});

it('reloads the open page on a Host workbench change and on a reset cursor',async()=>{
 render(<BotProjectsSettings botId="reader"/>);
 await screen.findByText(label('bots.projects.empty'));
 await waitFor(()=>expect(finishPoll).toBeTypeOf('function'));
 rows[0].linked=true;
 await act(async()=>{finishPoll!({cursor:1,events:[{type:'workbench-changed',botId:'reader'}],reset:false});});
 await screen.findByRole('button',{name:i18n.t('bots.projects.remove',{name:'Registered project'})});
 rows[0].linked=false;
 await act(async()=>{finishPoll!({cursor:2,events:[],reset:true});});
 await screen.findByText(label('bots.projects.empty'));
});
