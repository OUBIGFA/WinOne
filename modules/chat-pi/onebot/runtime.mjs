// Local OneBot service boundary. This connects the reused content services to
// durable profile ownership; Pi dispatch and IM transports plug in separately.
import fs from 'node:fs';
import path from 'node:path';
import {Database} from 'bun:sqlite';
import {createHash} from 'node:crypto';
import {createOneBotContentService, createOneBotPaths, requireBotId, oneBotError} from './content-service.mjs';
import {OneBotProfileStore} from './profile-store.mjs';
import {buildBotSystemPrompt} from '../cindy/bots/botSystemPrompt.ts';
import {createBotLifecycleService} from '../cindy/bots/botLifecycleService.ts';
import {withIMScope} from './im-scope.mjs';
import {retryDeletedBotCleanup} from './lifecycle-host.mjs';
import {searchOneBotHistory} from './history.mjs';
import {botLinkedProjectAccess} from '../cindy/bots/botLinkedProjects.ts';
import {inspectResources} from '../resources.mjs';
import {callOneBotIMLanes} from './im-lanes.mjs';
import {createOneBotTaskService} from './task-host.mjs';
import {chatPreview, creationInput, presentProfile, storeAvatarImage, decodeAvatarImage, transcriptStats} from './bots-service.mjs';
import {queueBotInvitation, setBotInvitationWelcomeDispatch} from '../cindy/bots/botInvitation.ts';
import {notifyNewMakerDefaultsChanged} from './invitation-host.mjs';
import {todoActionText} from '../cindy/bots/shared/teammate-todo.ts';
import {collectTaskResponsibilityScopes, readTaskResponsibilities, responsibleSessionIds} from '../cindy/bots/botTaskResponsibility.ts';
import {createOneBotRoutines} from './routines.mjs';
import {createOneBotAutonomy} from './autonomy.mjs';
import {getBotAutonomyGlance, getBotAutonomySettingsView, resetBotAutonomySchedulerSettings, setBotAutonomySchedulerBotId} from '../cindy/autonomy/hub.ts';
import {signalBotAutonomyBotsChanged} from '../cindy/autonomy/botsChangedSignal.ts';
import {createOneBotLearningScan} from './learning-scan.mjs';
import {createOneBotTeammateTools} from './teammate-tools.mjs';
import {pendingLearningKey} from './learning-scan.mjs';
import {mergeBotLearningReceipts, readBotLearningReceipts} from '../../../third_party/cindy/maker-shared/botLearning.ts';
import {GroupToolAuthorizationError} from '../cindy/tasks/botGroupToolAuthorization.ts';
import {createOneBotVoice} from './voice.mjs';
import {createIMSecrets} from './im-secrets.mjs';
import {createLogger} from './logger.ts';
import {createOneBotImportedConnections} from './import-connections.mjs';

export const ONEBOT_CONTENT_METHODS = new Set([
  'profile.content.read', 'profile.content.write',
  'memory.list', 'memory.read', 'memory.update', 'memory.delete', 'memory.importDocument', 'memory.index', 'memory.search', 'memory.write', 'memory.consolidate',
  'skills.list', 'skills.query', 'skills.read', 'skills.readSource', 'skills.save', 'skills.saveSource', 'skills.delete', 'skills.mounts',
  'todos.read', 'todos.patch', 'todos.ingest', 'todos.prepareAction', 'todos.settleAction',
]);
const PROFILE_METHODS = new Set(['profiles.list', 'profiles.get', 'profiles.create', 'profiles.update', 'profiles.versions','lifecycle.action']);
const PRIVATE_METHODS = new Set(['sessions.bind','sessions.find','sessions.lookup','runtime.context','session.tool','lifecycle.pendingCleanup','lifecycle.retryCleanup','history.query','projects.query','projects.link','capabilities.query',
  'imSource.stamp',
  'bots.previews','bots.setAvatar','sessions.replaceCanonical','defaults.changed','transcripts.stats','todos.actionRequest','workbench.get','workbench.followScopes',
  'autonomy.activity','messages.learning']);
/** Cindy BOT_AUTONOMY_IPC and the todo daily-check toggle (desktop methods). */
const AUTONOMY_METHODS = new Set(['autonomy.glance', 'autonomy.settings', 'autonomy.setSettings', 'autonomy.resetSettings', 'todos.dailyCheck']);

export function createOneBotRuntime({directory, event = () => {},lifecycle = {},tasks,host,routineDispatch,masterKey,imDispatcher,language = 'zh-CN'}) {
  const paths = createOneBotPaths(directory);
  const lockFile = path.join(paths.root, 'owner-lock.db');
  paths.assertPath(lockFile);
  const lock = new Database(lockFile, {create: true});
  let profiles, content;
  try {
    // OS-held SQLite write lock, not a PID file: crash releases ownership and
    // a second Host cannot run two memory writers or consume one IM lane twice.
    lock.exec('PRAGMA busy_timeout=0; CREATE TABLE IF NOT EXISTS owner(id INTEGER PRIMARY KEY); BEGIN IMMEDIATE;');
    profiles = new OneBotProfileStore(paths.root);
    content = createOneBotContentService({directory: paths.root, readBot: async id => profiles.owner(id),
      requestRefresh: async sessionId => invalidateBot(profiles.sessionOwner(sessionId).botId)});
  } catch (error) {
    profiles?.close(); lock.close(true);
    if (error.code === 'SQLITE_BUSY' || error.code === 'SQLITE_LOCKED') throw oneBotError('ALREADY_RUNNING', '该数据目录已有 OneBot 服务运行');
    throw error;
  }
  let closing = false, closePromise, tail = Promise.resolve();
  const external = new Set();
  const owner={paths,key:profiles.generation,closed:false,profiles,content,event,lifecycle,host,language};
  owner.api=call;owner.reconcile=reconcile;
  // The original importer writes its encrypted checkpoint before creating the
  // profile. This private capability admits that owned Home only; desktop
  // creation retains its refusal to adopt arbitrary pre-existing folders.
  owner.createImportedProfile=input=>{const operation=tail.then(()=>execute('profiles.create',{input},true));tail=operation.catch(()=>{});return operation;};
  if(masterKey)owner.importSecrets=createIMSecrets(paths,masterKey,createLogger('import'),128*1024*1024);
  const learning = createOneBotLearningScan(owner);
  let taskService, voice, wechat;
  async function ensureWechat(){
    if(wechat)return wechat;
    if(!tasks||!imDispatcher||!owner.importSecrets)throw oneBotError('NOT_READY','微信执行服务尚未就绪');
    taskService??=createOneBotTaskService(owner,tasks);
    wechat=(await import('./wechat.mjs')).createOneBotWechat(owner,{dispatcher:imDispatcher});return wechat;
  }
  // The task store holds the coordinator's delegation records; read it only when it exists.
  function taskServiceIfAny(){
    if(!tasks)return null;
    if(!taskService&&!fs.existsSync(path.join(paths.root,'tasks.db')))return null;
    taskService??=createOneBotTaskService(owner,tasks);
    return taskService;
  }
  // Cindy readTaskOwnership's delegation source, read without opening the writer.
  owner.delegationRows=async()=>{
    const file=path.join(paths.root,'tasks.db');
    paths.assertPath(file);
    if(!fs.existsSync(file))return [];
    const db=new Database(file,{readonly:true});
    try{return db.query('SELECT requesting_bot_id AS botId,child_session_id AS sessionId FROM bot_delegations WHERE child_session_id IS NOT NULL').all();}
    finally{db.close();}
  };
  const autonomy = createOneBotAutonomy({owner, host,
    readSessionExecution: id => tasks?.readSessionExecution?.(id) ?? null,
    isCompletionHandledByTeammate: async (sessionId, botId, execution) => {
      const service = taskServiceIfAny();
      return service ? service.isCompletionHandledByTeammate(sessionId, botId, execution) : false;
    },
    ensureOwnBuiltins: botId => routines.ensureNightly(botId)});
  const routines = createOneBotRoutines({paths, profiles, dispatch: routineDispatch, event, autonomy, owner});
  owner.routines=routines;
  const importedConnections=createOneBotImportedConnections(owner);
  let importRecovery;
  // Cindy's teammate tools: routines, creation, workbench, Todo and task digest.
  const teammateTools = createOneBotTeammateTools({owner, paths, profiles, routines, host, event,
    call: (method, params) => call(method, params),
    startBackground: () => startBackground(),
    prStatuses: async sessionId => (await taskServiceIfAny()?.prStatuses(sessionId)) ?? null});
  // Cindy's teammate invitation (skills, portrait, welcome) runs beside requests,
  // under this owner. The Host bridge supplies its session and model facts.
  const invitationCallbacks={
    canStartWelcome:config=>host.routeAvailable(config),
    createCanonicalSession:input=>host.createCanonicalSession(input),
    broadcastProfileChanged:({botId})=>{try{profileChanged(profiles.get(botId));}catch(error){if(error.code!=='NOT_FOUND')throw error;}},
  };
  const queueInvitation=(botId,retry=false)=>{if(host)withIMScope(owner,()=>queueBotInvitation(botId,invitationCallbacks,retry));};
  owner.queueInvitation=queueInvitation;
  if(host)withIMScope(owner,()=>setBotInvitationWelcomeDispatch(async input=>{
    const result=await host.welcome(input);
    if(result.ok&&result.clientId)await input.onQueued(result.clientId);
    return {ok:result.ok===true};
  }));
  const lifecycleService=createBotLifecycleService({
    maker:{closeSession:async id=>{if(!lifecycle.closeSession)throw oneBotError('NOT_READY','OneBot 会话控制尚未就绪');await lifecycle.closeSession(id);}},
    getDelegationService:()=>lifecycle.cancelDelegations?{cancelDelegationsForBot:lifecycle.cancelDelegations}:null,
    ...(lifecycle.restartRuntime?{restartRuntime:lifecycle.restartRuntime}:{}),
    onPaused:async id=>{await routines.pauseBot(id);await wechat?.pauseBot(id);await lifecycle.onPaused?.(id);},
    onResumed:async id=>{await routines.reconcile();await wechat?.startSaved();await lifecycle.onResumed?.(id);},
    onBeforeDelete:async id=>{await routines.pauseBot(id);await lifecycle.onBeforeDelete?.(id);if(owner.importSecrets)await (await import('../cindy/import/main/bot-import/host.ts')).prepareCompanionImportDeletion(id);content.releaseBot(id);},
    onDeleted:async(id,assertOwner)=>{await routines.removeBot(id);await wechat?.removeBot(id);await lifecycle.onDeleted?.(id,assertOwner);if(owner.importSecrets)await (await import('./import-host.mjs')).finishCompanionEnvironmentRemoval(paths.root,id,assertOwner);profiles.cleanupCompleted(id,'imPending');},
    onLifecycleChanged:id=>lifecycle.onLifecycleChanged?.(id),
  });

  // Background tasks a teammate started itself (Cindy listBotDelegationChildren):
  // newest first, read from the task store without opening its writer.
  function delegatedSessionIds(botId){
    const file=path.join(paths.root,'tasks.db');
    paths.assertPath(file);
    if(!fs.existsSync(file))return [];
    const db=new Database(file,{readonly:true});
    try{
      const rows=db.query('SELECT child_session_id AS id FROM bot_delegations WHERE requesting_bot_id=? AND child_session_id IS NOT NULL ORDER BY created_at DESC').all(botId);
      return [...new Set(rows.map(row=>row.id))];
    }finally{db.close();}
  }
  // Cindy readBotResponsibilityScope: its own background tasks plus the tasks it took over.
  async function responsibilityScope(botId){
    const delegated=delegatedSessionIds(botId).sort();
    return {sessionIds:responsibleSessionIds(delegated,await readTaskResponsibilities(paths.root,botId)),delegatedSessionIds:delegated};
  }
  function invalidateBot(botId){
    for(const session of profiles.sessions(botId))if(session.role!=='history'&&session.archivedAt==null)event({type:'runtime-refresh-requested',sessionId:session.id});
  }
  function profileChanged(profile,change='updated'){
    event({type:'profile-changed',botId:profile.id,version:profile.currentVersion,payload:{botId:profile.id,change}});
    invalidateBot(profile.id);
    // Cindy bots.ts: profile changes (status, allowSchedulerFollow) re-read follow-up ownership.
    signalBotAutonomyBotsChanged();
  }

  async function flushContent(id) {
    const patch = profiles.pendingContent(id);
    if (!patch) return;
    await content.call('profile.content.write', {botId: id, patch});
    profiles.contentApplied(id);
  }
  async function reconcile(id) {
    await flushContent(id);
    const current = profiles.get(id);
    const files = await content.call('profile.content.read', {botId: id});
    if (current.identitySource !== files.identitySource || current.capabilities.userContextSource !== files.userContextSource) {
      // External edits become the next immutable profile version, not a change
      // to a currently running turn. Permission config is NEVER read from Home.
      const updated = profiles.update({id, expectedVersion: current.currentVersion,
        identitySource: files.identitySource, userContextSource: files.userContextSource});
      profiles.contentApplied(id); // files already contain this exact version
      profileChanged(updated);
      return updated;
    }
    return current;
  }
  // Every desktop profile response carries the same durable link set. The
  // Windows Host joins Chat-owned titles/timestamps without asking the UI to
  // invent sessions or fetch a second, potentially different profile version.
  const projectProfile = profile => presentProfile(paths, profile, profiles.sessions(profile.id));
  async function execute(method, params, importedCreation=false) {
    if(method==='imSource.stamp'){
      profiles.sessionOwner(params.sessionId);
      if(!['wechat','telegram','feishu','wecom','dingtalk','discord'].includes(params.channel)||typeof params.inputId!=='string'||typeof params.owner!=='boolean'||typeof params.text!=='string')throw oneBotError('INVALID_PARAMS','IM 来源无效');
      profiles.patchMessageMeta(params.sessionId,params.inputId,{imSource:{im:params.channel,userText:params.text,contentFormat:'user-text',origin:{chat:{kind:params.group?'group':'direct'},sender:{isOwner:params.owner}}}});return {ok:true};
    }
    if(method==='history.query')return searchOneBotHistory(profiles,params);
    if(method==='defaults.changed'){withIMScope(owner,notifyNewMakerDefaultsChanged);return {ok:true};}
    if(method==='bots.previews'){
      // Host-listed canonical conversations; a read failure keeps the row without a preview.
      if(!Array.isArray(params.items))throw oneBotError('INVALID_PARAMS','预览范围必须由 Host 提供');
      const previews=[];
      for(const item of params.items){
        requireBotId(item.botId);
        try{previews.push({botId:item.botId,...await chatPreview(item.session,Number.isFinite(item.lastReadAt)?item.lastReadAt:null)});}
        catch{previews.push({botId:item.botId,text:null,at:null,role:null,unread:0});}
      }
      return previews;
    }
    if(method==='sessions.replaceCanonical')return profiles.replaceCanonical(params);
    if(method==='messages.learning'){
      // Cindy messages.agent_meta.botLearning for the page: receipts only.
      const link=profiles.sessionOwnerOrNull(String(params.sessionId??''));
      if(!link)throw oneBotError('NOT_FOUND','该会话不属于 OneBot');
      const receipts={};
      for(const row of profiles.messageMetaFor(params.sessionId)){const items=readBotLearningReceipts(row.data.botLearning);if(items.length)receipts[row.clientId]=items;}
      return {receipts};
    }
    if(method==='workbench.followScopes'){
      const active=profiles.list().filter(profile=>profile.status==='active').map(profile=>profile.id);
      return collectTaskResponsibilityScopes(active,responsibilityScope);
    }
    if(method==='transcripts.stats'){
      if(!Array.isArray(params.items))throw oneBotError('INVALID_PARAMS','会话范围必须由 Host 提供');
      const result=[];
      for(const item of params.items)result.push({key:item.key,...await transcriptStats(item.session,typeof item.clientId==='string'?item.clientId:undefined)});
      return result;
    }
    if(method==='lifecycle.pendingCleanup')return profiles.pendingCleanup();
    if(method==='lifecycle.retryCleanup')return withIMScope(owner,()=>retryDeletedBotCleanup(requireBotId(params.botId)));
    if (method === 'sessions.bind') return profiles.bindSession(params);
    if (method === 'sessions.find') return profiles.findSession(params.botId,params.routeKey);
    if (method === 'sessions.lookup') return profiles.sessionOwner(params.sessionId);
    if (method === 'session.tool') {
      const owner = profiles.sessionOwner(params.sessionId);
      const profile = await reconcile(owner.botId);
      if (profile.status !== 'active' || owner.role === 'history') throw oneBotError('READ_ONLY', '此 OneBot 会话不可写');
      if (params.method==='projects.list'||params.method==='projects.set') {
        if (!['canonical','thread'].includes(owner.role))throw oneBotError('BOT_MAIN_TASK_REQUIRED','只有 OneBot 主任务可以读取或修改关联项目');
        const args=params.args??{},write=params.method==='projects.set';
        if (write&&params.ownerAuthorized!==true)throw oneBotError('OWNER_ONLY','修改关联项目需要当前轮次的主人授权');
        if (args.include_available!==undefined&&typeof args.include_available!=='boolean')throw oneBotError('INVALID_PARAMS','include_available 必须为布尔值');
        if (write&&(typeof args.project_id!=='string'||!/^[a-f0-9]{64}$/.test(args.project_id)||typeof args.linked!=='boolean'))throw oneBotError('INVALID_PARAMS','关联项目操作无效');
        const projects=await execute(write?'projects.link':'projects.query',{botId:owner.botId,catalog:params.catalog,id:args.project_id,linked:args.linked});
        return {ok:true,projects:projects.filter(project=>project.linked||args.include_available).map(project=>({
          project_id:project.id,name:project.name,working_dir:project.path,device_id:project.deviceId??null,
          linked:project.linked,available_locally:project.available,
          ...(!project.deviceId&&project.available?{context_locations:['AGENTS.md','CLAUDE.md','README.md','.cindy/project-knowledge/TOC.md']}:{}),
        }))};
      }
      // Todo tools follow Cindy's workbench caller rule (teammate.todos.*).
      const allowed = new Set(['memory.list','memory.read','memory.write','memory.search','memory.consolidate','skills.query','skills.read','skills.save','skills.delete']);
      if (!allowed.has(params.method)) throw oneBotError('UNKNOWN_METHOD', '此操作不属于 OneBot 工具');
      if (params.method.startsWith('memory.') && profile.capabilities.memory === false) throw oneBotError('DISABLED', 'OneBot 记忆已关闭');
      // Cindy authorizeGroupTool: a group lane's Skill access needs a live Chat
      // Server grant, which a local group never issues. Memory stays available.
      // The original error reads its localized text from the task locale.
      if (owner.role === 'group' && params.method.startsWith('skills.')) withIMScope({tasksNative: {language}}, () => { throw new GroupToolAuthorizationError(); });
      if (owner.archivedAt != null) throw oneBotError('READ_ONLY', '此 OneBot 会话已归档');
      const authoredType=params.method==='memory.write'?params.args?.input?.type:params.method==='memory.consolidate'?params.args?.input?.target?.type:undefined;
      if (['memory.write','memory.consolidate'].includes(params.method)&&!['user','feedback','project','reference'].includes(authoredType))throw oneBotError('INVALID_PARAMS','工具不能写入系统内部记忆类型');
      // IM must provide a host-owned, per-turn requester lease before mutations;
      // an arbitrary model parameter cannot turn a group sender into the owner.
      if (owner.role === 'channel' && params.ownerAuthorized !== true && !['memory.list','memory.read','memory.search','skills.query','skills.read'].includes(params.method)) throw oneBotError('OWNER_ONLY', '只有主人触发的轮次才能修改 OneBot 长期内容');
      const result=await content.call(params.method, {...params.args, botId: owner.botId});
      if(['skills.save','skills.delete'].includes(params.method))event({type:'capabilities-changed',botId:owner.botId});
      // Cindy botLearningTracker: a main-chat turn's own memory/Skill saves are
      // shown under that turn's reply once it finishes (learning scan).
      const receipt=params.method==='memory.write'?{kind:'memory',key:result.filename,title:params.args?.input?.title,action:(params.args?.input?.mode??'create')==='create'?'created':'updated'}
        :params.method==='memory.consolidate'?{kind:'memory',key:result.filename??`${params.args?.input?.target?.type}_${params.args?.input?.target?.name}.md`,title:params.args?.input?.target?.title,action:'updated'}
        :params.method==='skills.save'?{kind:'skill',key:result.record?.slug,title:result.record?.name,action:result.created?'created':'updated'}:null;
      if(receipt&&owner.role==='canonical'&&typeof params.requestId==='string'&&params.requestId){
        const key=pendingLearningKey(params.requestId),pending=readBotLearningReceipts(profiles.messageMeta(params.sessionId,key).botLearningPending);
        const valid=readBotLearningReceipts([receipt]);
        if(valid.length)profiles.patchMessageMeta(params.sessionId,key,{botLearningPending:mergeBotLearningReceipts(pending,valid)});
      }
      return result;
    }
    if (method === 'runtime.context') {
      const owner = params.sessionId ? profiles.sessionOwner(params.sessionId) : {botId: requireBotId(params.botId), role: params.role === 'channel' ? 'channel' : 'canonical', routeKey: params.routeKey};
      const profile = await reconcile(owner.botId);
      // A paused teammate keeps its main chat (Cindy create-canonical-session accepts it); it runs no turn.
      if (profile.status !== 'active' && !(params.lifecycleRecovery===true&&profile.status==='error') && !(params.sessionCreation===true&&profile.status==='paused')) throw oneBotError('INACTIVE', 'OneBot 当前未启用');
      const home = paths.home(profile.id), workspace = owner.role === 'channel'
        ? path.join(home,'channel-workspaces',createHash('sha256').update(String(owner.routeKey)).digest('hex'))
        : path.join(home, 'workspace');
      paths.assertPath(workspace); fs.mkdirSync(workspace, {recursive: true});
      const files = await content.call('profile.content.read', {botId: profile.id});
      const mounts = await content.call('skills.mounts', {botId: profile.id});
      const memoryEnabled = profile.capabilities.memory !== false;
      const memorySnapshot = memoryEnabled ? await content.call('memory.index', {botId: profile.id}) : '';
      // Cindy botProfileRuntime: Bot Mode is the owner's chat (main chat and
      // private topics); teammate messages and tasks belong to the main chat.
      // Each guidance block is enabled only where its tools are mounted
      // (onebot/tools.mjs). WinOne Chat has no cindy_helper session-control or
      // cindy_scheduler surface, so those blocks stay off.
      const ownerChat = owner.role === 'canonical' || owner.role === 'thread';
      const partnerActionsEnabled = owner.role === 'canonical';
      const prompt = buildBotSystemPrompt({displayName: profile.name, identity: files.identitySource, userProfile: files.userContextSource,
        homeDir: home, systemPromptOverride: files.systemPromptOverride, memorySnapshot, skillIndex: mounts.skills,
        teammates: ownerChat && partnerActionsEnabled ? profiles.list().filter(bot => bot.id !== profile.id && bot.status === 'active')
          .sort((a, b) => a.name.localeCompare(b.name)).map(bot => ({id: bot.id, name: bot.name, description: bot.description ?? null})) : [],
        capabilities: {toolsets: memoryEnabled ? ['memory'] : [], memoryEnabled, ownSkillsEnabled: ownerChat || owner.role === 'channel',
          partnerActionsEnabled, routinesEnabled: ownerChat, botCreationEnabled: ownerChat, botModeEnabled: ownerChat,
          sessionControlEnabled: false, automationEnabled: false}});
      const environmentFile=path.join(home,'environment.json');
      const importedEnvironment=fs.existsSync(environmentFile)?createHash('sha256').update(fs.readFileSync(environmentFile)).digest('hex'):'';
      return {botId: profile.id, profileVersion: profile.currentVersion, canonicalSessionId: profile.canonicalSessionId, name: profile.name,
        homeDir: home, workspace, role:owner.role, capabilities: profile.capabilities, systemPrompt: prompt.full, skillDirectories: mounts.skills.map(skill => skill.path), memoryEnabled, importedEnvironment};
    }
    if (method === 'profiles.create') {
      // Cindy createBotProfile: validated portrait bytes become a managed image
      // address before the row exists; an invitation continues after the reply.
      const draftEntry=params.input?.creationDraftToken?await withIMScope(owner,async()=>(await import('../cindy/import/main/maker-ipc/botCreationDraft.ts')).readBotCreationDraft(params.input.creationDraftToken)):undefined;
      if(draftEntry&&profiles.list().some(profile=>profile.id===draftEntry.botId))return projectProfile(profiles.get(draftEntry.botId));
      const {input, avatarImage, prepareInvitation} = creationInput(params.input, language,draftEntry);
      if(input.id&&profiles.pendingCleanup().some(row=>row.botId===input.id))throw oneBotError('PRECONDITION_FAILED','OneBot 的删除清理尚未完成');
      // Never claim a pre-existing unregistered Home and overwrite user files.
      if (input.id && fs.existsSync(paths.home(requireBotId(input.id)))&&!importedCreation) throw oneBotError('ALREADY_EXISTS', 'OneBot 目录已存在，未覆盖其中的资料');
      const profile = profiles.create(input, avatarImage ? {avatar: storeAvatarImage(paths, avatarImage)} : {});
      await flushContent(profile.id);
      profileChanged(profile, 'created');
      if (prepareInvitation) queueInvitation(profile.id);
      return projectProfile(profile);
    }
    if (method === 'profiles.list') {
      const result = [];
      for (const profile of profiles.list()) {
        result.push(projectProfile(await reconcile(profile.id)));
        // Resume persisted invitations (Cindy local-db:bots:list); finished ones are no-ops.
        if (profile.status !== 'archived') queueInvitation(profile.id);
      }
      return result;
    }
    const id = requireBotId(params.botId ?? params.input?.id);
    const profile = await reconcile(id);
    if(method==='capabilities.query'){
      const workspace=path.join(paths.home(id),'workspace');paths.assertPath(workspace);fs.mkdirSync(workspace,{recursive:true});
      // Inspect the whole registered catalog without a model, extension loading
      // or MCP startup. Selection is applied only by the execution loader.
      const resources=await inspectResources({...params.resources,cwd:workspace,onebot:undefined});
      const personal=await content.call('skills.list',{botId:id});
      return {entries:{
        skill:[...personal.map(skill=>({id:`personal:${skill.slug}`,name:skill.name,description:skill.description,available:skill.enabled!==false,personal:true})),
          ...resources.skills.map(skill=>({id:skill.name,name:skill.name,description:skill.description,available:true}))],
        toolset:resources.plugins.map(plugin=>({id:plugin.id,name:plugin.name,description:plugin.description,available:plugin.enabled!==false})),
        mcp:(params.resources.mcpServers??[]).map(server=>({id:server.name,name:server.name,description:server.description??(server.url?'HTTP':'stdio'),available:server.enabled!==false})),
      }};
    }
    if (method==='projects.query'||method==='projects.link') {
      if (!Array.isArray(params.catalog)||params.catalog.some(project=>!project||typeof project.path!=='string'||!path.isAbsolute(project.path)||typeof project.name!=='string'||typeof project.exists!=='boolean')) {
        throw oneBotError('INVALID_PARAMS','关联项目需要 Host 提供的目录列表');
      }
      if (method==='projects.link'&&(typeof params.id!=='string'||typeof params.linked!=='boolean'))throw oneBotError('INVALID_PARAMS','关联项目操作无效');
      const captured=profiles.owner(id),file=path.join(paths.home(id),'workbench.json');
      const guard=()=>{captured.assertCurrent();paths.assertPath(file);};
      // Reuse the original project selection IDs, path normalization and
      // workbench transaction. This scope carries facts, never new permissions.
      return withIMScope({...owner,projectCatalog:params.catalog,projectGuard:guard},async()=>{
        guard();
        const access=await botLinkedProjectAccess(id);
        const result=method==='projects.query'?await access.read():await access.set(params.id,params.linked,guard);
        guard();return result;
      });
    }
    if(method==='lifecycle.action')return withIMScope(owner,()=>lifecycleService.run({...params,botId:id}));
    if (method === 'profiles.get') return projectProfile(profile);
    if (method === 'profiles.versions') return profiles.versions(id);
    if (method === 'workbench.get') return {responsibleSessionIds: (await responsibilityScope(id)).sessionIds};
    if (method === 'todos.actionRequest') {
      // Cindy todoAccess.act: the owner asks the companion to advance one agreed
      // next step. The Host queues the instruction in its main chat and settles.
      if (profile.status !== 'active' || !profile.canonicalSessionId) throw oneBotError('HOST_NOT_READY', '伙伴主对话尚未就绪');
      const prepared = await content.call('todos.prepareAction', {botId: id, id: params.id, revision: params.revision, requestId: params.requestId});
      event({type: 'workbench-changed', botId: id});
      if (!prepared.dispatch) return {todo: prepared.todo, dispatch: false};
      const todo = prepared.todo;
      return {todo, dispatch: true, sessionId: profile.canonicalSessionId,
        displayText: todoActionText(todo.title, todo.next?.label ?? todo.title, typeof params.locale === 'string' ? params.locale : 'en'),
        message: JSON.stringify({kind: 'todo-action', todo_id: todo.id, revision: todo.revision, next: todo.next, outcome: todo.outcome,
          scope: 'Advance only this agreed next step under existing permissions. Read the same Todo, preserve sources, and update it after a real receipt. Recording or reading external messages does not authorize sending, spending or broader access.'})};
    }
    if (method === 'profiles.update' && params.input?.retryInvitation === true) {
      queueInvitation(id, true);
      return projectProfile(profile);
    }
    if (method === 'bots.setAvatar') {
      // Cindy setBotProfileAvatar: bytes are published first, then the address moves with a version.
      const image = decodeAvatarImage(params.avatarImageBase64);
      if (!image) throw oneBotError('INVALID_PARAMS', '请选择头像图片');
      if (profile.status === 'archived') throw oneBotError('PRECONDITION_FAILED', '已停止的 Bot 不能修改头像');
      const url = storeAvatarImage(paths, image);
      profiles.txUpdateProfile({id, avatar: url, identitySource: profile.identitySource, capabilitiesJson: JSON.stringify(profile.capabilities),
        expectedCurrentVersion: profile.currentVersion});
      const updated = profiles.get(id);
      profileChanged(updated);
      return projectProfile(updated);
    }
    if (method === 'profiles.update') {
      const updated = profiles.update({...params.input, id});
      await flushContent(id);
      await routines.reconcile();
      if(updated.currentVersion!==profile.currentVersion)profileChanged(updated);
      return projectProfile(updated);
    }
    if (method === 'profile.content.write' && (params.patch?.identitySource !== undefined || params.patch?.userContextSource !== undefined)) {
      if (params.expectedVersion !== profile.currentVersion) throw oneBotError('PRECONDITION_FAILED', 'OneBot 配置已更新，请重新读取后重试');
    }
    const result = await content.call(method, {...params, botId: id});
    if(['skills.save','skills.saveSource','skills.delete'].includes(method))event({type:'capabilities-changed',botId:id});
    if (method === 'profile.content.write') await reconcile(id);
    return result;
  }

  function call(method, params = {}) {
    if (closing) return Promise.reject(oneBotError('NOT_READY', 'OneBot 服务正在关闭'));
    if (!params || typeof params !== 'object' || Array.isArray(params)) return Promise.reject(oneBotError('INVALID_PARAMS', 'OneBot 参数必须是对象'));
    if(method.startsWith('wechat.')){
      const work=tail.then(()=>withIMScope(owner,async()=>(await ensureWechat()).call(method.slice(7),params)));
      external.add(work);work.then(()=>external.delete(work),()=>external.delete(work));return work;
    }
    if(method.startsWith('importedTools.')){
      const work=tail.then(()=>withIMScope(owner,async()=>{
        const result=await importedConnections.call(method.slice(14),params);
        if(method==='importedTools.call'){const link=profiles.sessionOwnerOrNull(params.sessionId);if(link)invalidateBot(link.botId);}
        return result;
      }));
      external.add(work);work.then(()=>external.delete(work),()=>external.delete(work));return work;
    }
    if(method==='lifecycle.action'&&params.action==='delete'&&owner.importSecrets){
      // Imports can enqueue profile writes: drain before taking that queue,
      // otherwise deleting while an import finishes would deadlock both jobs.
      const work=withIMScope(owner,async()=>{
        await (await import('../cindy/import/main/bot-import/host.ts')).drainCompanionImports();
        const operation=tail.then(()=>execute(method,params));tail=operation.catch(()=>{});return operation;
      });
      external.add(work);work.then(()=>external.delete(work),()=>external.delete(work));return work;
    }
    if(method.startsWith('market.')||method.startsWith('import.')||method==='bots.generateDraft'||method==='bots.generateAvatar'){
      const work=tail.then(()=>withIMScope(owner,async()=>{
        owner.routines=routines;
        if(method==='bots.generateDraft'||method==='bots.generateAvatar'){
          const api=await import('../cindy/import/main/maker-ipc/botCreationDraft.ts');
          return method==='bots.generateDraft'?api.generateBotCreationDraft(params.request,profiles.list().map(profile=>profile.name)):api.generateBotCreationAvatar(params.token);
        }
        const api=await import('../cindy/import/main/bot-import/host.ts');
        if(method==='market.list')return api.listNativeBotPackages();
        if(method==='market.invite')return api.inviteNativeBotPackage(params.input,'winone-desktop');
        if(method==='market.status'||method==='import.status')return api.getCompanionImportResult(params.requestId);
        if(method==='import.sources')return api.listCompanionImportSources('winone-desktop');
        if(method==='import.preview')return api.previewCompanionImport(params.sourceId,'winone-desktop');
        if(method==='import.start')return api.startCompanionImport(params.selection,'winone-desktop');
        throw oneBotError('UNKNOWN_METHOD','不支持的导入操作');
      }));external.add(work);work.then(()=>external.delete(work),()=>external.delete(work));return work;
    }
    if (method.startsWith('voice.')) {
      const work=tail.then(()=>withIMScope(owner,()=>{
        if(method==='voice.start'&&tasks)taskService??=createOneBotTaskService(owner,tasks);
        voice??=createOneBotVoice(owner);
        return voice.call(method.slice(6),params);
      }));
      external.add(work);work.then(()=>external.delete(work),()=>external.delete(work));return work;
    }
    if (method === 'learning.scan') {
      const work = tail.then(() => withIMScope(owner, () => learning.scan()));
      external.add(work); work.then(() => external.delete(work), () => external.delete(work));
      return work;
    }
    if (AUTONOMY_METHODS.has(method)) {
      // Cindy bot-autonomy/ipc.ts and local-db:bots:todos:daily-check, in this owner scope.
      const work = tail.then(() => withIMScope(owner, async () => {
        if (method === 'todos.dailyCheck') return routines.call('dailyCheck', params);
        if (method === 'autonomy.glance') {
          profiles.get(requireBotId(params.botId));
          return getBotAutonomyGlance(params.botId, undefined, false);
        }
        if (method === 'autonomy.settings') return getBotAutonomySettingsView();
        if (method === 'autonomy.resetSettings') return resetBotAutonomySchedulerSettings();
        const value = params.patch?.schedulerBotId;
        if (value !== null && (typeof value !== 'string' || !value.trim() || value.length > 128)) throw oneBotError('INVALID_PARAMS', 'schedulerBotId must be a teammate id or null');
        return setBotAutonomySchedulerBotId(value);
      }));
      external.add(work); work.then(() => external.delete(work), () => external.delete(work));
      return work;
    }
    if (method.startsWith('teammate.') || method === 'workbench.readTask') {
      // Private Host calls bound to the caller's conversation (teammate tools)
      // and the owner's workbench view. They read conversations through the
      // Host bridge, so they never hold the profile transaction queue.
      const work = tail.then(() => withIMScope(owner, () => {
        if (method === 'workbench.readTask') {
          profiles.get(requireBotId(params.botId));
          return teammateTools.readForOwner(params.botId, params.taskId);
        }
        return teammateTools.call(method, params);
      }));
      external.add(work); work.then(() => external.delete(work), () => external.delete(work));
      return work;
    }
    if (method === 'routines.policy') {
      // Private Host call: the report policy of the routine run in this chat.
      const work = tail.then(() => withIMScope(owner, () => routines.policy(params)));
      external.add(work); work.then(() => external.delete(work), () => external.delete(work));
      return work;
    }
    if (method.startsWith('routines.')) {
      // Scheduler callbacks may reread a profile through the private bridge.
      // Do not hold the profile mutation queue while awaiting their execution.
      const work = tail.then(() => withIMScope(owner, () => routines.call(method.slice('routines.'.length), params)));
      external.add(work); work.then(() => external.delete(work), () => external.delete(work));
      return work;
    }
    if (method.startsWith('imLanes.')) {
      // Native lane reset may read the runtime context through this process.
      // Never hold the profile transaction queue while awaiting that callback.
      const work = tail.then(() => withIMScope(owner, () => callOneBotIMLanes(method.slice('imLanes.'.length), params)));
      external.add(work); work.then(() => external.delete(work), () => external.delete(work));
      return work;
    }
    if (method.startsWith('groups.')) {
      // Cindy's local group chats run members' turns through the native task
      // client; rounds continue in the background, never in the profile queue.
      if(!tasks)return Promise.reject(oneBotError('NOT_READY','OneBot 任务执行接口尚未就绪'));
      const work=tail.then(()=>{taskService??=createOneBotTaskService(owner,tasks);return taskService.groups(method.slice('groups.'.length),params);});
      external.add(work); work.then(() => external.delete(work), () => external.delete(work));
      return work;
    }
    if (method.startsWith('tasks.')) {
      if(!tasks)return Promise.reject(oneBotError('NOT_READY','OneBot 任务执行接口尚未就绪'));
      if(method==='tasks.recover'&&!taskService&&!fs.existsSync(path.join(paths.root,'tasks.db')))return Promise.resolve({tracked:0});
      return tail.then(()=>{taskService??=createOneBotTaskService(owner,tasks);return taskService.call(method,params);});
    }
    if (method === 'autonomy.activity') {
      // Host activity pushes bypass the profile queue: they only update the watch.
      return withIMScope(owner, async () => autonomy.activity(params));
    }
    if (!PROFILE_METHODS.has(method) && !ONEBOT_CONTENT_METHODS.has(method) && !PRIVATE_METHODS.has(method)) return Promise.reject(oneBotError('UNKNOWN_METHOD', '不支持的 OneBot 操作'));
    // Serialize profile journals with content operations. No network/model work
    // is awaited here; later dispatch runs outside this storage transaction queue.
    const operation = tail.then(() => execute(method, params));
    tail = operation.catch(() => {});
    return operation;
  }
  function close() {
    if (closePromise) return closePromise;
    closing = true;
    closePromise = (async () => {
      await tail;
      await withIMScope(owner,()=>wechat?.close());
      await withIMScope(owner,()=>voice?.close());
      await withIMScope(owner,()=>importedConnections.close());
      await Promise.allSettled([...external]);
      if(owner.routines)await withIMScope(owner,async()=>(await import('../cindy/import/main/bot-import/host.ts')).drainCompanionImports());
      await taskService?.close();
      try { await routines.close(); }
      finally {
        try { await content.close(); }
        finally { owner.closed=true;owner.importSecrets?.close();try { profiles.close(); } finally { lock.exec('ROLLBACK'); lock.close(true); } }
      }
    })();
    return closePromise;
  }
  async function startBackground() {
    if (closing) return;
    if(imDispatcher&&fs.existsSync(path.join(paths.root,'wechat-channel.json')))await withIMScope(owner,async()=>(await ensureWechat()).startSaved());
    const started=await withIMScope(owner, () => (host && profiles.list().some(bot => bot.status === 'active')) || fs.existsSync(path.join(paths.root, 'routines', 'routines.json')) ? routines.start() : Promise.resolve());
    if(owner.importSecrets&&!importRecovery&&fs.existsSync(path.join(paths.root,'companion-imports'))){
      importRecovery=withIMScope(owner,async()=>{await (await import('../cindy/import/main/bot-import/host.ts')).recoverCompanionImports();});
      external.add(importRecovery);importRecovery.then(()=>external.delete(importRecovery),error=>{external.delete(importRecovery);event({type:'diagnostic',level:'warn',scope:'import',message:error.code??'IMPORT_RECOVERY_FAILED'});});
    }
    return started;
  }
  return {call, close, startBackground,attachIMService:im=>{owner.im=im;}};
}
