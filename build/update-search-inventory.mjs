// Keep the packaged search payload tied to its reviewed dependency notices.
import {readFile,writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
const root=resolve(import.meta.dirname,'..');
const catalog=JSON.parse(await readFile(resolve(root,'third_party/search-notices.json'),'utf8'));
if(catalog.schemaVersion!==1||!Array.isArray(catalog.packages))throw Error('Invalid search notice inventory');
const paths=new Set(['shared/chat/rg.exe','shared/chat/search-runtime.json','shared/chat/licenses/search-notices.json']);
for(const item of catalog.packages)for(const notice of item.notices){if(!/^licenses\/[A-Za-z0-9._-]+\.txt$/.test(notice.file))throw Error('Invalid search notice path');paths.add('shared/chat/'+notice.file);}
const content=[...paths].sort().join('\n')+'\n',file=resolve(root,'internal/distribution/chat_search_runtime_paths.txt');
if(process.argv.includes('--check')){if(await readFile(file,'utf8')!==content)throw Error('Search runtime inventory differs from reviewed notices');}
else await writeFile(file,content);
console.log('Search runtime inventory: '+paths.size+' files');
