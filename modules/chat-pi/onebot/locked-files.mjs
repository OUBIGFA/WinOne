// Bun on Windows opens readFile handles without delete sharing. Keep reads
// and atomic replacement of the same learning file apart; no retry hides an
// I/O error, and the original state transaction remains responsible for CAS.
import fs from 'node:fs/promises';
import path from 'node:path';
import {withBotProfileLocks} from '../cindy/bots/botProfileLock.ts';
const lock = (files, work) => withBotProfileLocks(files.map(file => {
  const absolute = path.resolve(file);
  return 'learning-file:' + (process.platform === 'win32' ? absolute.toLowerCase() : absolute);
}), work);
export default {...fs,
  readFile: (file, ...options) => lock([file], () => fs.readFile(file, ...options)),
  writeFile: (file, ...options) => lock([file], () => fs.writeFile(file, ...options)),
  rename: (from, to) => lock([from, to], () => fs.rename(from, to)),
  rm: (file, ...options) => lock([file], () => fs.rm(file, ...options)),
};
