// Cindy control-point bridge, shared by scheduler and OneBot. The catalog is
// refreshed before mutations; execution always rechecks the actual Host.
export function unavailableControlPoint(target) {
  return target.onFailure === 'allow' ? {action: 'allow'} : {action: target.onFailure, ghostId: target.pluginId, ghostName: target.pluginId, reason: '', failure: true};
}

export function createControlPointClient({baseUrl, key}) {
  const base = new URL(baseUrl);
  if (base.protocol !== 'http:' || base.hostname !== '127.0.0.1' || !key || key.length < 16) throw Error('Private plugin bridge is required');
  const catalogs = new Map();
  async function post(method, params, signal) {
    const response = await fetch(baseUrl + '/plugin-' + method, {method: 'POST', headers: {Authorization: 'Bearer ' + key, 'Content-Type': 'application/json'}, body: JSON.stringify(params), signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(35_000)]) : AbortSignal.timeout(35_000)});
    if (!response.ok) throw Error((await response.text()).slice(0, 2048) || 'Plugin control point is unavailable');
    return response.json();
  }
  return {
    async refresh(point) {
      const rows = await post('candidates', {point});
      const values = new Map();
      for (const row of rows) values.set(row.pluginId, row.pluginId);
      catalogs.set(point, values);
      return rows;
    },
    resolve(point, id) {
      const resolved = catalogs.get(point)?.get(id);
      if (!resolved) throw Error('Plugin pre-run check is unavailable or not approved');
      return resolved;
    },
    async resolveRemote(point, id) { return (await post('resolve', {point, pluginId: id})).pluginId; },
    async screen(point, data, target, signal) {
      try { return await post('screen', {point, data, target}, signal); }
      catch (error) { if (signal?.aborted) throw error; return unavailableControlPoint(target); }
    },
  };
}
