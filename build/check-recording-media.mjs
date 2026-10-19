// Decode the actual outputs with Chromium, checking orientation, dimensions
// and toolbar exclusion. Only explicitly selected repository test files are served.
import {createRequire} from 'node:module';
import {readFile,mkdir,writeFile} from 'node:fs/promises';
import {createServer} from 'node:http';
import {resolve,join} from 'node:path';
import {execFileSync} from 'node:child_process';
import assert from 'node:assert/strict';
const [synthetic,live,out,...options]=process.argv.slice(2),output=resolve(out||'_temp/recording-media-check');
if(!synthetic)throw Error('Pass synthetic and live recording fixture directories');
execFileSync('git',['check-ignore','--quiet','--no-index',join(output,'probe')]);await mkdir(output,{recursive:true});
const fixtures=['gif','mp4'].map(format=>({name:'synthetic.'+format,kind:'synthetic',format,file:resolve(synthetic,'synthetic.'+format)}));
if(live)for(const kind of ['live',...(options.includes('--panel')?['panel']:[])])for(const format of ['gif','mp4'])fixtures.push({name:kind+'.'+format,kind,format,file:resolve(live,kind+'.'+format)});
const files=new Map(await Promise.all(fixtures.map(async item=>['/'+item.name,{...item,bytes:await readFile(item.file)}])));
const server=createServer((req,res)=>{
 if(req.url==='/'){res.writeHead(200,{'Content-Type':'text/html; charset=utf-8'});res.end('<!doctype html><meta charset="utf-8"><title>WinOne recording verification</title>');return;}
 const file=files.get(req.url);
 if(!file){res.writeHead(404);res.end();return;}
 const type=file.format==='mp4'?'video/mp4':'image/gif';const total=file.bytes.length;
 const match=/^bytes=(\d+)-(\d*)$/.exec(req.headers.range||'');
 if(match){const start=+match[1],end=Math.min(match[2]?+match[2]:total-1,total-1);if(start>end){res.writeHead(416);res.end();return;}res.writeHead(206,{'Content-Type':type,'Accept-Ranges':'bytes','Content-Range':`bytes ${start}-${end}/${total}`,'Content-Length':end-start+1});res.end(file.bytes.subarray(start,end+1));}
 else{res.writeHead(200,{'Content-Type':type,'Content-Length':total,'Accept-Ranges':'bytes'});res.end(file.bytes);}
});
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE||resolve('dist/resources/tools/playwright-1.58.2/node_modules/playwright'));
const liveEvidence=live?JSON.parse(await readFile(resolve(live,'..','live-results.json'),'utf8')):[];
const panelEvidence=options.includes('--panel')?JSON.parse(await readFile(resolve(live,'..','panel-results.json'),'utf8')):[];
const results=[];let browser;
try{
 await new Promise((done,reject)=>{server.once('error',reject);server.listen(0,'127.0.0.1',done);});
 browser=await chromium.launch({headless:true,...(process.env.WINONE_CHROMIUM?{executablePath:process.env.WINONE_CHROMIUM}:{channel:'msedge'})});
 const page=await browser.newPage({viewport:{width:800,height:520}});
 for(const fixture of fixtures){
  await page.goto(`http://127.0.0.1:${server.address().port}/`);
  const result=await page.evaluate(async({name,format,url})=>{
   const canvas=document.createElement('canvas'),ctx=canvas.getContext('2d',{willReadFrequently:true});
   const samples=[];
   function sample(image,index,count,timestampMs,durationMs){
    ctx.drawImage(image,0,0);
    const {width,height}=canvas,pixels=ctx.getImageData(0,0,width,height).data;
    const pixel=(x,y)=>[...pixels.slice((y*width+x)*4,(y*width+x)*4+4)];
    // The generated live fixture is white along these edge spans. Inspect
    // enough pixels to detect either half of a dashed border entering the crop.
    let edgeMinimum=255;
    for(let x=185;x<width-12;x+=3)for(const y of [0,height-1])edgeMinimum=Math.min(edgeMinimum,...pixel(x,y).slice(0,3));
    for(let y=100;y<260;y+=3)for(const x of [0,width-1])edgeMinimum=Math.min(edgeMinimum,...pixel(x,y).slice(0,3));
    const png=index===0||index===Math.floor(count/2)||index===count-1?canvas.toDataURL('image/png').split(',')[1]:undefined;
    samples.push({index,timestampMs,durationMs,topLeft:pixel(20,20),bottomLeft:pixel(20,height-20),excludedArea:pixel(410,210),edgeMinimum,png});
   }
   let duration,frameCount;
   if(format==='gif'){
    // Decode/composite every GIF frame, rather than inspecting only the first
    // image loaded by <img>. This also checks the held final-frame duration.
    const decoder=new ImageDecoder({data:new Uint8Array(await(await fetch(url)).arrayBuffer()),type:'image/gif'});
    try{
     await decoder.completed;await decoder.tracks.ready;
     frameCount=decoder.tracks.selectedTrack.frameCount;
     for(let index=0;index<frameCount;index++){
      const {image}=await decoder.decode({frameIndex:index});
      try{canvas.width=image.displayWidth;canvas.height=image.displayHeight;sample(image,index,frameCount,image.timestamp/1000,image.duration/1000);}finally{image.close();}
     }
     const last=samples.at(-1);duration=(last.timestampMs+last.durationMs)/1000;
    }finally{decoder.close();}
   }else{
    const video=document.createElement('video');video.muted=true;video.preload='auto';document.body.append(video);
    function event(name,action){return new Promise((resolve,reject)=>{
     const fail=()=>finish(Error('Media decode failed: '+url)),done=()=>finish();
     const deadline=setTimeout(()=>finish(Error('Media decode timed out: '+url)),10000);
     function finish(error){clearTimeout(deadline);video.removeEventListener(name,done);video.removeEventListener('error',fail);error?reject(error):resolve();}
     video.addEventListener(name,done,{once:true});video.addEventListener('error',fail,{once:true});action();
    });}
    await event('loadeddata',()=>{video.src=url;});
    canvas.width=video.videoWidth;canvas.height=video.videoHeight;duration=video.duration;
    // Explicit seeks cover startup, changing content, resume and the static
    // tail without relying on playback callbacks that may drop test samples.
    const times=[0,.15,.3,.5,.7,.85,1].map(fraction=>Math.min(duration-.001,duration*fraction));
    for(const [index,time] of times.entries()){
     if(time>0)await event('seeked',()=>{video.currentTime=time;});
     sample(video,index,times.length,time*1000,null);
    }
   }
   return {name,width:canvas.width,height:canvas.height,duration,frameCount,samples};
  },{name:fixture.name,format:fixture.format,url:`http://127.0.0.1:${server.address().port}/${fixture.name}`});
  for(const sample of result.samples){
   if(sample.png)await writeFile(join(output,fixture.name+(sample.index===0?'':'.frame-'+sample.index)+'.png'),Buffer.from(sample.png,'base64'));
   delete sample.png;
  }
  results.push(result);
  assert.deepEqual([result.width,result.height],[640,360]);
  assert.ok(result.samples.length>1,fixture.name+' did not decode multiple frames');
  for(const sample of result.samples){
   const label=fixture.name+' sample '+sample.index;
   assert.ok(sample.topLeft[0]>180&&sample.topLeft[2]<60,label+' is upside down or has incorrect channels');
   assert.ok(sample.bottomLeft[2]>180&&sample.bottomLeft[0]<60,label+' lost the lower orientation marker');
   if(fixture.kind!=='synthetic'){
    assert.ok(sample.excludedArea[1]>170&&Math.abs(sample.excludedArea[0]-sample.excludedArea[1])<25&&Math.abs(sample.excludedArea[1]-sample.excludedArea[2])<25,label+' captured the excluded controls or a black rectangle');
    assert.ok(sample.edgeMinimum>235,label+' captured an outline or lost an edge of the target');
   }
  }
  if(fixture.kind==='synthetic')assert.ok(Math.abs(result.duration-6)<.025,fixture.name+' lost the static tail');
  else{
   const saved=(fixture.kind==='panel'?panelEvidence:liveEvidence).find(item=>item.format===fixture.format)?.saved;
   assert.ok(saved,fixture.name+' status evidence is missing');
   assert.ok(Math.abs(result.duration*1000-saved.durationMs)<40,fixture.name+' saved duration differs from the recording clock');
  }
 }
 console.log(JSON.stringify({decoded:results.length,output,results:results.map(({name,width,height,duration,frameCount,samples})=>({name,width,height,duration,frameCount,checkedSamples:samples.length}))}));
}finally{
 // Missing/unsupported browsers and assertion failures must not leave a local
 // server holding the test process open or orphan a browser after a full disk.
 try{await writeFile(join(output,'report.json'),JSON.stringify(results,null,2));}
 finally{
  try{if(browser)await browser.close();}
  finally{server.closeAllConnections();if(server.listening)await new Promise(done=>server.close(done));}
 }
}
