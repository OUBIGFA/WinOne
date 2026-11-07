import {GhostExternalLinkGate} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/previewGate.ts';
import {runGhostExternalLinkNavigation} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostExternalLinkNavigation.ts';
import {t} from './seams/i18n.ts';

// Host session facts replace Cindy's desktop database. The callback is scoped
// to this private worker; plugin code never gets a database or a file path.
let readModel;
export function setGhostContextReader(reader) { readModel = reader; }
export async function readWinOneMessageHookContext(sessionId) {
  if (!readModel) throw Error('Plugin session context is not connected to the Host');
  const model = await readModel(sessionId);
  return typeof model === 'string' && model ? {model} : {};
}

// Native plugin pages have no preload/Go bridge. Only WinOne's authenticated UI
// reports a page attachment and its actual embedded keyboard focus. Reuse the
// upstream URL/rate/confirmation gate, retaining its captured owner lifecycle.
export function createGhostPageContext({find,current,host,ui,log,noteUserGesture,previewNavigation,now}) {
  const pages = new Map();
  const gate = new GhostExternalLinkGate({now, declaredExternalUrls(id) {
    const ghost = find(id);
    return ghost && current(ghost)
      ? [...(ghost.manifest.network?.secrets ?? []), ...(ghost.manifest.node?.secretBindings ?? [])].flatMap(item => item.url ? [item.url] : [])
      : null;
  }});
  const lookup = pageId => {
    const page = pages.get(pageId);
    return page && current(page.ghost) ? page : null;
  };
  return {
    attach({pageId,pluginId}) {
      const ghost = find(pluginId);
      if (typeof pageId !== 'string' || !/^[A-Za-z0-9-]{1,64}$/.test(pageId) || !ghost || !current(ghost)) throw Error('插件页面已不可用');
      if (pages.has(pageId)) throw Error('插件页面标识已被使用');
      pages.set(pageId, {ghost,pluginId});
      return {ok:true};
    },
    binding(pageId){const page=lookup(pageId);return page?{pluginId:page.pluginId,ghost:page.ghost}:null;},
    detach({pageId}) { pages.delete(pageId); return {ok:true}; },
    gesture({pageId,focused}) {
      const page = lookup(pageId);
      if (page && focused === true) noteUserGesture?.(page.pluginId);
      return {ok:true};
    },
    async navigation({pageId,url,focused}) {
      if(await previewNavigation?.({pageId,url,focused}))return {ok:true};
      const page = lookup(pageId);
      if (!page || typeof url !== 'string' || url.length > 8192) return {ok:true};
      const active = () => lookup(pageId) === page;
      const owner = {isDestroyed: () => !active()};
      const hostContents = {isDestroyed: () => !active()};
      const guestContents = {hostWebContents: hostContents, isDestroyed: () => !active(), isFocused: () => focused === true};
      await runGhostExternalLinkNavigation({ghostId:page.pluginId,url,hostContents,guestContents}, {
        gate, resolveOwner: () => owner, isOwnerActive: active, logger: log,
        translate: t,
        showMessageBox: async (_owner, options) => {
          const answer = await ui('confirm', {pageId,ghostName:page.ghost.manifest.name,body:options.detail,confirmText:options.buttons[0],cancelText:options.buttons[1]});
          return {response:answer?.confirmed === true ? 0 : 1};
        },
        openExternal: address => host('open-external', {url:address}),
      });
      // Like the original navigation handler, this is a handled attempt, not a
      // claim that a browser opened. The sandbox sees no authorization oracle.
      return {ok:true};
    },
    stop(id) { for (const [pageId,page] of pages) if (page.pluginId === id) pages.delete(pageId); },
    close() { pages.clear(); },
  };
}
