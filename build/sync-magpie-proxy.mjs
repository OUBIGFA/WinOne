// Copy the upstream proxy validator exactly; --write regenerates, otherwise
// verify the checked-in code and its source/license hashes. The old web form
// helper belonged to the removed proxy settings page and is no longer part
// of the frontend build.
import fs from 'node:fs';
import path from 'node:path';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
const root=path.resolve(import.meta.dirname,'..');
const arg=process.argv.indexOf('--source');
const source=path.resolve(arg<0?path.join(root,'dist/resources/sources/magpie-42ec270'):process.argv[arg+1]);
const revision=JSON.parse(fs.readFileSync(path.join(root,'third_party/sources.json'),'utf8')).magpie.commit;
const hash=bytes=>createHash('sha256').update(bytes).digest('hex');
const read=file=>execFileSync('git',['-C',source,'show',revision+':'+file],{maxBuffer:8*1024*1024});
const settings=read('internal/settings/settings.go'),license=read('LICENSE');
const notice='licenses/upstream-magpie-'+revision+'-0-LICENSE.txt';
if(hash(license)!==hash(fs.readFileSync(path.join(root,'third_party',notice))))throw Error('Magpie license mismatch');
const settingsText=settings.toString('utf8');
const checkStart=settingsText.indexOf('func CheckProxy(p string) error {'),checkEnd=settingsText.indexOf('\n// DefaultPort',checkStart);
if(checkStart<0||checkEnd<0)throw Error('Magpie proxy validation boundary changed');
const goTarget='internal/ai/magpie_proxy.go';
const goCode='// Generated from yetone/magpie '+revision+'; MIT.\n// Only package and function names change; see third_party/magpie-proxy-source.json.\npackage ai\n\nimport ("fmt";"net/url";"slices";"strings")\n\n'+settingsText.slice(checkStart,checkEnd).replace('func CheckProxy(', 'func validateMagpieProxy(')+'\n';
const outputs={[goTarget]:execFileSync(process.env.WINONE_GOFMT||'gofmt',[],{input:goCode}).toString('utf8')};
const manifest={schemaVersion:1,repository:'https://github.com/yetone/magpie',revision,license:'MIT',notice,sourceFiles:{'internal/settings/settings.go':hash(settings),LICENSE:hash(license)},files:Object.fromEntries(Object.entries(outputs).map(([file,bytes])=>[file,hash(bytes)])),adaptations:['Rename the upstream CheckProxy for the Host, preserving its validation and gofmt formatting']};
for(const [file,bytes] of Object.entries({...outputs,'third_party/magpie-proxy-source.json':JSON.stringify(manifest,null,2)+'\n'})){
 const destination=path.join(root,file);
 if(process.argv.includes('--write'))fs.writeFileSync(destination,bytes);
 else if(fs.readFileSync(destination,'utf8').replaceAll('\r\n','\n')!==bytes)throw Error('Magpie proxy source differs: '+file);
}
console.log('Magpie proxy validator and notices verified: '+revision);
