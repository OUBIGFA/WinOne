// WinOne resolves system preference into data-theme. The upstream class is
// retained only for class-only embedding; it must not override a native value.

import { useEffect, useState } from 'react';

export function readIsDarkMode(): boolean {
  if (typeof document === 'undefined') return false;
  const root = document.documentElement;
  return root.dataset.theme ? root.dataset.theme === 'dark' : root.classList.contains('dark');
}

export function useIsDarkMode(): boolean {
  const [isDark, setIsDark] = useState(readIsDarkMode);

  useEffect(() => {
    const root = document.documentElement;
    const observer = new MutationObserver(() => {
      setIsDark(readIsDarkMode());
    });
    observer.observe(root, { attributes: true, attributeFilter: ['class', 'data-theme'] });
    return () => observer.disconnect();
  }, []);

  return isDark;
}
