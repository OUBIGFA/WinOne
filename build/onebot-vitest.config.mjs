import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {createRequire} from 'node:module';

const root=fileURLToPath(new URL('../',import.meta.url));
// Reuse WinOne's locked test toolchain; do not install a second Vitest tree.
const require=createRequire(new URL('../apps/desktop/frontend/package.json',import.meta.url));
export default {
 root,
 resolve: {alias: {vitest: path.join(path.dirname(require.resolve('vitest/package.json')),'dist/index.js')}},
 test: {
  environment: 'node',
  include: ['modules/chat-pi/cindy/autonomy/__tests__/*.test.ts','modules/chat-pi/cindy/bots/**/__tests__/*.test.ts','modules/chat-pi/cindy/im/**/__tests__/*.test.ts','modules/chat-pi/cindy/wechat-ilink/**/__tests__/*.test.ts','modules/chat-pi/onebot/**/*.test.ts'],
  maxWorkers: 1,
  testTimeout: 15000,
  hookTimeout: 15000,
  reporters: process.env.WINONE_ONEBOT_TEST_OUTPUT?['default','json']:['default'],
  outputFile: process.env.WINONE_ONEBOT_TEST_OUTPUT?path.join(process.env.WINONE_ONEBOT_TEST_OUTPUT,'result.json'):undefined,
 },
};
