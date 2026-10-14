// Unmodified upstream BrowserPanel, b5ea2bd312e9e9c007b84859d550314ddae4acb5, MIT.
import { useState } from 'react';
import {
  ArrowLeft,
  ArrowRight,
  ArrowClockwise,
  ArrowsOutSimple,
  ArrowsInSimple,
  DotsThree,
  Folder,
  Globe,
  ListBullets,
  Plus,
  PlusSquare,
  Square,
  X,
  Link,
  Files,
  Terminal,
  NotePencil,
  ChatCircle,
  DownloadSimple,
  UploadSimple,
  SidebarSimple,
} from '@phosphor-icons/react';
import { Button, IconButton, DesktopMenu, InternalScrollArea, Input, ListStack, Tooltip } from 'chatgpt-desktop-kit';
import { Popover, PopoverTrigger, PopoverContent } from 'chatgpt-desktop-kit/compat/popover';
export function BrowserPanel({
  full,
  onFull,
  onClose,
  onInfo,
}: {
  full: boolean;
  onFull: () => void;
  onClose: () => void;
  onInfo: (title: string, body: string) => void;
}) {
  const [address, setAddress] = useState('');
  const unavailable = (title: string) => onInfo(title, 'Unavailable in this demo.');
  return (
    <aside
      className={`client-right-panel${full ? ' client-right-panel--full' : ''}`}
      aria-label="Browser pane"
      data-desktop-surface="client.browser"
    >
      <div className="client-tabbar">
        <span className="client-browser-tab">
          <Globe size={14} />
          New tab
          <IconButton aria-label="Close tab" actionId="client.tab.close" onClick={onClose}>
            <X size={12} />
          </IconButton>
        </span>
        <IconButton aria-label="Add tab" actionId="client.tab.add" disabled>
          <Plus size={16} />
        </IconButton>
        <div className="client-spacer" />
        <Tooltip label={full ? 'Restore pane' : 'Expand pane'}>
          <IconButton
            aria-label={full ? 'Restore pane' : 'Expand pane'}
            actionId="client.pane.expand"
            onClick={onFull}
          >
            {full ? <ArrowsInSimple size={14} /> : <ArrowsOutSimple size={14} />}
          </IconButton>
        </Tooltip>
        <Tooltip label="Hide tabs">
          <IconButton aria-label="Hide browser pane" actionId="client.pane.hide" onClick={onClose}>
            <SidebarSimple size={16} />
          </IconButton>
        </Tooltip>
      </div>
      <div className="client-browserbar">
        <div className="client-browser-history">
          <IconButton aria-label="Browser back" actionId="client.browser.back" disabled>
            <ArrowLeft size={15} />
          </IconButton>
          <IconButton aria-label="Browser forward" actionId="client.browser.forward" disabled>
            <ArrowRight size={15} />
          </IconButton>
          <IconButton
            aria-label="Refresh tab"
            actionId="client.browser.refresh"
            onClick={() => setAddress('')}
          >
            <ArrowClockwise size={16} />
          </IconButton>
        </div>
        <Input
          aria-label="Search or enter a URL"
          placeholder="Search or enter a URL"
          value={address}
          onChange={(e) => setAddress(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && address.trim()) unavailable('Open website');
          }}
        />
        <IconButton
          aria-label="Browser side chat"
          actionId="client.tab.tool.side-chat"
          onClick={() => unavailable('Side chat')}
        >
          <ChatCircle size={16} />
        </IconButton>
        <DesktopMenu
          align="end"
          trigger={
            <Button
              className="desktop-icon-control"
              size="sm"
              variant="ghost"
              iconOnly
              icon={<DotsThree size={18} />}
              actionId="client.browser.actions.open"
              data-desktop-action="client.browser.actions.open"
            >
              Browser actions
            </Button>
          }
          items={[
            { id: 'client.browser.downloads', label: 'Downloads', icon: <DownloadSimple />, disabled: true },
            { id: 'client.browser.share', label: 'Share', icon: <UploadSimple />, disabled: true },
          ]}
        />
      </div>
      <InternalScrollArea>
        <div className="client-new-tab">
          <h2>Tools</h2>
          <div className="client-tools-grid">
            {[
              { id: 'changes', label: 'Changes', icon: PlusSquare, shortcut: 'Ctrl+Shift+G' },
              { id: 'terminal', label: 'Terminal', icon: Terminal, shortcut: 'Ctrl+`' },
              { id: 'files', label: 'Files', icon: Files, shortcut: 'Ctrl+P' },
              { id: 'new-page', label: 'New page', icon: NotePencil, shortcut: '' },
            ].map((item) => (
              <Button
                key={item.id}
                actionId={`client.tab.tool.${item.id}`}
                icon={<item.icon size={15} />}
                onClick={() => unavailable(item.label)}
              >
                {item.label}
                {item.shortcut && <kbd>{item.shortcut}</kbd>}
              </Button>
            ))}
          </div>
          <h2>Suggested</h2>
          <div className="client-suggested">
            {['Design review', 'Build pipeline', 'Release checklist', 'Team workspace'].map((label) => (
              <Button key={label} actionId="client.tab.suggested.open" onClick={() => unavailable(label)}>
                <Globe size={28} />
                <span>{label}</span>
              </Button>
            ))}
          </div>
        </div>
      </InternalScrollArea>
    </aside>
  );
}
