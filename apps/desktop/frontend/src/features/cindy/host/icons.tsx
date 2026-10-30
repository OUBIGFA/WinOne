// WinOne seam for Cindy vendor and source marks (b7e46bc). WinOne runs only
// the Pi harness, so every agent identity renders the vendored PiMark; the
// Claude/Codex marks and remote-source overlay have no WinOne state to show.
import type {ReactNode} from 'react';
import {PiMark} from '../components/icons/PiMark';

type MarkProps = {size?: number; className?: string};
export const ClaudeMark = PiMark;
export const CodexMark = PiMark;
export function RemoteSourceMark({children}: {children: ReactNode}) {
  return <>{children}</>;
}
export type Vendor = 'cc' | 'codex' | 'pi';
export function agentKindToVendor(_kind?: string | null): Vendor {
  return 'pi';
}
export function VendorIcon({size = 14, className}: MarkProps & {vendor: Vendor}) {
  return <PiMark size={size} className={className}/>;
}
