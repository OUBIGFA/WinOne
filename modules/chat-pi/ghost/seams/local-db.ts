// Only the plugin owner-local ledger is exposed here. Ordinary Chat history and
// model facts continue through the private Host context reader, not these rows.
export {sessions, messages, pluginTaskRequests, mediaBlobs, mediaRefs} from '../../cindy/tasks/db/schema.ts';

type PluginDatabase = {drizzle: any};
let provider: (() => PluginDatabase) | undefined;
let snapshot: {client: PluginDatabase} | null = null;
export function configureGhostDatabase(read: () => PluginDatabase): () => void {
  provider = read;
  snapshot = null;
  return () => { if (provider === read) { provider = undefined; snapshot = null; } };
}
export function getDbClient(): PluginDatabase {
  if (!provider) throw new Error('Plugin ledger is not ready');
  const client = provider();
  if (snapshot?.client !== client) snapshot = {client};
  return client;
}
export function getCurrentDbClientSnapshot(): {client: PluginDatabase} | null {
  if (!provider) return null;
  getDbClient();
  return snapshot;
}
