// Child fixture owned by the native private-desktop test. Prints only its
// loopback endpoint; no public discovery service or user profile is involved.
const path = require('node:path');
const Tracker = require(path.resolve(__dirname, '../../modules/flashchat/node_modules/bittorrent-tracker/server'));
const tracker = new Tracker({udp:false,http:false,ws:true,stats:false,interval:5000});
tracker.on('error', error => { console.error(error.message); process.exit(1); });
tracker.listen(0, '127.0.0.1', () => console.log('ws://127.0.0.1:' + tracker.http.address().port));
