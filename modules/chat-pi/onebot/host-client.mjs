// Private Host bridge calls used by Cindy's invitation worker and the teammate
// projection. The bridge already authenticates this worker; these routes carry
// only Host-owned session ids and model-route facts, never renderer paths.
import {readBranch} from './history.mjs';
import {messageRow} from './bots-service.mjs';
import {inspectResources} from '../resources.mjs';

export function createOneBotHostClient({baseUrl, key}) {
  async function post(route, body, {timeout = 30000, signal} = {}) {
    const response = await fetch(`${baseUrl}/onebot/${route}`, {method: 'POST',
      headers: {Authorization: 'Bearer ' + key, 'Content-Type': 'application/json'}, body: JSON.stringify(body),
      signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(timeout)]) : AbortSignal.timeout(timeout)});
    if (!response.ok) throw Error((await response.text()).slice(0, 2048) || `OneBot Host ${route} failed`);
    return response.json();
  }
  return {
    openWechatAuthorization:url=>post('wechat-authorization',{url}),
    portraitCatalog:()=>post('portrait-catalog',{}),
    portraitRequest:input=>post('portrait-request',input,{timeout:180000}),
    async creationCapabilities(botId){
      const resources=await post('creation-resources',{botId}),found=await inspectResources(resources);
      return {skill:found.skills.map(item=>({name:item.name,description:item.description??''})),mcp:(resources.mcpServers??[]).map(item=>({id:item.name,name:item.name})),toolset:found.plugins.map(item=>({id:item.id,name:item.name}))};
    },
    /** Local AI routes for the trusted voice worker; never a renderer response. */
    voiceResources: () => post('voice-resources', {}),
    voiceRecord: input => post('voice-record', input),
    voiceSystemAudio: input => post('voice-system-audio', input),
    async auxiliaryTextStream(request) {
      const {signal,onText,...payload}=request;
      const response=await fetch(baseUrl+'/onebot/aux-stream',{method:'POST',headers:{Authorization:'Bearer '+key,'Content-Type':'application/json'},body:JSON.stringify(payload),signal:signal?AbortSignal.any([signal,AbortSignal.timeout((request.timeoutMs??60000)+5000)]):AbortSignal.timeout((request.timeoutMs??60000)+5000)});
      if(!response.ok)throw Error('语音模型请求失败');
      const decoder=new TextDecoder();let pending='',result;
      for await(const bytes of response.body){pending+=decoder.decode(bytes,{stream:true});let end;
        while((end=pending.indexOf(String.fromCharCode(10)))>=0){const line=pending.slice(0,end);pending=pending.slice(end+1);if(!line)continue;const item=JSON.parse(line);if(item.type==='text')onText?.(item.text);if(item.type==='result')result=item.result;}
      }
      if(!result)throw Error('语音模型回复未完整接收');
      return result;
    },
    /** A saved standing instruction, tied to the exact original canonical chat. */
    authorizeRoutine: (input, signal) => post('authorize-routine', input, {signal}),
    /** Global installed skill metadata for duplicate detection, without personal skills. */
    installedSkills: botId => post('skills-catalog', {botId}).then(value => value.entries.skill.filter(skill => !skill.personal)),
    /** The Chat record (title, files) of a OneBot conversation. */
    conversation: sessionId => post('conversation', {sessionId}),
    /** Lane metadata/activity are read-only; control uses the same Chat engine. */
    laneSessions: () => post('lanes', {}),
    restartLane: sessionId => post('lane-restart', {sessionId}),
    archiveLane: sessionId => post('lane-archive', {sessionId}),
    /** Cindy ensureBotGroupLaneSession: a member's lane in one group, or its 分工 session for one plan. */
    groupLane: input => post('group-lane', input, {timeout: 60000}),
    /** Archived group lanes leave the runtime as well (Cindy closeLanes). */
    closeGroupLanes: sessionIds => post('group-lanes-close', {sessionIds}),
    /** Whether a 项目文件夹 is a git repository (Cindy detectRepo). */
    worktreeState: directory => post('worktree-state', {directory}),
    /** A deleted group's own folder goes to the Recycle Bin (Cindy trashGroupFolder). */
    recycleGroupFolder: directory => post('group-folder-recycle', {directory}, {timeout: 60000}).then(() => undefined),
    recycleStagedGroupFolder: directory => post('group-staged-recycle', {directory}, {timeout: 60000}).then(() => undefined),
    /** An existing local folder usable as a group's 项目文件夹 (Cindy validateExistingLocalProjectDirectory). */
    groupProjectDir: directory => post('group-project-dir', {directory}),
    /** Transcript rows of one role, newest last (Cindy `messages` reads in the invitation worker). */
    async transcriptRows(sessionId, role) {
      const branch = await readBranch(await post('conversation', {sessionId}));
      return branch.filter(entry => entry.type === 'message' && entry.message?.role === role).map(entry => ({id: entry.id}));
    },
    /** One message of a companion chat by its page client id, as Cindy's `messages` row. */
    async messageRow(sessionId, clientId) {
      const row = messageRow(await readBranch(await post('conversation', {sessionId})), clientId);
      return row ? [row] : [];
    },
    /** One bounded auxiliary-model call (Cindy requestAuxiliaryText); failures are results, not throws. */
    async auxiliaryText(request) {
      try {
        return await post('aux', {prompt: request.prompt, responseInstructions: request.responseInstructions, maxTokens: request.maxTokens,
          timeoutMs: request.timeoutMs, providerId: request.providerId, model: request.model}, {timeout: (request.timeoutMs ?? 20000) + 5000, signal: request.signal});
      } catch (error) {
        return {ok: false, reason: request.signal?.aborted ? 'aborted' : String(error.message ?? error).slice(0, 200)};
      }
    },
    /** A host-written assistant message in a OneBot chat (a Market opening). */
    createMessage: (sessionId, row, options) => post('message', {sessionId, clientId: row.clientId, content: row.content,
      starterOptions: row.agentMeta?.botStarterOptions ?? [], requireNoUserMessage: options.requireNoUserMessage}),
    /** Cindy's welcome dispatch: a hidden, tool-free first turn in the canonical chat. */
    welcome: input => post('welcome', {sessionId: input.targetSessionId, clientId: input.clientId, message: input.message,
      persistedContent: input.persistedContent, toolsDisabled: input.toolsDisabled === true, retry: input.retry === true}),
    createCanonicalSession: input => post('canonical', input),
    /** Teammate follow-up: every non-idle conversation's activity (Chat and OneBot stores). */
    activitySnapshots: () => post('autonomy-activity', {}).then(result => result.snapshots ?? []),
    /** Cindy LocalSessionMeta facts for conversations, without transcripts. */
    sessionMetas: ids => post('autonomy-sessions', {ids}).then(result => result.sessions ?? []),
    /** An unattended task is waiting for its owner: a Windows tray reminder. */
    remind: input => post('remind', input),
    /** Any conversation's record and transcript location (workbench, task digest). */
    taskConversation: sessionId => post('task-conversation', {sessionId}),
    /** Live activity of a conversation and the calling teammate's model chain. */
    taskFacts: (sessionId, callerSessionId) => post('task-facts', {sessionId, callerSessionId}),
    /** continue_workbench_task delivery into an owner task in ordinary Chat. */
    sendToTask: input => post('workbench-send', input, {timeout: 60000}),
    /** A pre-run check's self-test in the teammate chat's folder (Host command Job). */
    testPreRunHook: input => post('routine-hook-test', input, {timeout: 70000}),
    /** Whether the teammate's first model route can run now (Cindy canStartWelcome). */
    routeAvailable: capabilities => post('route-check', {capabilities}).then(result => result.available === true),
  };
}
