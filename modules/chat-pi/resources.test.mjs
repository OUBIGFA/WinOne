import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import {discoverResources,inspectResources} from './resources.mjs';

test('MCP exposure choices preserve their upstream meaning and reject unknown values',async()=>{
 for(const exposure of ['direct','deferred','codemode','hidden']){
  const result=await discoverResources({mcpServers:[{name:'fixture',url:'http://127.0.0.1:12345',exposure}]});
  assert.equal(result.mcpServers[0].config.exposure,exposure);
 }
 await assert.rejects(discoverResources({mcpServers:[{name:'fixture',url:'http://127.0.0.1:12345',exposure:'unknown'}]}),/exposure/i);
});

test('configured skills and Codex plugin skills/MCP resolve within their explicit package',async()=>{
 const root=await fs.mkdtemp(path.join(process.env.WINONE_CHAT_TEST_DIRECTORY??os.tmpdir(),'chat-resources-'));
 const skill=path.join(root,'skills','example');await fs.mkdir(skill,{recursive:true});
 await fs.writeFile(path.join(skill,'SKILL.md'),'---\nname: example\ndescription: A reusable example\n---\nUse EXAMPLE_MARKER.');
 const plugin=path.join(root,'plugin');await fs.mkdir(path.join(plugin,'.codex-plugin'),{recursive:true});
 await fs.mkdir(path.join(plugin,'skills','plugin-skill'),{recursive:true});
 await fs.writeFile(path.join(plugin,'.codex-plugin','plugin.json'),JSON.stringify({name:'example-plugin',skills:'./skills',mcpServers:'./.mcp.json'}));
 await fs.writeFile(path.join(plugin,'.mcp.json'),JSON.stringify({mcpServers:{example:{url:'http://127.0.0.1:12345/mcp'}}}));
 const result=await discoverResources({skillDirectories:[path.dirname(skill)],pluginDirectories:[plugin],mcpServers:[]});
 assert.ok(result.skillPaths.includes(path.dirname(skill)));
 assert.ok(result.skillPaths.includes(path.join(plugin,'skills')));
 assert.equal(result.plugins[0].name,'example-plugin');
 assert.equal(result.mcpServers[0].config.url,'http://127.0.0.1:12345/mcp');
 assert.equal(result.diagnostics.length,0);
});

test('model-free resource inspection reads metadata without executing plugins',async()=>{
 const root=await fs.mkdtemp(path.join(process.env.WINONE_CHAT_TEST_DIRECTORY??os.tmpdir(),'chat-resource-inspection-'));
 const cwd=path.join(root,'workspace'),plugin=path.join(root,'plugin'),marker=path.join(root,'executed.txt');
 await fs.mkdir(cwd);await fs.mkdir(plugin);await fs.writeFile(path.join(cwd,'AGENTS.md'),'Inspect these rules.');
 await fs.writeFile(path.join(plugin,'package.json'),JSON.stringify({name:'inert-plugin',pi:{extensions:['./extension.mjs']}}));
 await fs.writeFile(path.join(plugin,'extension.mjs'),`import fs from 'node:fs';fs.writeFileSync(${JSON.stringify(marker)},'should not run');export default ()=>{};`);
 const result=await inspectResources({cwd,agentDir:path.join(root,'agent'),pluginDirectories:[plugin]});
 assert.ok(result.agents.some(item=>item.path===path.join(cwd,'AGENTS.md')));assert.equal(result.plugins[0].name,'inert-plugin');
 await assert.rejects(fs.access(marker),{code:'ENOENT'});
});

test('default resources discover shared and in-repository skills without loading project extensions',async()=>{
 const root=await fs.mkdtemp(path.join(process.env.WINONE_CHAT_TEST_DIRECTORY??os.tmpdir(),'chat-default-skills-'));
 const cwd=path.join(root,'repo','nested'),home=path.join(root,'home'),agentDir=path.join(root,'agent');
 await fs.mkdir(cwd,{recursive:true});await fs.mkdir(path.join(root,'repo','.git'));
 for(const [directory,name] of [[path.join(home,'.agents','skills'),'shared-skill'],[path.join(root,'repo','.agents','skills'),'ancestor-skill'],[path.join(cwd,'.pi','skills'),'project-skill']]){
  const folder=path.join(directory,name);await fs.mkdir(folder,{recursive:true});await fs.writeFile(path.join(folder,'SKILL.md'),`---\nname: ${name}\ndescription: ${name} instructions\n---\nUse this skill.`);
 }
 const marker=path.join(root,'must-not-run');await fs.mkdir(path.join(cwd,'.pi','extensions'),{recursive:true});await fs.writeFile(path.join(cwd,'.pi','extensions','unsafe.mjs'),`import fs from 'node:fs';fs.writeFileSync(${JSON.stringify(marker)},'executed');export default ()=>{};`);
 // HOME is Pi's documented discovery seam; no real user profile is modified.
 const previous=process.env.HOME;process.env.HOME=home;
 try{
  const result=await inspectResources({cwd,agentDir});
  assert.deepEqual(result.skills.map(skill=>skill.name).sort(),['ancestor-skill','project-skill','shared-skill']);
  await assert.rejects(fs.access(marker),{code:'ENOENT'});
 }finally{if(previous===undefined)delete process.env.HOME;else process.env.HOME=previous;}
});

test('plugin manifests cannot silently load resources outside their package',async()=>{
 const root=await fs.mkdtemp(path.join(process.env.WINONE_CHAT_TEST_DIRECTORY??os.tmpdir(),'chat-plugin-boundary-'));
 await fs.mkdir(path.join(root,'.codex-plugin'));
 await fs.writeFile(path.join(root,'.codex-plugin','plugin.json'),JSON.stringify({name:'bad-plugin',skills:'../outside'}));
 await assert.rejects(discoverResources({pluginDirectories:[root]}),/outside|boundary/);
});
