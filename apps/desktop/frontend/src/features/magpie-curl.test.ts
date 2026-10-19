import {expect,it} from 'vitest';
import {curlOf} from './magpie-curl.js';
const examples=import.meta.glob<string>('../../tests/fixtures/curl/*',{query:'?raw',import:'default',eager:true});

it.each([
 ['openai-chat.sh','openai','https://api.openai.com/v1',''],
 ['openai-responses.sh','responses','https://api.openai.com/v1','gpt-6-astra'],
 ['anthropic.sh','anthropic','https://api.anthropic.com','claude-opus-5-5'],
 ['deepseek.sh','openai','https://api.deepseek.com/v1','deepseek-flash'],
 ['zhipu.sh','openai','https://open.bigmodel.cn/api/paas/v4','glm-5.3'],
 ['moonshot.sh','openai','https://api.moonshot.cn/v1','kimi-k3'],
 ['openrouter.sh','openai','https://openrouter.ai/api/v1','~openai/gpt-sol-latest'],
 ['bailian-cmd.bat','openai','https://{WorkspaceId}.cn-beijing.maas.aliyuncs.com/compatible-mode/v1','qwen3.8-max'],
 ['sap-ai-core.ps1','','','meta--llama3.1-70b-instruct'],
 ['unsloth-studio.ps1','openai','http://127.0.0.1:8888/v1',''],
])('reads the upstream vendor example %s with LF and CRLF', (file,api,base,model)=>{
 const text=examples['../../tests/fixtures/curl/'+file];
 for(const value of [text.replace(/\r\n/g,'\n'),text.replace(/\r?\n/g,'\r\n')]){
  expect(curlOf(value)).toMatchObject({api,base,model,key:''});
 }
});
it('reads a real key while leaving ordinary paste alone',()=>{
 expect(curlOf('https://example.invalid/v1')).toBeNull();
 expect(curlOf("curl https://example.invalid/v1/messages -H 'x-api-key: fixture-token-123'" )).toMatchObject({api:'anthropic',base:'https://example.invalid',key:'fixture-token-123'});
});
