// Default transport for host-owned runtime archives, not plugin-selected URLs.
// PluginDownloadSlot supplies its own DNS-pinned SSRF request for every hop.
// Keeping redirects manual lets Cindy's transport enforce its original policy.
export function requestResponse(url: string, init: RequestInit): Promise<Response> {
  return fetch(url, {...init, redirect: 'manual', credentials: 'omit'});
}
