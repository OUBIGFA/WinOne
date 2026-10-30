// The original Radix dialog keeps focus trapping, dismissal and portals.
// Only attach WinOne's existing Cindy theme scope to the portaled surfaces.
import {forwardRef,type ComponentPropsWithoutRef,type ComponentRef} from 'react';
import * as Dialog from '@radix-ui/react-dialog';
export * from '@radix-ui/react-dialog';
export const Content=forwardRef<ComponentRef<typeof Dialog.Content>,ComponentPropsWithoutRef<typeof Dialog.Content>>(({className='',...props},ref)=><Dialog.Content {...props} ref={ref} className={'cindy-panel-popup '+className}/>);
export const Overlay=forwardRef<ComponentRef<typeof Dialog.Overlay>,ComponentPropsWithoutRef<typeof Dialog.Overlay>>(({className='',...props},ref)=><Dialog.Overlay {...props} ref={ref} className={'cindy-panel-popup '+className}/>);
Content.displayName='OneBotDialogContent';
Overlay.displayName='OneBotDialogOverlay';
