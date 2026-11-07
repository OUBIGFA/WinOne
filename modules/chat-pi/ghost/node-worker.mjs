// Entry of a plugin's Node worker. Cindy's bootstrap expects Electron's parentPort;
// here the same frames travel over the child's IPC channel to the plugin host.
import {GHOST_NODE_CHILD_MODE_FLAG} from '../cindy/ghost/apps/desktop/src/shared/ghost.ts';
const childMode = process.argv[3] === GHOST_NODE_CHILD_MODE_FLAG;
const listeners = new Set();
process.on('message', data => { for (const listener of listeners) listener({data}); });
process.parentPort = {
  postMessage(message) { process.send(message); },
  on(event, listener) { if (event === 'message') listeners.add(listener); return this; },
  off(event, listener) { if (event === 'message') listeners.delete(listener); return this; },
  once(event, listener) { const once = value => { listeners.delete(once); listener(value); }; if (event === 'message') listeners.add(once); return this; },
  removeListener(event, listener) { return this.off(event, listener); },
  start() {},
};
await import('../cindy/ghost/apps/desktop/src/main/cindy-brain/nodeRuntimeWorkerProcess.ts');
if (childMode) {
  // Electron's child mode behaves like a plain CLI. Node/Bun's IPC listener
  // otherwise keeps even a finished script alive forever. A consuming virtual
  // stdin still needs the channel until EOF, just like a real stdin pipe.
  const stdin = process.stdin;
  const updateRef = () => {
    const reading = !stdin.readableEnded && (stdin.listenerCount('data') > 0 || stdin.listenerCount('readable') > 0);
    if (reading) process.channel?.ref(); else process.channel?.unref();
  };
  stdin.on('newListener', () => queueMicrotask(updateRef));
  stdin.on('removeListener', updateRef);
  stdin.on('end', updateRef);
  updateRef();
}
