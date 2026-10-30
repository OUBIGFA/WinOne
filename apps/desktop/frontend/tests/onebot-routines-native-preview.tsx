// Original production components on the real native IPC. This fixture supplies
// no fake database, scheduler, model result or alternate implementation.
import {createRoot} from 'react-dom/client';
import {i18n, setApplicationLanguage} from '../src/i18n';
import {BotRoutines} from '../src/features/cindy/bots/BotRoutines';
import {ConfirmDialogProvider} from '../src/features/cindy/panel-ui/confirm-dialog-provider';
import {ToastContainer} from '../src/features/cindy/components/ui/toast';
import '../src/features/cindy/host/locales';
import '../src/features/cindy/host/onebotLocales';
import '../src/styles/shadcn.css';
import '../src/style.css';
import '../src/features/cindy/panel-ui/theme.css';
import '../src/features/cindy/theme.css';
import '../src/features/cindy/panel-ui/foundation.css';

const query = new URLSearchParams(location.search);
const botId = query.get('botId');
if (!botId) throw Error('An isolated test teammate id is required');
await setApplicationLanguage('en-US');
document.documentElement.dataset.theme = 'light';
Object.assign(window, {oneBotRoutineFixture: {
  t: (key: string) => i18n.t(key),
  async appearance(language: string, theme: string) {
    await setApplicationLanguage(language);
    document.documentElement.dataset.theme = theme;
  },
}});
createRoot(document.getElementById('root')!).render(<ConfirmDialogProvider>
  <main className="cindy-chat-ui h-full w-full overflow-y-auto p-4" data-onebot-routines-fixture>
    <BotRoutines botId={botId}/>
  </main>
  <ToastContainer/>
</ConfirmDialogProvider>);
