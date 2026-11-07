// Keep Cindy's cwd/parent-identity snapshot worker intact. Electron's utility
// process transport is replaced with a private Bun IPC child in the same
// Windows job as the Host-owned plugin worker.
import path from 'node:path';
import {fork} from 'node:child_process';
import {fileURLToPath} from 'node:url';
let workerFile;
export function setGhostSnapshotWorker(file) { workerFile = file; }
export function mutateGhostSnapshotWithStableParent(request) {
  const file = workerFile ?? fileURLToPath(new URL('./snapshot-worker.mjs', import.meta.url));
  // Original Electron IPC preserves bigint file identities. JSON-mode fork
  // drops that contract (or throws); Node/Bun's advanced IPC preserves it.
  const child = fork(file, [], {execPath: process.execPath, cwd: request.parentDir, serialization: 'advanced', stdio: ['ignore', 'ignore', 'pipe', 'ipc']});
  return new Promise((resolve, reject) => {
    let settled = false, ready = false;
    const finish = error => {
      if (settled) return;
      settled = true; clearTimeout(timer); child.kill();
      error ? reject(error) : resolve();
    };
    const timer = setTimeout(() => finish(Error('ghost snapshot worker timed out')), 30_000);
    timer.unref?.();
    child.on('message', message => {
      if (!message || typeof message !== 'object') return;
      if (!ready && message.type === 'ready') {
        ready = true;
        const {parentDir: _parentDir, ...workerRequest} = request;
        try {
          child.send({type: 'mutate', request: workerRequest}, error => { if (error) finish(error); });
        } catch (error) { finish(error); }
      } else if (message.ok === true) finish();
      else if (message.ok === false) finish(Error(String(message.message ?? 'ghost snapshot worker failed')));
    });
    child.on('error', finish);
    child.on('exit', code => { if (!settled) finish(Error(`ghost snapshot worker exited (${code})`)); });
  });
}
