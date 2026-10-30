// WinOne seam for Cindy chat-surface pieces the OneBot pages embed (Cindy
// b7e46bc, Apache-2.0): the session view, provider onboarding and the IM
// default-route form. A teammate's conversation is WinOne's own Chat surface in
// the OneBot transport scope; Cindy's chat store and composer are not shipped.
import {useParams} from 'react-router-dom';
import {useTranslation} from 'react-i18next';
import {Sparkles} from 'lucide-react';
import {ChatPage} from '../../chat';
import {Button} from '../panel-ui/button';
import type {ComposerBotMention} from '../lib/fileTypes';
import type {BotChatBinding} from '../bots/botChatPresentation';
import {cindyNavigation} from './store';
import {useProviders} from './providers';

/** What a teammate's chat adds to an ordinary WinOne conversation. */
export interface OneBotChatBinding {
  identity: BotChatBinding;
  mentions: readonly ComposerBotMention[];
  invitationStage?: string;
  unreadBoundaryAt?: number | null;
  onReadThrough?: (at: number) => void;
  readOnly?: boolean;
  /** A private-chat topic beside the main chat: no header registration of its own. */
  topic?: boolean;
}

/** Cindy's CCAgentSessionView props that the OneBot pages pass. */
export interface OneBotSessionViewProps {
  sessionIdProp?: string;
  botMentions?: readonly ComposerBotMention[];
  botIdentity?: BotChatBinding;
  botInvitationStage?: string;
  botUnreadBoundaryAt?: number | null;
  onBotReadThrough?: (at: number) => void;
  readOnly?: boolean;
  compact?: boolean;
  compactToolbar?: boolean;
  viewVisible?: boolean;
  navigationMode?: string;
  sidebarTargetSessionId?: string;
}

export function CCAgentSessionView(props: OneBotSessionViewProps) {
  const params = useParams();
  const sessionId = props.sessionIdProp ?? params.sessionId;
  if (!sessionId) return null;
  const bot: OneBotChatBinding | undefined = props.botIdentity ? {
    identity: props.botIdentity,
    mentions: props.botMentions ?? [],
    invitationStage: props.botInvitationStage,
    unreadBoundaryAt: props.botUnreadBoundaryAt,
    onReadThrough: props.onBotReadThrough,
    readOnly: props.readOnly,
    topic: props.navigationMode === 'sidebar-embedded',
  } : undefined;
  return <ChatPage key={sessionId} requestedConversationId={sessionId} transportScope="onebot" embeddedConversation bot={bot}/>;
}

// hooks/useProviderOnboarding.ts: a teammate needs a configured AI service.
export function useProviderOnboarding(_options?: {dismissible?: boolean}): {visible: boolean} {
  const {providers, loading} = useProviders();
  return {visible: !loading && !providers.some(provider => provider.connected && Object.values(provider.models).some(models => !!models?.length))};
}

// components/onboarding/ConnectProviderCard.tsx: WinOne's models come from AI services.
export function ConnectProviderCard(_props: {dismissible?: boolean}) {
  const {t} = useTranslation();
  return <section className="flex w-full max-w-md flex-col items-center gap-3 rounded-xl border border-[var(--border-default)] bg-[var(--surface-elevated)] p-6 text-center">
    <Sparkles size={22} className="text-[var(--text-secondary)]" aria-hidden="true"/>
    <h2 className="text-16 font-medium text-[var(--text-primary)]">{t('onebot.connectAI.title')}</h2>
    <p className="text-13 leading-5 text-[var(--text-secondary)]">{t('onebot.connectAI.description')}</p>
    <Button variant="cta" size="lg" compact type="button" onClick={() => cindyNavigation().openAIServices()}>{t('onebot.connectAI.action')}</Button>
  </section>;
}

// components/settings/ImDefaultSettingsSection.tsx: WinOne binds personal WeChat
// to a OneBot teammate only, so the companion-less default route form never shows.
export type ImDefaultSettingsSummary = {agentKind: string; model: string};
export function ImDefaultSettingsSection(_props: {channel: string; embedded?: boolean; onSummaryChange?: (summary: ImDefaultSettingsSummary | null) => void}): null {
  return null;
}
