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
const names={select:"Move",pin:"Pin","pin-active":"Pin",overflow:"Ellipsis",grip:"GripVertical",confirm:"Check",save:"Save",saveAs:"FolderOpen",undo:"Undo2",redo:"Redo2",cancel:"X",region:"ScanLine",rectangle:"Square",ellipse:"Circle",pen:"Pencil",cover:"PaintBucket",arrow:"MoveUpRight",mosaic:"WandSparkles",scroll:"GalleryVerticalEnd",video:"Video",gif:null,pause:"Pause",resume:"Play",stop:"Square",chevron:"ChevronRight",blur:"Blend",text:"Type",ocr:"ScanText",translate:"Languages"};
const trayOnly=process.argv.includes("--tray-only");
const output=resolve(root,"modules/capture/NativeUI/Assets/Icons");if(!trayOnly)await mkdir(output,{recursive:true});
const context=await chromium.launchPersistentContext(resolve(root,"_temp/native-icon-generator-"+Date.now()),{headless:true,executablePath:process.env.WINONE_CHROMIUM_PATH});
try{
 const page=await context.newPage();
 for(const [id,name] of Object.entries(trayOnly?{}:names))for(const size of [18,24,36,48,72])for(const [suffix,color] of [["","#171717"],["-dark","#fafafa"]]){
  let svg=id==="gif"
   ? `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 24 24"><text x="12" y="15.5" text-anchor="middle" font-family="Segoe UI, sans-serif" font-weight="600" font-size="10" fill="${color}">GIF</text></svg>`
   : renderToStaticMarkup(React.createElement(icons[name],{size,color,strokeWidth:1.8}));
  // Shared result windows rotate the official Pin by 45 degrees when pinned.
  if(id==="pin-active")svg=svg.replace(/(<svg[^>]*>)(.*)(<\/svg>)/s,'$1<g transform="rotate(45 12 12)">$2</g>$3');
  const png=await page.evaluate(async({svg,size})=>{const img=new Image();img.src="data:image/svg+xml,"+encodeURIComponent(svg);await img.decode();const canvas=document.createElement("canvas");canvas.width=canvas.height=size;canvas.getContext("2d").drawImage(img,0,0);return canvas.toDataURL("image/png").split(",")[1];},{svg,size});
  await writeFile(resolve(output,`${id}${suffix}-${size}.png`),Buffer.from(png,"base64"));
 }
 const trayOutput=resolve(root,"third_party/systray/assets");await mkdir(trayOutput,{recursive:true});
 // Rasterize the official path, never a text-font approximation. A large local
 // source supports fractional DPI; the native menu filters it to 16 DIP.
 for(const [theme,color] of [["light","#262626"],["dark","#e9e9e9"]]){
  const svg=renderToStaticMarkup(React.createElement(icons.ChevronRight,{size:96,color,strokeWidth:1.75}));
  const png=await page.evaluate(async svg=>{const image=new Image();image.src="data:image/svg+xml,"+encodeURIComponent(svg);await image.decode();const canvas=document.createElement("canvas");canvas.width=canvas.height=96;canvas.getContext("2d").drawImage(image,0,0);return canvas.toDataURL("image/png").split(",")[1];},svg);
  await writeFile(resolve(trayOutput,`chevron-right-${theme}.png`),Buffer.from(png,"base64"));
 }
 const hostOutput=resolve(root,"internal/branding/menu-icons");await mkdir(hostOutput,{recursive:true});
 // These are alpha templates. The popup's existing theme/DPI snapshot tints
 // them; the neutral source is also readable in the native contrast fallback.
 for(const [id,name] of [["open","AppWindow"],["exit","LogOut"]]){
  const svg=renderToStaticMarkup(React.createElement(icons[name],{size:96,color:"#737373",strokeWidth:1.75}));
  const png=await page.evaluate(async svg=>{const image=new Image();image.src="data:image/svg+xml,"+encodeURIComponent(svg);await image.decode();const canvas=document.createElement("canvas");canvas.width=canvas.height=96;canvas.getContext("2d").drawImage(image,0,0);return canvas.toDataURL("image/png").split(",")[1];},svg);
  await writeFile(resolve(hostOutput,`${id}.png`),Buffer.from(png,"base64"));
 }
 const license=await readFile(resolve(root,"apps/desktop/frontend/node_modules/lucide-react/LICENSE"),"utf8");
 await writeFile(resolve(trayOutput,"Lucide-LICENSE.txt"),license);
 await writeFile(resolve(hostOutput,"LICENSE.txt"),license);
 if(!trayOnly){
 await writeFile(resolve(output,"LICENSE.txt"),license);
 await writeFile(resolve(output,"README.md"),"# 截图工具栏图标\n\n由已锁定的 lucide-react 1.27.0 生成，许可证见 LICENSE.txt。置顶状态沿用全局 Pin 的 45 度旋转；GIF 使用明确的文字标识而非胶片图标。在仓库运行 node build/generate-capture-icons.mjs 可重新生成。运行软件时无需浏览器或联网。\n");
 }
 console.log("Generated local Lucide PNG icons.");
}finally{await context.close();}
