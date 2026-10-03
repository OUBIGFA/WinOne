// Rasterize the same locked Lucide icons used by WinOne's React UI.
import {createRequire} from "node:module";
import {mkdir,readFile,writeFile} from "node:fs/promises";
import {resolve,dirname} from "node:path";
import {fileURLToPath} from "node:url";
const here=dirname(fileURLToPath(import.meta.url));
const root=resolve(here,"..");
const require=createRequire(resolve(root,"apps/desktop/frontend/package.json"));
const React=require("react"),{renderToStaticMarkup}=require("react-dom/server"),icons=require("lucide-react");
const {chromium}=require(process.env.WINONE_PLAYWRIGHT_MODULE||"playwright");
const names={pin:"Pin",overflow:"Ellipsis",grip:"GripVertical",confirm:"Check",save:"Save",saveAs:"FolderOpen",undo:"Undo2",redo:"Redo2",cancel:"X",region:"ScanLine",rectangle:"Square",ellipse:"Circle",pen:"Pencil",cover:"PaintBucket",arrow:"MoveUpRight",mosaic:"Grid2X2",blur:"Blend",text:"Type"};
const output=resolve(root,"modules/capture/NativeUI/Assets/Icons");await mkdir(output,{recursive:true});
const context=await chromium.launchPersistentContext(resolve(root,"_temp/native-icon-generator-"+Date.now()),{headless:true,executablePath:process.env.WINONE_CHROMIUM_PATH});
try{
 const page=await context.newPage();
 for(const [id,name] of Object.entries(names))for(const size of [18,24,36,48,72])for(const [suffix,color] of [["","#171717"],["-dark","#fafafa"]]){
  const svg=renderToStaticMarkup(React.createElement(icons[name],{size,color,strokeWidth:1.8}));
  const png=await page.evaluate(async({svg,size})=>{const img=new Image();img.src="data:image/svg+xml,"+encodeURIComponent(svg);await img.decode();const canvas=document.createElement("canvas");canvas.width=canvas.height=size;canvas.getContext("2d").drawImage(img,0,0);return canvas.toDataURL("image/png").split(",")[1];},{svg,size});
  await writeFile(resolve(output,`${id}${suffix}-${size}.png`),Buffer.from(png,"base64"));
 }
 const license=await readFile(resolve(root,"apps/desktop/frontend/node_modules/lucide-react/LICENSE"),"utf8");
 await writeFile(resolve(output,"LICENSE.txt"),license);
 await writeFile(resolve(output,"README.md"),"# 截图工具栏图标\n\n由已锁定的 lucide-react 1.27.0 生成，许可证见 LICENSE.txt。在仓库运行 node build/generate-capture-icons.mjs 可重新生成。运行软件时无需浏览器或联网。\n");
 console.log("Generated local Lucide PNG icons.");
}finally{await context.close();}
