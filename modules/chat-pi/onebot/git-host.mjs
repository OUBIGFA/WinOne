// WinOne has no Cindy-managed gh installation. Reuse the pinned system lookup
// and the user's existing GitHub CLI sign-in; never inspect Cindy's data root.
import {systemGhBinary} from '../cindy/tasks/git/ghBinary.ts';
export {systemGhBinary};
export async function resolveGhBinary(){return systemGhBinary();}
