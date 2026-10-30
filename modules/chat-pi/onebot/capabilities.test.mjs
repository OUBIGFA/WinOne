import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {createOneBotRuntime} from './runtime.mjs';

const base=process.env.WINONE_ONEBOT_TEST_OUTPUT;
if(!base||!path.isAbsolute(base))throw Error('An isolated OneBot test output is required');
test('capability catalog exposes selectable metadata without loading code or exposing connection secrets',async()=>{
 const directory=await fs.mkdtemp(path.join(base,'capabilities-'));
 const plugin=path.join(directory,'plugin'),marker=path.join(directory,'must-not-execute');
 await fs.mkdir(path.join(plugin,'skills','shared'),{recursive:true});
 await fs.writeFile(path.join(plugin,'skills','shared','SKILL.md'),'---\nname: shared\ndescription: Shared reusable skill\n---\nPrivate skill body');
 await fs.writeFile(path.join(plugin,'package.json'),JSON.stringify({name:'optional-tools',description:'Optional local tools',pi:{extensions:['./extension.mjs'],skills:['./skills']}}));
 await fs.writeFile(path.join(plugin,'extension.mjs'),`import fs from 'node:fs';fs.writeFileSync(${JSON.stringify(marker)},'executed');export default ()=>{};`);
 const runtime=createOneBotRuntime({directory});
 try{
  await runtime.call('profiles.create',{input:{id:'reader',name:'Reader',capabilities:{toolsetMode:'allowlist',toolsets:[]}}});
  await runtime.call('skills.save',{botId:'reader',input:{slug:'personal',name:'personal',description:'Personal reusable skill',body:'Private personal body'}});
  const result=await runtime.call('capabilities.query',{botId:'reader',resources:{agentDir:path.join(directory,'agent'),pluginDirectories:[plugin],mcpServers:[{name:'private-server',url:'https://private.example.test/mcp',headers:{Authorization:'Bearer PRIVATE_CREDENTIAL'},enabled:true}]}});
  assert.deepEqual(result.entries.toolset,[{id:'optional-tools',name:'optional-tools',description:'Optional local tools',available:true}]);
  assert.ok(result.entries.skill.some(item=>item.id==='shared'&&item.available));
  assert.ok(result.entries.skill.some(item=>item.id==='personal:personal'&&item.personal&&item.available));
  assert.equal(result.entries.mcp[0].id,'private-server');
  const serialized=JSON.stringify(result);
  for(const row of Object.values(result.entries).flat()){assert.equal('path' in row,false);assert.equal('headers' in row,false);assert.equal('config' in row,false);}
  for(const privateValue of ['PRIVATE_CREDENTIAL','private.example.test','Private personal body','Private skill body',plugin])assert.ok(!serialized.includes(privateValue));
  await assert.rejects(fs.access(marker),{code:'ENOENT'});
 }finally{await runtime.close();}
});
