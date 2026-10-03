import net from "node:net";
import {readFile,access} from "node:fs/promises";
import {dirname,resolve} from "node:path";

export async function readNativeState(path){
 const folder=dirname(path);let descriptor;
 try{descriptor=JSON.parse(await readFile(resolve(folder,"state-pipe.json"),"utf8"));}
 catch(error){if(error.code==="ENOENT")return JSON.parse(await readFile(path,"utf8"));throw error;}
 if(descriptor.version!==1||!/^WinOneNativeState-[a-f0-9]{32}$/.test(descriptor.name))throw Error("Invalid native state endpoint");
 try{return await new Promise((accept,reject)=>{
  const socket=net.createConnection("\\\\.\\pipe\\"+descriptor.name);let bytes=Buffer.alloc(0),finished=false;
  socket.setTimeout(5000,()=>socket.destroy(Error("Native state read timed out")));
  socket.on("error",reject);socket.on("end",()=>{if(!finished)reject(Error("Incomplete native state"));});
  socket.on("data",chunk=>{
   bytes=Buffer.concat([bytes,chunk]);if(bytes.length<4)return;
   const length=bytes.readUInt32LE(0);if(length>1024*1024){socket.destroy(Error("Oversized native state"));return;}
   if(bytes.length<4+length)return;
   try{const value=JSON.parse(bytes.subarray(4,4+length));finished=true;socket.end(Buffer.from([1]));accept(value);}catch(error){socket.destroy(error);}
  });
 });}catch(error){
  // A final snapshot is authoritative only after the observer marks completion.
  try{await access(resolve(folder,"state-complete.json"));}catch{throw error;}
  return JSON.parse(await readFile(path,"utf8"));
 }
}
