// A portaled portrait picker must retain the same scoped tokens as its trigger.
import {forwardRef,type ComponentPropsWithoutRef,type ComponentRef} from 'react';
import * as Popover from '@radix-ui/react-popover';
export * from '@radix-ui/react-popover';
export const Content=forwardRef<ComponentRef<typeof Popover.Content>,ComponentPropsWithoutRef<typeof Popover.Content>>(({className='',...props},ref)=><Popover.Content {...props} ref={ref} className={'cindy-panel-popup '+className}/>);
Content.displayName='OneBotPopoverContent';
