// Owned extension-side wire client. Cryptographic proofs and every HTTP method
// follow the pinned bridge contract; this is not a replacement bridge/policy.
import {generateKeyPairSync, sign, createHash, randomUUID} from 'node:crypto';

export class LocalBrowserClient {
  constructor(port, extensionId = 'a'.repeat(32)) {
    this.baseUrl = 'http://127.0.0.1:' + port; this.extensionId = extensionId; this.id = randomUUID();
    const keys = generateKeyPairSync('ec', {namedCurve: 'prime256v1'}); this.privateKey = keys.privateKey;
    this.publicKey = keys.publicKey.export({format: 'jwk'});
    this.publicKeyText = JSON.stringify({kty: 'EC', crv: 'P-256', x: this.publicKey.x, y: this.publicKey.y, ext: true});
    this.encodedKey = Buffer.from(this.publicKeyText).toString('base64url');
    this.headers = {'x-my-browser-extension': extensionId, 'x-my-browser-origin': 'chrome-extension://' + extensionId,
      'x-my-browser-client': this.id, 'x-my-browser-version': '0.4.13', 'x-my-browser-family': 'chrome', 'x-my-browser-key': this.encodedKey};
  }
  proof(value) {return sign('sha256', Buffer.from(value), {key: this.privateKey, dsaEncoding: 'ieee-p1363'}).toString('base64url');}
  async request(path, body, headers = {}) {
    const response = await fetch(this.baseUrl + path, {method: body === undefined ? 'GET' : 'POST', headers: {...this.headers, ...headers,
      ...(this.session ? {'x-my-browser-session': this.session} : {}), ...(body === undefined ? {} : {'Content-Type': 'application/json'})},
      ...(body === undefined ? {} : {body: typeof body === 'string' ? body : JSON.stringify(body)})});
    return {status: response.status, value: await response.json()};
  }
  async connect(nativeToken) {
    let response = await this.request('/health');
    response = await this.request('/health', undefined, {'x-my-browser-proof': this.proof(response.value.challenge)});
    if (nativeToken && response.value.native_binding_required) {
      const bind = await fetch(this.baseUrl + '/native/bind', {method: 'POST', headers: {Authorization: 'Bearer ' + nativeToken, 'Content-Type': 'application/json'},
        body: JSON.stringify({clientId: this.id, origin: 'chrome-extension://' + this.extensionId, publicKey: this.encodedKey})});
      if (!bind.ok) throw Error('Original private native binding rejected the fixture');
      response = await this.request('/health', undefined, {'x-my-browser-proof': this.proof(response.value.challenge)});
    }
    this.session = response.value.session; return response;
  }
  async learning(path, body) {
    const challenge = await this.request('/learning/challenge');
    const raw = JSON.stringify(body), digest = createHash('sha256').update(raw).digest('base64url');
    const payload = challenge.value.challenge + '\\nPOST\\n' + path + '\\n' + digest;
    return this.request(path, raw, {'x-my-browser-learning-challenge': challenge.value.challenge, 'x-my-browser-proof': this.proof(payload)});
  }
  trusted() {return {id: this.id, origin: 'chrome-extension://' + this.extensionId, extensionId: this.extensionId, publicKey: this.publicKeyText};}
}

// OS registration is the one test seam: the real native binding writes owned
// manifests/configs/binary copies, while every Registry command is captured and
// applied only to this in-memory fixture. Never execute reg.exe/PowerShell.
export function isolatedRegistry() {
  const values = new Map(), commands = [];
  return {values, commands, async run(file, args) {
    commands.push({file, args});
    if (/powershell\.exe$/i.test(file)) return {stdout: JSON.stringify([...new Set(values.values())])};
    if (!/reg\.exe$/i.test(file) || !['ADD', 'DELETE'].includes(args[0])) throw Error('Unknown fixture OS command');
    if (args[0] === 'ADD') values.set(args[1], args[args.indexOf('/d') + 1]);
    else values.delete(args[1]);
    return {stdout: ''};
  }};
}
