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
export function getAllPaths() {
  return getAll().flatMap(meta=>meta.quarantinePath?[meta.path,meta.quarantinePath]:[meta.path]);
}
export async function del(sessionId) {
  // Keep the native session's historical worktree_path for snapshot restore.
  db().query('DELETE FROM bot_worktrees WHERE session_id=?').run(sessionId);
}
function cleanupDatabase() {
  const value=db();
  value.exec('CREATE TABLE IF NOT EXISTS bot_worktree_safe_directory_cleanups (path TEXT PRIMARY KEY)');
  return value;
}
export function getPendingSafeDirectoryCleanups() {
  return cleanupDatabase().query('SELECT path FROM bot_worktree_safe_directory_cleanups ORDER BY path').all().map(row=>row.path);
}
export async function addPendingSafeDirectoryCleanups(paths) {
  const value=cleanupDatabase();
  value.transaction(()=>{const insert=value.query('INSERT OR IGNORE INTO bot_worktree_safe_directory_cleanups(path) VALUES (?)');for(const path of paths)insert.run(path);})();
}
export async function removePendingSafeDirectoryCleanups(paths) {
  const value=cleanupDatabase();
  value.transaction(()=>{const remove=value.query('DELETE FROM bot_worktree_safe_directory_cleanups WHERE path=?');for(const path of paths)remove.run(path);})();
}
export async function set(sessionId, meta) {
  if (meta.sessionId !== sessionId) throw Error('Worktree session identity mismatch');
  db().query('INSERT OR REPLACE INTO bot_worktrees VALUES (?,?)').run(sessionId, JSON.stringify(meta));
  await setWorktreePathInDb(sessionId, meta.path);
}
