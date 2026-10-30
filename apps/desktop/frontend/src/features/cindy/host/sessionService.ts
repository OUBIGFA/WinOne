// WinOne seam for Cindy lib/sessionService (b7e46bc): sessions are WinOne Chat
// conversations served by the Host catalog.
import {chatCall} from '../../../bridge';
import type {ChatCatalog, Conversation} from '../../chat-state';
import type {SessionReference} from '../shared/sessionReference';
import {announceSessionsChanged} from './electronAPI';
import {publishCindyHost} from './store';
import type {Session, SessionStatus} from './sessionTypes';

export function conversationToSession(conversation: Conversation): Session {
  return {
    id: conversation.id,
    title: conversation.title,
    agentKind: 'pi',
    status: conversation.archived ? 'archived' : 'active',
    workingDir: conversation.directory || null,
    model: conversation.model.model, providerId:conversation.model.provider, effort:conversation.thinkingLevel,
    permissionMode:conversation.permission,fastMode:conversation.fastMode,
    pinnedAt: conversation.pinned ? conversation.updatedAt : null,
    createdAt: conversation.createdAt,
    updatedAt: conversation.updatedAt,
    workspaceKind: conversation.projectId ? 'project' : 'dialogue',
    // The index has no separate send time; a conversation with messages last
    // changed when its latest turn or rename was saved.
    userSendAt: conversation.hasMessages ? conversation.updatedAt : null,
    runtimeEffective: {agentKind: 'pi', model: conversation.model.model, providerId: conversation.model.provider || null, effort: conversation.thinkingLevel ?? null, fastMode: !!conversation.fastMode},
  };
}

async function catalog(): Promise<ChatCatalog> {
  const value = await chatCall<ChatCatalog>('catalog');
  publishCindyHost({catalog: value});
  return value;
}

export async function get(id:string):Promise<Session|null>{
 const value=(await catalog()).conversations.find(conversation=>conversation.id===id);
 return value?conversationToSession(value):null;
}

export async function list(limit = 50, filter: 'active' | 'archived' | 'all' = 'active'): Promise<Session[]> {
  const {conversations} = await catalog();
  return conversations
    .filter((conversation) => filter === 'all' || (filter === 'archived') === !!conversation.archived)
    .sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
    .slice(0, limit)
    .map(conversationToSession);
}

export async function resolveReferences(ids: readonly string[]): Promise<SessionReference[]> {
  const {conversations} = await catalog();
  const byId = new Map(conversations.map((conversation) => [conversation.id, conversation]));
  return ids.map((sessionId) => {
    const conversation = byId.get(sessionId);
    if (!conversation) return {sessionId, state: 'missing'};
    return {sessionId, state: 'available', status: conversation.archived ? 'archived' : 'active', title: conversation.title, agentKind: 'pi', agentDeviceId: null};
  });
}

export async function update(id: string, patch: {status?: SessionStatus; pinnedAt?: string | null; title?: string}): Promise<void> {
  if (patch.status === 'deleted') {
    await chatCall('conversation.delete', {id});
  } else {
    await chatCall('conversation.update', {
      id,
      ...(patch.title ? {title: patch.title} : {}),
      ...(patch.status ? {archived: patch.status === 'archived'} : {}),
      ...(patch.pinnedAt !== undefined ? {pinned: patch.pinnedAt !== null} : {}),
    });
  }
  await catalog();
  announceSessionsChanged([id]);
}
