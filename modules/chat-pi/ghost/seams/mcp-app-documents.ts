// Electron's document registry is replaced by the native WebView lifetime.
// The reused runtime still owns caller cleanup; no guest receives these hooks.
let clearDocuments: (() => void) | undefined;
let cleanupCaller: ((caller: string) => Promise<void>) | undefined;

export function setMcpAppCallerCleanup(cleanup?: (caller: string) => Promise<void>): void {
  cleanupCaller = cleanup;
}
export function clearMcpAppDocuments(): void { clearDocuments?.(); }
export function configureNativeMcpAppDocuments(clear: () => void): () => void {
  clearDocuments = clear;
  return () => { if (clearDocuments === clear) clearDocuments = undefined; };
}
export async function releaseNativeMcpAppCaller(caller: string): Promise<void> {
  await cleanupCaller?.(caller);
}
