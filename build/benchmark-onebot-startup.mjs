// Measure a fresh worker process through the same JSON-lines protocol as Host.
// Each sample owns a fresh profile; this is first use, not a cold OS-cache test.
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {spawn, spawnSync} from 'node:child_process';
import {createInterface} from 'node:readline';
import {once} from 'node:events';
import {createServer} from 'node:http';
import {createHash} from 'node:crypto';

const root=fileURLToPath(new URL('../',import.meta.url));
const [bunArgument,workerArgument,outputArgument,countArgument='5']=process.argv.slice(2);
if(!bunArgument||!workerArgument||!outputArgument)throw Error('Usage: <bun> <worker> <output directory> [samples]');
const bun=path.resolve(bunArgument),worker=path.resolve(workerArgument),output=path.resolve(outputArgument);
const relative=path.relative(path.join(root,'_temp'),output),count=Number(countArgument);
if(!relative||relative.startsWith('..')||path.isAbsolute(relative)||!Number.isInteger(count)||count<1||count>50)throw Error('Use an isolated _temp directory and 1–50 samples');
if(spawnSync('git',['check-ignore','-q','--no-index',output],{cwd:root}).status!==0)throw Error('Output must be ignored');
await fs.mkdir(output,{recursive:true});
const realRelative=path.relative(await fs.realpath(path.join(root,'_temp')),await fs.realpath(output));
if(!realRelative||realRelative.startsWith('..')||path.isAbsolute(realRelative))throw Error('Output resolves outside _temp');
const report=path.join(output,'startup.json');
try{await fs.access(report);throw Error('Choose a new output directory');}catch(error){if(error.code!=='ENOENT')throw error;}
const home=path.join(output,'home');await fs.mkdir(home);
const rows=[];
// Empty profiles need no Host callbacks. A local bridge makes the task/group
// service available while making any unexpected callback fail visibly.
let callbacks=0;
const server=createServer((request,response)=>{callbacks++;response.writeHead(500);response.end('Unexpected startup callback');});
server.listen(0,'127.0.0.1');await once(server,'listening');server.unref();
for(let sample=0;sample<count;sample++){
  const directory=path.join(output,`profile-${sample}`);
  const start=performance.now();
  const child=spawn(bun,[worker],{windowsHide:true,stdio:['pipe','pipe','pipe'],env:{...process.env,HOME:home,USERPROFILE:home,TEMP:output,TMP:output,NO_PROXY:'localhost,127.0.0.1,::1'}});
  const exit=once(child,'exit');
  const lines=createInterface({input:child.stdout});
  const waiting=new Map();let sequence=0,stderr='';
  child.stderr.on('data',bytes=>{stderr+=bytes;});
  let resolveReady,rejectReady;
  const ready=new Promise((resolve,reject)=>{resolveReady=resolve;rejectReady=reject;});
  child.on('error',rejectReady);
  child.on('exit',code=>{const error=Error(`Worker exited (${code}): ${stderr}`);rejectReady(error);for(const request of waiting.values())request.reject(error);});
  lines.on('line',line=>{
    const message=JSON.parse(line);
    if(message.type==='ready')resolveReady();
    if(message.type==='response'){
      const request=waiting.get(message.id);waiting.delete(message.id);
      if(message.ok)request?.resolve(message.value);else request?.reject(Error(message.error));
    }
  });
  const timeout=setTimeout(()=>child.kill(),30000);
  function call(method,params={}){const id=String(++sequence);return new Promise((resolve,reject)=>{waiting.set(id,{resolve,reject});child.stdin.write(JSON.stringify({id,method,params})+'\n');});}
  try{
    await ready;const readyMs=performance.now()-start;
    await call('init',{directory,masterKey:Buffer.alloc(32,7).toString('base64'),dispatchBaseUrl:`http://127.0.0.1:${server.address().port}`,dispatchKey:'startup-fixture-key-'.repeat(3)});const initMs=performance.now()-start;
    const profiles=await call('profiles.list');
    await call('bots.previews',{items:[]});
    await call('groups.list');
    rows.push({sample:sample+1,readyMs,initMs,homeMs:performance.now()-start,profiles:profiles.length});
  }finally{
    child.stdin.end();const [code]=await exit;clearTimeout(timeout);lines.close();
    if(code!==0)throw Error(`Worker did not exit cleanly (${code}): ${stderr}`);
  }
}
server.close();
if(callbacks)throw Error(`Empty startup made ${callbacks} unexpected Host callbacks`);
const median=field=>{const values=rows.map(row=>row[field]).sort((a,b)=>a-b),middle=Math.floor(values.length/2);return values.length%2?values[middle]:(values[middle-1]+values[middle])/2;};
const workerBytes=await fs.readFile(worker);
const result={worker,workerSHA256:createHash('sha256').update(workerBytes).digest('hex'),workerBytes:workerBytes.length,samples:rows,median:{readyMs:median('readyMs'),initMs:median('initMs'),homeMs:median('homeMs')}};
await fs.writeFile(report,JSON.stringify(result,null,2)+'\n','utf8');
console.log(JSON.stringify(result.median));
