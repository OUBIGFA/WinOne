// WinOne seam for react-router-dom inside vendored Cindy pages (b7e46bc).
// WinOne pages are not URL routes: Cindy's task and settings paths become
// WinOne navigation, and the automation page keeps a fixed location.
import {cindyNavigation} from './store';

export interface Location {pathname: string; search: string; hash: string; state: unknown; key: string}
const location: Location = {pathname: '/cc-agent/scheduled', search: '', hash: '', state: null, key: 'winone'};

function go(path: string) {
  const url = new URL(path, 'winone://app');
  const session = /^\/cc-agent\/(?:orca\/|files\/)?([^/]+)$/.exec(url.pathname);
  if (session && session[1] !== 'scheduled') {
    cindyNavigation().openConversation(decodeURIComponent(session[1]!));
    return;
  }
  if (url.pathname === '/settings') {
    const tab = url.searchParams.get('tab');
    if (tab === 'providers') cindyNavigation().openAIServices();
    else if (tab === 'im-bot') cindyNavigation().openIMSettings();
  }
  // Other paths address the page that is already shown.
}
export function useNavigate() {
  return (to: string | number, _options?: {replace?: boolean; state?: unknown}) => {
    if (typeof to === 'string') go(to);
  };
}
export function useLocation(): Location {
  return location;
}
export function useMatch(pattern: string): {params: Record<string, string>} | null {
  const names: string[] = [];
  const source = pattern.replace(/:[A-Za-z]+/g, (name) => { names.push(name.slice(1)); return '([^/]+)'; });
  const match = new RegExp('^' + source + '$').exec(location.pathname);
  if (!match) return null;
  return {params: Object.fromEntries(names.map((name, index) => [name, decodeURIComponent(match[index + 1]!)]))};
}
