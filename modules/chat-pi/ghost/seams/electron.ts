// The Electron surface vendored plugin code reads in the main process. A plugin's
// Node worker runs in Bun with Node APIs; Electron's utilityProcess channel maps
// onto a child's IPC channel, which the worker bootstrap sees as process.parentPort.
import {spawn} from 'node:child_process';
import {EventEmitter} from 'node:events';
import os from 'node:os';
import {ghostDataRoot} from './app-session.ts';

let runtimeExecutable = process.execPath;
let nodeWorkerScript = '';
let openExternalHandler: (url: string) => Promise<void> = async () => { throw new Error('Opening links is unavailable'); };
export function configureGhostElectron(options: {runtime?: string; nodeWorker?: string; openExternal?: (url: string) => Promise<void>}): void {
  if (options.nodeWorker) nodeWorkerScript = options.nodeWorker;
  if (options.runtime) runtimeExecutable = options.runtime;
  if (options.openExternal) openExternalHandler = options.openExternal;
}

export const app = {
  isPackaged: true,
  getPath(name: string): string {
    if (name === 'temp') return os.tmpdir();
    return ghostDataRoot();
  },
  getLocale(): string { return 'zh-CN'; },
  getVersion(): string { return '0.0.0'; },
};

export const shell = {openExternal: (url: string) => openExternalHandler(url)};

class WorkerProcess extends EventEmitter {
  pid?: number;
  stdout: NodeJS.ReadableStream | null;
  stderr: NodeJS.ReadableStream | null;
  private child: ReturnType<typeof spawn>;
  constructor(entry: string, args: string[], options: {cwd?: string; env?: NodeJS.ProcessEnv; stdio?: unknown}) {
    super();
    this.child = spawn(runtimeExecutable, [nodeWorkerScript || entry, ...args], {cwd: options.cwd, env: {...(options.env ?? process.env), WINONE_PLUGIN_WORKER: '1'}, stdio: ['ignore', 'pipe', 'pipe', 'ipc'], windowsHide: true});
    this.pid = this.child.pid;
    this.stdout = this.child.stdout;
    this.stderr = this.child.stderr;
    this.child.on('spawn', () => this.emit('spawn'));
    this.child.on('message', message => this.emit('message', message));
    this.child.on('error', error => this.emit('error', 'FatalError', entry, String(error)));
    this.child.on('exit', code => this.emit('exit', code ?? 0));
  }
  postMessage(message: unknown): void { this.child.send?.(message as object); }
  kill(): boolean { return this.child.kill(); }
}

export const utilityProcess = {
  fork(entry: string, args: string[] = [], options: {cwd?: string; env?: NodeJS.ProcessEnv; stdio?: unknown} = {}) { return new WorkerProcess(entry, args, options); },
};
