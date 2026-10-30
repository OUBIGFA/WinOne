// Hosts Cindy's complete automation page (features/scheduler, b7e46bc) inside
// WinOne Chat. Cindy's App supplies the confirm-dialog provider, the toast
// container and router navigation; WinOne supplies them here.
import './locales';
// Same token layers as Chat: Cindy defaults (base layer), then WinOne's
// theme-aware mapping, then Cindy component foundations.
import '../panel-ui/theme.css';
import '../theme.css';
import '../panel-ui/foundation.css';
import {useEffect} from 'react';
import {ConfirmDialogProvider} from '../panel-ui/confirm-dialog-provider';
import {ToastContainer} from '../components/ui/toast';
import {SchedulerPage} from '../scheduler/SchedulerPage';
import {ProjectAutomationNotifyBridge} from '../scheduler/components/ProjectAutomationNotifyBridge';
import type {AIProvider, AISelection} from '../../ai-configuration';
import {publishCindyHost, setCindyNavigation} from './store';

export function SchedulerHost({providers, defaultModel, onOpenConversation, onOpenAIServices, onOpenIMSettings}: {
  providers: AIProvider[] | null;
  defaultModel: AISelection | null;
  onOpenConversation: (id: string) => void;
  onOpenAIServices: () => void;
  onOpenIMSettings: () => void;
}) {
  useEffect(() => { publishCindyHost({providers, defaultModel}); }, [providers, defaultModel]);
  useEffect(() => setCindyNavigation({openConversation: onOpenConversation, openAIServices: onOpenAIServices, openIMSettings: onOpenIMSettings}), [onOpenConversation, onOpenAIServices, onOpenIMSettings]);
  return <ConfirmDialogProvider>
    <div className="cindy-chat-ui flex h-full min-h-0 w-full flex-col">
      <SchedulerPage/>
    </div>
    <ProjectAutomationNotifyBridge/>
    <ToastContainer/>
  </ConfirmDialogProvider>;
}
