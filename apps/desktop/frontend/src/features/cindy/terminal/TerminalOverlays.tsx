import {AlertTriangle,RotateCw} from 'lucide-react';
import {useTranslation} from 'react-i18next';
import {Button} from '../panel-ui/button';
import {Spinner} from '../panel-ui/spinner';
// Cindy's complete exited/error overlays; WinOne supplies retry and PTY state.
export function ExitedOverlay({
  exit,
  restarting,
  onRestart,
  t,
}: {
  exit: { code: number | null; signal: string | null };
  restarting: boolean;
  onRestart: () => void;
  t: ReturnType<typeof useTranslation>['t'];
}) {
  const message = exit.signal
    ? t('rightSidebar.terminal.processKilled', { signal: exit.signal })
    : t('rightSidebar.terminal.processExited', { code: exit.code ?? 0 });
  return (
    <div className="pointer-events-none absolute inset-0 flex items-end justify-center pb-6">
      <div className="pointer-events-auto flex items-center gap-3 rounded-md border border-white/15 bg-black/80 px-4 py-2 text-sm text-white shadow-lg backdrop-blur">
        <span>{message}</span>
        <Button
          variant="secondary"
          size="xs"
          compact
          loading={restarting}
          type="button"
          onClick={onRestart}
          disabled={restarting}
        >
          <Spinner icon={RotateCw} size={12} spinning={restarting} />
          {t('rightSidebar.terminal.restart')}
        </Button>
      </div>
    </div>
  );
}

export function ErrorOverlay({
  message,
  restarting,
  onRetry,
  t,
}: {
  message: string;
  restarting: boolean;
  onRetry: () => void;
  t: ReturnType<typeof useTranslation>['t'];
}) {
  return (
    <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
      <div className="pointer-events-auto flex max-w-[90%] flex-col items-center gap-2 rounded-md border border-white/15 bg-black/80 px-4 py-3 text-sm text-white shadow-lg backdrop-blur">
        <div className="flex items-center gap-2 text-red-300">
          <AlertTriangle size={14} />
          <span>{message}</span>
        </div>
        <Button
          variant="secondary"
          size="xs"
          compact
          loading={restarting}
          type="button"
          onClick={onRetry}
          disabled={restarting}
        >
          <Spinner icon={RotateCw} size={12} spinning={restarting} />
          {t('rightSidebar.terminal.restart')}
        </Button>
      </div>
    </div>
  );
}
