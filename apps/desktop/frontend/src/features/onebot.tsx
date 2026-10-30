// OneBot: Cindy's teammates feature (renderer/features/bots, Cindy b7e46bc,
// Apache-2.0) as a WinOne page. The page owns one memory router with Cindy's
// `/bots` route tree, the teammate list that Cindy shows in its task sidebar,
// and the content header slot that Cindy's title bar provides. main.tsx loads
// this module on first use: the teammate store starts reading profiles when
// its module loads, so WinOne's startup never starts the OneBot process.
import './onebot.css';
import './cindy/host/locales';
import './cindy/host/onebotLocales';
import './cindy/panel-ui/theme.css';
import './cindy/theme.css';
import './cindy/panel-ui/foundation.css';
import {useEffect, useMemo, useState} from 'react';
import {createMemoryRouter, Navigate, Outlet, RouterProvider, useNavigate, useParams, useRouteError, useSearchParams, type RouteObject} from 'react-router-dom';
import {useTranslation} from 'react-i18next';
import {CircleAlert} from 'lucide-react';
import {captureCall, chatCall} from '../bridge';
import type {AIConfiguration, AISelection} from './ai-configuration';
import type {ChatCatalog} from './chat-state';
import {ConfirmDialogProvider} from './cindy/panel-ui/confirm-dialog-provider';
import {ToastContainer} from './cindy/components/ui/toast';
import {Button} from './cindy/panel-ui/button';
import {publishCindyHost, setCindyNavigation} from './cindy/host/store';
import {ContentHeaderSlot, MainViewHistoryProvider} from './cindy/host/onebotShell';
import {CCAgentSessionView} from './cindy/host/onebotUI';
import {readLoadedBotProfiles} from './cindy/bots/botSessionOwners';
import {BotsFeatureLayout} from './cindy/bots/BotsFeatureLayout';
import {BotsHomeView} from './cindy/bots/BotsHomeView';
import {BotsListView} from './cindy/bots/BotsListView';
import {BotRosterView} from './cindy/bots/BotRosterView';
import {BotMarketRoute} from './cindy/bots/BotMarketDialog';
import {BotDirectMessageView} from './cindy/bots/BotDirectMessageView';
import {BotGroupChatView} from './cindy/bots/BotGroupChatView';
import {hydrateVoiceData} from './cindy/host/onebotVoiceInput';
import {BotSessionView} from './cindy/bots/BotSessionView';
import {BotHistorySessionView} from './cindy/bots/BotHistorySessionView';
import {TeammatesSection} from './cindy/bots/TeammatesSection';
import {useBotUnreadSync} from './cindy/bots/useBotUnreadSync';

/** The model Chat's settings route a conversation to (Host `Configuration.Selected("chat")`). */
function chatTaskModel(configuration: AIConfiguration): AISelection | null {
  const selection = 'chat' in configuration.tasks ? configuration.tasks.chat : configuration.tasks.default;
  return selection?.provider && selection.model ? selection : null;
}

/** The OneBot page's data for the Cindy hooks: AI services, Chat's default model and the Chat catalog. */
function useHostData(): {ready: boolean; error: string; retry: () => void} {
  const [state, setState] = useState<{ready: boolean; error: string}>({ready: false, error: ''});
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let alive = true;
    void Promise.all([
      captureCall<AIConfiguration>('ai.configuration.get'),
      chatCall<{defaultThinkingLevel?: string}>('settings.get'),
      chatCall<ChatCatalog>('catalog'),
      hydrateVoiceData(),
    ]).then(([configuration, preferences, catalog]) => {
      if (!alive) return;
      publishCindyHost({providers: configuration.providers, defaultModel: chatTaskModel(configuration), defaultThinkingLevel: preferences.defaultThinkingLevel ?? '', catalog});
      setState({ready: true, error: ''});
    }).catch(error => {
      if (alive) setState({ready: false, error: error instanceof Error ? error.message : String(error)});
    });
    return () => { alive = false; };
  }, [attempt]);
  return {...state, retry: () => setAttempt(value => value + 1)};
}

/** Cindy's MainLayout pieces around the bots routes: the teammate list, the header bar and unread sync. */
function OneBotShell() {
  useBotUnreadSync();
  const {t} = useTranslation();
  const navigate=useNavigate();
  useEffect(()=>{const open=(event:Event)=>{const value=(event as CustomEvent<{botId?:string;sessionId?:string;page?:string}>).detail;if(value.botId&&value.sessionId)navigate('/bots/'+encodeURIComponent(value.botId)+'/session/'+encodeURIComponent(value.sessionId)+'?settings=1&settingsPage='+encodeURIComponent(value.page??'home'));};window.addEventListener('winone-onebot-settings',open);return()=>window.removeEventListener('winone-onebot-settings',open);},[navigate]);
  return <div className="onebot-page flex h-full min-h-0 w-full bg-[var(--surface)]">
    <aside aria-label={t('onebot.teammates')} className="flex w-[264px] shrink-0 flex-col overflow-y-auto border-r border-[var(--border-default)] bg-[var(--sidebar-bg,var(--surface))] px-2 py-2">
      <TeammatesSection/>
    </aside>
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <MainViewHistoryProvider>
        <ContentHeaderSlot render={header => header ? <header className="flex h-12 shrink-0 items-center border-b border-[var(--border-default)] px-3">{header}</header> : null}>
          <div className="min-h-0 min-w-0 flex-1"><Outlet/></div>
        </ContentHeaderSlot>
      </MainViewHistoryProvider>
    </div>
  </div>;
}

/**
 * Cindy's `/cc-agent/:sessionId`: a OneBot conversation (an IM lane, a topic
 * or history) opens here as an ordinary task view; any other task is a WinOne
 * Chat conversation, such as a teammate's delegated background task.
 */
function TaskRoute() {
  const {sessionId = ''} = useParams();
  const navigate = useNavigate();
  const {t} = useTranslation();
  const owned = (readLoadedBotProfiles() ?? []).some(bot => bot.sessions.some(session => session.id === sessionId));
  useEffect(() => {
    if (owned || !sessionId) return;
    window.dispatchEvent(new CustomEvent('winone-open-chat', {detail: {id: sessionId}}));
    navigate(-1);
  }, [navigate, owned, sessionId]);
  if (owned) return <CCAgentSessionView sessionIdProp={sessionId}/>;
  return <p className="p-6 text-13 text-[var(--text-secondary)]" role="status">{t('onebot.openInChat')}</p>;
}

/** Cindy's settings routes: AI services own providers; Chat settings own tools, MCP servers and Skills. */
function SettingsRoute() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const tab = params.get('tab');
  useEffect(() => {
    window.dispatchEvent(new CustomEvent('winone-open-page', {detail: tab === 'providers' ? {id: 'ai-services'} : {id: 'chat', section: 'settings'}}));
    navigate(-1);
  }, [navigate, tab]);
  return null;
}

function RouteError() {
  const error = useRouteError();
  const {t} = useTranslation();
  return <section className="m-6 flex max-w-lg flex-col gap-3 rounded-xl border border-[var(--border-default)] bg-[var(--surface-elevated)] p-5" role="alert">
    <CircleAlert size={20} className="text-[var(--text-danger)]" aria-hidden/>
    <h2 className="text-15 font-medium text-[var(--text-primary)]">{t('onebot.loadFailed')}</h2>
    <p className="break-words text-12 text-[var(--text-secondary)] [overflow-wrap:anywhere]">{error instanceof Error ? error.message : String(error)}</p>
  </section>;
}

// Cindy's router subtree for teammates (router.tsx), local routes only:
// other computers were removed with Cindy cloud.
const routes: RouteObject[] = [{
  element: <OneBotShell/>,
  errorElement: <RouteError/>,
  children: [
    {index: true, element: <Navigate to="/bots" replace/>},
    {
      path: 'bots',
      element: <BotsFeatureLayout/>,
      children: [
        {index: true, element: <BotsHomeView/>},
        {path: 'list', element: <BotsListView/>},
        {path: 'roster', element: <BotRosterView/>},
        {path: 'market/:packageId?', element: <BotMarketRoute/>},
        {path: 'groups/:groupId', element: <BotGroupChatView/>},
        {path: ':botId/direct/:threadId', element: <BotDirectMessageView/>},
        {path: ':botId', element: <BotsHomeView/>},
        {path: ':botId/session/:sessionId', element: <BotSessionView/>},
        {path: ':botId/history/:sessionId', element: <BotHistorySessionView/>},
      ],
    },
    {path: 'cc-agent/:sessionId', element: <TaskRoute/>},
    {path: 'settings', element: <SettingsRoute/>},
    {path: '*', element: <Navigate to="/bots" replace/>},
  ],
}];

export function OneBotPage() {
  const {t} = useTranslation();
  const host = useHostData();
  const router = useMemo(() => createMemoryRouter(routes, {initialEntries: ['/bots']}), []);
  useEffect(() => setCindyNavigation({
    openConversation: id => window.dispatchEvent(new CustomEvent('winone-open-chat', {detail: {id}})),
    openAIServices: () => window.dispatchEvent(new CustomEvent('winone-open-page', {detail: {id: 'ai-services'}})),
    openIMSettings: () => undefined,
  }), []);
  useEffect(() => () => router.dispose(), [router]);
  if (!host.ready) {
    return <div className="flex h-full items-center justify-center p-6" role="status">
      {host.error ? <div className="flex max-w-md flex-col items-center gap-3 text-center">
        <p className="text-13 text-[var(--text-secondary)]">{t('onebot.loadFailed')}</p>
        <p className="break-words text-12 text-[var(--text-danger)] [overflow-wrap:anywhere]">{host.error}</p>
        <Button variant="secondary" size="sm" compact type="button" onClick={host.retry}>{t('onebot.retry')}</Button>
      </div> : <p className="text-13 text-[var(--text-secondary)]">{t('onebot.loading')}</p>}
    </div>;
  }
  return <ConfirmDialogProvider>
    <div className="cindy-chat-ui flex h-full min-h-0 w-full flex-col">
      <RouterProvider router={router}/>
    </div>
    <ToastContainer/>
  </ConfirmDialogProvider>;
}
