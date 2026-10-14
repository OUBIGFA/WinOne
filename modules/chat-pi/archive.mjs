// Cindy's share envelope and crypto are vendored unchanged. This adapter only
// collects WinOne's Pi-owned snapshot and maps it to Cindy's portable entries.
import {createHash,randomBytes} from 'node:crypto';
import {createReadStream} from 'node:fs';
import fs from 'node:fs/promises';
import path from 'node:path';
import JSZip from 'jszip';
import {parseSessionEntries,migrateSessionEntries} from '@earendil-works/pi-coding-agent';
import {buildPlainFile,sealPayload} from './cindy/xdtshareCrypto.ts';
import {XDTSHARE_MIN_READER_VERSION,validateManifest} from './cindy/xdtshareFormat.pure.ts';
import packageInfo from './package.json' with {type:'json'};

export const shareExportSizeLimitBytes=256*1024*1024;
const digest=value=>createHash('sha256').update(value).digest('hex');
const inside=(root,file)=>{const relative=path.relative(root,file);return relative===''||relative!=='..'&&!relative.startsWith('..'+path.sep)&&!path.isAbsolute(relative);};
const fileKey=value=>process.platform==='win32'?path.resolve(value).toLowerCase():path.resolve(value);
const instant=value=>typeof value==='number'&&Number.isFinite(value)?value:Number.isFinite(Date.parse(value))?Date.parse(value):0;
const imageExtensions={'image/png':'.png','image/jpeg':'.jpg','image/gif':'.gif','image/webp':'.webp','image/svg+xml':'.svg'};
const safeName=value=>path.basename(String(value??'attachment')).replace(/[\\/\x00-\x1f]/g,'_').slice(0,160)||'attachment';

async function destination(targetPath){
 if(typeof targetPath!=='string'||!path.isAbsolute(targetPath)||targetPath.includes('\0')||path.extname(targetPath).toLowerCase()!=='.cshare')throw Error('Choose a .cshare file destination');
 const parent=await fs.realpath(path.dirname(targetPath));if(!(await fs.stat(parent)).isDirectory())throw Error('The export folder does not exist');
 const target=path.join(parent,path.basename(targetPath)),existing=await fs.lstat(target).catch(error=>{if(error.code==='ENOENT')return null;throw error;});
 if(existing&&(!existing.isFile()||existing.isSymbolicLink()))throw Error('The export destination must be a regular file');
 return {parent,target};
}

// Stop before a concurrently growing file can exceed the same memory budget
// checked by stat. No file read can allocate an unbounded Buffer first.
async function readBounded(file,maximum,signal){
 const chunks=[];let size=0;
 for await(const chunk of createReadStream(file,{signal})){size+=chunk.length;if(size>maximum)return {oversize:size};chunks.push(chunk);}
 return {buffer:Buffer.concat(chunks,size)};
}

async function managedRoots(options){
 const roots=[path.join(options.conversation.sessionDirectory,'attachments'),...(options.attachmentRoots??[])];
 // The Host supplies its own catalog path, not a renderer value. Registered
 // session roots keep forked histories' original attachment copies available.
 if(options.catalogPath){const stat=await fs.stat(options.catalogPath);if(stat.size>4*1024*1024)throw Error('Chat catalog is too large');const catalog=JSON.parse(await fs.readFile(options.catalogPath,'utf8'));for(const conversation of catalog.conversations??[])if(path.isAbsolute(conversation.sessionDirectory??''))roots.push(path.join(conversation.sessionDirectory,'attachments'));}
 const canonical=[];
 for(const root of roots){if(!path.isAbsolute(root))throw Error('Invalid managed attachment root');try{canonical.push(await fs.realpath(root));}catch(error){if(!['ENOENT','ENOTDIR'].includes(error.code))throw error;}}
 return [...new Set(canonical)];
}

async function resolveAttachment(file,roots,conversation){
 if(typeof file!=='string'||!path.isAbsolute(file)||file.includes('\0'))return null;
 const choices=[file];
 // Existing Save location copies preserve the relative attachment layout while
 // native Pi history stays byte-identical. Resolve that copied location too.
 const parts=file.replaceAll('\\','/').split('/'),marker=parts.lastIndexOf('attachments');
 if(marker>=0&&parts.length>marker+2){const suffix=parts.slice(marker+1);if(suffix.every(part=>part&&part!=='.'&&part!=='..'))choices.push(path.join(conversation.sessionDirectory,'attachments',...suffix));}
 for(const candidate of choices){
  try{const resolved=await fs.realpath(candidate);if(!roots.some(root=>inside(root,resolved)))continue;const stat=await fs.stat(resolved);if(stat.isFile())return {file:resolved,bytes:stat.size};}
  catch(error){if(!['ENOENT','ENOTDIR','EACCES','EPERM'].includes(error.code))throw error;}
 }
 return null;
}

function sessionSnapshot(conversation,portableId,entries){
 const assistants=entries.filter(entry=>entry.type==='message'&&entry.message?.role==='assistant').map(entry=>entry.message);
 const finite=value=>typeof value==='number'&&Number.isFinite(value)&&value>=0?value:0;
 const totalTokenUsage=assistants.reduce((sum,message)=>sum+finite(message.usage?.totalTokens),0),totalCostUsd=assistants.reduce((sum,message)=>sum+finite(message.usage?.cost?.total),0);
 const last=assistants.at(-1)?.usage;
 return {title:conversation.title,workspaceKind:conversation.projectId?'project':'dialogue',model:conversation.model?.model??'',effort:conversation.thinkingLevel??'off',permissionMode:conversation.permission??'ask',sdkSessionId:portableId,totalTokenUsage,totalCostUsd,contextTokens:finite(last?.input)+finite(last?.cacheRead)+finite(last?.cacheWrite),contextWindow:0,fastMode:!!conversation.fastMode,planModeEnabled:false,agentKind:'pi',codexHistoryHasProductPrompt:null,clearedAt:null,userSendAt:null,createdAt:instant(conversation.createdAt),updatedAt:instant(conversation.updatedAt)};
}

export async function exportConversationArchive(options){
 options?.signal?.throwIfAborted();
 const conversation=options?.conversation;
 if(!conversation||typeof conversation.id!=='string'||!path.isAbsolute(conversation.sessionDirectory??'')||!path.isAbsolute(conversation.directory??''))throw Error('Invalid registered Chat conversation');
 if(options.password!==undefined&&(typeof options.password!=='string'||options.password.length>4096))throw Error('Invalid share password');
 if(options.excludeMedia!==undefined&&typeof options.excludeMedia!=='boolean')throw Error('Invalid media export choice');
 const target=await destination(options.targetPath),limitBytes=shareExportSizeLimitBytes;
 let transcript,entries=[],portableId=null,transcriptBytes=0;
 const missingTranscripts=[];
 if(conversation.sessionFile){
  if(!inside(conversation.sessionDirectory,conversation.sessionFile))throw Error('The Pi transcript is outside the registered session folder');
  try{
   const realFile=await fs.realpath(conversation.sessionFile),realRoot=await fs.realpath(conversation.sessionDirectory);
   if(!inside(realRoot,realFile))throw Error('The Pi transcript resolves outside the registered session folder');
   const stat=await fs.stat(realFile);if(!stat.isFile())throw Error('The Pi transcript is not a regular file');transcriptBytes=stat.size;
   if(transcriptBytes>limitBytes)return {status:'oversize',totalBytes:transcriptBytes,mediaBytes:0,transcriptBytes,messagesBytes:0,limitBytes};
   const read=await readBounded(realFile,limitBytes,options.signal);if(read.oversize)return {status:'oversize',totalBytes:read.oversize,mediaBytes:0,transcriptBytes:read.oversize,messagesBytes:0,limitBytes};
   transcript=read.buffer;transcriptBytes=transcript.length;
  }catch(error){if(!['ENOENT','ENOTDIR','EACCES','EPERM'].includes(error.code))throw error;missingTranscripts.push('pi-missing-'+digest(conversation.sessionFile).slice(0,32)+'.jsonl');}
 }
 if(transcript?.length){
  const text=new TextDecoder('utf-8',{fatal:true}).decode(transcript);entries=parseSessionEntries(text);
  if(entries.length!==text.split('\n').filter(line=>line.trim()).length||entries[0]?.type!=='session')throw Error('The Pi transcript contains an incomplete or damaged record. Wait for the current write and retry.');
  migrateSessionEntries(entries);portableId='pi-'+digest(transcript).slice(0,32)+'.jsonl';
 }else if(conversation.hasMessages&&!missingTranscripts.length){missingTranscripts.push('pi-missing-'+digest(conversation.id).slice(0,32)+'.jsonl');}
 const roots=await managedRoots(options),media=[],byPath=new Map(),byInline=new Map();
 async function addAttachment(attachment){
  const source=attachment?.path;if(typeof source!=='string')return;
  const key=fileKey(source);if(byPath.has(key))return;
  const resolved=await resolveAttachment(source,roots,conversation),filename=safeName(attachment.name??source);
  const prior=resolved?media.find(item=>item.file&&fileKey(item.file)===fileKey(resolved.file)):undefined;
  if(prior){byPath.set(key,prior);return;}
  let dropped=false;
  // Distinguish a file absent at the source from one that exists but cannot be
  // included (for example, an out-of-scope path in imported history).
  if(!resolved&&path.isAbsolute(source)){try{dropped=(await fs.stat(source)).isFile();}catch(error){dropped=!['ENOENT','ENOTDIR'].includes(error.code);}}
  const item={url:'xdt-file://winone-share/'+digest(source).slice(0,24)+'/'+encodeURIComponent(filename),scheme:'xdt-file',kind:'loose',filename,zipPath:null,bytes:resolved?.bytes??0,file:resolved?.file,dropped};
  media.push(item);byPath.set(key,item);
 }
 function addImage(part,key){
  if(typeof part.data!=='string')return undefined;
  const fingerprint=digest(part.data);if(byInline.has(fingerprint))return byInline.get(fingerprint);
  const filename='image-'+digest(key).slice(0,20)+(imageExtensions[part.mimeType]??'.bin');
  const item={url:'xdt-image://winone-share/'+filename,scheme:'xdt-image',kind:'image',imageHost:'winone-share',filename,zipPath:null,bytes:Buffer.byteLength(part.data,'base64'),inline:part.data};
  media.push(item);byInline.set(fingerprint,item);return item;
 }
 for(const entry of entries){
  if(entry.type==='custom'&&entry.customType==='winone.input')for(const attachment of entry.data?.attachments??[])await addAttachment(attachment);
  if(entry.type==='message'&&Array.isArray(entry.message?.content))entry.message.content.forEach((part,index)=>{if(part.type==='image')addImage(part,entry.id+':'+index);});
 }
 for(const attachment of options.draft?.attachments??[])await addAttachment(attachment);
 const portableAttachment=attachment=>{const item=byPath.get(fileKey(attachment.path));return {...attachment,title:attachment.name,path:item?.url??attachment.path,url:item?.url??attachment.path};};
 const portablePart=(part,key)=>part.type==='image'&&part.data?{type:'image',mimeType:part.mimeType,url:addImage(part,key)?.url}:part;
 const byId=new Map(entries.filter(entry=>entry.id).map(entry=>[entry.id,entry])),activeIds=new Set();let leaf=entries.at(-1);
 while(leaf?.id&&!activeIds.has(leaf.id)){activeIds.add(leaf.id);leaf=byId.get(leaf.parentId);}
 const messages=[];let receipt;
 const modelByTime=new Map(),thinkingByTime=new Map();
 for(const entry of entries)if(entry.type==='custom'){if(entry.customType==='winone.model')modelByTime.set(entry.data?.timestamp,entry.data?.identity);if(entry.customType==='winone.thinking')thinkingByTime.set(entry.data?.timestamp,entry.data?.blocks);}
 const addRow=(entry,suffix,role,content,toolUseId=null)=>{
  const message=entry.message,createdAt=instant(message.timestamp??entry.timestamp);
  messages.push({id:entry.id+suffix,clientId:entry.id+suffix,role,content:JSON.stringify(content),toolUseId,agentKind:'pi',agentMeta:JSON.stringify({sdkSessionId:portableId,piEntryId:entry.id,...modelByTime.get(message.timestamp),thinkingBlocks:thinkingByTime.get(message.timestamp)}),createdAt,rewindAt:activeIds.has(entry.id)?null:instant(conversation.updatedAt)});
 };
 for(const entry of entries){
  if(entry.type==='custom'&&entry.customType==='winone.input'){receipt=entry.data;continue;}
  if(entry.type!=='message'||!entry.message)continue;
  const message=entry.message,parts=Array.isArray(message.content)?message.content:[],portable=parts.map((part,index)=>portablePart(part,entry.id+':'+index));
  const text=typeof message.content==='string'?message.content:parts.filter(part=>part.type==='text').map(part=>part.text).join('\n');
  if(message.role==='user'){
   addRow(entry,'','user',{text:typeof receipt?.text==='string'?receipt.text:text,files:(receipt?.attachments??[]).map(portableAttachment),images:portable.filter(part=>part.type==='image'),content:portable});receipt=undefined;
  }else if(message.role==='assistant'&&parts.length){
   parts.forEach((part,index)=>{const suffix=':'+index;if(part.type==='thinking')addRow(entry,suffix,'thinking',{text:part.thinking,...thinkingByTime.get(message.timestamp)?.find(block=>block.index===index)});else if(part.type==='toolCall')addRow(entry,suffix,'tool_use',{toolName:part.name,input:part.arguments,toolUseId:part.id},part.id);else addRow(entry,suffix,'assistant',{text:part.text??'',content:portable[index],...(part.type==='image'?{images:[portable[index]]}:{})});});
  }else if(message.role==='toolResult')addRow(entry,'','tool_result',{text,content:portable,details:message.details,isError:message.isError,toolName:message.toolName},message.toolCallId??null);
  else addRow(entry,'',message.role==='assistant'?'assistant':'system',{...message,content:typeof message.content==='string'?message.content:portable,text});
 }
 const draft={...options.draft,attachments:(options.draft?.attachments??[]).map(portableAttachment)};
 const session=sessionSnapshot(conversation,portableId,entries);
 const rows=messages.map(message=>JSON.stringify(message)).join('\n'),messagesBytes=Buffer.byteLength(rows);
 const metadata=JSON.stringify({conversation:{...conversation,sessionFile:portableId,sessionDirectory:'.'},draft},null,2);
 let mediaBytes=options.excludeMedia?0:media.reduce((sum,item)=>sum+item.bytes,0);
 let totalBytes=messagesBytes+transcriptBytes+mediaBytes+Buffer.byteLength(metadata)+Buffer.byteLength(JSON.stringify(session));
 const oversize=()=>({status:'oversize',totalBytes,mediaBytes,messagesBytes,transcriptBytes,limitBytes});
 if(totalBytes>limitBytes)return oversize();
 const zip=new JSZip(),manifestEntries=[],mediaMap=[];
 const addEntry=(zipPath,content)=>{const buffer=typeof content==='string'?Buffer.from(content,'utf8'):content;zip.file(zipPath,buffer);manifestEntries.push({path:zipPath,bytes:buffer.length,sha256:digest(buffer)});};
 addEntry('session.json',JSON.stringify(session,null,2));addEntry('messages.jsonl',rows);addEntry('winone/conversation.json',metadata);addEntry('winone/draft.json',JSON.stringify(draft,null,2));
 const refs=portableId?[{sdkSessionId:portableId,path:'transcripts/pi/'+portableId}]:missingTranscripts.map(sdkSessionId=>({sdkSessionId,path:null}));
 if(portableId)addEntry(refs[0].path,transcript);
 let mediaMissing=0,mediaDropped=0,mediaExcluded=0,actualMediaBytes=0;
 for(let index=0;index<media.length;index++){
  options.signal?.throwIfAborted();
  const item=media[index],entry={url:item.url,scheme:item.scheme,kind:item.kind,filename:item.filename,zipPath:null,...(item.imageHost?{imageHost:item.imageHost}:{})};
  if(options.excludeMedia){mediaExcluded++;mediaMap.push({...entry,excluded:true});continue;}
  let buffer;
  if(item.inline)buffer=Buffer.from(item.inline,'base64');
  else if(item.file){try{const remaining=limitBytes-(totalBytes-mediaBytes)-actualMediaBytes,read=await readBounded(item.file,remaining,options.signal);if(read.oversize){mediaBytes=actualMediaBytes+read.oversize;totalBytes=messagesBytes+transcriptBytes+mediaBytes+Buffer.byteLength(metadata);return oversize();}buffer=read.buffer;}catch(error){if(!['ENOENT','ENOTDIR','EACCES','EPERM','EIO'].includes(error.code))throw error;mediaDropped++;}}
  if(buffer){entry.zipPath='media/'+(item.kind==='image'?'images/':'loose/')+(index+1)+'-'+item.filename;actualMediaBytes+=buffer.length;addEntry(entry.zipPath,buffer);}else{mediaMissing++;if(item.dropped)mediaDropped++;}
  mediaMap.push(entry);
 }
 addEntry('media-map.json',JSON.stringify({entries:mediaMap},null,2));
 const fidelity=!portableId?'db-only':missingTranscripts.length||mediaMissing||mediaExcluded?'partial':'full';
 const manifest={formatVersion:XDTSHARE_MIN_READER_VERSION,minReaderVersion:XDTSHARE_MIN_READER_VERSION,appVersion:packageInfo.version,platform:process.platform,exportedAt:new Date().toISOString(),agentKind:'pi',title:conversation.title,workspaceKind:conversation.projectId?'project':'dialogue',originalWorkingDir:conversation.directory,sdkSessionIds:portableId?[portableId]:[],activeSdkSessionId:portableId,exportFidelity:fidelity,counts:{messages:messages.length,media:mediaMap.filter(item=>item.zipPath).length},entries:manifestEntries,transcripts:refs};
 validateManifest(manifest);const manifestJSON=JSON.stringify(manifest,null,2);zip.file('manifest.json',manifestJSON);
 const unpackedBytes=manifestEntries.reduce((sum,entry)=>sum+entry.bytes,0)+Buffer.byteLength(manifestJSON);if(unpackedBytes>limitBytes){totalBytes=unpackedBytes;return oversize();}
 const zipBytes=await zip.generateAsync({type:'nodebuffer',compression:'DEFLATE',compressionOptions:{level:6}});
 const fileBytes=options.password?await sealPayload(zipBytes,options.password):buildPlainFile(zipBytes);
 options.signal?.throwIfAborted();
 // The native-selected destination is checked again before the atomic write.
 // Passwords and plaintext archive bytes never enter logs or desktop frames.
 const checked=await destination(options.targetPath);if(fileKey(checked.target)!==fileKey(target.target))throw Error('The export destination changed during collection');
 const temporary=target.target+'.'+randomBytes(8).toString('hex')+'.tmp';let owned=false;
 try{const file=await fs.open(temporary,'wx',0o600);owned=true;try{await file.writeFile(fileBytes);await file.sync();}finally{await file.close();}options.signal?.throwIfAborted();await fs.rename(temporary,target.target);owned=false;}
 finally{if(owned)await fs.rm(temporary,{force:true});}
 return {status:'ok',filePath:options.targetPath,fidelity,unpackedBytes,totalBytes:unpackedBytes,mediaBytes:actualMediaBytes,fileBytes:fileBytes.length,mediaMissing,mediaDropped,mediaExcluded,missingTranscripts,orcaWorkers:0,messageCount:messages.length};
}
