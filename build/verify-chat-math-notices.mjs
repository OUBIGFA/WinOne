// Source-intake and shipped-copy gates for the actual native MathJax bundle.
// Upstream font licenses live in OpenType metadata, not a package LICENSE file.
import fs from 'node:fs';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {fileURLToPath} from 'node:url';

const root=fileURLToPath(new URL('../',import.meta.url));
const readJSON=file=>JSON.parse(fs.readFileSync(file,'utf8'));
const hash=file=>createHash('sha256').update(fs.readFileSync(file)).digest('hex');
const catalogFile=path.join(root,'third_party/chat-math-notices.json');
export function verifyMathNotices({catalog=readJSON(catalogFile),bundle,metafile}={}) {
 const moduleRoot=path.join(root,'modules/chat-pi');
 const lock=readJSON(path.join(moduleRoot,'package-lock.json'));
 const expectedFiles=new Set(['shared/chat/math.js','shared/chat/licenses/chat-math-notices.json','shared/chat/licenses/'+catalog.fontEvidence.file]);
 const checked=(file,digest)=>{if(hash(file)!==digest)throw Error('Math notice/resource changed: '+file);};
 const names=new Set();
 for(const component of catalog.components){
  if(names.has(component.name))throw Error('Duplicate math dependency: '+component.name);
  names.add(component.name);
  if(!component.notices?.length||component.licenseReview?.status==='blocked')throw Error('Math dependency lacks original license evidence: '+component.name);
  const dir=path.join(moduleRoot,component.sourceDirectory);
  const installed=readJSON(path.join(dir,'package.json'));
  if(installed.name!==component.name||installed.version!==component.version||lock.packages[component.sourceDirectory]?.version!==component.version)throw Error('Math dependency version differs: '+component.name);
  for(const notice of component.notices){
   if(!notice.sourceFile&&!notice.sourceURL&&!notice.generatedBy&&!notice.authoredNotice)throw Error('Math notice has no provenance: '+notice.file);
   checked(path.join(root,'third_party',notice.file),notice.sha256);
   if(notice.sourceFile)checked(path.join(dir,notice.sourceFile),notice.sha256);
   if(bundle)checked(path.join(bundle,notice.file),notice.sha256);
   expectedFiles.add('shared/chat/'+notice.file);
  }
 }
 const evidencePath=path.join(root,'third_party',catalog.fontEvidence.file);
 // Git stores this JSON as LF. Hashing a Windows-translated copy would break
 // source verification on a clean checkout even though the parsed JSON agrees.
 if(fs.readFileSync(evidencePath,'utf8').includes('\r'))throw Error('Font evidence must use canonical LF bytes');
 checked(evidencePath,catalog.fontEvidence.sha256);
 const evidence=readJSON(evidencePath);
 const font=catalog.components.find(item=>item.name===evidence.package);
 if(!font||font.version!==evidence.version)throw Error('Font evidence version differs');
 const fontDir=path.join(moduleRoot,font.sourceDirectory);
 const evidencedFonts=new Set();
 for(const file of evidence.files){
  checked(path.join(fontDir,file.sourceFile),file.sha256);
  if(file.sourceFile.endsWith('.woff2')){
   if(!file.notices?.length)throw Error('Font copyright evidence is empty');
   evidencedFonts.add(path.basename(file.sourceFile));
  }
 }
 const actualFonts=fs.readdirSync(path.join(fontDir,'chtml/woff2')).filter(file=>file.endsWith('.woff2'));
 if(!actualFonts.length||actualFonts.length!==evidencedFonts.size||actualFonts.some(file=>!evidencedFonts.has(file)))throw Error('Font copyright coverage differs');
 if(bundle){
  checked(path.join(bundle,'licenses',catalog.fontEvidence.file),catalog.fontEvidence.sha256);
  checked(path.join(bundle,'licenses/chat-math-notices.json'),hash(catalogFile));
 }
 for(const resource of catalog.resources){
  const archive=path.join(root,resource.source);
  checked(archive,resource.sha256);
  const integrity='sha512-'+createHash('sha512').update(fs.readFileSync(archive)).digest('base64');
  if(integrity!==resource.integrity||integrity!==lock.packages[font.sourceDirectory].integrity)throw Error('Font archive differs from npm lock integrity');
  if(bundle)checked(path.join(bundle,resource.file),resource.sha256);
  expectedFiles.add('shared/chat/'+resource.file);
 }
 const inventory=fs.readFileSync(path.join(root,'internal/distribution/chat_math_runtime_paths.txt'),'utf8').trim().split(/\r?\n/);
 if(inventory.length!==expectedFiles.size||inventory.some(file=>!expectedFiles.has(file)))throw Error('Math runtime inventory differs from reviewed resources');
 // Metafile inputs include the transitive modules, not just our direct imports.
 if(metafile){
  const graph=readJSON(metafile);
  const used=new Set();
  for(const [file,input] of Object.entries(graph.inputs)){
   const match=file.replaceAll('\\','/').match(/node_modules\/((?:@[^/]+\/)?[^/]+)\//);
   if(match){
    if(!names.has(match[1]))throw Error('Unreviewed bundled math dependency: '+match[1]);
    used.add(match[1]);
   }
   if(input.imports.some(item=>item.external))throw Error('Math bundle has an external runtime import: '+file);
  }
  if(used.size!==names.size)throw Error('Math catalog differs from bundled dependencies');
 }
 return {dependencies:names.size,fonts:actualFonts.length,files:expectedFiles.size};
}
if(process.argv[1]&&path.resolve(process.argv[1])===fileURLToPath(import.meta.url)){
 const result=verifyMathNotices({bundle:process.argv[2]||undefined,metafile:process.argv[3]||undefined});
 console.log(`Math notices verified: ${result.dependencies} bundled dependencies, ${result.fonts} original font records, ${result.files} runtime files`);
}
