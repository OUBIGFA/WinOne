// Reuse the same locked Lucide artwork and WinOne logo as the existing UI.
import {createRequire} from "node:module";
import {mkdir,writeFile,readFile} from "node:fs/promises";
import {resolve,dirname} from "node:path";
import {fileURLToPath} from "node:url";
const root=resolve(dirname(fileURLToPath(import.meta.url)),"..");
const require=createRequire(resolve(root,"apps/desktop/frontend/package.json"));
const React=require("react"),{renderToStaticMarkup}=require("react-dom/server"),icons=require("lucide-react");
const output=resolve(root,"apps/result-ui/assets");await mkdir(output,{recursive:true});
const toolbarIcons=new Set(["translate","explain","summarize","polish","code","conversation","service","copy","book","idea","search","document","check","write","mail","globe","learn","calculate","terminal","braces","tasks","text","case","wand","astroid","quick-assistant"]);
for(const [id,name] of Object.entries({"quick-assistant":"Astroid",book:"BookOpen",idea:"Lightbulb",document:"FileText",write:"NotebookPen",mail:"Mail",globe:"Globe",learn:"GraduationCap",calculate:"Calculator",terminal:"Terminal",braces:"Braces",tasks:"ListChecks",text:"TextCursorInput",case:"CaseSensitive",wand:"WandSparkles",astroid:"Astroid",search:"Search",check:"Check",model:"Sparkle",thinking:"Sparkles","thinking-complete":"Layers",translate:"Languages",explain:"MessageCircleQuestion",summarize:"ListCheck",polish:"PenLine",plus:"Plus",minus:"Minus",code:"CodeXml",conversation:"MessageCircleMore",collapse:"ChevronsUp",service:"PlugZap","provider-default":"Layers",chevron:"ChevronDown",grip:"GripVertical",pin:"Pin",unpin:"PinOff",close:"X",arrow:"ArrowRight",copy:"Copy",clear:"Eraser",restart:"RotateCcw",retry:"RefreshCw",send:"ArrowUp",stop:"Square",speak:"Volume2"})){
 await writeFile(resolve(output,id+".svg"),renderToStaticMarkup(React.createElement(icons[name],{size:24,strokeWidth:1.8})),"utf8");
 // The floating selection toolbar uses the same artwork at a lighter weight.
 if(toolbarIcons.has(id))await writeFile(resolve(output,"toolbar-"+id+".svg"),renderToStaticMarkup(React.createElement(icons[id==="check"?"CheckCheck":name],{size:24,strokeWidth:1.5})),"utf8");
}
await writeFile(resolve(output,"logo.png"),await readFile(resolve(root,"internal/branding/logo.png")));
await writeFile(resolve(output,"LICENSE.txt"),await readFile(resolve(root,"apps/desktop/frontend/node_modules/lucide-react/LICENSE")));

