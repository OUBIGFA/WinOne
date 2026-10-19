// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {act,cleanup,render,screen} from '@testing-library/react';
import {ChatResourcesPanel} from './chat-resources';
const host=vi.hoisted(()=>({call:vi.fn()}));
vi.mock('../bridge',()=>({chatCall:host.call}));
afterEach(()=>{cleanup();vi.clearAllMocks();});
it('does not claim that skills are missing before resource discovery finishes',async()=>{
 let finish!:(value:unknown)=>void;host.call.mockImplementation(()=>new Promise(resolve=>{finish=resolve;}));
 render(<ChatResourcesPanel id='resources' onOpenFile={()=>{}}/>);
 expect(screen.queryByText('未发现资源')).toBeNull();
 await act(async()=>finish({agents:[],skills:[{name:'discovered-skill',description:'Shared skill',path:'C:/fixture/SKILL.md'}],plugins:[],mcpServers:[]}));
 expect(screen.getByRole('button',{name:/discovered-skill/})).toBeTruthy();
});
