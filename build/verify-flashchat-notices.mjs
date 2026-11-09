// Reuse the frontend license verifier for the complete, separately locked app.
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {verifyFrontendNotices,updateFrontendNotices} from './verify-frontend-notices.mjs';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const file=path.join(root,'third_party/flashchat-runtime-notices.json');
let catalog=fs.existsSync(file)?JSON.parse(fs.readFileSync(file,'utf8')):{schemaVersion:1,components:[]};
if(process.argv.includes('--update')){
 catalog=updateFrontendNotices(root,catalog,'modules/flashchat');
 fs.writeFileSync(file,JSON.stringify(catalog,null,2)+'\n');
}
console.log('Flash chat dependency licenses verified: '+verifyFrontendNotices(root,catalog,'modules/flashchat'));
