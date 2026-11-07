// One-shot-lifetime TeX renderer for native Chat: JSON lines in, vector formulas
// out. It runs apart from the window, so a malformed formula or a slow layout
// never touches input handling, and it exits when its parent closes stdin.
import {convert} from './convert.mjs';

const MAX_LINE = 256 << 10;
let buffer = Buffer.alloc(0);

function answer(line) {
  let request;
  try { request = JSON.parse(line); } catch { process.exit(1); }
  let value;
  try { value = {id: request.id, ok: true, ...convert(request.tex, request.display === true)}; }
  catch (error) { value = {id: request.id, ok: false, error: String(error.message ?? error).slice(0, 512)}; }
  process.stdout.write(JSON.stringify(value) + '\n');
}

process.stdin.on('data', chunk => {
  buffer = Buffer.concat([buffer, chunk]);
  for (;;) {
    const newline = buffer.indexOf(10);
    if (newline < 0) break;
    const line = buffer.subarray(0, newline).toString('utf8');
    buffer = buffer.subarray(newline + 1);
    answer(line);
  }
  if (buffer.length > MAX_LINE) process.exit(1);
});
process.stdin.on('end', () => process.exit(0));
process.stdin.on('error', () => process.exit(1));
