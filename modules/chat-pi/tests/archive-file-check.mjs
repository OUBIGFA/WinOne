// Read-only verifier for the private-desktop save workflow. The fixture password
// arrives only in this test process's environment, never a product command line.
import fs from 'node:fs/promises';
import {createHash} from 'node:crypto';
import JSZip from 'jszip';
import {openPayload} from '../cindy/xdtshareCrypto.ts';
import {validateManifest} from '../cindy/xdtshareFormat.pure.ts';
const [archivePath,transcriptPath,attachmentPath]=process.argv.slice(2);
const bytes=await fs.readFile(archivePath),opened=openPayload(bytes,process.env.WINONE_CHAT_ARCHIVE_TEST_PASSWORD),zip=await JSZip.loadAsync(opened.zipBytes);
const manifest=validateManifest(JSON.parse(await zip.file('manifest.json').async('string')));
for(const entry of manifest.entries){const content=await zip.file(entry.path).async('nodebuffer');if(content.length!==entry.bytes||createHash('sha256').update(content).digest('hex')!==entry.sha256)throw Error('Archive entry digest mismatch: '+entry.path);}
const transcript=await zip.file(manifest.transcripts[0].path).async('nodebuffer');if(!transcript.equals(await fs.readFile(transcriptPath)))throw Error('The native Pi transcript was changed or truncated');
const messages=await zip.file('messages.jsonl').async('string');if(!messages.includes('NATIVE_BEFORE_MOVE')||!messages.includes('NATIVE_AFTER_MOVE'))throw Error('Native UI export lost conversation history');
const draft=await zip.file('winone/draft.json').async('string');if(!draft.includes('UNSENT_NATIVE_DRAFT'))throw Error('Native UI export lost its draft');
const media=JSON.parse(await zip.file('media-map.json').async('string')).entries;
const originalAttachment=await fs.readFile(attachmentPath);
let attachment=false;for(const entry of media)if(entry.zipPath&&(await zip.file(entry.zipPath).async('nodebuffer')).equals(originalAttachment))attachment=true;
if(!attachment)throw Error('Native UI export lost its attachment');
console.log(JSON.stringify({encrypted:opened.encrypted,fidelity:manifest.exportFidelity,messages:manifest.counts.messages,media:manifest.counts.media,entryDigests:true,nativeTranscriptIdentical:true,draft:true,attachmentByteIdentical:true}));
