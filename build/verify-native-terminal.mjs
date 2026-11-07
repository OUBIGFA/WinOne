// Source and artifact gate for MyGo's original libghostty-vt runtime.
// Registration is explicit; this verifier never refreshes expected hashes.
import fs from 'node:fs';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {fileURLToPath} from 'node:url';
const root=fileURLToPath(new URL('../',import.meta.url));
const argument=name=>{const index=process.argv.indexOf(name);return index<0?undefined:process.argv[index+1];};
const sha=file=>createHash('sha256').update(fs.readFileSync(file)).digest('hex');
const spec=JSON.parse(fs.readFileSync(path.join(root,'third_party/native-terminal.json'),'utf8'));
const desktop=JSON.parse(fs.readFileSync(path.join(root,'third_party/desktop-mygo-notices.json'),'utf8'));
const registered=desktop.components.find(item=>item.kind==='native'&&item.version===spec.library.revision);
if(!registered)throw Error('Native terminal notices are missing from the desktop inventory');
const notices=spec.components.flatMap(component=>component.notices);
for(const notice of notices){
 if(sha(path.join(root,'third_party',notice.file))!==notice.sha256)throw Error('Native terminal original notice changed: '+notice.file);
 if(!registered.notices.some(item=>item.file===notice.file&&item.sha256===notice.sha256))throw Error('Native terminal desktop notice differs: '+notice.file);
}
const binding=fs.readFileSync(path.join(root,'apps/desktop-mygo/chat/workspace/terminal.go'),'utf8').match(/terminalLibrarySHA256\s*=\s*"([a-f0-9]{64})"/);
if(!binding||binding[1]!==spec.library.sha256)throw Error('Native terminal runtime binding differs from the reviewed artifact');
const source=argument('--mygo-source');
if(source){
 const file=path.join(source,'plugins/terminal/mygo-plugin.json');
 if(sha(file)!==spec.mygoManifestSHA256)throw Error('MyGo native terminal manifest differs from its reviewed source');
 const record=JSON.parse(fs.readFileSync(file,'utf8')).libraries.find(item=>item.name==='libghostty-vt')?.files?.[spec.library.platform];
 if(!record||record.sha256!==spec.library.sha256||record.url!==spec.library.url||record.name!==spec.library.name)throw Error('MyGo native terminal artifact differs from the recorded release');
}
const bundle=argument('--bundle');
const library=argument('--library')??(bundle?path.join(bundle,spec.library.name):undefined);
if(library&&sha(library)!==spec.library.sha256)throw Error('Native terminal library checksum mismatch');
if(bundle){
 if(sha(path.join(bundle,'licenses/native-terminal.txt'))!==sha(path.join(root,'third_party/native-terminal.json')))throw Error('Shipped native terminal source record differs');
 for(const notice of notices){if(sha(path.join(bundle,'licenses',path.basename(notice.file)))!==notice.sha256)throw Error('Shipped native terminal notice differs: '+notice.file);}
}
console.log(`Native terminal verified: ${notices.length} original notices${library?', Windows library SHA-256 matched':''}`);
