// Real byte store, SQLite ledger and original Cindy ingest/compensation logic.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {createGhostMediaStore} from './platform-media.mjs';
import {createGhostModels} from './platform-models.mjs';
import {openGhostTaskLedger} from './task-ledger.mjs';
import {setGhostDataRoot} from './seams/app-session.ts';
import {recordBlob,addRef} from '../cindy/ghost/apps/desktop/src/main/cindy-media/ledger.ts';
import {GHOST_CINDY_DEPOSIT_QUOTA_BYTES} from '../cindy/ghost/apps/desktop/src/shared/ghost.ts';

const root=fileURLToPath(new URL('../../../',import.meta.url));
const image=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/l1sAAAAASUVORK5CYII=','base64');
async function fixture(t){
 const base=process.env.WINONE_CHAT_TEST_DIRECTORY??path.join(root,'_temp/ghost-media-tests');
 await fs.mkdir(base,{recursive:true});const directory=await fs.mkdtemp(path.join(base,'media-'));
 setGhostDataRoot(directory);
 let ledger,store;
 const ghosts=new Map(['one','two'].map(id=>[id,{enabled:true,manifest:{id,cindy:{media:['deposit']}},approval:{state:'approved',revision:'original'}}]));
 const open=()=>{ledger=openGhostTaskLedger(directory);store=createGhostMediaStore({database:()=>ledger.db,find:id=>ghosts.get(id),current:ghost=>ghost.enabled,callInfo:()=>null});};
 open();
 t.after(async()=>{await store.close();ledger.close();});
 return {get store(){return store;},get ledger(){return ledger;},ghosts,async reopen(){await store.close();ledger.close();open();}};
}

test('deposit ownership and exact-byte quota survive reopening and stay separate from generated artwork',async t=>{
 const f=await fixture(t);
 const first=await f.store.deposit({ghostId:'one',buffer:image,mimeType:'image/png'});
 await f.store.deposit({ghostId:'one',buffer:image,mimeType:'image/png'});
 assert.equal(await f.store.usage('one'),image.length,'duplicate content is counted once');
 assert.equal(await f.store.read('two',first.hash),null,'knowing the hash is not ownership');
 await f.reopen();
 assert.equal(await f.store.usage('one'),image.length);
 assert.deepEqual((await f.store.read('one',first.hash)).buffer,image);
 assert.equal(await f.store.release({ghostId:'two',hash:first.hash}),false);
 assert.equal(await f.store.release({ghostId:'one',hash:first.hash}),true);
 assert.equal(await f.store.read('one',first.hash),null);
 await f.store.generated({ghostId:'one',buffer:image,mimeType:'image/png',ownerScopeKey:'local'});
 await f.store.deposit({ghostId:'one',buffer:image,mimeType:'image/png'});
 await f.store.release({ghostId:'one',hash:first.hash});
 assert.deepEqual((await f.store.read('one',first.hash)).buffer,image,'releasing a deposit must not revoke the gallery');
 assert.equal(await f.store.usage('one'),0);
});

test('a returned media URL grants only its current task a handoff, never another plugin or task',async t=>{
 const f=await fixture(t),written=await f.store.generated({ghostId:'one',buffer:image,mimeType:'image/png',ownerScopeKey:'local'});
 assert.equal(await f.store.canHandOff('two',written.hash,'other-task'),false);
 await f.store.deliver('two','wrong-task',JSON.stringify({url:written.url}));
 assert.equal(await f.store.canHandOff('two',written.hash,'wrong-task'),false);
 await f.store.deliver('one','task',JSON.stringify({url:written.url}));
 assert.equal(await f.store.canHandOff('two',written.hash,'task'),true);
 assert.equal(await f.store.canHandOff('two',written.hash,'other-task'),false);
 await f.store.grant('two',written.hash,'task');
 await f.reopen();
 assert.deepEqual((await f.store.read('two',written.hash)).buffer,image);
});

test('the original deposit slot checks its persisted quota before writing new bytes',async t=>{
 const f=await fixture(t),db=f.ledger.db.drizzle;
 // A desensitized pre-existing ledger entry models a full quota; the new
 // upload below is real PNG data. No giant disk allocation or mock quota reader.
 await recordBlob({hash:'a'.repeat(64),ext:'.png',mimeType:'image/png',bytes:GHOST_CINDY_DEPOSIT_QUOTA_BYTES},db);
 await addRef({hash:'a'.repeat(64),refKind:'ghost-deposit',refId:'one',originKind:'user'},db);
 const models=createGhostModels({find:id=>f.ghosts.get(id),host:async()=>({models:[],defaults:null}),log:{warn(){},info(){}},dispatcher:{},mediaStore:f.store});
 const result=await models.request('one',{kind:'deposit_media',data:image.toString('base64')});
 assert.equal(result.ok,false);
 assert.match(result.message,/配额|上限|quota/i);
 assert.equal(await f.store.usage('one'),GHOST_CINDY_DEPOSIT_QUOTA_BYTES);
});
