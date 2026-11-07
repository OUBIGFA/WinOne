// Transport shim only: the actual mutation and all no-follow/identity checks
// stay in the pinned Cindy worker. Initialize the shim before its module loads.
process.parentPort = {
  postMessage: message => process.send?.(message),
  on: (event, callback) => {
    if (event !== 'message') throw Error('Unsupported snapshot worker event');
    process.on('message', data => callback({data}));
  },
};
await import('../cindy/ghost/apps/desktop/src/main/cindy-brain/ghostSnapshotWorkerProcess.ts');
