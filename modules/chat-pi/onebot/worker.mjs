// Private JSON-lines worker used by WinOne's existing chat.Runner. No listeners,
// real-profile defaults, account discovery or IM side effects on startup.
import {once} from 'node:events';
import {createOneBotRuntime} from './runtime.mjs';
import {setOneBotDiagnosticSink} from './logger.ts';
import {createOneBotIMService} from './im-service.mjs';
import {createOneBotIMDispatcher} from './im-dispatch.mjs';
import {createOneBotLifecycleClient} from './lifecycle-client.mjs';
import {createOneBotTaskClient} from './task-client.mjs';
import {createOneBotHostClient} from './host-client.mjs';
import {createScheduleDispatcher} from '../scheduler-dispatch.mjs';

const MAX_LINE = 1024 * 1024;
let runtime, im, tasks, initializing = false, closing = false, writes = Promise.resolve(), queuedBytes = 0;
const requests = new Map();
function emit(value) {
  const bytes = Buffer.from(JSON.stringify(value) + '\n');
  if (bytes.length > MAX_LINE || queuedBytes + bytes.length > 4 * MAX_LINE) return Promise.reject(Error('OneBot protocol output exceeded'));
  queuedBytes += bytes.length;
  const output = writes.then(async () => { if (!process.stdout.write(bytes)) await once(process.stdout, 'drain'); });
  writes = output.finally(() => { queuedBytes -= bytes.length; });
  return writes;
}
function fail(error) {
  const message = String(error?.message ?? error);
  const code = error?.errorCode ?? error?.code ?? /^\[([A-Z_]+)\]/.exec(message)?.[1] ?? 'INTERNAL';
  // chat.Runner preserves the error text; use Cindy's existing IPC code prefix
  // too, so editor conflict handling survives the Go bridge.
  return {error: message.startsWith(`[${code}]`) ? message : `[${code}] ${message}`, errorCode: code};
}
async function shutdown(error) {
  if (closing) return;
  closing = true;
  try { await im?.close(); await tasks?.close(); await Promise.allSettled([...requests.values()]); await runtime?.close(); await writes; }
  catch (failure) { error ??= failure; }
  if (error) console.error('OneBot shutdown:', String(error.message ?? error));
  process.exit(error ? 1 : 0);
}
function event(value) { void emit({type: 'onebot_event', event: value}).catch(shutdown); }
setOneBotDiagnosticSink(diagnostic => event({type: 'diagnostic', ...diagnostic}));
async function handle(request) {
  if (request.method === 'init') {
    if (runtime || initializing) throw Object.assign(Error('OneBot 已初始化'), {code: 'ALREADY_INITIALIZED'});
    initializing = true;
    try {
      const {directory,masterKey,dispatchBaseUrl,dispatchKey,language}=request.params??{};
      const lifecycle=dispatchBaseUrl&&dispatchKey?createOneBotLifecycleClient({baseUrl:dispatchBaseUrl,key:dispatchKey,im:()=>im}):undefined;
      tasks=dispatchBaseUrl&&dispatchKey?createOneBotTaskClient({baseUrl:dispatchBaseUrl,key:dispatchKey,language,event,settle:params=>runtime.call('tasks.settle',params),interaction:params=>runtime.call('tasks.interaction',params)}):undefined;
      const host=dispatchBaseUrl&&dispatchKey?createOneBotHostClient({baseUrl:dispatchBaseUrl,key:dispatchKey}):undefined;
      const routineDispatch=host?createScheduleDispatcher({baseUrl:dispatchBaseUrl,key:dispatchKey,
        authorize:(schedule,ctx,prompt)=>host.authorizeRoutine({sessionId:schedule.targetSessionId,runId:ctx.runId,prompt,scheduleId:schedule.id},ctx.signal)}):undefined;
      const dispatch=dispatchBaseUrl&&dispatchKey?createOneBotIMDispatcher({baseUrl:dispatchBaseUrl,key:dispatchKey}):undefined;
      runtime = createOneBotRuntime({directory,event,lifecycle,tasks,host,routineDispatch,masterKey,imDispatcher:dispatch,language:language==='en-US'?'en':'zh-CN'});
      // Content-only callers do not need a network bridge. Partial IM init is
      // rejected; no fallback key or real-profile discovery is permitted.
      if(masterKey||dispatchBaseUrl||dispatchKey) {
        im=createOneBotIMService({directory,masterKey,readBot:botId=>runtime.call('profiles.get',{botId}),dispatch,
          lanes:(method,params)=>runtime.call('imLanes.'+method,params),event});
        runtime.attachIMService(im);
      }
      // The init response must release Host.ensure before a recovered rule can
      // call back for a profile or start its canonical Pi conversation.
      setTimeout(()=>{void runtime?.startBackground().catch(error=>event({type:'diagnostic',level:'warn',scope:'routines',message:String(error.message??error)}));},0);
      return {ready: true};
    } catch(error) { await im?.close(); im=undefined; await tasks?.close(); tasks=undefined; await runtime?.close(); runtime=undefined; throw error; }
    finally { initializing = false; }
  }
  if (!runtime) throw Object.assign(Error('OneBot 尚未初始化'), {code: 'NOT_READY'});
  if(request.method.startsWith('imConnection.')) {
    const methods={list:'list',add:'add',remove:'remove',listGroups:'groups.list',setGroupActivation:'groups.setActivation',
      listLanes:'lanes.list',resetLane:'lanes.reset',archiveLane:'lanes.archive',listAdoptable:'listAdoptable',adopt:'adopt'};
    const method=methods[request.method.slice('imConnection.'.length)];
    if(!method)throw Object.assign(Error('不支持的 IM 连接操作'),{code:'UNKNOWN_METHOD'});
    if(!im)throw Object.assign(Error('OneBot IM 私有桥尚未初始化'),{code:'NOT_READY'});
    return im.call(method,request.params);
  }
  if(request.method.startsWith('im.')) {
    if(!im)throw Object.assign(Error('OneBot IM 私有桥尚未初始化'),{code:'NOT_READY'});
    return im.call(request.method.slice(3),request.params);
  }
  const result=await runtime.call(request.method, request.params);
  if(['profiles.create','profiles.list','lifecycle.action'].includes(request.method))void runtime.startBackground().catch(error=>event({type:'diagnostic',level:'warn',scope:'routines',message:String(error.message??error)}));
  return result;
}
function accept(line) {
  let request;
  try { request = JSON.parse(line); }
  catch (error) { void shutdown(error); return; }
  if (!request || typeof request.id !== 'string' || !request.id || request.id.length > 128 || typeof request.method !== 'string') {
    void shutdown(Error('Invalid OneBot request')); return;
  }
  if (requests.has(request.id) || requests.size >= 128) { void shutdown(Error('OneBot request queue exceeded or duplicate id')); return; }
  const work = handle(request).then(value => emit({type: 'response', id: request.id, ok: true, value: value ?? null}),
    error => emit({type: 'response', id: request.id, ok: false, ...fail(error)}));
  requests.set(request.id, work);
  work.then(() => requests.delete(request.id), error => { requests.delete(request.id); void shutdown(error); });
}
let buffer = Buffer.alloc(0);
process.stdin.on('data', chunk => {
  if (closing) return;
  buffer = Buffer.concat([buffer, chunk]);
  while (!closing) {
    const newline = buffer.indexOf(10);
    if (newline < 0) break;
    const line = buffer.subarray(0, newline); buffer = buffer.subarray(newline + 1);
    if (line.length > MAX_LINE) { void shutdown(Error('OneBot protocol input exceeded')); return; }
    accept(line.toString('utf8'));
  }
  if (buffer.length > MAX_LINE) void shutdown(Error('OneBot protocol input exceeded'));
});
process.stdin.on('end', () => { void shutdown(buffer.length ? Error('Incomplete OneBot request') : undefined); });
process.stdin.on('error', shutdown);
process.stdout.on('error', shutdown);
void emit({type: 'ready', protocol: 1}).catch(shutdown);
