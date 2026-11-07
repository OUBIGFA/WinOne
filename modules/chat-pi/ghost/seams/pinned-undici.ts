// Cindy's generated SSRF policy owns DNS checks and pinned lookup. WinOne
// supplies ordinary Undici constructors: no OpenClaw managed-proxy process or
// global dispatcher is installed. All caller-supplied connection options remain.
import {Agent, EnvHttpProxyAgent, ProxyAgent} from 'undici';
export function createHttp1Agent(options = {}, timeoutMs?: number) {
 return new Agent({...options, allowH2: false, ...(timeoutMs ? {bodyTimeout: timeoutMs, headersTimeout: timeoutMs} : {})});
}
export function createHttp1EnvHttpProxyAgent(options = {}, timeoutMs?: number) {
 return new EnvHttpProxyAgent({...options, allowH2: false, ...(timeoutMs ? {bodyTimeout: timeoutMs, headersTimeout: timeoutMs} : {})});
}
export function createHttp1ProxyAgent(options: ConstructorParameters<typeof ProxyAgent>[0], timeoutMs?: number) {
 const settings=typeof options === 'string' ? {uri: options} : options;
 return new ProxyAgent({...settings, allowH2: false, ...(timeoutMs ? {bodyTimeout: timeoutMs, headersTimeout: timeoutMs} : {})});
}
