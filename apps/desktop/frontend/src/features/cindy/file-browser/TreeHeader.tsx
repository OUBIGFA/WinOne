import {useTranslation} from 'react-i18next';
import {Search,ChevronsDownUp,RefreshCw,X as XIcon} from 'lucide-react';
import {Tip} from '../panel-ui/tooltip';
import {cn} from '../adapter';
type TreeMode='tree'|'search';
export function TreeHeader({
  workdir,
  mode,
  onToggleSearch,
  onCollapseAll,
  onRefresh,
}: {
  workdir: string;
  mode: TreeMode;
  onToggleSearch: () => void;
  onCollapseAll: () => void;
  onRefresh: () => void;
}) {
  const { t } = useTranslation();
  // workdir basename:POSIX 用最后一段(/Users/sam/Documents/Cindy → Cindy);
  // Windows 'C:\\Users\\sam\\Cindy' 也按 / 和 \ 切。空值兜底空串。
  const displayName = workdir.split(/[/\\]/).filter(Boolean).pop() ?? '';

  return (
    <div className="flex shrink-0 items-center justify-between pt-2 pb-1 pl-3 pr-2">
      <span className="truncate text-sm font-semibold text-foreground" title={workdir}>
        {displayName}
      </span>
      <div className="flex shrink-0 items-center gap-1.5">
        {mode === 'search' ? (
          // search 模式:refresh / collapse 只对文件树有意义,搜索时不显示;只留 X
          // 退出搜索回到 tree 模式(替代 search 按钮位置,跟 doc 模式同 ergonomics)。
          <Tip text={t('ccAgent.workdirBrowse.searchPanel.exit')}>
            <button
              type="button"
              onClick={onToggleSearch}
              className="flex size-5 items-center justify-center rounded-md text-sidebar-action-icon hover:bg-sidebar-item-active hover:text-sidebar-item-active-foreground"
            >
              <XIcon size={14} strokeWidth={2} />
            </button>
          </Tip>
        ) : (
          <>
            <Tip text={t('ccAgent.workdirBrowse.searchPanel.searchFiles')}>
              <button
                type="button"
                onClick={onToggleSearch}
                className="flex size-5 items-center justify-center rounded-md text-sidebar-action-icon hover:bg-sidebar-item-active hover:text-sidebar-item-active-foreground"
              >
                <Search size={14} strokeWidth={2} />
              </button>
            </Tip>
            <Tip text={t('ccAgent.workdirBrowse.treeAction.collapseAll')}>
              <button
                type="button"
                onClick={onCollapseAll}
                className="flex size-5 items-center justify-center rounded-md text-sidebar-action-icon hover:bg-sidebar-item-active hover:text-sidebar-item-active-foreground"
              >
                <ChevronsDownUp size={14} strokeWidth={2} />
              </button>
            </Tip>
            <Tip text={t('ccAgent.workdirBrowse.treeAction.refresh')}>
              <button
                type="button"
                onClick={onRefresh}
                className="flex size-5 items-center justify-center rounded-md text-sidebar-action-icon hover:bg-sidebar-item-active hover:text-sidebar-item-active-foreground"
              >
                <RefreshCw size={14} strokeWidth={2} />
              </button>
            </Tip>
          </>
        )}
      </div>
    </div>
  );
}

/**
 * 文件树 root 加载失败占位。device-too-old = 对方设备版本过旧(老被控端没有
 * remote-op channel,能力全有全无),提示升级;其余给通用失败 + 重试。
 */
