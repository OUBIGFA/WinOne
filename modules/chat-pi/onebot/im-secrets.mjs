// Synchronous encrypted KV adapter for Cindy IMHost.secrets. The 32-byte key
// arrives only through the private Host init pipe; Go stores it with DPAPI.
import fs from 'node:fs';
import path from 'node:path';
import {createCipheriv,createDecipheriv,createHash,randomBytes,randomUUID} from 'node:crypto';
export function createIMSecrets(paths, encodedKey, log, maximumBytes=128*1024) {
 const key=Buffer.from(encodedKey??'','base64');
 let closed=false;
 const directory=path.join(paths.root,'im-secrets');
 const available=()=>!closed&&key.length===32;
 function fileFor(name){
  if(typeof name!=='string'||!/^[A-Za-z0-9_-]{1,256}$/.test(name))throw Error('Invalid IM secret name');
  const file=path.join(directory,createHash('sha256').update(name).digest('hex')+'.sealed');paths.assertPath(file);return file;
 }
 function readResult(name){
  if(!available())return {kind:'error'};
  try{
   const file=fileFor(name);
   let bytes;try{const info=fs.statSync(file);if(info.size>maximumBytes+30)throw Error('IM secret is too large');bytes=fs.readFileSync(file);}catch(error){if(error.code==='ENOENT')return {kind:'missing'};throw error;}
   if(bytes[0]!==1||bytes.length<30)throw Error('Invalid IM secret envelope');
   const decipher=createDecipheriv('aes-256-gcm',key,bytes.subarray(1,13));decipher.setAAD(Buffer.from(name));decipher.setAuthTag(bytes.subarray(13,29));
   return {kind:'value',value:Buffer.concat([decipher.update(bytes.subarray(29)),decipher.final()]).toString('utf8')};
  }catch{log.warn('IM 安全存储读取失败');return {kind:'error'};}
 }
 return {
  isAvailable:available,
  readResult,
  read(name){const result=readResult(name);return result.kind==='value'?result.value:null;},
  write(name,plaintext){
   if(!available()||typeof plaintext!=='string'||Buffer.byteLength(plaintext)>maximumBytes)return false;
   let temporary;
   try{
    const file=fileFor(name);paths.assertPath(directory);fs.mkdirSync(directory,{recursive:true});
    const nonce=randomBytes(12),cipher=createCipheriv('aes-256-gcm',key,nonce);cipher.setAAD(Buffer.from(name));
    const ciphertext=Buffer.concat([cipher.update(plaintext,'utf8'),cipher.final()]);
    const envelope=Buffer.concat([Buffer.from([1]),nonce,cipher.getAuthTag(),ciphertext]);
    temporary=file+'.'+randomUUID()+'.tmp';fs.writeFileSync(temporary,envelope,{flag:'wx',mode:0o600});paths.assertPath(file);fs.renameSync(temporary,file);return true;
   }catch{if(temporary)try{fs.unlinkSync(temporary);}catch{}log.warn('IM 安全存储写入失败');return false;}
  },
  remove(name){if(!available())throw Error('IM 安全存储不可用');const file=fileFor(name);try{fs.unlinkSync(file);}catch(error){if(error.code!=='ENOENT')throw error;}},
  close(){closed=true;key.fill(0);},
 };
}
