// Explicit real-Git lifecycle regression. The original WorktreeManager, SQLite
// registry, checkout and recovery journal run unchanged; only the native Host's
// read-only ownership boundary is supplied here. The Go integration separately
// covers the same boundary through an actual Host and ordinary Pi Worker.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import {fileURLToPath} from 'node:url';
import {createGhostWorktrees} from './worktrees.mjs';
import {openGhostTaskLedger} from './task-ledger.mjs';
import {createOneBotPaths} from '../onebot/content-service.mjs';
import {createIMSecrets} from '../onebot/im-secrets.mjs';
import {withIMScope} from '../onebot/im-scope.mjs';
import {configureWorktreeRecoverySecrets} from '../onebot/worktree-host.mjs';
import {randomBytes,createHash} from 'node:crypto';

const execute=promisify(execFile),repository=fileURLToPath(new URL('../../../',import.meta.url));
const enabled=!!process.env.WINONE_GHOST_SANDBOX;
async function fixture(t){
 const output=process.env.WINONE_CHAT_TEST_DIRECTORY??path.join(repository,'_temp/ghost-task-tests');
 await fs.mkdir(output,{recursive:true});const root=await fs.mkdtemp(path.join(output,'git-worktree-')),repo=path.join(root,'repo');
 await fs.mkdir(repo);const prior=process.env.GIT_CONFIG_GLOBAL;process.env.GIT_CONFIG_GLOBAL=path.join(root,'gitconfig');
 const git=async(args,cwd=repo)=>(await execute('git',args,{cwd,windowsHide:true})).stdout.trim();
 await git(['init','--initial-branch=main']);await git(['config','core.autocrlf','false']);await fs.writeFile(path.join(repo,'tracked.txt'),'original\n');
 await git(['add','--','tracked.txt']);
 await git(['-c','user.name=WinOne fixture','-c','user.email=fixture@example.invalid','commit','-m','test: isolated worktree fixture\n\nAssisted-by: Cindy (https://cindy.app)']);
 const ledger=openGhostTaskLedger(path.join(root,'profile')),rows=new Map(),warnings=[];
 const host=async(_channel,input)=>{
  if(input.method==='get')return rows.get(input.taskId)??null;
  if(input.method==='references')return [...rows.values()];
  throw Error('Unexpected native worktree operation: '+input.method);
 };
 const worktrees=createGhostWorktrees({root:path.join(root,'profile'),ledger,host,log:{info(){},warn:message=>warnings.push(message)}});
 const create=async id=>{
  ledger.created(id);
  const source=await worktrees.resolve(repo,undefined,repo),directory=await worktrees.create(id,repo,source.sourceBranch);
  rows.set(id,{id,status:'active',workingDir:directory,worktreePath:directory,currentDatabase:true,source:'chat'});
  return directory;
 };
 t.after(async()=>{await worktrees.close();ledger.close();if(prior===undefined)delete process.env.GIT_CONFIG_GLOBAL;else process.env.GIT_CONFIG_GLOBAL=prior;});
 return {root,repo,ledger,rows,worktrees,warnings,git,create};
}

test('real Worker worktree archive preserves staged and new content and restores it after reopening',{skip:!enabled},async t=>{
 const f=await fixture(t),directory=await f.create('worker-recover');
 assert.equal(await fs.readFile(path.join(directory,'tracked.txt'),'utf8'),'original\n','full checkout completed before returning');
 await fs.writeFile(path.join(directory,'tracked.txt'),'staged\n');await f.git(['add','--','tracked.txt'],directory);
 await fs.writeFile(path.join(directory,'tracked.txt'),'unstaged\n');await fs.writeFile(path.join(directory,'new.txt'),'new user content\n');
 f.rows.get('worker-recover').status='archived';
 const removed=await f.worktrees.archive('worker-recover');
 assert.equal(removed.status,'removed',JSON.stringify({removed,warnings:f.warnings}));
 await assert.rejects(fs.stat(directory),{code:'ENOENT'});
 // Inspect the persisted recovery evidence through this same service, not by
 // guessing a snapshot name or replacing its production recovery algorithm.
 const restored=await f.worktrees.restore('worker-recover',directory);
 assert.equal(restored.ready,true,JSON.stringify({restored,warnings:f.warnings}));
 assert.equal(await fs.readFile(path.join(directory,'tracked.txt'),'utf8'),'unstaged\n');
 assert.equal(await fs.readFile(path.join(directory,'new.txt'),'utf8'),'new user content\n');
 assert.equal(await f.git(['show',':tracked.txt'],directory),'staged');
 const before=await f.worktrees.get('worker-recover');await f.worktrees.close();
 const reopened=createGhostWorktrees({root:path.join(f.root,'profile'),ledger:f.ledger,host:async(_channel,input)=>input.method==='references'?[...f.rows.values()]:f.rows.get(input.taskId),log:{warn:message=>f.warnings.push(message),info(){}}});
 t.after(()=>reopened.close());
 assert.equal((await reopened.get('worker-recover')).generation,before.generation,'metadata survives owner restart');
 assert.equal((await reopened.restore('worker-recover',directory)).ready,true);
 assert.equal(await fs.readFile(path.join(directory,'new.txt'),'utf8'),'new user content\n');
});

test('real pre-created compensation keeps dirty, explicitly kept and still-owned checkouts',{skip:!enabled},async t=>{
 const f=await fixture(t),directory=await f.create('worker-compensate');
 assert.equal((await f.worktrees.discard('worker-compensate')).status,'preserved','active Host ownership is not a cancellation');
 f.rows.get('worker-compensate').status='archived';
 await fs.writeFile(path.join(directory,'new.txt'),'do not discard\n');
 assert.equal((await f.worktrees.discard('worker-compensate')).status,'preserved');
 assert.equal(await fs.readFile(path.join(directory,'new.txt'),'utf8'),'do not discard\n');
 await fs.unlink(path.join(directory,'new.txt'));await fs.writeFile(path.join(directory,'.worktree-keep'),'');
 assert.equal((await f.worktrees.discard('worker-compensate')).status,'preserved');
 await fs.unlink(path.join(directory,'.worktree-keep'));
 f.rows.set('another-reader',{id:'another-reader',status:'active',workingDir:directory,currentDatabase:true,source:'chat'});
 assert.equal((await f.worktrees.discard('worker-compensate')).status,'preserved','another native task still references the path');
 f.rows.delete('another-reader');
 const removed=await f.worktrees.discard('worker-compensate');assert.equal(removed.status,'discarded',JSON.stringify({removed,warnings:f.warnings}));
 assert.equal(removed.branchDeleted,true);await assert.rejects(fs.stat(directory),{code:'ENOENT'});
 assert.equal(await f.git(['branch','--format=%(refname:short)']),'main');
});

test('original physical recovery archives keep private keys across owner restart and reject tampering',{skip:!enabled},async t=>{
 const f=await fixture(t),root=path.join(f.root,'profile'),paths=createOneBotPaths(root),master=randomBytes(32).toString('base64');
 const source=path.join(f.root,'archive-source'),restored=path.join(f.root,'archive-restored');await fs.mkdir(source);
 const bytes=Buffer.from('private recovery fixture\n');await fs.writeFile(path.join(source,'document.txt'),bytes);
 const api=await import('../cindy/worktree/main/worktree/recoveryArchive.ts');
 let secrets=createIMSecrets(paths,master,console),clear=configureWorktreeRecoverySecrets(root,secrets);
 const scope={paths,closed:false};
 // The original archive filename contract uses a hexadecimal resource ID,
 // just like the production worktree registry, never an arbitrary label.
 const resourceId=createHash('sha256').update(source).digest('hex');
 const archive=await withIMScope(scope,()=>api.createRecoveryArchive(source,resourceId));
 const encrypted=await fs.readFile(path.join(root,'worktree-recycle',archive.file));
 assert.equal(encrypted.includes(bytes),false,'archive does not store fixture plaintext');
 clear();secrets.close();secrets=createIMSecrets(paths,master,console);clear=configureWorktreeRecoverySecrets(root,secrets);
 t.after(()=>{clear();secrets.close();});
 await withIMScope(scope,()=>api.verifyRecoveryArchive(archive));
 await fs.mkdir(restored);
 await withIMScope(scope,()=>api.extractRecoveryArchive(archive,restored));
 assert.deepEqual(await fs.readFile(path.join(restored,'document.txt')),bytes);
 const tag=Buffer.from(archive.tag,'base64');tag[0]^=1;
 await assert.rejects(withIMScope(scope,()=>api.verifyRecoveryArchive({...archive,tag:tag.toString('base64')})));
});
