import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import {createOneBotRuntime} from './runtime.mjs';

const base=process.env.WINONE_ONEBOT_TEST_OUTPUT;
if(!base||!path.isAbsolute(base))throw Error('Run via build/test-onebot.mjs');
const sessionId='VOICEAAAAAAAAAAAAAAAAAAAAA';
test('dictation submits real WAV bytes with the OpenAI language code and restores the previous mute state',async()=>{
 const directory=await fs.mkdtemp(path.join(base,'dictation-')), events=[],requests=[];
 let muted=false;
 const server=http.createServer(async(req,res)=>{const chunks=[];for await(const part of req)chunks.push(part);const form=await new Response(Buffer.concat(chunks),{headers:{'Content-Type':req.headers['content-type']}}).formData();requests.push({model:form.get('model'),language:form.get('language'),bytes:Buffer.from(await form.get('file').arrayBuffer())});res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({text:'语音输入已接通'}));});
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 const runtime=createOneBotRuntime({directory,event:event=>events.push(event),host:{voiceResources:async()=>({providers:[{id:'asr',name:'ASR',baseUrl:'http://127.0.0.1:'+server.address().port+'/v1',key:'fixture-key',models:['whisper-1']}]}),voiceSystemAudio:async input=>{if(input.muted!==undefined)muted=input.muted;return {muted};}}});
 try{
  await runtime.call('voice.input.settings',{patch:{language:'zh-CN',refinementEnabled:false}});
  await runtime.call('voice.input.mute');assert.equal(muted,true);
  const started=await runtime.call('voice.input.start',{});
  const samples=Buffer.alloc(6400);for(let i=0;i<3200;i++)samples.writeInt16LE(Math.round(Math.sin(i/12)*12000),i*2);
  await runtime.call('voice.input.audio',{runId:started.runId,pcm:samples.toString('base64')});
  await runtime.call('voice.input.stop',{runId:started.runId});
  assert.equal(requests.length,1);assert.equal(requests[0].model,'whisper-1');assert.equal(requests[0].language,'zh');
  assert.equal(requests[0].bytes.subarray(0,4).toString(),'RIFF');
  assert.ok(events.some(event=>event.type==='voice-input'&&event.payload.type==='submitted'&&event.payload.text==='语音输入已接通'));
  await runtime.call('voice.input.restore');assert.equal(muted,false);
  muted=true;await runtime.call('voice.input.mute');await runtime.close();assert.equal(muted,true);
 }finally{await runtime.close();server.closeAllConnections();await new Promise(resolve=>server.close(resolve));}
});
test('voice preview uses the selected local AI route and keeps credentials out of the renderer',async()=>{
 const directory=await fs.mkdtemp(path.join(base,'voice-')), requests=[];
 const pcm=Buffer.from([0,0,1,0,2,0,3,0]);
 const server=http.createServer(async(req,res)=>{let body='';for await(const chunk of req)body+=chunk;requests.push({url:req.url,key:req.headers.authorization,body:JSON.parse(body)});res.writeHead(200,{'Content-Type':'audio/pcm'});res.end(pcm);});
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 const resources={providers:[{id:'voice-service',name:'Speech service',baseUrl:'http://127.0.0.1:'+server.address().port+'/v1',key:'fixture-private-key',models:[{id:'tts',name:'Speech',mode:'audio_speech',nativeApi:'openai-audio-speech'}]}]};
 const host={voiceResources:async()=>resources};
 let runtime=createOneBotRuntime({directory,host});
 try{
  await runtime.call('profiles.create',{input:{id:'speaker',name:'Speaker'}});
  await runtime.call('sessions.bind',{botId:'speaker',sessionId,routeKey:'canonical',role:'canonical'});
  const models=await runtime.call('voice.models');
  assert.equal(models.length,1);assert.equal(models[0].providerId,'voice-service');
  assert.ok(!JSON.stringify(models).includes('fixture-private-key'));
  const selection={providerId:'voice-service',agent:'pi',modelId:'tts',protocol:'openai',voice:'alloy',speed:1};
  await runtime.call('voice.setPreference',{botId:'speaker',selection,sentencePlayback:false});
  const result=await runtime.call('voice.preview',{input:{...selection,previewId:'sample',botId:'speaker',sessionId,text:'本机试听'}});
  assert.equal(result.sampleRate,24000);
  const audio=await runtime.call('voice.readPreview',{previewId:'sample'});
  assert.deepEqual(Buffer.from(audio.pcm,'base64'),pcm);
  assert.equal(requests[0].url,'/v1/audio/speech');
  assert.equal(requests[0].key,'Bearer fixture-private-key');
  assert.equal(requests[0].body.model,'tts');assert.equal(requests[0].body.input,'本机试听');
  await runtime.call('voice.stopPreview',{previewId:'sample'});
  await assert.rejects(runtime.call('voice.preview',{input:{...selection,previewId:'other',botId:'speaker',sessionId:'OTHERAAAAAAAAAAAAAAAAAAAAA',text:'reject'}}),/主对话/);
  assert.equal(requests.length,1);
  await runtime.close();runtime=createOneBotRuntime({directory,host});
  const saved=await runtime.call('voice.preference',{botId:'speaker'});
  assert.equal(saved.selection.modelId,'tts');assert.equal(saved.sentencePlayback,false);
 }finally{await runtime.close();await new Promise(resolve=>server.close(resolve));}
});
