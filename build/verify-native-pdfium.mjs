import fs from 'node:fs';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {fileURLToPath} from 'node:url';
const root=fileURLToPath(new URL('../',import.meta.url));
const arg=name=>{const i=process.argv.indexOf(name);return i<0?undefined:process.argv[i+1];};
const hash=file=>createHash('sha256').update(fs.readFileSync(file)).digest('hex');
const record=JSON.parse(fs.readFileSync(path.join(root,'third_party/native-pdfium.json'),'utf8'));
const desktop=JSON.parse(fs.readFileSync(path.join(root,'third_party/desktop-mygo-notices.json'),'utf8')).components.find(c=>c.kind==='native'&&c.version===record.version);
if(!desktop)throw Error('PDFium desktop notices are missing');
for(const notice of record.notices){if(hash(path.join(root,'third_party',notice.file))!==notice.sha256||!desktop.notices.some(n=>n.file===notice.file&&n.sha256===notice.sha256))throw Error('PDFium original notice differs: '+notice.file);}
const api=fs.readFileSync(path.join(root,'apps/desktop-mygo/chat/pdfium/api_windows.go'),'utf8');
if(api.match(/LibrarySHA256\s*=\s*"([a-f0-9]{64})"/)?.[1]!==record.library.sha256)throw Error('PDFium C ABI binding names another artifact');
const source=arg('--source');if(source){for(const file of record.headers){if(hash(path.join(source,'include',file.file))!==file.sha256)throw Error('PDFium header differs from the reviewed ABI: '+file.file);}}
const bundle=arg('--bundle'),library=arg('--library')??(bundle?path.join(bundle,record.library.name):undefined);
if(library&&hash(library)!==record.library.sha256)throw Error('PDFium library checksum mismatch');
if(bundle){
 if(hash(path.join(bundle,'licenses/native-pdfium.txt'))!==hash(path.join(root,'third_party/native-pdfium.json')))throw Error('Shipped PDFium source record differs');
 for(const notice of record.notices){if(hash(path.join(bundle,'licenses',path.basename(notice.file)))!==notice.sha256)throw Error('Shipped PDFium notice differs: '+notice.file);}
}
console.log(`PDFium verified: ${record.notices.length} original notices${library?', library matched':''}`);
