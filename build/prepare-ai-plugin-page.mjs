// Reuse Magpie's complete third-party operations and sandboxed pages, with
// WinOne's install-first layout and flat settings lists. The host owns accounts.
import {createRequire} from 'node:module';
import {readFile,writeFile} from 'node:fs/promises';
import {join,resolve} from 'node:path';
import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';
const root=resolve(import.meta.dirname,'..'),sourceRoot=resolve(process.argv[2]),assets=join(sourceRoot,'internal/gui/assets');
const require=createRequire(join(root,'apps/desktop/frontend/package.json'));
const {parse}=require('@babel/parser'),traverse=require('@babel/traverse').default,html=require('parse5');
const read=name=>readFile(join(assets,name),'utf8');
const original=await read('plugins.js'),edits=[];
// The install form stays outside the changing discovery body. A refresh,
// locale change or npm result must not erase a path while the user types it.
const installForm=`function manual() {
    let box = manual.form;
    if (!box) {
      box = manual.form = el("form", "pm-manual");
      const label = el("label", "pm-mlabel");
      label.htmlFor = "winone-plugin-spec";
      const spec = el("input", "pm-minput");
      spec.id = "winone-plugin-spec"; spec.spellcheck = false; spec.autocomplete = "off";
      spec.onkeydown = (ev) => ev.stopPropagation();
      const go = el("button", "text primary"); go.type = "submit";
      box.onsubmit = (ev) => {
        ev.preventDefault(); const s = spec.value.trim();
        if (!s || busy.size) return;
        act("+", "add", { spec: s }, () => status(t("{name} is installed", { name: s }), "ok"));
      };
      box.append(label, spec, go);
    }
    const spec = box.querySelector("input"), go = box.querySelector('button[type="submit"]');
    box.querySelector("label").textContent = t("Install a plugin");
    spec.placeholder = t("npm package, GitHub repo (github:owner/repo) or a folder");
    spec.setAttribute("aria-label", t("Plugin"));
    spec.disabled = busy.has("+"); go.disabled = busy.size > 0;
    go.textContent = t(busy.has("+") ? "Installing…" : "Install");
    if (mine?.picker && !box.querySelector(".pm-browse")) box.insertBefore(browse(spec, go), go);
    const picker = box.querySelector(".pm-browse");
    if (picker) { picker.disabled = busy.size > 0; picker.title = t("Choose a folder on this computer"); picker.setAttribute("aria-label", t("Choose a folder")); }
    return box;
  }`;

traverse(parse(original,{sourceType:'script'}),{
 FunctionDeclaration(p){
  if(p.node.id?.name==='manual')edits.push([p.node.start,p.node.end,installForm]);
  if(['loadBuiltins','drawBuiltin'].includes(p.node.id?.name))edits.push([p.node.start,p.node.end,'']);
 },
 CallExpression(p){if(p.node.callee.name==='loadBuiltins')edits.push([p.node.start,p.node.end,'Promise.resolve()']);},
 ArrayExpression(p){if(p.node.elements[0]?.value==='builtin'){
  const list=p.parentPath;
  if(!list.isArrayExpression())throw Error('Magpie plugin tabs changed');
  const next=list.node.elements[1];
  edits.push([p.node.start,next.start,'']);
 }},
 IfStatement(p){if(original.slice(p.node.test.start,p.node.test.end)==='tab === "builtin"'&&p.node.consequent.type==='ReturnStatement'&&p.node.consequent.argument?.callee?.name==='drawBuiltin')edits.push([p.node.start,p.node.end,'']);}
});
let manager=original;
for(const [from,to,value]of edits.sort((a,b)=>b[0]-a[0]))manager=manager.slice(0,from)+value+manager.slice(to);
// WinOne keeps the full discovery/help content but the manual install task is
// always reachable, including when npm/GitHub is loading or unavailable.
for(const [from,to] of [
 ['replaceKeeping(page, [head(), body]);','replaceKeeping(page, [head(), manual(), body]);'],
 ['out.push(manual());',''],
 ['box.append(text, art);','box.append(text);'],
 ['t("Plugins extend magpie")','t("About plugins")'],
 // The built-in tab is owned elsewhere in WinOne; locate the Installed tab
 // by segs' semantic kind list, not the upstream three-tab numeric position.
 ['tabs.querySelectorAll(".opt")[2].dataset.sub = "installed";','tabs.querySelectorAll(".opt")[tabs.dataset.kind.split("|").indexOf("installed")].dataset.sub = "installed";'],
 ]){if(manager.split(from).length!==2)throw Error('Plugin presentation boundary changed: '+from);manager=manager.replace(from,to);}
// A saved selection of the removed built-in tab must reopen Discover.
manager=manager.replace('let query = "";', 'if (tab === "builtin") tab = "discover";\n  let query = "";');
// Bundled capabilities already have their own WinOne settings. Preserve the
// installed registry and all other discovery/search results from upstream.
const filters=[
 ['listings = r.listings || [];','listings = (r.listings || []).filter(x => !window.bootPrefs?.bundledPackages?.includes(x.package));'],
 ['tagged = (r.repos || []).map(ghListing);','tagged = (r.repos || []).map(ghListing).filter(x => !window.bootPrefs?.bundledPackages?.includes(x.package) && !window.bootPrefs?.bundledPackages?.includes(x.github?.package));'],
 ['list: r.hits','list: r.hits.filter(x => !window.bootPrefs?.bundledPackages?.includes(x.package))'],
];
for(const [from,to]of filters){if(!manager.includes(from))throw Error('Magpie discovery boundary changed: '+from);manager=manager.replace(from,to);}
let source=await read('app.js');
const adapt=[];
traverse(parse(source,{sourceType:'script'}),{FunctionDeclaration(p){
 if(p.node.id?.name==='closeModal')p.traverse({IfStatement(s){if(source.slice(s.node.test.start,s.node.test.end).startsWith('adding &&'))adapt.push([s.node.start,s.node.end,'']);}});
}});
for(const [from,to,value]of adapt.sort((a,b)=>b[0]-a[0]))source=source.slice(0,from)+value+source.slice(to);
let program;
traverse(parse(source,{sourceType:'script'}),{Program(p){program=p;}});
function references(path){const result=new Set();path.traverse({Identifier(p){if(!p.isReferencedIdentifier()&&!p.isBindingIdentifier())return;const b=p.scope.getBinding(p.node.name);if(!b||b.scope===program.scope)result.add(p.node.name);}});return result;}
const originalPages=await read('pluginpages.js'),kernel=await read('kernel.js');
if(!originalPages.includes('f.src = p.entry.url;'))throw Error('Magpie plugin page URL boundary changed');
const pageSource=originalPages
 .replace('f.src = p.entry.url;','f.src = window.winOnePluginURL(p.entry.url);')
 .replace('f.src = e.url;','f.src = window.winOnePluginURL(e.url);')
 .replace('img.src = e.icon;','img.src = window.winOnePluginURL(e.icon);')
 // WinOne owns the surrounding navigation. A replaces-only extension still
 // needs an accessible tab when its upstream application page is not present.
 .replace('at.after(b);','if (at) at.after(b); else nav.append(b);')
 .replace('navSubs();\n    // a reload','window.winOnePluginLoaded?.(list);\n    // a reload')
 .replace('async function act(key, id, context) {','window.winOnePluginAction = act;\n  async function act(key, id, context) {');
const queue=['scrollOnPurpose','closeModal'],reasons=new Map(),selected=new Set(),names=new Set();
for(const text of [manager,pageSource,kernel])traverse(parse(text,{sourceType:'script'}),{Identifier(p){if((p.isReferencedIdentifier()||p.isBindingIdentifier())&&!p.scope.getBinding(p.node.name))queue.push(p.node.name);}});
const overrides=new Set(['mode','view','state','params','loadProviders','openProvider','pluginSignIn','renderProviders','show','fit','load','renderPluginDot','renderBuiltins','navSubs','setSetTab','setTab']);
while(queue.length){
 const name=queue.shift();if(names.has(name)||overrides.has(name))continue;names.add(name);
 const binding=program.scope.getBinding(name);if(!binding)continue;
 let top=binding.path;while(!top.parentPath.isProgram())top=top.parentPath;
 if(selected.has(top))continue;selected.add(top);
 for(const dependency of references(top)){if(!reasons.has(dependency))reasons.set(dependency,name);queue.push(dependency);}
}
for(const name of ['renderAgents','renderSettings','loadSettings','loadInstalls','layoutNav','navHides','setPick'])if(names.has(name)){
 const chain=[name];while(reasons.has(chain.at(-1))&&!chain.includes(reasons.get(chain.at(-1))))chain.push(reasons.get(chain.at(-1)));
 throw Error('Plugin manager depends on application page '+chain.reverse().join(' → '));
}
const shared=[...selected].sort((a,b)=>a.node.start-b.node.start).map(p=>source.slice(p.node.start,p.node.end)).join('\n\n');
const document=html.parse(await read('index.html')),nodes=[];
function walk(n){nodes.push(n);for(const c of n.childNodes||[])walk(c);}walk(document);
const only=id=>{const node=nodes.find(n=>n.attrs?.some(a=>a.name==='id'&&a.value===id));if(!node)throw Error('Missing upstream widget '+id);return html.serializeOuter(node);};
const page='<!doctype html><html class="winone-plugins winone-ai-page" lang="en" data-settings-ui><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>WinOne · Plugins</title>'+
 '<script src="winone-plugin-bridge.js"></script><script src="boot.js"></script><script src="theme.js"></script><script src="compat.js"></script>'+
 '<link rel="stylesheet" href="app.css"><link rel="stylesheet" href="plugins.css"><link rel="stylesheet" href="plugins/library/library.css"><link rel="stylesheet" href="winone-ai-settings.css"><link rel="stylesheet" href="winone-plugins.css"></head>'+
 '<body class="window win"><nav id="nav" aria-label="Plugin pages"></nav><main id="plugin-content"><section id="view-plugins" class="view"></section><template data-slot="page"></template></main>'+
 '<span id="status" class="status" role="status" aria-live="polite"></span>'+only('modal')+only('pop')+
 '<script src="i18n.js"></script><script src="kernel.js"></script><script src="winone-plugin-shared.js"></script><script src="winone-plugin-host.js"></script><script src="winone-plugin-manager.js"></script><script src="winone-plugin-pages.js"></script></body></html>\n';
const revision=execFileSync('git',['-C',sourceRoot,'rev-parse','HEAD'],{encoding:'utf8',windowsHide:true}).trim(),hash=s=>createHash('sha256').update(s).digest('hex');
await writeFile(join(assets,'winone-plugins.html'),page,'utf8');
await writeFile(join(assets,'winone-plugin-manager.js'),manager,'utf8');
await writeFile(join(assets,'winone-plugin-shared.js'),shared,'utf8');
await writeFile(join(assets,'winone-plugin-pages.js'),pageSource,'utf8');
await writeFile(join(assets,'winone-plugin-source.json'),JSON.stringify({revision,upstreamSHA256:hash(original),managerSHA256:hash(manager),sharedSHA256:hash(shared),pagesSHA256:hash(pageSource),adaptations:['built-in features owned by WinOne','provider navigation owned by WinOne','standalone document and shared widgets','WinOne install-first layout, persistent install draft, flat lists and shared settings design']},null,2)+'\n','utf8');
console.log('Prepared Magpie '+revision.slice(0,8)+' third-party manager with '+selected.size+' shared declarations');
