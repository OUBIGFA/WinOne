import type {ITerminalOptions} from '@xterm/xterm';
// Exact Cindy xterm preset. WinOne keeps its existing ConPTY ownership.
export const CINDY_TERMINAL_OPTIONS: ITerminalOptions = {
  // Codex 同款字号 / 字体;cursorBlink 跟 iTerm 默认行为对齐。
  fontFamily: '"SF Mono", Menlo, Monaco, "Cascadia Code", "Courier New", monospace',
  fontSize: 13,
  lineHeight: 1.2,
  cursorBlink: true,
  cursorStyle: 'block',
  // scrollback 默认 1000,放大到 5000 给 long-running 任务多一点回溯空间。
  scrollback: 5000,
  // 允许 OSC 8 超链接(WebLinksAddon 也会处理裸 URL)
  allowProposedApi: false,
  // 反色风格的暗黑主题。RSB 视觉是黑底白字。
  theme: {
    background: '#1c1c1c',
    foreground: '#e6e6e6',
    cursor: '#e6e6e6',
    cursorAccent: '#1c1c1c',
    selectionBackground: '#3a4a5a',
  },
};
