// Registered folders and their display names come from the WinOne Host for
// this request. The renderer and Bot-owned files never provide this catalog.
import {oneBotOwnerScope} from './im-scope.mjs';

export async function getBotRemoteSettingsSource(botId) {
 const owner=oneBotOwnerScope();
 owner.projectGuard();
 return {source:owner.profiles.get(botId)};
}
export async function listRecentWorkdirs() {
 const owner=oneBotOwnerScope();
 owner.projectGuard();
 return owner.projectCatalog.map(project=>({path:project.path,exists:project.exists}));
}
export async function listProjectAliases() {
 const owner=oneBotOwnerScope();
 owner.projectGuard();
 return owner.projectCatalog.map(project=>({projectKey:`local:${project.path}`,alias:project.name}));
}
// WinOne's project catalog has no separate hidden-project preference. A removed
// registration disappears from the catalog; an existing Bot link stays visible.
export function loadSidebarSettingsSnapshot(){return {hiddenProjectKeys:[]};}
