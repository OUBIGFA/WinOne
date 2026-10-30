// Upstream modules that vendored code only reaches through injected
// dependencies in WinOne. Reaching one is a wiring error, never a fallback.
const unavailable = (name: string) => () => { throw new Error(`${name} is supplied by the WinOne host`); };
export const requestUtilityText = unavailable('requestUtilityText');
export const hasSystemNode = unavailable('hasSystemNode');
export const executePreRunHook = unavailable('executePreRunHook');
