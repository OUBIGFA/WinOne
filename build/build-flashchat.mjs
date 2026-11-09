// Build the complete pinned web app for the native Flash Chat container.
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {spawnSync} from 'node:child_process';

const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const source=path.join(root,'modules/flashchat');
const output=path.join(root,'apps/desktop-mygo/pages/flashchat/assets');
const work=fs.mkdtempSync(path.join(root,'_temp/flashchat-build-'));
const staged=path.join(work,'assets');
const env={...process.env,VITE_HOMEPAGE:'https://chitchatter.im/',VITE_ROUTER_TYPE:'browser',VITE_GITHUB_REPO:'https://github.com/jeremyckahn/chitchatter',VITE_STREAMSAVER_URL:'/streamsaver/mitm.html'};
function run(args){const result=spawnSync(process.execPath,args,{cwd:source,env,stdio:'inherit'});if(result.status!==0)throw Error('Flash Chat build failed: '+args[0]);}
if(process.argv.includes('--install'))run([path.join(path.dirname(process.execPath),'node_modules/npm/bin/npm-cli.js'),'ci','--no-audit','--no-fund']);
run([path.join(root,'build/verify-flashchat-notices.mjs')]);
run(['node_modules/typescript/bin/tsc','--noEmit']);
if(process.argv.includes('--test'))run(['node_modules/vitest/vitest.mjs','run','--maxWorkers=3','--reporter=dot']);
run(['node_modules/vite/bin/vite.js','build','--outDir',staged]);
run(['node_modules/parcel/lib/bin.js','build','sdk/sdk.ts','--no-content-hash','--dist-dir',path.join(staged,'sdk'),'--cache-dir',path.join(root,'dist/resources/cache/flashchat-parcel')]);
fs.mkdirSync(path.join(staged,'streamsaver'),{recursive:true});
for(const name of ['mitm.html','sw.js'])fs.copyFileSync(path.join(source,'node_modules/streamsaver',name),path.join(staged,'streamsaver',name));
// Replace only outputs this builder owns. Keep old artifacts in the ignored
// task directory for rollback; never delete or overwrite unrecognized files.
const record=path.join(output,'.generated.json');
const old=fs.existsSync(record)?JSON.parse(fs.readFileSync(record,'utf8')):[];
const existing=fs.readdirSync(output).filter(name=>!['README.txt','.generated.json'].includes(name));
if(existing.some(name=>!old.includes(name)))throw Error('Unrecognized file in generated Flash Chat assets');
fs.mkdirSync(path.join(work,'previous'));
for(const name of existing){if(path.basename(name)!==name)throw Error('Invalid asset path');fs.renameSync(path.join(output,name),path.join(work,'previous',name));}
const current=fs.readdirSync(staged);
for(const name of current)fs.renameSync(path.join(staged,name),path.join(output,name));
fs.writeFileSync(record,JSON.stringify(current)+'\n');
console.log('Complete Flash Chat application built.');
