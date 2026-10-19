// Cindy 779b5237 removes HarmonyOS's global :lang override so Latin text keeps
// Inter. Apply the same patch without dropping any @font-face or font weight.
import {readFile,writeFile} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import {createHash} from 'node:crypto';
const file=fileURLToPath(new URL('../apps/desktop/frontend/node_modules/harmonyos-sans-sc-webfont-splitted/dist/index.css',import.meta.url));
const original=(await readFile(file,'utf8')).replaceAll('\r\n','\n');
const hash=value=>createHash('sha256').update(value).digest('hex');
const patchedHash='28fb7aca0aec84a30db6787ce90ce16eb74eae0d99503a52e7c606ff0e33e1ef';
if(hash(original)!==patchedHash){
 if(hash(original)!=='37a3c95dabb03ed7c5ee850c35aade9e9dea27763f83ec2a4c173d8d3310f83e')throw Error('Cindy HarmonyOS font source changed; review the upstream patch');
 const patched=original.slice(0,original.lastIndexOf('/* Language */')).trimEnd()+'\n';
 if(hash(patched)!==patchedHash)throw Error('Cindy font patch did not reproduce its expected content');
 await writeFile(file,patched,'utf8');
}
