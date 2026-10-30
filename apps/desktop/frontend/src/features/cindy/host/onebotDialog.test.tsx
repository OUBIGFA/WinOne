// @vitest-environment jsdom
import {afterEach,expect,it} from 'vitest';
import {cleanup,render,screen,waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import * as Dialog from './onebotDialog';
afterEach(cleanup);
it('scopes actual portaled dialogs without changing focus or close behavior',async()=>{
 render(<Dialog.Root><Dialog.Trigger>Open</Dialog.Trigger><Dialog.Portal><Dialog.Overlay className="modal-scrim"/><Dialog.Content className="modal-panel"><Dialog.Title>Group</Dialog.Title><Dialog.Description>Choose teammates</Dialog.Description><input aria-label="Name"/><Dialog.Close>Close</Dialog.Close></Dialog.Content></Dialog.Portal></Dialog.Root>);
 const user=userEvent.setup();await user.click(screen.getByRole('button',{name:'Open'}));
 const dialog=screen.getByRole('dialog');expect(dialog.classList.contains('cindy-panel-popup')).toBe(true);
 expect(document.querySelector('.modal-scrim')?.classList.contains('cindy-panel-popup')).toBe(true);
 expect(document.activeElement).toBe(screen.getByRole('textbox',{name:'Name'}));
 await user.keyboard('{Escape}');await waitFor(()=>expect(screen.queryByRole('dialog')).toBeNull());
 expect(document.activeElement).toBe(screen.getByRole('button',{name:'Open'}));
});
