import fs from 'node:fs/promises';
import path from 'node:path';
import {extractCommandOutputPathCandidates} from './cindy/commandOutputPaths.ts';
import {generateUnifiedPatch} from './node_modules/@earendil-works/pi-coding-agent/dist/core/tools/edit-diff.js';

const textLimit=64*1024;
async function snapshot(file){
 try{
  const stat=await fs.stat(file);if(!stat.isFile())return null;
  let content;
  if(stat.size<=textLimit){const bytes=await fs.readFile(file);try{content=new TextDecoder('utf-8',{fatal:true}).decode(bytes);if(content.includes('\0'))content=undefined;}catch{content=undefined;}}
  return {size:stat.size,modified:stat.mtimeMs,content};
 }catch(error){if(error.code==='ENOENT')return null;throw error;}
}

// Cindy's complete command-output extractor supplies candidates only. Actual
// stat/content comparisons establish creation or modification after success.
export async function observeFileChanges(cwd,tool,args,execute){
 const candidates=(tool==='write'||tool==='edit')?[args.path]:tool==='powershell'?extractCommandOutputPathCandidates(args.command??''):[];
 const paths=[...new Set(candidates.filter(value=>typeof value==='string'&&value).map(value=>path.resolve(cwd,value)))];
 const before=await Promise.all(paths.map(snapshot));
 const result=await execute();if(result?.isError||!paths.length)return result;
 const files=[];let bytes=0;
 for(const [index,file] of paths.entries()){
  const after=await snapshot(file),prior=before[index];if(!after)continue;
  if(prior&&prior.size===after.size&&prior.modified===after.modified&&prior.content===after.content)continue;
  const change={path:file,name:path.basename(file),status:prior?'M':'A',modified:after.modified,size:after.size};
  if(after.content!==undefined&&(!prior||prior.content!==undefined)){
   const patch=generateUnifiedPatch(file,prior?.content??'',after.content);
   const captureBytes=Buffer.byteLength(patch)+Buffer.byteLength(prior?.content??'')+Buffer.byteLength(after.content);
   // Keep exact before/after text for a turn-level review and reversible action.
   // The cap applies to the whole receipt; unavailable captures remain explicit.
   if(bytes+captureBytes<256*1024){Object.assign(change,{patch,captured:true,beforeExists:!!prior,beforeText:prior?.content??'',afterText:after.content,resolvedPath:await fs.realpath(file)});bytes+=captureBytes;}
  }
  files.push(change);
 }
 return {...result,details:{...result?.details,files}};
}
