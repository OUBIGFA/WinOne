// Reuse Magpie's complete privacy surfaces in WinOne's Privacy tab: Settings ›
// Privacy's "Try it" (app.js) and Insights › Masking, which current Magpie
// ships as the redact plugin's own page (plugins/redact: i18n.js, redact.css,
// client.js, page.js). Reuse the plugin on Magpie's kernel with explicit
// host/record-identity boundaries below; only the app.js declarations it
// needs are extracted. Never import the shell.
import {createRequire} from 'node:module';
import {readFile,writeFile} from 'node:fs/promises';
import {resolve,join} from 'node:path';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
const root=resolve(import.meta.dirname,'..'),source=resolve(process.argv[2]),assets=join(source,'internal/gui/assets');
const require=createRequire(join(root,'apps/desktop/frontend/package.json')),{parse}=require('@babel/parser'),traverse=require('@babel/traverse').default;
const read=name=>readFile(join(assets,name),'utf8');
const text=await read('app.js');
const pluginFiles=['plugins/redact/i18n.js','plugins/redact/client.js','plugins/redact/page.js','plugins/redact/redact.css'];
const plugin=Object.fromEntries(await Promise.all(pluginFiles.map(async name=>[name,await read(name)])));
let program;traverse(parse(text,{sourceType:'script'}),{Program(p){program=p;}});
// The host owns these: WinOne's document state, height, and its own Privacy
// form (masking is turned on and its rules edited there, saved by WinOne).
const overrides=new Set(['params','state','view','mode','setShown','fit','openSettingsAt','openMaskRecord','loadSettings','savePrefs','prefsKeep','show','load']);
const seed=new Set(['renderMaskTry','translateStatic','setLocale','status','el','t','api']);
// Every free name the plugin page uses must come from app.js (or the kernel).
for(const name of ['plugins/redact/client.js','plugins/redact/page.js'])traverse(parse(plugin[name],{sourceType:'script'}),{Identifier(p){if((p.isReferencedIdentifier()||p.isBindingIdentifier())&&!p.scope.getBinding(p.node.name))seed.add(p.node.name);}});
const selected=new Set(),seen=new Set(),queue=[...seed];
function refs(p){p.traverse({Identifier(id){if(!id.isReferencedIdentifier()&&!id.isBindingIdentifier())return;const b=id.scope.getBinding(id.node.name);if(!b||b.scope===program.scope)queue.push(id.node.name);}});}
while(queue.length){const name=queue.shift();if(seen.has(name)||overrides.has(name))continue;seen.add(name);const b=program.scope.getBinding(name);if(!b)continue;let top=b.path;while(!top.parentPath.isProgram())top=top.parentPath;if(selected.has(top))continue;selected.add(top);refs(top);}
for(const name of ['renderSettings','renderProviders','renderAgents','openSettings','layoutNav'])if(seen.has(name))throw Error('Privacy records depend on application shell: '+name);
for(const name of ['maskPageOpen','maskOneOpen','maskLoad','drawMaskCard','drawMaskList'])if(!plugin['plugins/redact/page.js'].includes('function '+name+'('))throw Error('Magpie masking page contract changed: '+name);
if(!plugin['plugins/redact/client.js'].includes('key: "masking"'))throw Error('Magpie masking page registration changed');
let shared=[...selected].sort((a,b)=>a.node.start-b.node.start).map(p=>text.slice(p.node.start,p.node.end)).join('\n\n');
// The standalone document has no app modal, and a retryable error must stay
// visible (ms=0). Retain the upstream status widget without those shell assumptions.
for(const [from,to] of [['!!msg && !m.hidden','!!msg && !!m && !m.hidden'],['if (msg) status.t =','if (msg && ms > 0) status.t =']]){
 if(shared.split(from).length!==2)throw Error('Review privacy status boundary: '+from);
 shared=shared.replace(from,to);
}
// An unavailable summary is not a perpetual skeleton or an empty success.
let page=plugin['plugins/redact/page.js'];
// WinOne's public record DTO uses decimal strings: UnixNano IDs exceed JS's
// exact integer range. Preserve that identity through both deep-link entries.
const client=plugin['plugins/redact/client.js'].replace('maskOneOpen(+key)', 'maskOneOpen(String(key))');
if(client===plugin['plugins/redact/client.js']||!page.includes('const k = +params.get("key");'))throw Error('Review masking record identity boundary');
page=page.replace('const k = +params.get("key");','const k = params.get("key");');
// Settings belong to WinOne's draft transaction, not a second Magpie file.
// Keep the action, but let the parent stage the requested per-kind changes.
const blockedAction=`      b.disabled = true;
      (prefs ? Promise.resolve() : loadSettings())
        .then(() => writingPrefs(api("settings/redact-actions", { actions: { ...(prefs.redactActions || {}), ...Object.fromEntries(kinds.map((k) => [k, "mask"])) } })))
        .then((ns) => { prefs = ns; if (state) state.settings = ns; b.textContent = t("Masked from now on"); status(t("Saved"), "ok", 1500); })
        .catch((e) => { b.disabled = false; status(t(e.message), "err"); });`;
if(page.split(blockedAction).length!==2)throw Error('Review masking blocked-request action');
page=page.replace(blockedAction,'      window.winOneMaskKinds(kinds);');
const swallowed='}).catch(() => {}).finally(() => { if (maskBusy === p) maskBusy = null; });';
if(page.split(swallowed).length!==2)throw Error('Review masking summary error handling');
page=page.replace(swallowed,'}).catch(error => { window.winOneMaskingError(error); }).finally(() => { if (maskBusy === p) maskBusy = null; });');
await writeFile(join(assets,'winone-masking-shared.js'),'// Generated from Magpie app.js: only privacy page dependencies. See winone-masking-source.json.\n'+shared+'\n');
await writeFile(join(assets,'winone-masking-client.js'),client);
await writeFile(join(assets,'winone-masking-page.js'),page);
const html=`<!doctype html><html class="winone-ai-page winone-masking" lang="en" data-settings-ui><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>WinOne · Privacy</title><script src="winone-plugin-bridge.js"></script><script src="boot.js"></script><script src="theme.js"></script><script src="compat.js"></script><link rel="stylesheet" href="app.css"><link rel="stylesheet" href="plugins/library/library.css"><link rel="stylesheet" href="plugins/redact/redact.css"><link rel="stylesheet" href="winone-ai-settings.css"><link rel="stylesheet" href="winone-masking.css"></head><body class="window win"><div id="setPage-privacy" class="winone-masking-privacy"><section><h2 data-t>Try it</h2><div id="maskTryList"></div></section></div><template data-slot="page"></template><div id="status" role="status" class="status"></div><script src="i18n.js"></script><script src="kernel.js"></script><script src="plugins/redact/i18n.js"></script><script src="winone-masking-client.js"></script><script src="winone-masking-prehost.js"></script><script src="winone-masking-shared.js"></script><script src="winone-masking-page.js"></script><script src="winone-masking-host.js"></script></body></html>`;
await writeFile(join(assets,'masking.html'),html);
const revision=execFileSync('git',['-C',source,'rev-parse','HEAD'],{encoding:'utf8'}).trim(),hash=value=>createHash('sha256').update(value).digest('hex');
await writeFile(join(assets,'winone-masking-source.json'),JSON.stringify({revision,upstreamSHA256:hash(text),plugin:Object.fromEntries(pluginFiles.map(name=>[name,hash(plugin[name])])),sharedSHA256:hash(shared),clientSHA256:hash(client),pageSHA256:hash(page),declarations:selected.size,adaptations:['WinOne Privacy placement and shared grayscale controls','visible summary failure/retry','turning masking on and its rules open WinOne Privacy','blocked-request masking changes use WinOne draft transaction','decimal-string record IDs without number coercion','no application shell']},null,2)+'\n');
console.log('Prepared Magpie '+revision.slice(0,8)+' privacy page (redact plugin) with '+selected.size+' shared declarations');
