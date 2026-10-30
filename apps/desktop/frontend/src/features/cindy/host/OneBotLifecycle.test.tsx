// @vitest-environment jsdom
import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import {cleanup, fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import {i18n} from '../../../i18n';
import {BotLifecycleSettings} from '../bots/BotLifecycleSettings';
import type {BotProfile} from '../bots/botStore';
import './onebotLocales';

const bridge=vi.hoisted(()=>({call:vi.fn()}));
vi.mock('../../../bridge',()=>({chatCall:bridge.call}));
afterEach(cleanup);
beforeEach(async()=>{vi.resetAllMocks();await i18n.changeLanguage('en-US');});
const bot:BotProfile={id:'reader',name:'Reader',description:'',avatar:'',avatarColor:'violet',enabled:true,status:'active',skills:[],createdAt:1,sessions:[],
 capabilities:{model:'',effort:'',fastMode:false,harness:'pi',modelChain:[],skillMode:'allowlist',skillsExcluded:[],toolsetMode:'inherit',toolsets:[],mcpMode:'inherit',mcpServers:[],memory:true,permissions:'ask'}};
const label=(key:string)=>i18n.t(key);

it('waits for the settings guard before restarting the same OneBot',async()=>{
 const beforeAction=vi.fn(async()=>false);
 bridge.call.mockResolvedValue({botId:'reader',action:'restart',status:'active'});
 render(<BotLifecycleSettings bot={bot} mode="actions" beforeAction={beforeAction} onOpenSession={()=>{}}/>);
 fireEvent.click(screen.getByRole('button',{name:label('bots.lifecycle.restart')}));
 await waitFor(()=>expect(beforeAction).toHaveBeenCalledOnce());
 expect(bridge.call).not.toHaveBeenCalled();
 beforeAction.mockResolvedValue(true);
 fireEvent.click(screen.getByRole('button',{name:label('bots.lifecycle.restart')}));
 await screen.findByText(label('bots.lifecycle.restarted'));
 expect(bridge.call).toHaveBeenCalledWith('onebot.lifecycle.action',{botId:'reader',action:'restart'});
});

it('preserves history and keeps a failed delete dialog open for retry',async()=>{
 const onDeleted=vi.fn();
 bridge.call.mockRejectedValueOnce(Error('[IO_ERROR] Unable to recycle')).mockResolvedValueOnce({botId:'reader',action:'delete',status:'deleted'});
 render(<BotLifecycleSettings bot={bot} mode="actions" onDeleted={onDeleted} onOpenSession={()=>{}}/>);
 fireEvent.click(screen.getByRole('button',{name:label('bots.lifecycle.deleteTitle')}));
 const dialog=await screen.findByRole('dialog');
 fireEvent.click(within(dialog).getByRole('button',{name:label('bots.lifecycle.delete')}));
 await screen.findByText(label('bots.lifecycle.deleteFailed'));
 expect(onDeleted).not.toHaveBeenCalled();
 expect(bridge.call).toHaveBeenCalledWith('onebot.lifecycle.action',{
  botId:'reader',action:'delete',confirmName:'Reader',keepTaskHistory:true,worktreeDisposition:'retain',
 });
 fireEvent.click(within(dialog).getByRole('button',{name:label('bots.lifecycle.delete')}));
 await waitFor(()=>expect(onDeleted).toHaveBeenCalledWith('reader'));
});

it('searches the OneBot history scope and passes the selected message to navigation',async()=>{
 const onOpenSession=vi.fn();
 bridge.call.mockResolvedValue({query:'receipt',results:[{session:{id:'history-1',title:'Receipt',updatedAt:'2026-10-26T08:00:00Z'},contentHit:{messageId:'message-1',messageClientId:'client-1',preview:'Read the receipt',createdAt:'2026-10-26T08:00:00Z'}}]});
 render(<BotLifecycleSettings bot={bot} mode="history" onOpenSession={onOpenSession}/>);
 fireEvent.change(screen.getByRole('textbox',{name:label('bots.historySearch.title')}),{target:{value:'receipt'}});
 fireEvent.click(screen.getByRole('button',{name:label('bots.historySearch.search')}));
 fireEvent.click(await screen.findByText('Read the receipt'));
 expect(bridge.call).toHaveBeenCalledWith('onebot.history.search',{botId:'reader',query:'receipt',limit:20});
 expect(onOpenSession).toHaveBeenCalledWith('history-1',{kind:'conversation-search',sessionId:'history-1',messageId:'message-1',messageClientId:'client-1'});
});
