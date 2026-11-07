// WinOne seam for Cindy lib/secondaryWindow.ts and lib/sidebarWindow.ts
// (2d8d3c3). Cindy opens extra session and sidebar windows; WinOne shows its
// pages in one window, so every page runs as the primary window.
export function isSecondaryWindow(): boolean { return false; }
export function isSidebarWindow(): boolean { return false; }
export function getBootSessionId(): string | null { return null; }
export function getBootDeviceId(): string | null { return null; }
export function getBootMessageClientId(): string | null { return null; }
