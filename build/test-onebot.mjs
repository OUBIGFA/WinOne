// Isolated OneBot tests. TMP/HOME must never point at the developer's actual
// profile: upstream filesystem tests intentionally create and remove fixtures.
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {spawnSync} from 'node:child_process';
const root=fileURLToPath(new URL('../',import.meta.url));
const argument=process.argv.indexOf('--output');
const output=path.resolve(root,argument<0?'_temp/onebot-tests/'+Date.now():process.argv[argument+1]);
const relative=path.relative(path.join(root,'_temp'),output);
if(!relative||relative.startsWith('..')||path.isAbsolute(relative))throw Error('OneBot test output must be inside _temp/<task>');
const ignored=spawnSync('git',['check-ignore','-q','--no-index',output],{cwd:root});
if(ignored.status!==0)throw Error('OneBot test output must be Git-ignored');
fs.mkdirSync(output,{recursive:true});
const realRelative=path.relative(fs.realpathSync(path.join(root,'_temp')),fs.realpathSync(output));
if(!realRelative||realRelative.startsWith('..')||path.isAbsolute(realRelative))throw Error('OneBot test output resolves outside the temporary tree');
if(fs.existsSync(path.join(output,'result.json')))throw Error('Choose a new output directory to preserve previous evidence');
const home=path.join(output,'home');
fs.mkdirSync(home,{recursive:true});
// Production workers bypass proxies for loopback (internal/chat StartRunner).
// Windows variable names are case-insensitive: keep one NO_PROXY entry.
const inherited=Object.fromEntries(Object.entries(process.env).filter(([name])=>name.toUpperCase()!=='NO_PROXY'));
const bypass=[process.env.NO_PROXY,'localhost,127.0.0.1,::1'].filter(Boolean).join(',');
const env={...inherited,NO_PROXY:bypass,HOME:home,USERPROFILE:home,TMPDIR:output,TEMP:output,TMP:output,WINONE_ONEBOT_TEST_OUTPUT:output};
const bunArgument=process.argv.indexOf('--bun');
const bun=bunArgument<0?process.env.WINONE_CHAT_BUN_EXE:path.resolve(process.argv[bunArgument+1]);
const filterArgument=process.argv.indexOf('--filter');
const filter=filterArgument<0?[]:['-t',process.argv[filterArgument+1]];
if(process.argv.includes('--runtime-only')&&!bun)throw Error('--runtime-only requires --bun <BunExe>');
if(!process.argv.includes('--runtime-only')){
 const result=spawnSync(process.execPath,[
  path.join(root,'apps/desktop/frontend/node_modules/vitest/vitest.mjs'),
  'run','--config',path.join(root,'build/onebot-vitest.config.mjs'),
 ],{cwd:root,stdio:'inherit',env});
 if(result.error)throw result.error;
 if(result.status!==0)process.exit(result.status??1);
}
if(bun){
 // Bun executes the production SQLite path; Vitest above keeps the pinned
 // upstream unit tests without substituting or mocking Bun's database.
 const result=spawnSync(bun,['test',path.join(root,'modules/chat-pi/onebot'), '--timeout','20000',...filter],
  {cwd:root,env,encoding:'utf8',maxBuffer:20*1024*1024});
 fs.writeFileSync(path.join(output,'runtime.log'),(result.stdout??'')+(result.stderr??''));
 fs.writeFileSync(path.join(output,'runtime-result.json'),JSON.stringify({exitCode:result.status,error:result.error?.message??null},null,2)+'\n');
 process.stdout.write(result.stdout??'');process.stderr.write(result.stderr??'');
 if(result.error)throw result.error;
 process.exitCode=result.status??1;
}
