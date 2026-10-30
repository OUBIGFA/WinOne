// WebView2 cannot expose File paths like Electron. Keep the original Cindy
// attachment flow by staging bounded chunks in the group's existing folder.
// Reads and native actions resolve only managed files or a saved group's work.
import fs from 'node:fs/promises';
import fsSync from 'node:fs';
import path from 'node:path';
import {randomUUID} from 'node:crypto';
import {eq} from 'drizzle-orm';
import {botGroups, botGroupPlans, botGroupMessages} from '../cindy/tasks/db/schema.ts';
import {isDangerousAttachmentName} from '../cindy/tasks/shared/attachmentSafety.ts';
import {decodeGroupImage, storeAvatarImage} from './bots-service.mjs';
import {oneBotError} from './content-service.mjs';
import {resolveWorkspacePathCached,resolveWorkspacePathBatchCached} from '../cindy/tasks/pathResolver.ts';

const MAX_FILE = 20 << 20, CHUNK = 256 << 10;
const media = /^cindy-media:\/\/blobs\/([0-9a-f]{64})\.(png|jpg|jpeg|gif|webp)$/;
const inside = (root, file) => {const relative = path.relative(root.toLowerCase(), file.toLowerCase()); return relative === '' || relative !== '..' && !relative.startsWith('..' + path.sep) && !path.isAbsolute(relative);};
const safeName = value => (String(value).split(/[\\/]/).pop().replace(/[\u0000-\u001f<>:"|?*]/g, '_').replace(/^[.\s]+|[.\s]+$/g, '') || 'attachment').slice(0, 120);

export function createGroupFiles(scope) {
  const {paths, taskStore, host} = scope, uploads = new Map();
  const readableRoots=()=>[path.join(paths.root,'bot-groups'),...scope.profiles.list().map(profile=>path.join(paths.home(profile.id),'workspace')),
    ...taskStore.drizzle.select().from(botGroups).all().map(row=>row.projectDir).filter(Boolean),...taskStore.drizzle.select().from(botGroupPlans).all().map(row=>row.workDir).filter(Boolean)];
  const resolverOptions=()=>{const roots=readableRoots().flatMap(root=>{try{return [fsSync.realpathSync(root)];}catch{return [];}});return {isPathAllowed:file=>{try {const actual=fsSync.realpathSync(file);return roots.some(root=>inside(root,actual));}catch{return false;}}};};
  const group = id => {const row = typeof id === 'string' ? taskStore.drizzle.select().from(botGroups).where(eq(botGroups.id, id)).get() : null; if (!row) throw oneBotError('NOT_FOUND', '群聊不存在'); return row;};
  const staged = token => {const upload = uploads.get(token); if (!upload) throw oneBotError('NOT_FOUND', '附件上传已失效，请重新选择文件'); group(upload.groupId); return upload;};
  async function resolve(value) {
    if (typeof value !== 'string' || !value) throw oneBotError('INVALID_PARAMS', '群聊文件位置无效');
    const image = media.exec(value);
    const file = image ? path.join(paths.root, 'media', 'blobs', image[1] + '.' + image[2]) : value;
    if (!path.isAbsolute(file)) throw oneBotError('INVALID_PARAMS', '群聊文件位置必须是绝对路径');
    const actual = await fs.realpath(file);
    const blobRoot=path.join(paths.root,'media','blobs');
    const namedBlob=path.dirname(path.resolve(file)).toLowerCase()===blobRoot.toLowerCase()&&media.test('cindy-media://blobs/'+path.basename(file));
    const roots = [...readableRoots(), ...(image||namedBlob ? [blobRoot] : [])];
    let permitted = false;
    for (const root of roots) {
      const resolvedRoot = await fs.realpath(root).catch(error => {if (error.code === 'ENOENT') return null; throw error;});
      if (resolvedRoot && inside(resolvedRoot, actual)) {permitted = true; break;}
    }
    if (!permitted) {
      // A chosen file can stay in place in Cindy's native attachment path. Its
      // persisted message grants preview access only to that exact file.
      for (const row of taskStore.drizzle.select().from(botGroupMessages).all()) {
        const entries = typeof row.attachments === 'string' ? JSON.parse(row.attachments) : row.attachments ?? [];
        if (entries.some(item => item.path && path.resolve(item.path).toLowerCase() === path.resolve(file).toLowerCase())) {permitted = true; break;}
      }
    }
    if (!permitted) throw oneBotError('OUTSIDE_SCOPE', '文件不属于群聊附件或已选择的项目');
    const stat = await fs.stat(actual);
    if (!stat.isFile()) throw oneBotError('INVALID_PARAMS', '群聊附件不是普通文件');
    return {path: actual, size: stat.size, modified: stat.mtimeMs};
  }
  async function describe(params) {
    const item = await resolve(params.path);
    const file = await fs.open(item.path, 'r');
    let mimeType = 'application/octet-stream';
    try {const {buffer, bytesRead} = await file.read(Buffer.alloc(64), 0, 64, 0); try {mimeType = decodeGroupImage(buffer.subarray(0, bytesRead).toString('base64')).mimeType;} catch { /* Non-images stay ordinary files. */ }} finally {await file.close();}
    return {...item, resolvedPath: item.path, name: path.basename(item.path), mimeType, kind: mimeType.startsWith('image/') ? 'image' : 'file', downloadOnly: isDangerousAttachmentName(item.path) || item.path.endsWith('.bin')};
  }
  return {
    async begin(params) {
      group(params.groupId);
      if (!Number.isSafeInteger(params.size) || params.size < 0 || params.size > MAX_FILE || typeof params.name !== 'string' || !params.name || params.name.length > 255) throw oneBotError('INVALID_PARAMS', '附件不得超过 20 MB');
      const token = randomUUID(), directory = path.join(paths.root, 'bot-groups', params.groupId, 'attachments', 'staged', token);
      paths.assertPath(directory);
      let name = safeName(params.name);
      if (isDangerousAttachmentName(name)) name += '.bin';
      await fs.mkdir(directory, {recursive: true});
      const file = path.join(directory, name);
      await fs.writeFile(file, Buffer.alloc(0), {flag: 'wx'});
      uploads.set(token, {path: file, groupId: params.groupId, size: params.size, written: 0});
      return {token};
    },
    async chunk(params) {
      const upload = staged(params.token);
      if (params.offset !== upload.written) throw oneBotError('CONFLICT', '附件分段顺序不一致');
      if (typeof params.base64 !== 'string' || params.base64.length > Math.ceil(CHUNK / 3) * 4) throw oneBotError('INVALID_PARAMS', '附件分段过大');
      const bytes = Buffer.from(params.base64, 'base64');
      if (upload.written + bytes.length > upload.size || !bytes.length) throw oneBotError('INVALID_PARAMS', '附件大小与声明不一致');
      await fs.appendFile(upload.path, bytes);
      upload.written += bytes.length;
      return {offset: upload.written};
    },
    async finish(params) {
      const upload = staged(params.token);
      if (upload.written !== upload.size) throw oneBotError('INCOMPLETE', '附件尚未传输完成');
      let url;
      if (params.image) url = storeAvatarImage(paths, decodeGroupImage((await fs.readFile(upload.path)).toString('base64')));
      const file = await describe({path: upload.path});
      uploads.delete(params.token);
      return {...file, ...(url ? {url} : {})};
    },
    describe,
    resolvePath:params=>resolveWorkspacePathCached(params.href,params.workingDir,resolverOptions()),
    resolvePathBatch:params=>resolveWorkspacePathBatchCached(params.hrefs,params.workingDir,resolverOptions()),
    draftUrls(params) {if(!Array.isArray(params.urls)||params.urls.length>4096||params.urls.some(url=>typeof url!=='string'||!media.test(url)))throw oneBotError('INVALID_PARAMS','草稿图片引用无效');scope.profiles.db.query('INSERT OR REPLACE INTO bot_media_draft_urls VALUES(1,?)').run(JSON.stringify(params.urls));return {ok:true};},
    async remoteImage(params) {
      const url=new URL(params.url);
      if(!['http:','https:'].includes(url.protocol)||url.username||url.password)throw oneBotError('INVALID_PARAMS','图片地址无效');
      const response=await fetch(url,{signal:AbortSignal.timeout(20000)});
      if(!response.ok||!response.body)throw oneBotError('READ_FAILED','图片下载失败');
      const chunks=[];let size=0;
      for await(const bytes of response.body){size+=bytes.byteLength;if(size>MAX_FILE)throw oneBotError('ATTACHMENT_TOO_LARGE','图片不得超过 20 MB');chunks.push(bytes);}
      const image=decodeGroupImage(Buffer.concat(chunks).toString('base64'));
      return {url:storeAvatarImage(paths,image),mimeType:image.mimeType};
    },
    async classify(params) {try {await resolve(params.path); return {kind:'other'};} catch(error) {if(error.code==='INVALID_PARAMS'&&error.message==='群聊附件不是普通文件')return {kind:'directory'};throw error;}},
    async read(params) {
      const item = await resolve(params.path), offset = params.offset ?? 0, limit = params.limit ?? CHUNK;
      if (!Number.isSafeInteger(offset) || offset < 0 || !Number.isSafeInteger(limit) || limit < 1 || limit > CHUNK) throw oneBotError('INVALID_PARAMS', '文件读取范围无效');
      const file = await fs.open(item.path, 'r');
      try {const {buffer, bytesRead} = await file.read(Buffer.alloc(limit), 0, limit, offset); return {...item, base64: buffer.subarray(0, bytesRead).toString('base64'), next: offset + bytesRead, done: offset + bytesRead >= item.size};} finally {await file.close();}
    },
    async image(params) {
      const item = await resolve(params.path);
      if (item.size > MAX_FILE) throw oneBotError('ATTACHMENT_TOO_LARGE', '图片不得超过 20 MB');
      const decoded = decodeGroupImage((await fs.readFile(item.path)).toString('base64'));
      return {url: storeAvatarImage(paths, decoded), mimeType: decoded.mimeType};
    },
    async discard(params) {
      if (!Array.isArray(params.paths) || params.paths.length > 32) throw oneBotError('INVALID_PARAMS', '暂存附件无效');
      const referenced = new Set(taskStore.drizzle.select().from(botGroupMessages).all().flatMap(row => {const entries = typeof row.attachments === 'string' ? JSON.parse(row.attachments) : row.attachments ?? []; return entries.map(item => item.path).filter(Boolean);}));
      for (const file of params.paths) {
        if (referenced.has(file)) continue;
        const relative = path.relative(path.join(paths.root, 'bot-groups'), file).split(path.sep);
        if (relative.length !== 5 || relative[1] !== 'attachments' || relative[2] !== 'staged' || !/^[0-9a-f-]{36}$/.test(relative[3])) continue;
        await host.recycleStagedGroupFolder(path.dirname(file));
      }
      return {ok: true};
    },
  };
}
