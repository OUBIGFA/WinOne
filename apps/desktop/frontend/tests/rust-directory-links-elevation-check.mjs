// Exercise the shipped standard Desktop → Host → fixed elevated helper chain.
// The driver never accepts or dismisses Windows consent and never writes the
// Explorer menu. Its sources and link entries all belong to this output dir.
import {createRequire} from 'node:module';
import {writeFile,mkdir,lstat,readFile,readlink} from 'node:fs/promises';
import {resolve} from 'node:path';
const {chromium}=createRequire(import.meta.url)(process.env.WINONE_PLAYWRIGHT_MODULE);
const [endpoint,output]=process.argv.slice(2);
const wait=ms=>new Promise(resolve=>setTimeout(resolve,ms));
const stage=value=>writeFile(resolve(output,'stage.txt'),value);
let browser,page;
try{
 const deadline=Date.now()+30000;
 while(Date.now()<deadline){try{browser=await chromium.connectOverCDP(endpoint,{timeout:1000});break;}catch{await wait(100);}}
 if(!browser)throw Error('Isolated WebView2 did not start');
 while(Date.now()<deadline){page=browser.contexts().flatMap(context=>context.pages()).find(page=>page.url().includes('winone.localhost'));if(page)break;await wait(100);}
 if(!page)throw Error('Current UI document did not load');
 await page.getByRole('navigation').waitFor();
 const snapshot=await page.evaluate(()=>window.go.main.App.Snapshot());
 if(snapshot.administrator!==false)throw Error('This test must begin with the standard Host');
 const call=(method,params={})=>page.evaluate(async({method,params})=>JSON.parse(await window.go.main.App.DirectoryLinks(method,JSON.stringify(params))),{method,params});
 const fixture=resolve(output,'fixtures');await mkdir(fixture,{recursive:true});
 const source=resolve(fixture,'source 中文.txt');const link=resolve(fixture,'elevated link.txt');
 const contents='Kept source for an explicitly elevated link.\n';await writeFile(source,contents);
 const request={source,directory:fixture,linkName:'elevated link.txt',name:'isolated elevation',kind:'symlink',relative:true};
 const review=await call('preview',request);
 let confirmationError='';try{await call('create.elevated',{token:review.id,confirmed:false});}catch(error){confirmationError=String(error);}
 if(!confirmationError)throw Error('Unconfirmed elevation was accepted');
 try{await lstat(link);throw Error('Unconfirmed request created an entry');}catch(error){if(error.code!=='ENOENT')throw error;}
 let replayError='';try{await call('create.elevated',{token:review.id,confirmed:true});}catch(error){replayError=String(error);}
 if(!replayError)throw Error('A consumed preview was accepted');
 const first=await call('preview',request);
 await stage('Awaiting first Windows consent. Choose No to verify cancellation.');
 let cancelled='';let created;
 try{created=await call('create.elevated',{token:first.id,confirmed:true});}catch(error){cancelled=String(error);}
 if(created)throw Error('The cancellation scenario was approved. Link exists; rerun with a fresh fixture and decline the first Windows prompt.');
 if(!cancelled.includes('取消管理员授权'))throw Error('Expected a real Windows consent cancellation: '+cancelled);
 try{await lstat(link);throw Error('Cancelled consent created an entry');}catch(error){if(error.code!=='ENOENT')throw error;}
 const cancelledRows=await call('list');
 if(!cancelledRows.some(row=>row.linkPath===link&&row.status==='failed'))throw Error('Cancellation was not recorded as an unsuccessful attempt');
 const retry=await call('preview',request);
 if(retry.id===first.id)throw Error('Retry did not receive a fresh preview');
 await stage('Cancellation verified. Awaiting second Windows consent; approve this one to create the isolated symbolic link.');
 created=await call('create.elevated',{token:retry.id,confirmed:true});
 if(created.status!=='active'||created.kind!=='symlink'||!created.accessible||!created.entryIdentity.file||!created.observedTarget.file)throw Error('Parent did not verify the created link: '+JSON.stringify(created));
 if(!(await lstat(link)).isSymbolicLink()||await readFile(link,'utf8')!==contents)throw Error('Returned success disagrees with the actual link');
 const targetText=await readlink(link);
 const inspected=await call('inspect',{id:created.id});
 if(inspected.status!=='active'||inspected.entryIdentity.file!==created.entryIdentity.file)throw Error('A later inspection did not retain verified link identity');
 let reused='';try{await call('create.elevated',{token:retry.id,confirmed:true});}catch(error){reused=String(error);}
 if(!reused)throw Error('A completed elevated preview was reused');
 // Removal takes the latest inspection token and only recycles this fixture
 // entry. Verifying source contents catches accidental target deletion.
 const removed=await call('remove',{id:created.id,confirmed:true,checkedAt:inspected.checkedAt});
 if(removed.status!=='removed'||await readFile(source,'utf8')!==contents)throw Error('Removing the link did not preserve the source');
 try{await lstat(link);throw Error('Recycled link remains on disk');}catch(error){if(error.code!=='ENOENT')throw error;}
 const after=await page.evaluate(()=>window.go.main.App.Snapshot());
 if(after.administrator!==false||after.settings.startup?.preferAdministrator!==snapshot.settings.startup?.preferAdministrator)throw Error('One-time elevation changed the application permission preference');
 await writeFile(resolve(output,'result.json'),JSON.stringify({standardHost:true,confirmationError,replayError,cancelled,retryAccepted:true,created,inspected,removed,targetText,sourcePreserved:true,administratorPreferenceUnchanged:true},null,2));
 await stage('PASS: cancellation, retry, elevated creation, parent verification and source-preserving recycle.');
 await page.evaluate(async()=>{const api=window.go.main.App;const snapshot=await api.Snapshot();await api.SaveSettings({...snapshot.settings,startup:{...snapshot.settings.startup,keepUIRunning:false}});void api.WindowCommand('close');}).catch(error=>{if(!String(error).includes('closed'))throw error;});
}catch(error){
 await writeFile(resolve(output,'node-error.txt'),String(error.stack||error));
 await stage('FAILED: '+String(error));
 if(page)await page.screenshot({path:resolve(output,'failure.png')}).catch(()=>{});
 process.exitCode=1;
}finally{if(browser)await browser.close().catch(()=>{});}
