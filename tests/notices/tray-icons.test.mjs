import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {createHash} from 'node:crypto';
const root=new URL('../../',import.meta.url);
const read=name=>readFileSync(new URL(name,root));
const sha=bytes=>createHash('sha256').update(bytes).digest('hex');
test('tray icons retain their locked official package, original licenses and digests',()=>{
 const sources=JSON.parse(read('third_party/sources.json'));
 const icons=sources.ui.trayMenuIcons;
 const pkg=JSON.parse(read('apps/desktop/frontend/package-lock.json')).packages['node_modules/lucide-react'];
 assert.equal(icons.version,pkg.version);
 assert.deepEqual(icons.icons,['AppWindow','LogOut']);
 const license=read(icons.licenseFile);
 assert.equal(sha(license),icons.licenseSha256);
 assert.match(license.toString(),/Copyright \(c\) 2026 Lucide Icons and Contributors/);
 assert.match(license.toString(),/Copyright \(c\) 2013-present Cole Bemis/);
 for(const [path,digest] of Object.entries(icons.files)) assert.equal(sha(read(path)),digest,path);
});
