// WinOne seam for Cindy utils/ipcError (b7e46bc). Host errors arrive as plain
// messages; Cindy's `[CODE] message` envelope is still decoded so vendored
// callers strip a protocol code when one is present.
export interface IpcError { code: string; message: string }
export {mapIpcErrorToI18nKey} from '../media/shared/ipcError';
const IPC_ERROR_MESSAGE_RE = /(?:^|: Error: )\[([A-Z0-9_]+)\]\s*([\s\S]*)$/;
export function extractIpcError(err: unknown): IpcError | null {
  if (!(err instanceof Error)) return null;
  const match = IPC_ERROR_MESSAGE_RE.exec(err.message);
  return match ? {code: match[1]!, message: match[2]!} : null;
}
