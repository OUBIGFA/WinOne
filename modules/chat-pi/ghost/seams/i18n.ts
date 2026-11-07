// Main-process translation seam for vendored Cindy plugin code, over the locale
// subset the sync extracts. The Host sets the language with each request.
import zh from '../../cindy/ghost/locales/zh-CN.json' with { type: 'json' };
import en from '../../cindy/ghost/locales/en.json' with { type: 'json' };

let language: 'zh-CN' | 'en' = 'zh-CN';
export function setGhostLanguage(value: string): void { language = value === 'en-US' || value === 'en' ? 'en' : 'zh-CN'; }

function lookup(tree: unknown, key: string): unknown {
  return key.split('.').reduce<unknown>((node, part) => (node && typeof node === 'object' ? (node as Record<string, unknown>)[part] : undefined), tree);
}

export function t(key: string, params?: Record<string, unknown>): string {
  const value = lookup(language === 'en' ? en : zh, key) ?? lookup(en, key);
  if (typeof value !== 'string') return key;
  return value.replace(/\{\{\s*([\w.]+)\s*\}\}/g, (whole, name) => (params && name in params ? String(params[name]) : whole));
}

export function getLocale(): string { return language; }
