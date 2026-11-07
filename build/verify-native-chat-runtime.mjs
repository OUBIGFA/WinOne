// Verify the native Chat protocol helpers against the build's actual digests.
// This is a current-build gate, not a new restriction on historical rollback
// manifests. It reads only the explicitly supplied bundle; nothing is written.
import fs from 'node:fs';
import path from 'node:path';
import {createHash} from 'node:crypto';

const directory=path.resolve(process.argv[2]??'_temp/build/current/shared/chat');
const runtime=JSON.parse(fs.readFileSync(path.join(directory,'runtime.json'),'utf8'));
const fields={workerSHA256:'worker.js',schedulerSHA256:'scheduler.js',scriptRunnerSHA256:'script-runner.js',ghostHostSHA256:'ghost.js',ghostSnapshotSHA256:'ghost-snapshot.js',ghostNodeWorkerSHA256:'nodeRuntimeWorkerProcess.js',worktreeRecoveryWorkerSHA256:'worktree-recovery-worker.js',ghostLibraryWorkerSHA256:'libraryDbWorker.js',ghostLibrarySqliteSHA256:'library-sqlite.js',ghostSDKSHA256:'ghost-sdk.js',ghostMailSHA256:'ghost-mail-sdk.js',ghostMailUiSHA256:'ghost-mail-ui-sdk.js'};
for(const[key,file]of Object.entries(fields)){
 const digest=createHash('sha256').update(fs.readFileSync(path.join(directory,file))).digest('hex');
 if(runtime[key]!==digest)throw Error(`Native Chat runtime mismatch: ${file}`);
}
console.log(`Native Chat protocol helpers verified: ${Object.keys(fields).length} files`);
