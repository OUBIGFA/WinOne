// Reproduce the reviewed Magpie ledger/line renderer without its app shell.
// --write updates generated source and provenance; the default only verifies.
import fs from 'node:fs';
import path from 'node:path';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import ts from '../apps/desktop/frontend/node_modules/typescript/lib/typescript.js';
const root=path.resolve(import.meta.dirname,'..');
const argument=process.argv.indexOf('--source');
const source=path.resolve(argument<0?path.join(root,'dist/resources/sources/magpie-42ec270'):process.argv[argument+1]);
const revision=JSON.parse(fs.readFileSync(path.join(root,'third_party/sources.json'),'utf8')).magpie.commit;
const sha=value=>createHash('sha256').update(value).digest('hex');
const files={};
function read(file){const bytes=execFileSync('git',['-C',source,'show',revision+':'+file],{maxBuffer:8*1024*1024});files[file]=sha(bytes);return bytes.toString('utf8');}
function between(text,start,end){const a=text.indexOf(start),b=text.indexOf(end,a+start.length);if(a<0||b<0)throw Error('Magpie extraction boundary changed: '+start);return text.slice(a,b).trim();}
function replace(text,from,to){if(text.split(from).length!==2)throw Error('Magpie adaptation boundary changed: '+from);return text.replace(from,to);}
const license='licenses/upstream-magpie-'+revision+'-0-LICENSE.txt';
if(sha(read('LICENSE'))!==sha(fs.readFileSync(path.join(root,'third_party',license))))throw Error('Magpie license mismatch');
const app=read('internal/gui/assets/app.js'),page=read('internal/gui/assets/plugins/usage/page.js'),css=read('internal/gui/assets/app.css');
let common=between(app,'const LED_METRICS =','\nlet quotaFocus =');
common=replace(common,'const LED_SHOWN = 7;','const LED_SHOWN = Math.max(7, options.shown || 7);');
common=replace(common,'${i + 1}','${i % 7 + 1}');
// Keep the existing compact overview, while retaining all upstream details in
// the tooltip/accessibility description. Trend rankings show every detail.
common=replace(common,'more.forEach((m, i) => l2.append(...(i ? [" · ", m] : [m])));',`b.title = more.map(m => typeof m === "string" ? m : m.textContent).join(" · ");
    b.setAttribute("aria-description", b.title);
    const brief = [];
    if (metric !== "calls") brief.push(t(x.calls === 1 ? "{n} request" : "{n} requests", { n: ledNum(x.calls) }));
    if (metric !== "cost" && x.cost) brief.push("≈" + fmtCost(x));
    if (metric !== "speed") brief.push(Math.round((100 * v) / sum) + "%");
    (compact ? brief : more).forEach((m, i) => l2.append(...(i ? [" · ", m] : [m])));`);
const credits=between(app,'function ledCredits(x) {','\n}',)+'\n}';
const rail=between(page,'function ledRail(rail, box) {','\n// the request list');
let lines=between(page,'// The chart as lines','\nfunction drawLedTrend()');
lines=replace(lines,'function ledLines(g, Y, x0, slot, segs) {','function ledLines(l, split, metric, g, Y, x0, slot, segs) {');
lines=replace(lines,'const l = ledger, pts = l?.series || [], metric = ledMetric, split = ledSplit;','const pts = l?.series || [];');
const original=[common,credits,rail,lines].join('\n\n');
const scoped=original.replaceAll('var(--','var(--usage-');
const header=`// @ts-nocheck
// Generated from yetone/magpie ${revision}. MIT.
// Run node build/sync-magpie-usage.mjs --write; provenance: third_party/magpie-usage-source.json.
export function magpieLedgerCharts(translate, language, options = {}) {
 const t=(key,values={})=>translate(key,values);
 const intlLang=()=>language;
 const numbers=new Intl.NumberFormat(language), compactNumbers=new Intl.NumberFormat(language,{notation:'compact',maximumFractionDigits:1});
 const ledNum=value=>numbers.format(value||0);
 const fmtN=value=>compactNumbers.format(value||0);
 const ledMoney=value=>new Intl.NumberFormat(language,{style:'currency',currency:'USD',maximumFractionDigits:2}).format(value||0);
 const fmtCost=value=>new Intl.NumberFormat(language,{style:'currency',currency:'USD',maximumFractionDigits:6}).format(value.cost||0);
 const ledTook=value=>(value/1000).toFixed(2)+' s';
 const tipLines=(...values)=>values.filter(Boolean).join('\\n');
 const el=(tag,cls='',text)=>{const e=document.createElement(tag);e.className=cls;if(text!=null)e.textContent=text;return e;};
 const icon=()=>el('span');
`;
const result=header+scoped+`\nreturn {
 drawLedColumns:(...args)=>{args[0].drawn=null;return drawLedColumns(...args);},
 drawLedLines:(box,l,split,metric,compact,chooseDay,focusDay)=>{box.drawn=(...args)=>ledLines(l,split,metric,...args);return drawLedColumns(box,l,split,metric,compact,chooseDay,focusDay);},
 drawLedRank,ledCategories,ledRail,cacheHitOf,ledGroupSpeed
};
}\n`;
// Read static locale object literals, never evaluate upstream JavaScript.
function chinese(file,plugin=false){
 const text=read(file),ast=ts.createSourceFile(file,text,ts.ScriptTarget.Latest,true,ts.ScriptKind.JS);let object;
 function visit(node){if(!plugin&&ts.isVariableDeclaration(node)&&node.name.getText(ast)==='I18N')object=node.initializer;
  if(plugin&&ts.isCallExpression(node)&&node.expression.getText(ast)==='magpie.strings')object=node.arguments[0];ts.forEachChild(node,visit);}
 visit(ast);const zh=object?.properties?.find(prop=>prop.name?.getText(ast)==='zh')?.initializer;
 if(!zh||!ts.isObjectLiteralExpression(zh))throw Error('Magpie Chinese dictionary boundary changed');
 return Object.fromEntries(zh.properties.filter(prop=>ts.isPropertyAssignment(prop)&&ts.isStringLiteral(prop.name)&&ts.isStringLiteral(prop.initializer)).map(prop=>[prop.name.text,prop.initializer.text]));
}
const translations={...chinese('internal/gui/assets/i18n.js'),...chinese('internal/gui/assets/plugins/usage/i18n.js',true)};
const keys=new Set();
const chartAST=ts.createSourceFile('ledger.js',original,ts.ScriptTarget.Latest,true,ts.ScriptKind.JS);
function collectKeys(node){if(ts.isStringLiteral(node))keys.add(node.text);ts.forEachChild(node,collectKeys);}
collectKeys(chartAST);
const locale=Object.fromEntries(Object.entries(translations).filter(([key])=>keys.has(key)));
const styles=[between(css,'.led-chart g.ln {','\n.led-chart .led-day'),between(css,'.led-chart .tip.speed {','\n.led-chart .none')].join('\n').replaceAll('.led-chart','.ai-usage .led-chart').replaceAll('var(--','var(--usage-');
const outputs={
 'apps/desktop/frontend/src/features/magpie-ledger-charts.ts':result,
 'apps/desktop/frontend/src/features/magpie-ledger-messages.ts':'// Generated from the same pinned Magpie ledger sources; MIT.\nexport const magpieLedgerChinese:Record<string,string>='+JSON.stringify(locale,null,2)+';\n',
 'apps/desktop/frontend/src/features/magpie-ledger-lines.css':'/* Generated Magpie line chart and speed tooltip rules; MIT. */\n'+styles+'\n',
};
const manifest={schemaVersion:1,repository:'https://github.com/yetone/magpie',revision,license:'MIT',notice:license,sourceFiles:files,files:Object.fromEntries(Object.entries(outputs).map(([file,bytes])=>[file,sha(bytes)])),adaptations:['DOM/locale helpers use WinOne; chart algorithms remain upstream','Namespace CSS tokens, retain expandable overview rankings and compact summaries','React supplies the immutable page/metric/split rather than Magpie app globals']};
outputs['third_party/magpie-usage-source.json']=JSON.stringify(manifest,null,2)+'\n';
for(const [file,bytes] of Object.entries(outputs)){
 const destination=path.join(root,file);
 if(process.argv.includes('--write'))fs.writeFileSync(destination,bytes);
 else if(fs.readFileSync(destination,'utf8').replaceAll('\r\n','\n')!==bytes)throw Error('Magpie usage source differs: '+file);
}
console.log('Magpie usage renderer and source notices verified: '+revision);
