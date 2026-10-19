import fs from 'node:fs/promises';
import path from 'node:path';
import {DefaultResourceLoader,SettingsManager} from '@earendil-works/pi-coding-agent';
import {validateMcpServerConfig} from './node_modules/@earendil-works/pi-coding-agent/dist/core/mcp-servers.js';
import {collectPiProjectResourceCliPaths} from './cindy/project-resource-cli.ts';

async function readJSON(file,optional=false){
 try{const info=await fs.stat(file);if(info.size>1024*1024)throw Error('Resource manifest is too large');return JSON.parse(await fs.readFile(file,'utf8'));}
 catch(error){if(optional&&error.code==='ENOENT')return null;throw error;}
}
async function exists(file){try{await fs.access(file);return true;}catch(error){if(error.code==='ENOENT')return false;throw error;}}
async function packagePath(root,relative){
 if(typeof relative!=='string'||path.isAbsolute(relative))throw Error('Plugin resource must stay inside its package boundary');
 const target=path.resolve(root,relative),rel=path.relative(root,target);
 if(rel==='..'||rel.startsWith('..'+path.sep))throw Error('Plugin resource is outside its package boundary');
 const real=await fs.realpath(target),resolved=path.relative(root,real);
 if(resolved==='..'||resolved.startsWith('..'+path.sep)||path.isAbsolute(resolved))throw Error('Plugin resource link is outside its package boundary');
 return real;
}
function serverEntry(name,config,source){
 if(!/^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/.test(name))throw Error('Invalid MCP server name: '+name);
 if(!config||typeof config!=='object'||!!config.command===!!config.url)throw Error('MCP server requires one command or URL: '+name);
 if(config.url){const url=new URL(config.url);if(!['https:','http:'].includes(url.protocol)||url.username||url.password)throw Error('Invalid MCP HTTP address');}
 for(const values of [config.env,config.headers])for(const [key,value] of Object.entries(values??{}))if(typeof value!=='string'||/[\0\r\n]/.test(key+value)||value.startsWith('!'))throw Error('MCP values cannot execute commands or contain newlines');
 // Keep WinOne's existing default while preserving each explicit upstream
 // mode. MCP decides whether discovery activates Code Mode or tool search.
 const validated=validateMcpServerConfig(name,{...config,exposure:config.exposure??'direct'});
 if(typeof validated==='string')throw Error(validated);
 return {name,config:validated,source,scope:'global'};
}

// Import the public Codex package surfaces and Pi extension packages from
// explicitly selected local directories. No project file can add a plugin.
export async function discoverResources(config={}){
 // Match Cindy's explicit project discovery without granting project settings
 // trust. Only Skill metadata is imported here; project extensions stay inert.
 const projectSkills=config.cwd?collectPiProjectResourceCliPaths(config.cwd).skills:[];
 const result={skillPaths:[...projectSkills,...(config.skillDirectories??[])],extensionPaths:[],promptPaths:[],plugins:[],mcpServers:[],diagnostics:[]};
 for(const configured of config.mcpServers??[]){const {name,...entry}=configured;result.mcpServers.push(serverEntry(name,entry,'WinOne settings'));}
 for(const directory of config.pluginDirectories??[]){
  const root=await fs.realpath(directory);
  let manifest=await readJSON(path.join(root,'.codex-plugin','plugin.json'),true);
  const pkg=await readJSON(path.join(root,'package.json'),true);
  const pi=pkg?.pi;
  manifest??=await readJSON(path.join(root,'.claude-plugin','plugin.json'),true);
  const name=manifest?.name??pkg?.name??path.basename(root);
  const paths=async(value,target)=>{for(const item of Array.isArray(value)?value:value?[value]:[])result[target].push(await packagePath(root,item));};
  await paths(manifest?.skills??pi?.skills??(await exists(path.join(root,'skills'))?'./skills':undefined),'skillPaths');
  await paths(pi?.extensions,'extensionPaths');
  await paths(pi?.prompts??manifest?.commands,'promptPaths');
  if(!manifest&&!pi){
   const entry=(await Promise.all(['index.ts','index.js','index.mjs'].map(async file=>await exists(path.join(root,file))?file:null))).find(Boolean);
   if(entry)result.extensionPaths.push(await packagePath(root,entry));
   else if(!await exists(path.join(root,'skills')))throw Error('No plugin manifest, skills or extension entry in '+root);
  }
  const mcp=manifest?.mcpServers??(await exists(path.join(root,'.mcp.json'))?'./.mcp.json':undefined);
  const documents=Array.isArray(mcp)?mcp:mcp?[mcp]:[];
  for(const document of documents){
   const data=typeof document==='string'?await readJSON(await packagePath(root,document)):document;
   const entries=data?.mcpServers??data;
   for(const [server,entry] of Object.entries(entries??{}))result.mcpServers.push(serverEntry(server,entry,root));
  }
  result.plugins.push({name,path:root,enabled:true});
 }
 const names=new Set();for(const entry of result.mcpServers){const name=entry.name.replaceAll('-','_');if(names.has(name))throw Error('MCP server name is duplicated: '+entry.name);names.add(name);}
 return result;
}

// Resource browsing remains useful without a configured model. This path only
// reads manifests/AGENTS/skills: it never starts MCP servers or loads extension
// code just because the user opened the resource sidebar.
export async function inspectResources(config){
 if(!path.isAbsolute(config.cwd)||!path.isAbsolute(config.agentDir))throw Error('Invalid resource location');
 const discovered=await discoverResources(config);
 const loader=new DefaultResourceLoader({cwd:config.cwd,agentDir:config.agentDir,settingsManager:SettingsManager.inMemory(),noExtensions:true,noPromptTemplates:true,noThemes:true,additionalSkillPaths:discovered.skillPaths});
 await loader.reload();return resourceSummary(loader,discovered);
}

export function resourceSummary(loader,discovered){
 return {
  agents:loader.getAgentsFiles().agentsFiles.map(({path})=>({path})),
  skills:loader.getSkills().skills.map(skill=>({name:skill.name,description:skill.description,path:skill.filePath})),
  plugins:discovered.plugins,
  mcpServers:discovered.mcpServers.map(entry=>({name:entry.name,enabled:entry.config.enabled!==false,status:entry.config.enabled===false?'disabled':'configured'})),
  diagnostics:[...discovered.diagnostics,...loader.getSkills().diagnostics.map(item=>item.path?`${item.path}: ${item.message}`:item.message),...loader.getExtensions().errors.map(item=>`${item.path}: ${item.error}`)],
 };
}
