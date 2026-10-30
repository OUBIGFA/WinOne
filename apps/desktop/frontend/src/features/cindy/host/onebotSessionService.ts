// WinOne seam for Cindy lib/sessionService in the OneBot pages (Cindy b7e46bc).
// A teammate's own conversations live in the OneBot Chat store; its delegated
// background tasks are ordinary WinOne Chat conversations. Read the ordinary
// catalog first so a task card in Chat never starts the OneBot process.
import {chatCall} from '../../../bridge';
import type {ChatCatalog} from '../../chat-state';
import {conversationToSession, get as getChatSession} from './sessionService';
import type {Session} from './sessionTypes';

export async function get(id: string): Promise<Session> {
  const ordinary = await getChatSession(id);
  if (ordinary) return ordinary;
  const {conversations} = await chatCall<ChatCatalog>('onebot.chat.catalog');
  const conversation = conversations.find(item => item.id === id);
  // Cindy rejects a missing session; callers recover by recreating the link.
  if (!conversation) throw Object.assign(new Error('Session not found'), {code: 'NOT_FOUND'});
  return {...conversationToSession(conversation), source: 'bot'};
}
