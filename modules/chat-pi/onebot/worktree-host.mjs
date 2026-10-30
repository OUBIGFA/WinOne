// Electron's app paths are scoped to this OneBot owner, including legacy lease
// discovery. A WinOne worker must never read Cindy's real profile or lease store.
import fs from 'node:fs/promises';
import path from 'node:path';
import {oneBotOwnerScope} from './im-scope.mjs';

export const app = {
  getPath(name) {
    const root = oneBotOwnerScope().paths.root;
    if (name === 'userData') return root;
    if (name === 'appData') return path.join(root, 'runtime');
    throw Error('Unsupported worktree application path: ' + name);
  },
};
// Cindy's probe scheduler already bounds concurrent directory probes. There is
// no separate Electron utility process in the Bun worker.
export const realpathWorkingDirectory = directory => fs.realpath(directory);
