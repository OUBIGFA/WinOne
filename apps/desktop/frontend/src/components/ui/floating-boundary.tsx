import {createContext} from "react";

// Capture menus stay inside the selected monitor's work area.
export const FloatingBoundary=createContext<{x:number;y:number;width:number;height:number}|undefined>(undefined);
