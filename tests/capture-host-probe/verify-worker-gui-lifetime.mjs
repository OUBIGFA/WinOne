import {readFile} from "node:fs/promises";
const path=process.argv[2];if(!path)throw Error("Provide repeat-observations.json");
const report=JSON.parse(await readFile(path,"utf8")),samples=report.observations;
if(report.complete!==true||!Array.isArray(samples)||samples.length<101)throw Error("A completed 100-repeat run is required");
for(let i=0;i<samples.length;i++){
 const item=samples[i];
 if(item.session!==i+2||item.processId!==samples[0].processId)throw Error("Session continuity or worker identity changed");
 for(const name of ["privateBytes","workingSetBytes","handles","gdi","user","gen0","gen1","gen2"])
  if(!Number.isSafeInteger(item[name])||item[name]<0)throw Error("Missing or invalid counter: "+name);
}
if(samples.some(item=>item.gdi!==samples[0].gdi))throw Error("GDI objects did not return to the same post-session level");
const steady=samples.slice(3);
if(steady.at(-1).user>steady[0].user)throw Error("USER objects grew after warmup");
const invalidManagedEstimates=samples.filter(item=>!Number.isSafeInteger(item.estimatedManagedBytes)||item.estimatedManagedBytes<0).map(item=>({session:item.session,value:item.estimatedManagedBytes}));
console.log(JSON.stringify({verified:"same-worker continuity and post-session GDI/USER lifetime",samples:samples.length-1,gdi:samples[0].gdi,finalPrivateBytes:samples.at(-1).privateBytes,invalidManagedEstimates,note:"Invalid managed estimates are retained, not clamped or used for memory conclusions. Memory budget and full product performance require separate verification."}));
