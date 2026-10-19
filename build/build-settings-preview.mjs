// Freeze the current UI for native visual checks while other work continues.
// This deliberately does not replace the full type-checked application build.
import {resolve,join} from 'node:path';
import {pathToFileURL} from 'node:url';
import {execFileSync} from 'node:child_process';
import {writeFile} from 'node:fs/promises';
const frontend=resolve('apps/desktop/frontend');
const output=resolve(process.env.WINONE_SETTINGS_ASSETS || '_temp/settings-native-preview/assets');
execFileSync('powershell.exe',['-NoProfile','-File',resolve('build/assert-temporary-path.ps1'),'-Path',output],{windowsHide:true});
execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')],{windowsHide:true});
const {build}=await import(pathToFileURL(join(frontend,'node_modules/vite/dist/node/index.js')));
const excludeChat=process.env.WINONE_SETTINGS_EXCLUDE_CHAT==='1';
// Explicit test scope only. A visit to Chat fails loudly; its unfinished source
// cannot prevent reviewing unrelated settings, and no production file changes.
const isolatedChat={name:'settings-review-scope',enforce:'pre',resolveId(source,importer){if(importer?.replaceAll('\\','/').endsWith('/src/main.tsx')&&source==='./features/chat')return 'virtual:settings-review-chat';},load(id){if(id==='virtual:settings-review-chat')return 'export function ChatPage(){throw new Error("Chat is excluded from this isolated settings review")}';}};
await build({root:frontend,configFile:join(frontend,'vite.config.ts'),plugins:excludeChat?[isolatedChat]:[],build:{outDir:output,emptyOutDir:false,rollupOptions:{input:{settings:join(frontend,'index.html')}}}});
await writeFile(join(output,'preview-info.json'),JSON.stringify({purpose:'isolated settings review, not an application release',excludeChat}));
console.log('Frozen UI assets for isolated native review: '+output);
