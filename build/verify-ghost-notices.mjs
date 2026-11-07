// Source/license check for the small OpenClaw network-policy subset Cindy
// vendors. No browser engine, profiles or managed proxy are imported.
import fs from 'node:fs';
import path from 'node:path';
import {createHash} from 'node:crypto';
import {fileURLToPath} from 'node:url';
const root=fileURLToPath(new URL('../',import.meta.url));
const spec=JSON.parse(fs.readFileSync(path.join(root,'third_party/ghost-notices.json'),'utf8'));
const source=JSON.parse(fs.readFileSync(path.join(root,'modules/chat-pi/cindy/ghost/packages/browser-control-runtime/upstream/browser-runtime.lock.json'),'utf8'));
if(spec.repository!==source.source.repo||spec.revision!==source.source.commit||spec.license!==source.source.license)throw Error('Ghost network policy attribution differs from the pinned Cindy source');
const bundle=process.argv[2]?path.resolve(process.argv[2]):null;
for(const notice of spec.notices){
 for(const file of [path.join(root,notice.path),...(bundle?[path.join(bundle,'licenses',path.basename(notice.path))]:[])]){
  if(createHash('sha256').update(fs.readFileSync(file)).digest('hex')!==notice.sha256)throw Error('Ghost network policy notice differs: '+file);
 }
}
console.log('Ghost network-policy source and original licenses verified');
