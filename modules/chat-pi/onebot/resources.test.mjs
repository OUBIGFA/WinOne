import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {createWinOneSession} from '../session.mjs';
import {discoverResources,inspectResources} from '../resources.mjs';

const base=process.env.WINONE_ONEBOT_TEST_OUTPUT;
if(!base||!path.isAbsolute(base))throw Error('An isolated OneBot test output is required');
async function skill(root,name,marker){
 const directory=path.join(root,name);await fs.mkdir(directory,{recursive:true});
 await fs.writeFile(path.join(directory,'SKILL.md'),`---\nname: ${name}\ndescription: ${marker}\n---\n${marker}\n`);
 return directory;
}
async function fixture(){
 const root=await fs.mkdtemp(path.join(base,'resources-')),cwd=path.join(root,'workspace');
 await fs.mkdir(cwd);
 const config={cwd,agentDir:path.join(root,'agent'),sessionDirectory:path.join(root,'sessions'),
  baseUrl:'http://127.0.0.1:12345',key:'isolated-model-key',model:{id:'fixture/model',name:'Fixture'},permission:'ask',
  onebot:{homeDir:root,baseUrl:'http://127.0.0.1:12345',key:'isolated-bot-key',memoryEnabled:false,skillDirectories:[],
   capabilities:{skillMode:'allowlist',skills:[],toolsetMode:'allowlist',toolsets:[],mcpMode:'allowlist',mcpServers:[]}}};
 return {root,cwd,config};
}

test('OneBot loads selected external skills and personal skills without ambient grants',async()=>{
 const f=await fixture(),home=path.join(f.root,'home');
 const previous=process.env.HOME;process.env.HOME=home;
 let session;
 try{
  await skill(path.join(home,'.agents','skills'),'ambient','AMBIENT_MUST_NOT_LOAD');
  await skill(path.join(f.cwd,'.pi','skills'),'project','PROJECT_MUST_NOT_LOAD');
  const selected=await skill(path.join(f.root,'shared'),'selected','SELECTED_SKILL');
  const own=await skill(path.join(f.root,'personal'),'own','PERSONAL_SKILL');
  f.config.skillDirectories=[path.dirname(selected),own];
  f.config.onebot.skillDirectories=[own];f.config.onebot.capabilities.skills=['selected'];
  session=await createWinOneSession(f.config);
  assert.deepEqual(session.resourceLoader.getSkills().skills.map(item=>item.name).sort(),['own','selected']);
  await session.close();session=null;
  f.config.onebot.capabilities.skillMode='inherit';
  session=await createWinOneSession(f.config);
  assert.deepEqual(session.resourceLoader.getSkills().skills.map(item=>item.name),['own']);
 }finally{await session?.close();if(previous===undefined)delete process.env.HOME;else process.env.HOME=previous;}
});

test('a same-named ambient skill cannot replace a OneBot personal skill',async()=>{
 const f=await fixture(),home=path.join(f.root,'home');
 const previous=process.env.HOME;process.env.HOME=home;
 let session;
 try{
  await skill(path.join(home,'.agents','skills'),'same-name','AMBIENT_COPY');
  const own=await skill(path.join(f.root,'personal'),'same-name','PERSONAL_COPY');
  f.config.skillDirectories=[own];f.config.onebot.skillDirectories=[own];
  session=await createWinOneSession(f.config);
  const loaded=session.resourceLoader.getSkills().skills;
  assert.equal(loaded.length,1);
  assert.equal(loaded[0].filePath,path.join(own,'SKILL.md'));
 }finally{await session?.close();if(previous===undefined)delete process.env.HOME;else process.env.HOME=previous;}
});

test('an excluded plugin never executes while its metadata remains available for selection',async()=>{
 const f=await fixture(),plugin=path.join(f.root,'plugin'),marker=path.join(f.root,'executed.txt');
 await fs.mkdir(plugin);
 await skill(path.join(plugin,'skills'),'from-plugin','PLUGIN_SKILL_DOCS');
 await fs.writeFile(path.join(plugin,'package.json'),JSON.stringify({name:'optional-fixture',pi:{extensions:['./extension.mjs'],skills:['./skills']}}));
 await fs.writeFile(path.join(plugin,'extension.mjs'),`import fs from 'node:fs';fs.writeFileSync(${JSON.stringify(marker)},'EXECUTED');export default ()=>{};`);
 f.config.pluginDirectories=[plugin];
 f.config.onebot.capabilities.skills=['from-plugin'];
 const catalog=await inspectResources({...f.config,onebot:undefined});
 assert.equal(catalog.plugins[0].name,'optional-fixture');
 await assert.rejects(fs.access(marker),{code:'ENOENT'});
 let session;
 try{
  session=await createWinOneSession(f.config);
  await assert.rejects(fs.access(marker),{code:'ENOENT'});
  assert.deepEqual(session.resourceLoader.getSkills().skills.map(skill=>skill.name),['from-plugin']);
  await session.close();session=null;
  f.config.onebot.capabilities.toolsets=[catalog.plugins[0].id];
  session=await createWinOneSession(f.config);
  assert.equal(await fs.readFile(marker,'utf8'),'EXECUTED');
 }finally{await session?.close();}
});

test('OneBot MCP selection keeps enabled listed servers and preserves ordinary Chat discovery',async()=>{
 const f=await fixture();
 f.config.mcpServers=[{name:'selected',url:'http://127.0.0.1:12345',enabled:true},{name:'unselected',url:'http://127.0.0.1:12346',enabled:true}];
 f.config.onebot.capabilities.mcpServers=['selected'];
 assert.deepEqual((await discoverResources(f.config)).mcpServers.map(item=>item.name),['selected']);
 assert.deepEqual((await discoverResources({...f.config,onebot:undefined})).mcpServers.map(item=>item.name),['selected','unselected']);
});
