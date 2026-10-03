import {defineConfig,mergeConfig} from "vite";
import base from "../vite.config.ts";
import {execFile} from "node:child_process";
import {promisify} from "node:util";
import {mkdir,readFile,writeFile} from "node:fs/promises";
import {dirname,resolve} from "node:path";
import {fileURLToPath} from "node:url";

const root=resolve(dirname(fileURLToPath(import.meta.url)),"../../../..");
const directory=resolve(root,"_temp/capture-editor-fixture");
const harness=resolve(root,"tests/capture-unit/bin/Release/net10.0-windows/WinOne.Capture.Tests.dll");
const run=()=>promisify(execFile)("dotnet",[harness,directory,"--editor-fixture"],{windowsHide:true});

// Local-only fixture with the actual renderer and bounded request bodies.
export default mergeConfig(base,defineConfig({plugins:[{name:"capture-editor-fixture",async configureServer(server){
  await mkdir(directory,{recursive:true});
  await run();
  let rendering=false;
  server.middlewares.use(async(request,response,next)=>{
    const source=request.url===`/capture-preview/${"a".repeat(32)}.png`||request.url===`/capture-preview/${"c".repeat(32)}.png`;
    const longSource=request.url===`/capture-source/${"e".repeat(32)}.png`;
    const original=longSource||request.url===`/capture-source/${"a".repeat(32)}.png`;
    const result=request.url===`/capture-edit-preview/${"b".repeat(32)}.png`;
    if(!source&&!original&&!result&&request.url!=="/fixture/render"){next();return;}
    try {
      response.setHeader("Cache-Control","no-store");
      if(source||original||result) {
        const data=await readFile(resolve(directory,original?"drafts":"previews",`${(longSource?"e":source||original?"a":"b").repeat(32)}.png`));
        response.setHeader("Content-Type","image/png");response.end(data);return;
      }
      if(request.method!=="POST"){response.statusCode=405;response.end();return;}
      if(rendering){response.statusCode=409;response.end("Renderer busy");return;}
      rendering=true;
      try {
        const chunks=[];let size=0;
        for await(const chunk of request){size+=chunk.length;if(size>768*1024)throw Error("Document too large");chunks.push(chunk);}
        const data=Buffer.concat(chunks);JSON.parse(data.toString("utf8"));
        await writeFile(resolve(directory,"document.json"),data);await run();
        response.end("ok");
      } finally {rendering=false;}
    } catch(error) {response.statusCode=500;response.end(String(error));}
  });
}}]}));
