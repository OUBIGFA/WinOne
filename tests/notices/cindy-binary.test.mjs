import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {syncCindy} from '../../build/sync-cindy.mjs';
test('Cindy synchronization preserves original WebP portrait bytes',()=>{
 const cindy='dist/resources/sources/cindy';
 if(!fs.existsSync(cindy+'/.git'))return;
 const result=syncCindy({cindy,mode:'check',spec:{schemaVersion:1,revision:'b7e46bcbbe41eeee2f16381fef00df61a31a3b8b',groups:[{source:'apps/desktop/resources/bot-packages/engineer-bot/0.5.1/assets/avatar.webp',target:'modules/chat-pi/cindy/bot-packages/engineer-bot/0.5.1/assets/avatar.webp',file:true}]}});
 assert.equal(result.files[0].sha256,result.files[0].sourceSHA256,'An asset must remain byte-identical to the pinned original');
});
