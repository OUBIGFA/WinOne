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
await build({root:frontend,configFile:join(frontend,'vite.config.ts'),build:{outDir:output,emptyOutDir:false,rollupOptions:{input:{settings:join(frontend,'index.html')}}}});
await writeFile(join(output,'preview-info.json'),JSON.stringify({purpose:'isolated settings review, not an application release'}));
console.log('Frozen UI assets for isolated native review: '+output);
