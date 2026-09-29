export interface Settings {
  schemaVersion: number;
  revision: number;
  theme: "system" | "light" | "dark";
}
export interface Module {
  id: string;
  name: string;
  state: string;
  detail: string;
}
export interface Snapshot {
  settings: Settings;
  modules: Module[];
}

interface NativeApp {
  Snapshot(): Promise<Snapshot>;
  SaveSettings(value: Settings): Promise<Settings>;
  WinTray(method: string, payload: string): Promise<string>;
  PickProgram(): Promise<string>;
}
declare global {
  interface Window {
    go?: { main?: { App?: NativeApp } };
  }
}

// A browser preview must not claim the Windows backend is connected.
function native(): NativeApp {
  const api = typeof window === "undefined" ? undefined : window.go?.main?.App;
  if (!api) throw new Error("请从 WinOne 桌面程序打开");
  return api;
}
export async function getSnapshot(): Promise<Snapshot> {
  return native().Snapshot();
}
export async function saveSettings(value: Settings): Promise<Settings> {
  return native().SaveSettings(value);
}

export async function wintrayCall<T>(
  method: string,
  params: unknown = null,
): Promise<T> {
  return JSON.parse(
    await native().WinTray(method, JSON.stringify(params)),
  ) as T;
}
export async function pickProgram(): Promise<string> {
  return native().PickProgram();
}
