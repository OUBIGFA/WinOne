import source from '../../../../../../../ghost/seams/mcp-app-sdk';

// Vite embeds the same pinned dependency as AppBridge. Only transient View
// documents carry these bytes when they fit the remote budget, never persisted cards.
export const hostAppSdkAsset = {
  source,
};
