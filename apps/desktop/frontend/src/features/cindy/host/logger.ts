// WinOne seam for Cindy lib/logger (b7e46bc). Renderer diagnostics go to the
// WebView console, which WinOne's desktop host already collects.
type Log = (...args: unknown[]) => void;
export interface Logger { trace: Log; debug: Log; info: Log; warn: Log; error: Log; fatal: Log }
export function createLogger(scope: string): Logger {
  const tag = `[${scope}]`;
  return {
    trace: () => {},
    debug: () => {},
    info: (...args) => console.info(tag, ...args),
    warn: (...args) => console.warn(tag, ...args),
    error: (...args) => console.error(tag, ...args),
    fatal: (...args) => console.error(tag, ...args),
  };
}
