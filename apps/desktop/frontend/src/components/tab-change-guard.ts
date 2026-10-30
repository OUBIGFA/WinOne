import {createContext} from "react";

// A tab switch hides or unmounts the section the user is editing, so it needs
// the same unsaved-change decision as leaving the page. Settings pages provide
// the shared navigation guard. Chat, standalone documents and tests that render
// without the shell provide none, so their tabs switch directly as before.
// The guard calls apply() immediately when nothing is dirty or busy, after a
// running operation finishes, or after the user confirms discarding changes.
export type TabChangeGuard=(apply:()=>void)=>void;
export const TabChangeGuardContext=createContext<TabChangeGuard|null>(null);
