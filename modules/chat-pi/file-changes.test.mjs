import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import {observeFileChanges} from './file-changes.mjs';

test('successful writes and command artifacts preserve real creation and before/after content',async()=>{
 const root=await fs.mkdtemp(path.join(process.env.WINONE_CHAT_TEST_DIRECTORY??os.tmpdir(),'changes-'));
 const file=path.join(root,'report.md');
 const first=await observeFileChanges(root,'write',{path:file},async()=>{await fs.writeFile(file,'初稿\n');return {content:[]};});
 assert.equal(first.details.files[0].status,'A');assert.match(first.details.files[0].patch,/\+初稿/);
 assert.equal(first.details.files[0].captured,true);assert.equal(first.details.files[0].beforeExists,false);
 assert.equal(first.details.files[0].beforeText,'');assert.equal(first.details.files[0].afterText,'初稿\n');
 const next=await observeFileChanges(root,'write',{path:file},async()=>{await fs.writeFile(file,'完成\n');return {content:[]};});
 assert.equal(next.details.files[0].status,'M');assert.match(next.details.files[0].patch,/-初稿/);assert.match(next.details.files[0].patch,/\+完成/);
 assert.equal(next.details.files[0].beforeText,'初稿\n');assert.equal(next.details.files[0].afterText,'完成\n');
 const command=await observeFileChanges(root,'powershell',{command:"Set-Content -Path './output.txt' -Value 'Done'"},async()=>{await fs.writeFile(path.join(root,'output.txt'),'Done');return {content:[]};});
 assert.equal(command.details.files[0]?.name,'output.txt');
 const unchanged=await observeFileChanges(root,'write',{path:file},async()=>({content:[]}));assert.equal(unchanged.details.files.length,0);
});
