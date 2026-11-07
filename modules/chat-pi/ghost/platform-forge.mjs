// Agent plugin authoring: Cindy's guide, scaffold and pack, with local install
// through the same inspected, confirmed path as a package the user picks.
import {FORGE_GUIDE, FORGE_PAGE_MANUAL, packGhostDir, scaffoldGhostDir} from '../cindy/ghost/apps/desktop/src/main/cindy-brain/forge.ts';

export function createGhostForge({root, manager, host, log}) {
  return {
    async call(method, params) {
      switch (method) {
        case 'forge-guide': return params.pages ? FORGE_PAGE_MANUAL : FORGE_GUIDE;
        case 'forge-scaffold': return scaffoldGhostDir(params.request);
        case 'forge-pack': return packGhostDir(params.request);
        case 'forge-install': {
          const packed = await packGhostDir(params.request);
          if (!packed.ok) return packed;
          const answer = await host('ui', {kind: 'install', path: packed.cindyPath, sessionId: params.session?.id});
          return answer?.ok ? {ok: true, action: answer.action, ghost: answer.ghost, cindyPath: packed.cindyPath} : {ok: false, errorCode: 'USER_CANCELLED', message: '用户没有确认安装'};
        }
      }
      throw Object.assign(Error('未知插件操作'), {code: 'UNKNOWN_METHOD'});
    },
  };
}
