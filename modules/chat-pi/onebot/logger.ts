// OneBot workers reserve stdout for the Host protocol. Reused Cindy services
// report diagnostics through the worker sink (stderr before it is installed).
export type OneBotDiagnostic = {
  level: 'debug' | 'info' | 'warn' | 'error';
  scope: string;
  message: string;
  detail: unknown[];
};
let sink = (event: OneBotDiagnostic): void => {
  console.error(`[OneBot:${event.scope}] ${event.message}`, ...event.detail);
};

export function setOneBotDiagnosticSink(next: typeof sink): () => void {
  const previous = sink;
  sink = next;
  return () => { if (sink === next) sink = previous; };
}

export function createLogger(scope: string) {
  const write = (level: OneBotDiagnostic['level']) => (message: string, ...detail: unknown[]) =>
    sink({level, scope, message, detail});
  return {debug: write('debug'), info: write('info'), warn: write('warn'), error: write('error')};
}
