// Compile the same local settings tokens/fonts for both authenticated AI pages.
// No second theme or online font host: this is a CSS-only build of the existing
// frontend dependencies, into an ignored, repository-owned source cache.
import {resolve,join,relative,isAbsolute} from 'node:path';
import {pathToFileURL} from 'node:url';
import {execFileSync} from 'node:child_process';
const root=resolve(import.meta.dirname,'..'),frontend=join(root,'apps/desktop/frontend');
const source=resolve(process.argv[2]||''),within=relative(root,source);
if(!within||within.startsWith('..')||isAbsolute(within))throw Error('AI design output must belong to this repository');
const output=join(source,'internal/gui/assets');
execFileSync('git',['-C',root,'check-ignore','--quiet','--no-index',join(output,'winone-ai-settings.css')],{windowsHide:true});
const {build}=await import(pathToFileURL(join(frontend,'node_modules/vite/dist/node/index.js')));
// Vite requires CSS splitting for a CSS entry. This single entry still emits
// one stylesheet; imported fonts remain local assets in the same AI bundle.
await build({root:frontend,configFile:false,publicDir:false,base:'./',logLevel:'warn',build:{outDir:output,emptyOutDir:false,copyPublicDir:false,cssCodeSplit:true,rollupOptions:{input:join(frontend,'src/styles/ai-embedded.css'),output:{assetFileNames:asset=>(asset.names||[asset.name]).some(name=>name?.endsWith('.css'))?'winone-ai-settings.css':'fonts/winone/[name]-[hash][extname]'}}}});
console.log('Prepared shared WinOne AI settings tokens, controls and local fonts');
