// Cindy's local `bots:*` data behaviour (main/localDb/ipc/bots.ts, Cindy
// b7e46bc, Apache-2.0) on the OneBot profile store: the profile projection the
// teammate pages read, list previews and unread counts from the Pi transcript,
// creation input and managed avatar images. Pi owns transcripts; the Host
// supplies the conversation records whose files are read here.
import fs from 'node:fs';
import path from 'node:path';
import {createHash, randomUUID} from 'node:crypto';
import {oneBotError} from './content-service.mjs';
import {readBranch} from './history.mjs';
import {visibleMessageTextForConversationSearch} from '../cindy/bots/search/conversationSearch.pure.ts';
import {isSyntheticTriggerText} from '../cindy/bots/shared/interruptedTurn.ts';
import {BOT_AVATAR_MAX_BYTES, isManagedBotAvatarUrl} from '../cindy/bots/shared/botAvatarValue.ts';
import {botInvitationProgress} from '../cindy/bots/shared/botInvitation.ts';
import {BOT_FAILURE_REASONS, isBotFailureAttentionWorthy} from '../cindy/bots/shared/botFailureReason.ts';
import {inferBotTemplatePresetId, isBotTemplatePresetId} from '../cindy/bots/shared/botTemplatePreset.ts';
import {normalizeBotWelcomeContext} from '../cindy/bots/shared/botWelcomeContext.ts';
import {SUPPORTED_LOCALES} from '../cindy/tasks/shared/locale.ts';

/** Anything above this is rendered as `99+` (Cindy CANONICAL_UNREAD_SCAN). */
const UNREAD_SCAN = 100;
const AVATAR_TYPES = {'image/png': 'png', 'image/jpeg': 'jpg', 'image/webp': 'webp'};
// Group chat images also accept GIF (Cindy's composer image types).
const MANAGED_IMAGE_TYPES = {...AVATAR_TYPES, 'image/gif': 'gif'};
const GROUP_IMAGE_MAX_BYTES = 20 << 20;

/** Upstream readProfile's shape for the teammate pages; internal fields stay for Host callers. */
export function presentProfile(paths, profile, sessions) {
  const capabilities = profile.capabilities ?? {};
  const failureReason = BOT_FAILURE_REASONS.includes(profile.attentionReason) ? profile.attentionReason : null;
  const invitation = botInvitationProgress(capabilities.invitation);
  if (invitation && isManagedBotAvatarUrl(profile.avatar)) invitation.avatarSkipped = false;
  // Invitation checkpoints (draft text, request ids) are main-owned and not displayed.
  const {invitation: _invitation, ...visible} = capabilities;
  return {
    ...profile,
    enabled: profile.status === 'active',
    invitation,
    userContextSource: typeof capabilities.userContextSource === 'string' ? capabilities.userContextSource : '',
    ...(capabilities.gender === 'female' || capabilities.gender === 'male' ? {gender: capabilities.gender} : {}),
    templateId: typeof capabilities.templateId === 'string' ? capabilities.templateId : inferBotTemplatePresetId(profile.identitySource ?? '') ?? undefined,
    failureReason,
    needsAttention: profile.attentionAt != null && failureReason !== null && isBotFailureAttentionWorthy(failureReason),
    homeDir: paths.home(profile.id),
    skills: Array.isArray(capabilities.skills) ? capabilities.skills.filter(item => typeof item === 'string') : [],
    // Cindy names Full access `trusted`; WinOne Chat names it bypassPermissions.
    capabilities: {...visible, permissions: visible.permissions === 'bypassPermissions' ? 'trusted' : visible.permissions},
    sessions,
  };
}

/**
 * Rows of a companion transcript that Cindy shows (readCanonicalChatPreview,
 * countCanonicalUnread, listBotDmThreads): user inputs the user wrote — not a
 * hidden host instruction or synthetic UI trigger — and completed assistant
 * replies; narration before a tool call is progress, not a reply. `clientId`
 * is the accepted input id for a user row and the Pi entry id otherwise.
 */
export function visibleRows(branch) {
  const rows = [];
  let receipt;
  for (const entry of branch) {
    if (entry.type === 'custom' && entry.customType === 'winone.input') receipt = entry.data;
    if (entry.type !== 'message' || !['user', 'assistant'].includes(entry.message?.role)) continue;
    const message = entry.message, at = Date.parse(entry.timestamp);
    if (message.role === 'user') {
      const input = receipt;
      receipt = undefined;
      if (input?.presentation?.hidden === true) continue;
      const text = visibleMessageTextForConversationSearch('user', typeof input?.text === 'string' ? input.presentation?.text ?? input.text : message.content);
      if (text && !isSyntheticTriggerText(text)) rows.push({clientId: input?.id ?? entry.id, role: 'user', text, at});
      continue;
    }
    if (message.stopReason === 'toolUse') continue;
    const text = visibleMessageTextForConversationSearch('assistant', message.content);
    if (text) rows.push({clientId: entry.id, role: 'assistant', text, at});
  }
  return rows;
}

const textOf = content => typeof content === 'string' ? content
  : Array.isArray(content) ? content.filter(part => part?.type === 'text' && typeof part.text === 'string').map(part => part.text).join('\n') : '';

/**
 * Cindy's `messages` row for one message of a companion chat, by the client id
 * the page shows (the reply-quote note rereads the original from here). A user
 * row carries Cindy's `{text, images, files}` envelope; an assistant row its
 * text. A message on an abandoned branch is not found, like a rewound row.
 */
export function messageRow(branch, clientId) {
  let receipt;
  for (const entry of branch) {
    if (entry.type === 'custom' && entry.customType === 'winone.input') receipt = entry.data;
    if (entry.type !== 'message') continue;
    const message = entry.message;
    if (message?.role === 'user') {
      const input = receipt;
      receipt = undefined;
      if ((input?.id ?? entry.id) !== clientId) continue;
      // Host-only triggers never become quoteable owner messages, even when
      // their receipt id is known from the persisted transcript.
      if (input?.presentation?.hidden === true) return null;
      const attachments = Array.isArray(input?.attachments) ? input.attachments : [];
      const image = file => typeof file?.mimeType === 'string' && file.mimeType.startsWith('image/');
      const text = typeof input?.text === 'string' ? input.presentation?.text ?? input.text : textOf(message.content);
      return {role: 'user', rewindAt: null, content: JSON.stringify({text,
        images: attachments.filter(image).map(file => ({originalName: file.name})),
        files: attachments.filter(file => !image(file)).map(file => ({name: file.name}))})};
    }
    if (message?.role === 'assistant' && entry.id === clientId) return {role: 'assistant', rewindAt: null, content: JSON.stringify(textOf(message.content))};
  }
  return null;
}

/**
 * Latest visible message and unread replies of a canonical chat. The text is
 * returned unformatted: the page applies Cindy's Markdown preview where the
 * Markdown parser lives.
 */
export async function chatPreview(session, lastReadAt) {
  if (!session?.sessionFile) return {text: null, at: null, role: null, unread: 0};
  const rows = visibleRows(await readBranch(session));
  const last = rows.at(-1);
  let unread = 0;
  if (lastReadAt != null) for (const row of rows) {
    if (row.role === 'assistant' && Number.isFinite(row.at) && row.at > lastReadAt && unread < UNREAD_SCAN) unread++;
  }
  return {text: last?.text ?? null, at: Number.isFinite(last?.at) ? last.at : null, role: last?.role ?? null, unread};
}

/** Cindy DM_THREAD_STATS_SCAN: a longer topic still shows its latest activity. */
const THREAD_STATS_SCAN = 500;
/** Topic entry counts (Cindy listBotDmThreads) and whether a message is still in a chat. */
export async function transcriptStats(session, clientId) {
  if (!session?.sessionFile) return {replyCount: 0, lastReplyAt: null, lastAssistantAt: null, hasClient: false};
  const rows = visibleRows(await readBranch(session));
  const recent = rows.slice(-THREAD_STATS_SCAN);
  const finite = value => Number.isFinite(value) ? value : null;
  return {
    replyCount: recent.length,
    lastReplyAt: finite(recent.at(-1)?.at),
    lastAssistantAt: finite(recent.findLast(row => row.role === 'assistant')?.at),
    hasClient: clientId !== undefined && rows.some(row => row.clientId === clientId),
  };
}

function sniffImage(bytes) {
  if (bytes.length >= 8 && bytes.subarray(0, 8).equals(Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]))) return 'image/png';
  if (bytes.length >= 3 && bytes[0] === 0xff && bytes[1] === 0xd8 && bytes[2] === 0xff) return 'image/jpeg';
  if (bytes.length >= 12 && bytes.toString('latin1', 0, 4) === 'RIFF' && bytes.toString('latin1', 8, 12) === 'WEBP') return 'image/webp';
  return null;
}

/** A group chat image: bounded base64 of PNG, JPEG, WebP or GIF bytes. */
export function decodeGroupImage(value) {
  if (typeof value !== 'string' || !value || value.length % 4 !== 0 || !/^[A-Za-z0-9+/]+={0,2}$/.test(value)) throw oneBotError('INVALID_ATTACHMENT', '图片数据无效');
  if (value.length > Math.ceil(GROUP_IMAGE_MAX_BYTES / 3) * 4) throw oneBotError('ATTACHMENT_TOO_LARGE', '图片不得超过 20 MB');
  const bytes = Buffer.from(value, 'base64');
  if (!bytes.length || bytes.length > GROUP_IMAGE_MAX_BYTES) throw oneBotError('ATTACHMENT_TOO_LARGE', '图片不得超过 20 MB');
  const gif = bytes.length >= 6 && ['GIF87a', 'GIF89a'].includes(bytes.toString('latin1', 0, 6));
  const mimeType = gif ? 'image/gif' : sniffImage(bytes);
  if (!mimeType) throw oneBotError('INVALID_ATTACHMENT', '图片只支持 PNG、JPEG、WebP 或 GIF');
  return {bytes, mimeType};
}

/** Cindy decodeBotAvatarImage: bounded base64 of PNG, JPEG or WebP bytes. */
export function decodeAvatarImage(value) {
  if (value === undefined) return null;
  if (typeof value !== 'string' || !value.length || value.length > Math.ceil(BOT_AVATAR_MAX_BYTES / 3) * 4 || value.length % 4 !== 0 || !/^[A-Za-z0-9+/]+={0,2}$/.test(value)) {
    throw oneBotError('INVALID_PARAMS', '头像图片格式无效，请重新选择 PNG、JPEG 或 WebP 图片');
  }
  const bytes = Buffer.from(value, 'base64');
  if (!bytes.length || bytes.length > BOT_AVATAR_MAX_BYTES) throw oneBotError('INVALID_PARAMS', '头像图片必须小于 5 MB');
  const mimeType = sniffImage(bytes);
  if (!mimeType) throw oneBotError('INVALID_PARAMS', '头像只支持 PNG、JPEG 或 WebP 图片');
  return {bytes, mimeType};
}

/**
 * Content-addressed avatar bytes under the OneBot data directory, named with
 * Cindy's managed address (`cindy-media://blobs/<sha256>.<ext>`). The file is
 * published before a profile refers to it; an unreferenced blob is harmless.
 */
export function storeAvatarImage(paths, image) {
  const hash = createHash('sha256').update(image.bytes).digest('hex'), ext = MANAGED_IMAGE_TYPES[image.mimeType];
  const directory = path.join(paths.root, 'media', 'blobs'), file = path.join(directory, `${hash}.${ext}`);
  paths.assertPath(file);
  fs.mkdirSync(directory, {recursive: true});
  if (!fs.existsSync(file)) {
    const temporary = path.join(directory, `.${hash}.${randomUUID()}.tmp`);
    paths.assertPath(temporary);
    fs.writeFileSync(temporary, image.bytes, {flag: 'wx'});
    try { fs.renameSync(temporary, file); }
    catch (error) { fs.rmSync(temporary, {force: true}); if (!fs.existsSync(file)) throw error; }
  }
  return `cindy-media://blobs/${hash}.${ext}`;
}

/**
 * Cindy createBotProfile's input contract. Returns the store input and the
 * invitation request; the caller stores avatar bytes first. Retired template
 * ids are accepted as ordinary teammates; creation drafts are not offered.
 */
export function creationInput(raw, defaultLocale,draftEntry) {
  const body = raw && typeof raw === 'object' && !Array.isArray(raw) ? raw : {};
  if (body.creationDraftToken !== undefined&&!draftEntry) throw oneBotError('PRECONDITION_FAILED', '伙伴草稿已过期，请重新生成');
  if(draftEntry){body.id=draftEntry.botId;body.name??=draftEntry.draft.name;body.description??=draftEntry.draft.description;}
  const templateId = body.templateId === 'dash' || body.templateId === 'lizi' ? undefined : body.templateId;
  if (templateId !== undefined && !isBotTemplatePresetId(templateId)) throw oneBotError('INVALID_PARAMS', '未知的伙伴模板');
  if(draftEntry&&templateId!==undefined)throw oneBotError('INVALID_PARAMS','请选择创建伙伴或预设伙伴');
  const requested = body.capabilities && typeof body.capabilities === 'object' && !Array.isArray(body.capabilities) ? body.capabilities : null;
  const skills = Array.isArray(body.skills) ? body.skills.filter(item => typeof item === 'string').slice(0, 100) : [];
  const gender = body.gender === 'female' || body.gender === 'male' ? body.gender : undefined;
  // Older clients may request a welcome; its supplied text is never an assistant message.
  const prepareInvitation = body.prepareInvitation === true || typeof body.welcomeMessage === 'string' && body.welcomeMessage.trim() !== '';
  const capabilities = {
    permissions: 'auto', toolsetMode: 'inherit', mcpMode: 'inherit',
    // A new teammate without its own route follows the teammate default chain.
    ...(requested ? {} : {modelOverride: null, modelChainOverride: null, skillMode: 'allowlist', skillsExcluded: [], toolsets: [], mcpServers: [], memory: true}),
    ...requested, toolCapabilityVersion: 1, skills,
    ...(gender ? {gender} : {}),
  };
  delete capabilities.invitation;
  if(draftEntry){capabilities.skillMode='allowlist';capabilities.skills=[...draftEntry.draft.skillRefs];capabilities.mcpServers=draftEntry.draft.mcpRefs;capabilities.toolsets=draftEntry.draft.toolsetRefs;}
  delete capabilities.templateId;
  if (templateId) capabilities.templateId = templateId;
  const avatarImage = decodeAvatarImage(body.avatarImageBase64);
  if (prepareInvitation) capabilities.invitation = {
    id: randomUUID(), stage: 'skills',
    locale: SUPPORTED_LOCALES.includes(body.locale) ? body.locale : defaultLocale,
    avatarRequested: body.generateAvatar === true && !avatarImage,
    welcomeContext: normalizeBotWelcomeContext(body.welcomeContext),
    ...(draftEntry?{draft:{...draftEntry.draft,background:draftEntry.draft.background+'\n\nCurrent profile (use this name and introduction):\n'+body.name+'\n'+body.description}}:{}),
  };
  return {
    input: {id: body.id, name: body.name ?? body.displayName, description: body.description, avatar: body.avatar,
      avatarColor: body.avatarColor, identitySource: body.identitySource, userContextSource: body.userContextSource, skills, capabilities},
    avatarImage, prepareInvitation,
  };
}
