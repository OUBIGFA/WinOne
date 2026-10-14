// Copy the locked renderer's font/CMap/codec resources for offline PDF viewing.
// This is generated output; the lockfile and upstream licenses are authoritative.
import {cp,mkdir,readFile} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import path from 'node:path';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const frontend=path.join(root,'apps/desktop/frontend'),source=path.join(frontend,'node_modules/pdfjs-dist');
const installed=JSON.parse(await readFile(path.join(source,'package.json'),'utf8'));
const manifest=JSON.parse(await readFile(path.join(frontend,'package.json'),'utf8'));
if(installed.version!==manifest.dependencies['pdfjs-dist'])throw Error('PDF renderer must match the exact locked version');
const output=path.join(frontend,'public/pdfjs');await mkdir(output,{recursive:true});
for(const name of ['cmaps','standard_fonts','wasm','iccs'])await cp(path.join(source,name),path.join(output,name),{recursive:true});
await cp(path.join(source,'web/images'),path.join(output,'images'),{recursive:true});
console.log('Prepared offline PDF renderer resources: '+installed.version);
