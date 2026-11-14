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
const revision=JSON.parse(fs.readFileSync(path.join(root,'third_party/sources.json'),'utf8')).magpie.commit;
const source=path.resolve(arg<0?path.join(root,'dist/resources/sources/magpie-'+revision.slice(0,7)):process.argv[arg+1]);
const hash=bytes=>createHash('sha256').update(bytes).digest('hex');
const read=file=>execFileSync('git',['-C',source,'show',revision+':'+file],{maxBuffer:8*1024*1024});
const settings=read('internal/settings/settings.go'),proxies=read('internal/settings/proxies.go'),license=read('LICENSE');
const notice='licenses/upstream-magpie-'+revision+'-0-LICENSE.txt';
if(hash(license)!==hash(fs.readFileSync(path.join(root,'third_party',notice))))throw Error('Magpie license mismatch');
const settingsText=settings.toString('utf8');
const checkStart=settingsText.indexOf('func CheckProxy(p string) error {'),checkEnd=settingsText.indexOf('\n// DefaultPort',checkStart);
if(checkStart<0||checkEnd<0)throw Error('Magpie proxy validation boundary changed');
const goTarget='internal/ai/magpie_proxy.go';
const proxiesText=proxies.toString('utf8');
const extract=(text,start)=>{const a=text.indexOf(start),b=text.indexOf('\n}',a);if(a<0||b<a)throw Error('Magpie proxy support boundary changed: '+start);return text.slice(a,b+2);};
const prefix=proxiesText.match(/^const ProxyRefPrefix = .*$/m)?.[0];if(!prefix)throw Error('Magpie proxy reference prefix changed');
const support=[extract(proxiesText,'type NamedProxy struct {'),prefix,extract(proxiesText,'func IsProxyRef('),extract(proxiesText,'func ProxyID('),extract(proxiesText,'func CheckProxyIn(')].join('\n\n');
// WinOne owns inline provider/account proxies; it has no upstream named-proxy
// store. The original CheckProxyIn with an empty list rejects an unresolved
// reference instead of accepting it as an address or using another profile.
const implementation=(support+'\n\n'+settingsText.slice(checkStart,checkEnd)).replaceAll('Load().Proxies','nil').replaceAll('CheckProxyIn','validateMagpieProxyIn').replaceAll('CheckProxy','validateMagpieProxy').replaceAll('IsProxyRef','isMagpieProxyRef').replaceAll('ProxyRefPrefix','magpieProxyRefPrefix').replaceAll('ProxyID','magpieProxyID').replaceAll('NamedProxy','magpieNamedProxy');
const goCode='// Generated from yetone/magpie '+revision+'; MIT.\n// Host uses inline proxies; named references are checked against an empty upstream list.\n// See third_party/magpie-proxy-source.json.\npackage ai\n\nimport ("fmt";"net/url";"slices";"strings")\n\n'+implementation+'\n';
const outputs={[goTarget]:execFileSync(process.env.WINONE_GOFMT||'gofmt',[],{input:goCode}).toString('utf8')};
const manifest={schemaVersion:1,repository:'https://github.com/yetone/magpie',revision,license:'MIT',notice,sourceFiles:{'internal/settings/settings.go':hash(settings),'internal/settings/proxies.go':hash(proxies),LICENSE:hash(license)},files:Object.fromEntries(Object.entries(outputs).map(([file,bytes])=>[file,hash(bytes)])),adaptations:['Rename the upstream proxy validator and its pure support for the Host, preserving validation and gofmt formatting','Check named references against an empty upstream list because WinOne has only inline provider/account proxies']};
for(const [file,bytes] of Object.entries({...outputs,'third_party/magpie-proxy-source.json':JSON.stringify(manifest,null,2)+'\n'})){
 const destination=path.join(root,file);
 if(process.argv.includes('--write'))fs.writeFileSync(destination,bytes);
 else if(fs.readFileSync(destination,'utf8').replaceAll('\r\n','\n')!==bytes)throw Error('Magpie proxy source differs: '+file);
}
console.log('Magpie proxy validator and notices verified: '+revision);
