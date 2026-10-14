import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import os from 'node:os';
import {discoverResources} from './resources.mjs';

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

test('plugin manifests cannot silently load resources outside their package',async()=>{
 const root=await fs.mkdtemp(path.join(process.env.WINONE_CHAT_TEST_DIRECTORY??os.tmpdir(),'chat-plugin-boundary-'));
 await fs.mkdir(path.join(root,'.codex-plugin'));
 await fs.writeFile(path.join(root,'.codex-plugin','plugin.json'),JSON.stringify({name:'bad-plugin',skills:'../outside'}));
 await assert.rejects(discoverResources({pluginDirectories:[root]}),/outside|boundary/);
});
