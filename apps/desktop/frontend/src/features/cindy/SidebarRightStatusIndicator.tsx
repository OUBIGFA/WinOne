import { CircleCheck, CircleAlert, CircleStop, MessageCircleQuestion, Pause } from 'lucide-react';
import { useTranslation } from '../../i18n';
import { Spinner } from './panel-ui/spinner';
import { cn } from './adapter';
import type { SidebarRightStatusKind } from './sidebarRightStatus';

/**
 * 会话行的右侧状态指示器。文字模式与列表模式共用这一份
 * 颜色、文案和选中反白规则，各宿主只决定定位。
 */
export function SidebarRightStatusIndicator({
  kind,
  isActive,
  className,
}: {
  kind: Exclude<SidebarRightStatusKind, 'time'>;
  isActive: boolean;
  className?: string;
}) {
  const { t } = useTranslation();

  if (kind === 'running') {
    return (
      <span data-sidebar-right-status={kind} role="img" className={cn('inline-flex size-4 items-center justify-center',className)} aria-label={t('chat.sidebarRunning')} title={t('chat.sidebarRunning')}>
        <Spinner size={13} strokeWidth={2} aria-hidden/>
      </span>
    );
  }

  const label = t(kind==='error'?'chat.sidebarError':kind==='awaiting'?'chat.sidebarAwaiting':kind==='paused'?'chat.taskPaused':kind==='interrupted'?'chat.sidebarInterrupted':'chat.sidebarDone');
  // Keep Cindy's status slot and priority. The Host supplies the requested
  // blue/red/orange semantic colors in both selected and unselected rows.
  const Icon=kind==='error'?CircleAlert:kind==='awaiting'?MessageCircleQuestion:kind==='paused'?Pause:kind==='interrupted'?CircleStop:CircleCheck;

  return (
    <span
      data-sidebar-right-status={kind}
      role="img"
      className={cn('inline-flex size-4 items-center justify-center', className)}
      aria-label={label}
      title={label}
    >
      <Icon size={13} strokeWidth={1.75} aria-hidden/>
    </span>
  );
}
