// Runs the React tests that cover what the MyGo window ships: the Chat tools
// page (chat-embed.html) and every source file it bundles. The settings
// window pages that MyGo replaced natively keep their tests for the Rust and
// legacy desktops, which run the full suite.
//
// The bundled files come from the same Vite configuration build:mygo uses,
// so the selection follows the page's real imports, not a hand-kept list.
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import {fileURLToPath} from "node:url";
import {build} from "vite";
import {startVitest} from "vitest/node";

const root=path.dirname(fileURLToPath(import.meta.url));
const source=path.join(root,"src")+path.sep;
const scratch=fs.mkdtempSync(path.join(os.tmpdir(),"winone-mygo-graph-"));
const shipped=new Set();
// A production build sets NODE_ENV for this process; the tests need React's
// development build (act), as a plain vitest run gets.
const environment=process.env.NODE_ENV;
try{
 await build({
  configFile:path.join(root,"vite.mygo.config.ts"),logLevel:"warn",
  build:{outDir:scratch,emptyOutDir:true,write:false},
  plugins:[{name:"winone-shipped-sources",generateBundle(){
   for(const id of this.getModuleIds()){
    const file=path.resolve(id.split("?")[0].replace(/^\0+/,""));
    if(file.startsWith(source))shipped.add(file);
   }
  }}],
 });
}finally{
 fs.rmSync(scratch,{recursive:true,force:true});
 if(environment===undefined)delete process.env.NODE_ENV;else process.env.NODE_ENV=environment;
}
if(shipped.size===0)throw new Error("The MyGo Chat tools page bundled no source files");
const vitest=await startVitest("test",[],{run:true,watch:false,related:[...shipped],pool:"threads"});
const failed=!vitest||vitest.state.getCountOfFailedTests()>0||vitest.state.getUnhandledErrors().length>0||process.exitCode;
await vitest?.close();
process.exit(failed?1:0);
