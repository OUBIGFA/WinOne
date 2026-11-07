// Cindy's plugin switchboard (ghost_list / ghost_info / ghost_manual / ghost_call and
// the forge tools) as Pi tools. The server and its schemas are upstream's; every
// question it asks goes to the plugin host, so answers are always the live ones.
import {randomUUID} from 'node:crypto';
import {Client} from '@modelcontextprotocol/sdk/client/index.js';
import {InMemoryTransport} from '@modelcontextprotocol/sdk/inMemory.js';
import {createCindyGhostsMcpServer, buildGhostRosterPrompt} from '../cindy/ghost/packages/cindy-tools/src/ghost/mcpServer.ts';

// Read-only questions need no approval; a call or a change does, as any tool would.
const READ_ONLY = new Set(['ghost_list', 'ghost_info', 'ghost_manual', 'ghost_forge_guide']);
const LOCAL_TOOLS = new Set([...READ_ONLY, 'connect_account', 'ghost_call', 'ghost_forge_scaffold', 'ghost_forge_pack', 'ghost_forge_install']);

export async function ghostAgentTools(config, check) {
  const bridge = config.ghostBridge;
  if (!bridge || config.model?.tools === false) return {tools: [], prompt: ''};
  const ask = async (method, params, signal) => {
    const response = await fetch(bridge.baseUrl + '/ghost-tool', {method: 'POST', headers: {Authorization: 'Bearer ' + bridge.key, 'Content-Type': 'application/json'}, body: JSON.stringify({method, params}), signal});
    if (!response.ok) throw Error((await response.text()).slice(0, 2048) || 'Plugin host is unavailable');
    return response.json();
  };
  let roster;
  try { roster = await ask('agent-roster', {}, AbortSignal.timeout(15_000)); } catch { return {tools: [], prompt: ''}; }
  if (!Array.isArray(roster) || !roster.length) return {tools: [], prompt: ''};

  const approval = new Map();
  const deps = {
    getRosterItems: () => roster,
    listAwakeGhosts: () => ask('agent-list', {}),
    getAwakeGhost: (ghostId, namespace) => ask('agent-info', {ghostId, namespace}),
    readGhostManual: request => ask('agent-manual', {ghostId: request.ghostId, namespace: request.namespace, path: request.path}),
    async callGhostTool({signal, ...request}) {
      const cancelKey = randomUUID();
      const stop = () => { void ask('agent-cancel', {cancelKey}, AbortSignal.timeout(5000)).catch(() => {}); };
      signal?.addEventListener('abort', stop, {once: true});
      try { return await ask('agent-call', {...request, cancelKey, approved: approval.get('call') === true}, signal); }
      finally { signal?.removeEventListener('abort', stop); }
    },
    async connectAccount(target, signal) {
      const cancelKey = randomUUID();
      const stop = () => { void ask('agent-cancel', {cancelKey}, AbortSignal.timeout(5000)).catch(() => {}); };
      signal?.addEventListener('abort', stop, {once: true});
      try { return await ask('agent-connect-account', {...target, cancelKey}, signal); }
      finally { signal?.removeEventListener('abort', stop); }
    },
    forgeGuide: () => ask('forge-guide', {}),
    forgePageManual: () => ask('forge-guide', {pages: true}),
    forgeScaffold: request => ask('forge-scaffold', {request}),
    forgePack: request => ask('forge-pack', {request}),
    forgeInstall: request => ask('forge-install', {request}),
    forgePublish: async () => ({ok: false, errorCode: 'UNSUPPORTED', message: 'WinOne 没有 Cindy 插件市场'}),
    forgePublishStatus: async () => ({ok: false, errorCode: 'UNSUPPORTED', message: 'WinOne 没有 Cindy 插件市场'}),
  };
  const server = createCindyGhostsMcpServer(deps);
  const [clientSide, serverSide] = InMemoryTransport.createLinkedPair();
  const client = new Client({name: 'winone-pi', version: '1.0.0'});
  await Promise.all([server.connect(serverSide), client.connect(clientSide)]);
  const listed = await client.listTools();
  const tools = listed.tools.filter(tool => LOCAL_TOOLS.has(tool.name)).map(tool => ({
    name: tool.name, label: tool.name, description: tool.description ?? '', parameters: tool.inputSchema,
    execute: async (id, args, signal) => {
      // Connection only requests a Host-owned card. The native form/browser
      // action is the consent; it never dispatches a business tool or files.
      if (!READ_ONLY.has(tool.name) && tool.name !== 'connect_account') {
        await check({id, tool: tool.name, args, external: true}, signal);
        approval.set('call', true);
      }
      const result = await client.callTool({name: tool.name, arguments: args}, undefined, {signal, timeout: 40 * 60_000, resetTimeoutOnProgress: true});
      const content = (result.content ?? []).map(part => part.type === 'image' ? {type: 'image', data: part.data, mimeType: part.mimeType} : {type: 'text', text: part.text ?? JSON.stringify(part)});
      if (result.isError) throw Error(content.map(part => part.text ?? '').join('\n') || 'Plugin call failed');
      return {content, details: result.structuredContent ?? {}};
    },
  }));
  return {tools, prompt: buildGhostRosterPrompt(roster)};
}
