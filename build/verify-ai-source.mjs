// Source-level license gate, run immediately on a Magpie upgrade and again
// before preparation. No model download, app discovery or code execution.
import {readFileSync} from 'node:fs';
import {resolve,join} from 'node:path';
import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';
import assert from 'node:assert/strict';
const root=resolve(import.meta.dirname,'..'),source=resolve(process.argv[2]??'');
if(!process.argv[2])throw Error('Usage: node build/verify-ai-source.mjs <magpie-source>');
const read=name=>readFileSync(join(root,name));
const sha=bytes=>createHash('sha256').update(bytes).digest('hex');
const sources=JSON.parse(read('third_party/sources.json'));
const revision=execFileSync('git',['-C',source,'rev-parse','HEAD'],{encoding:'utf8',windowsHide:true}).trim();
assert.equal(revision,sources.magpie.commit,'Magpie source differs from the reviewed revision');
for(const name of ['go.mod','go.sum'])assert.equal(readFileSync(join(source,name),'utf8').replaceAll('\r\n','\n'),read('modules/ai-magpie/upstream.'+name).toString().replaceAll('\r\n','\n'),'Review the changed dependency lock: '+name);
const notices=JSON.parse(read('third_party/ocr-ai-notices.json'));
for(const entry of notices.components)for(const n of entry.notices)assert.equal(sha(read('third_party/'+n.file)),n.sha256,n.file);
const upstream=notices.components.find(c=>c.kind==='upstream'&&c.name==='magpie'&&c.version===revision);
assert.ok(upstream,'Missing current upstream notice');
assert.equal(sha(readFileSync(join(source,'LICENSE'))),upstream.notices[0].sha256);
const privacy=JSON.parse(read('third_party/privacy-model.json'));
assert.equal(privacy.upstream,revision);
for(const notice of [...privacy.model.notices,...privacy.runtime.dependencies.map(d=>d.notice)])assert.equal(sha(read('third_party/'+notice.file)),notice.sha256,notice.file);
const pii=readFileSync(join(source,'internal/pii/pii.go'),'utf8');
for(const pin of [privacy.model.revision,privacy.model.file,privacy.model.size,privacy.model.sha256,privacy.runtime.sha256,privacy.runtime.tag])assert.ok(pii.includes(String(pin)),'Privacy download pin changed: '+pin);
assert.equal(createHash('sha1').update(Buffer.concat([Buffer.from(`blob ${read('third_party/licenses/openai-privacy-filter-LICENSE.txt').length}\0`),read('third_party/licenses/openai-privacy-filter-LICENSE.txt')])).digest('hex'),privacy.model.licenseGitBlob,'Model license must retain the original Git blob');
console.log(`AI source/license pins verified: ${revision}; ${notices.components.length} dependency records; opt-in privacy model and native runtime`);
