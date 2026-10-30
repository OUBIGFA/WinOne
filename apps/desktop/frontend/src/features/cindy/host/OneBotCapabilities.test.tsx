// @vitest-environment jsdom
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {act,cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {useState} from 'react';
import {i18n} from '../../../i18n';
import {BotCapabilitySettings} from '../bots/BotCapabilitySettings';
import {registerLoadedBotProfilesReader} from '../bots/botSessionOwners';
import type {BotCapabilities, BotProfile} from '../bots/botStore';
import './onebotLocales';
const bridge=vi.hoisted(()=>({call:vi.fn()}));
vi.mock('../../../bridge',()=>({chatCall:bridge.call,captureCall:bridge.call}));
let fail=false,clock=Date.UTC(2026,9,28);
let pending:Array<(value:unknown)=>void>=[];
const changes:Array<[string,string[]]>=[];
const catalogs:unknown[]=[];
const entries={
 toolset:[{id:'alpha',name:'Alpha tools',available:true},{id:'beta',name:'Beta tools',available:true},{id:'memory',name:'Memory',available:true}],
 mcp:[],
 skill:[{id:'personal:own',name:'Personal skill',available:true,personal:true},{id:'shared-review',name:'shared-review',available:true}],
};
const conversation={id:'CANONICAL',title:'Reader',directory:'C:/reader',sessionDirectory:'C:/reader',model:{provider:'service',model:'model'},permission:'ask',createdAt:'2026-10-28T00:00:00Z',updatedAt:'2026-10-28T00:00:00Z'};
const bot:BotProfile={id:'reader',name:'Reader',description:'',avatar:'',avatarColor:'violet',enabled:true,status:'active',skills:[],createdAt:1,canonicalSessionId:'CANONICAL',
 sessions:[{id:'CANONICAL',title:'Reader',updatedAt:1,role:'canonical',kind:'chat',status:'active'}],
 capabilities:{model:'',effort:'',fastMode:false,harness:'pi',modelChain:[],skillMode:'allowlist',skillsExcluded:[],toolsetMode:'inherit',toolsets:[],mcpMode:'inherit',mcpServers:[],memory:true,permissions:'ask'}};
beforeEach(async()=>{
 // The page's three catalog reads share one Host read for two seconds; each
 // test starts on a later clock so an earlier test's catalog is never reused.
 vi.useFakeTimers({toFake:['Date']});vi.setSystemTime(clock+=60_000);
 await i18n.changeLanguage('en-US');fail=false;changes.length=0;pending=[];catalogs.length=0;
 // The page resolves a companion from its loaded roster by canonical session.
 registerLoadedBotProfilesReader(()=>[bot]);
 bridge.call.mockReset().mockImplementation(async(method:string,params:any)=>{
  if(method==='onebot.events')return params.cursor===Number.MAX_SAFE_INTEGER?{cursor:1,events:[],reset:true}:new Promise(resolve=>pending.push(resolve));
  if(method==='catalog')return {projects:[],conversations:[]};
  if(method==='onebot.chat.catalog')return {projects:[],conversations:[conversation]};
  if(method==='onebot.capabilities.get'){
   catalogs.push(params);
   if(fail)throw Error('Catalog unavailable');
   return {entries,defaultModelChain:[]};
  }
  // The teammate's own Skills are listed from its Home, not the shared catalog.
  if(method==='onebot.skills.list'){expect(params).toEqual({botId:'reader'});return [{slug:'own',name:'Personal skill',enabled:true}];}
  if(method==='onebot.modelChain.get')return {modelChain:[],isCustomized:false};
  if(method==='onebot.bots.list')return [];
  if(method==='ai.configuration.get')return {providers:[],tasks:{}};
  throw Error('Unexpected '+method);
 });
});
afterEach(async()=>{cleanup();vi.useRealTimers();registerLoadedBotProfilesReader(null);await act(async()=>{for(const finish of pending)finish({cursor:1,events:[],reset:false});});});
function Fixture(){
 const [capabilities,setCapabilities]=useState<BotCapabilities>({harness:'pi',model:'',effort:'',fastMode:false,modelChain:[],modelChainOverride:null,
  memory:true,permissions:'ask',skillMode:'allowlist',skillsExcluded:[],toolsetMode:'inherit',toolsets:[],mcpMode:'allowlist',mcpServers:['missing'],mcpServerNames:{missing:'Former server'}});
 const [skills,setSkills]=useState<string[]>([]);
 return <BotCapabilitySettings expanded bot={bot} capabilities={capabilities} skills={skills}
  onChange={(kind,values)=>{changes.push([kind,values]);if(kind==='skill')setSkills(values);else setCapabilities(current=>({...current,[kind==='mcp'?'mcpServers':'toolsets']:values,[kind==='mcp'?'mcpMode':'toolsetMode']:'allowlist'}));}}/>;
}
it('reads one catalog for the canonical chat and converts inherited toolsets into an explicit selection',async()=>{
 render(<Fixture/>);
 const alpha=await screen.findByRole('switch',{name:'Alpha tools'});
 expect(catalogs).toEqual([{botId:'reader'}]);
 // Built-in memory/helper toolsets are configured elsewhere, as in Cindy.
 expect(screen.queryByRole('switch',{name:'Memory'})).toBeNull();
 expect(alpha.getAttribute('aria-checked')).toBe('true');
 fireEvent.click(alpha);
 expect(changes).toEqual([['toolset',['beta']]]);
 expect(alpha.getAttribute('aria-checked')).toBe('false');
 expect(screen.getByRole('switch',{name:'Beta tools'}).getAttribute('aria-checked')).toBe('true');
});
it('keeps personal skills read-only, preserves removable missing references and selects shared skills',async()=>{
 render(<Fixture/>);
 const own=await screen.findByRole('switch',{name:'Personal skill'});
 fireEvent.click(own);
 expect(changes).toEqual([]);
 expect(own.getAttribute('aria-checked')).toBe('true');
 const missing=screen.getByRole('switch',{name:'Former server'});
 expect(missing.getAttribute('aria-checked')).toBe('true');
 fireEvent.click(missing);
 fireEvent.click(screen.getByRole('switch',{name:'shared-review'}));
 // Shared Skills are referenced by name, as Cindy resolves them.
 expect(changes).toEqual([['mcp',[]],['skill',['shared-review']]]);
});
it('reports catalog failures and retries without inventing an empty successful catalog',async()=>{
 fail=true;render(<Fixture/>);
 const retry=await screen.findByRole('button',{name:i18n.t('bots.retry')});
 expect(screen.queryByRole('switch',{name:'Alpha tools'})).toBeNull();
 expect(changes).toEqual([]);
 fail=false;fireEvent.click(retry);
 await screen.findByRole('switch',{name:'Alpha tools'});
 await waitFor(()=>expect(screen.queryByRole('button',{name:i18n.t('bots.retry')})).toBeNull());
});
