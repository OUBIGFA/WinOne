// Persistent host storage for the unchanged Cindy WorktreeManager. The owner
// lock of profiles.db serializes writers; metadata and session snapshots share
// the existing OneBot stores instead of Electron's global singleton.
import {oneBotOwnerScope} from './im-scope.mjs';
import {setWorktreePathInDb} from './task-host.mjs';

function db() {
  const {profiles} = oneBotOwnerScope();
  profiles.assertOpen();
  return profiles.db;
}
export function get(sessionId) {
  const row = db().query('SELECT data FROM bot_worktrees WHERE session_id=?').get(sessionId);
  return row ? JSON.parse(row.data) : null;
}
export function getAll() {
  return db().query('SELECT data FROM bot_worktrees').all().map(row => JSON.parse(row.data));
}
export async function set(sessionId, meta) {
  if (meta.sessionId !== sessionId) throw Error('Worktree session identity mismatch');
  db().query('INSERT OR REPLACE INTO bot_worktrees VALUES (?,?)').run(sessionId, JSON.stringify(meta));
  await setWorktreePathInDb(sessionId, meta.path);
}
