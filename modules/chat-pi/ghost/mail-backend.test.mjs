import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {createHash} from 'node:crypto';
import {createLocalMailBackend, MAIL_PNG, MAIL_SOURCE} from './fixtures/mail-backend.mjs';
import {visibleMailAttachments, insertMailImage, removeMailAttachment, readMailCard} from '../cindy/ghost/packages/maker-shared/src/ghostCardForm.ts';

const repository = fileURLToPath(new URL('../../../', import.meta.url));
const system = {skip: !process.env.WINONE_GHOST_SANDBOX};
const values = card => Object.fromEntries(card.mail.fields.map(field => [field.name, field.value]));
async function until(read, accept, label, budget = 8000) {
  const deadline = Date.now() + budget; let last;
  while (Date.now() < deadline) {last = await read(); if (accept(last)) return last; await new Promise(resolve => setTimeout(resolve, 25));}
  assert.fail(label + ': ' + JSON.stringify(last));
}
async function fixture(t) {
  const base = process.env.WINONE_CHAT_TEST_DIRECTORY ?? path.join(repository, '_temp/plugin-task-close/resume-20261106/mail/backend');
  await fs.mkdir(base, {recursive: true}); const f = await createLocalMailBackend(await fs.mkdtemp(path.join(base, 'mail-')));
  t.after(() => f.close());
  f.stats = async () => {const result = await f.call('stats'); assert.equal(result.ok, true, JSON.stringify(result)); return result.result;};
  return f;
}
async function delivered(f, action, formValues, callId) {
  const result = await f.action(action, formValues, callId); assert.equal(result.ok, true, JSON.stringify(result));
}

// This fixture is a real installed provider, not a MailCardService substitute.
// Assertions wait for persisted provider/card outcomes, never dispatcher {ok}.
test('Mail backend original rich SDK owns local storage, HTML/CID and ordered autosave without sending', system, async t => {
  const f = await fixture(t), initial = await f.card();
  assert.equal(initial.mail.state, 'draft'); assert.equal(initial.mail.from, 'Sender <sender@example.invalid>');
  assert.equal(values(initial).bodyHtml, MAIL_SOURCE); assert.equal(visibleMailAttachments(values(initial)).length, 0);
  assert.ok(initial.mail.images[0].url.startsWith('cindy-media://blobs/'));
  const edits = {...values(initial), subject: 'Typed locally', bodyHtml: MAIL_SOURCE.replace('one two', 'one two LOCAL'), body: 'Hello one two LOCAL reference\n\nSigned'};
  await delivered(f, 'change', edits);
  assert.equal((await f.stats()).sends, 0);
  const saved = await until(f.stats, state => state.writes > 0 && state.headers.subject === edits.subject, 'actual local autosave did not commit');
  await until(f.card, card => card.mail.state === 'draft' && values(card).subject === edits.subject, 'Host did not finalize the provider autosave');
  assert.equal(saved.bodyHtml, edits.bodyHtml); assert.equal(saved.history[0].send, false);
  assert.ok(saved.selected.every(account => account === 'fixture-account'));
  const routing = await f.call('routing'); const encoded = JSON.stringify(routing.result);
  assert.ok(!encoded.includes('Typed locally') && !encoded.includes('one two LOCAL'), 'draft bodies must not enter routing KV');
  assert.equal(Object.values(routing.result.mailDraftCards)[0].state, 'draft');
  assert.equal(readMailCard((await f.card()).html).fields.find(field => field.name === 'bodyHtml').value, edits.bodyHtml);
});

test('Mail backend uses the original attachment picker/snapshot, MIME insert/remove and inline-image lifecycle', system, async t => {
  const f = await fixture(t); const selected = path.join(f.directory, 'selected.png'); await fs.writeFile(selected, MAIL_PNG);
  f.selections.push(selected); await delivered(f, 'attach', values(await f.card()));
  // Selection is snapshotted at the Host before dispatch. Replacing the user's
  // selected path cannot change bytes a delayed adapter later sees.
  await fs.writeFile(selected, 'changed after selection');
  let card = await until(f.card, card => JSON.parse(values(card).attachmentItems).length === 2 && card.mail.state === 'draft', 'actual attachment save was not published');
  let items = JSON.parse(values(card).attachmentItems); const added = items.find(item => item.key !== 'original');
  assert.equal(added.bytes, MAIL_PNG.length); assert.equal(added.mimeType, 'image/png');
  assert.equal(f.snapshots.length, 1); assert.notEqual(f.snapshots[0].source, selected);
  assert.ok(f.snapshots[0].source.startsWith(path.join(f.directory, 'mail-staging') + path.sep));
  assert.deepEqual(await fs.readFile(f.snapshots[0].path), MAIL_PNG);
  assert.equal(visibleMailAttachments(values(card))[0].key, added.key);
  await delivered(f, 'change', insertMailImage(values(card), 'attachmentItems', added.key));
  card = await until(f.card, card => visibleMailAttachments(values(card)).length === 0 && card.mail.state === 'draft', 'existing MIME image was not inserted');
  const inserted = JSON.parse(values(card).attachmentItems).find(item => item.key === added.key);
  assert.match(values(card).bodyHtml, new RegExp('cid:' + inserted.contentId.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')));
  assert.ok(!values(card).bodyHtml.includes('cindy-media:'), 'preview URLs never enter the provider document');
  await delivered(f, 'change', removeMailAttachment(values(card), 'attachmentItems', added.key));
  card = await until(f.card, card => JSON.parse(values(card).attachmentItems).length === 1 && card.mail.state === 'draft', 'MIME/image removal was not committed');
  assert.ok(!values(card).bodyHtml.includes(inserted.contentId)); assert.ok(values(card).bodyHtml.includes('.signature{color:#234567}'));
  const inline = path.join(f.directory, 'inline-selection.png'); await fs.writeFile(inline, MAIL_PNG); f.selections.push(inline);
  await delivered(f, 'image', values(card));
  card = await until(f.card, card => JSON.parse(values(card).attachmentItems).length === 2 && card.mail.state === 'draft', 'native inline picker did not save its MIME part');
  items = JSON.parse(values(card).attachmentItems); const image = items.find(item => item.key !== 'original');
  assert.ok(image.contentId); assert.ok(values(card).bodyHtml.includes('cid:' + image.contentId));
  assert.equal(visibleMailAttachments(values(card)).length, 0);
  assert.equal((await f.stats()).sends, 0);
  assert.equal(f.snapshots[0].sha256, createHash('sha256').update(MAIL_PNG).digest('hex'));
});

test('Mail backend edited HTML then attachment insert advances only the verified SDK MIME baseline', system, async t => {
  const f = await fixture(t), first = await f.card();
  const bodyHtml = '<html><head><style>.signature{color:#234567}</style></head><body><p>Typed <b>document</b><img src="cid:original@fixture"></p></body></html>';
  await delivered(f, 'change', {...values(first), bodyHtml, body: 'Typed document'});
  let card = await until(f.card, card => card.mail.state === 'draft' && values(card).bodyHtml.includes('Typed'), 'HTML edit did not finish original autosave');
  await until(f.stats, state => state.writes > 0, 'HTML provider write missing');
  const file = path.join(f.directory, 'after-edit.png'); await fs.writeFile(file, MAIL_PNG); f.selections.push(file);
  await delivered(f, 'attach', values(card));
  card = await until(f.card, card => card.mail.state === 'draft' && JSON.parse(values(card).attachmentItems).length === 2, 'attachment did not commit after HTML editing');
  const part = JSON.parse(values(card).attachmentItems).find(part => part.key !== 'original');
  await delivered(f, 'change', insertMailImage(values(card), 'attachmentItems', part.key));
  await until(f.stats, state => state.bodyHtml.includes('cid:image-1@cindy.local'), 'verified new MIME part was mistaken for a remote conflict');
  card = await until(f.card, card => card.mail.state === 'draft' && values(card).bodyHtml.includes('cid:image-1@cindy.local'), 'original SDK did not publish inserted image');
  await delivered(f, 'change', removeMailAttachment(values(card), 'attachmentItems', part.key));
  await until(f.stats, state => state.attachments.length === 1 && !state.bodyHtml.includes('image-1@cindy.local'), 'MIME baseline did not advance for removal');
  assert.equal((await f.stats()).sends, 0);
});

test('Mail backend waits for the real 10s grace, keeps Undo local, and sends exactly once with known receipt', system, async t => {
  const f = await fixture(t); let card = await f.card(); await delivered(f, 'send', values(card));
  card = await until(f.card, card => !!card.mail.pendingSend, 'Host did not accept real countdown');
  assert.ok(card.mail.pendingSend.deadline - Date.now() > 8500);
  assert.equal((await f.stats()).sends, 0); await delivered(f, 'cancel');
  card = await until(f.card, card => !card.mail.pendingSend && card.mail.state === 'draft', 'Undo did not retain the saved draft');
  await delivered(f, 'send', values(card));
  card = await until(f.card, card => !!card.mail.pendingSend, 'new countdown was not shown'); const deadline = card.mail.pendingSend.deadline;
  assert.equal((await f.stats()).sends, 0);
  await until(f.card, card => card.mail.state === 'sent', 'provider receipt was not published', 14000);
  const state = await f.stats(); assert.equal(state.sends, 1); assert.ok(state.history.find(entry => entry.send).at >= deadline);
  const repeat = await f.call('send'); assert.equal(repeat.result.data.alreadySent, true); assert.equal((await f.stats()).sends, 1);
  assert.equal(f.continuations.length, 1); assert.equal(f.continuations[0].mode, 'continue');
  assert.ok(f.continuations[0].prompt.includes('不要重复发送')); assert.ok((await f.card()).mail.continueLabel);
  await delivered(f, 'continue'); await until(() => f.continuations.length, count => count === 2, 'Continue did not reach original Agent slot');
  assert.equal((await f.stats()).sends, 1, 'failed continuation is never a send retry');
});

test('Mail backend changed provider content at countdown expiry gets a new full grace, not an automatic stale send', system, async t => {
  const f = await fixture(t);await delivered(f, 'send', values(await f.card()));
  const initial = await until(f.card, card => !!card.mail.pendingSend, 'initial countdown missing');const firstDeadline = initial.mail.pendingSend.deadline;
  const bodyHtml = MAIL_SOURCE.replace('one two', 'REMOTE one two');
  await f.call('mutate', {bodyHtml, body: 'Hello REMOTE one two reference\n\nSigned'});
  const reviewed = await until(f.card, card => !!card.mail.pendingSend && card.mail.pendingSend.deadline > firstDeadline + 8000, 'content learned at expiry did not receive a new full grace', 14000);
  assert.equal((await f.stats()).sends, 0, 'provider-confirmed stale attempt must never be called sent');
  assert.ok(values(reviewed).bodyHtml.includes('REMOTE one two'));assert.ok(reviewed.mail.pendingSend.deadline - Date.now() > 8500);
  await delivered(f, 'cancel');await until(f.card, card => card.mail.state === 'draft' && !card.mail.pendingSend, 'undo after renewed grace did not keep revised draft');
  assert.equal((await f.stats()).sends, 0);
});

test('Mail backend late receipt cannot be treated as sent early and repeated native actions never replay it', system, async t => {
  const f = await fixture(t); await f.call('mode', {mode: 'late'});
  await delivered(f, 'send', values(await f.card())); await until(f.card, card => !!card.mail.pendingSend, 'countdown missing');
  await delivered(f, 'now'); await until(f.stats, state => state.sends === 1, 'provider send did not commit');
  assert.equal((await f.card()).mail.state, 'sending'); assert.equal(f.continuations.length, 0);
  const refused = await f.action('send', values(await f.card())); assert.equal(refused.ok, false); assert.equal(refused.reason, 'mail-view-readonly');
  await f.call('settle'); await until(f.card, card => card.mail.state === 'sent', 'late known provider receipt did not settle');
  assert.equal((await f.stats()).sends, 1); assert.equal(f.continuations.length, 1);
});

test('Mail backend restart never resumes an unattended countdown and saved HTML editor values recover', system, async t => {
  const f = await fixture(t); const edited = {...values(await f.card()), subject: 'Restart retained', bodyHtml: MAIL_SOURCE.replace('one two', 'one two RESTORED'), body: 'Hello one two RESTORED reference\n\nSigned'};
  await delivered(f, 'change', edited); await until(f.stats, state => state.headers.subject === 'Restart retained', 'autosave before restart missing');
  await delivered(f, 'send', values(await f.card())); await until(f.card, card => !!card.mail.pendingSend, 'countdown before restart missing');
  await f.reopen();
  await new Promise(resolve => setTimeout(resolve, 10500));
  assert.equal((await f.stats()).sends, 0, 'a restarted Host/sandbox must not recreate the old timer');
  await delivered(f, 'cancel');
  const card = await until(f.card, card => !card.mail.pendingSend && card.mail.state === 'draft', 'recovery Undo missing');
  assert.equal(values(card).subject, edited.subject); assert.equal(values(card).bodyHtml, edited.bodyHtml);
  await delivered(f, 'send', values(card)); const pending = await until(f.card, card => !!card.mail.pendingSend, 'restored send did not start fresh grace');
  assert.ok(pending.mail.pendingSend.deadline - Date.now() > 8500); await delivered(f, 'cancel');
});

test('Mail backend lost or incomplete provider receipts stay unknown and reopening/reading never resends', system, async t => {
  for (const mode of ['unknown', 'incomplete', 'late']) {
    const f = await fixture(t); await f.call('mode', {mode}); await delivered(f, 'send', values(await f.card()));
    await until(f.card, card => !!card.mail.pendingSend, 'countdown missing for ' + mode); await delivered(f, 'now');
    await until(f.stats, state => state.sends === 1, 'provider did not commit ' + mode);
    if (mode !== 'late') await until(f.card, card => card.mail.state === 'error', 'uncertain outcome did not show review status');
    await f.reopen(); const repeated = await f.call('send');
    assert.equal(repeated.result.ok, false); assert.equal(repeated.result.execution, 'unknown');
    const read = await f.call('get'); assert.equal(read.result.data.readBack.draftId, 'fixture-draft'); assert.equal(read.result.data.draftCard, true);
    const card = await f.card(read.result.xdt_card_id); assert.equal(card.mail.state, 'unconfirmed');
    for (const action of ['send', 'change', 'attach', 'image', 'delete']) {
      const result = await f.action(action, values(card), card.callId); assert.equal(result.ok, false); assert.equal(result.reason, 'mail-view-readonly');
    }
    assert.equal((await f.stats()).sends, 1); assert.equal(f.continuations.length, 0);
    await f.close();
  }
});

test('Mail backend provider/card three-way merge keeps independent edits, conflicts, historical views and delete semantics', system, async t => {
  const f = await fixture(t); let card = await f.card();
  await delivered(f, 'change', {...values(card), body: values(card).body + ' LOCAL'}); await f.call('mutate', {body: 'REMOTE ' + values(card).body});
  card = await until(f.card, card => card.mail.state === 'draft' && values(card).body.startsWith('REMOTE ') && values(card).body.endsWith(' LOCAL'), 'original provider merge did not preserve independent edits');
  await delivered(f, 'change', {...values(card), body: 'Local replacement'}); await f.call('mutate', {body: 'Remote replacement'});
  card = await until(f.card, card => card.mail.state === 'error', 'overlap conflict was not retained');
  assert.equal(values(card).body, 'Local replacement'); const conflict = await f.call('get');
  assert.ok(conflict.result.data.conflicts.includes('body')); assert.equal(conflict.result.data.cardEdits.body, 'Local replacement');
  assert.equal((await f.call('send')).result.execution, 'not_executed'); assert.equal((await f.stats()).sends, 0);
  const next = await f.call('prime'); assert.ok(next.result.xdt_card_id); assert.notEqual(next.result.xdt_card_id, f.firstCallId);
  assert.equal((await f.card()).mail.supersededBy, next.result.xdt_card_id);
  const expired = await f.action('change', values(await f.card())); assert.equal(expired.ok, false); assert.equal(expired.reason, 'mail-view-expired');
  const current = await f.card(next.result.xdt_card_id); await delivered(f, 'delete', undefined, current.callId);
  await until(() => f.card(current.callId), card => card.mail.state === 'deleted', 'actual delete did not settle');
  const state = await f.stats(); assert.equal(state.deletes, 1); assert.equal(state.sends, 0);
  const readonly = await f.action('delete', undefined, current.callId); assert.equal(readonly.ok, false);
});

test('Mail backend original Reader uses explicit markRead/openDraft, lazy thread bodies and current draft ownership', system, async t => {
  const f = await fixture(t), url = 'https://mail.example.invalid/selected';
  assert.equal((await f.host.call('mail-preview', {sessionId: 'mail-task', url, read: false})).supported, true);
  assert.equal((await f.stats()).reads.length, 0);
  const background = await f.host.call('mail-preview', {sessionId: 'mail-task', url, read: true});
  assert.equal(background.message.bodyHtml, MAIL_SOURCE); assert.equal(background.message.conversation.messages.length, 8); assert.equal(background.draftCallId, undefined);
  assert.equal((await f.stats()).markReads, 0); assert.equal((await f.stats()).reads.length, 1, 'thread metadata must not fetch every hidden body');
  const opened = await f.host.call('mail-preview', {sessionId: 'mail-task', url, read: true, opened: true});
  assert.equal(opened.message.accountId, 'fixture-account'); assert.equal(opened.draftCallId, undefined); assert.ok(opened.providerId);
  assert.equal((await f.stats()).markReads, 1); assert.equal((await f.stats()).reads[1].openDraft, true);
  const expanded = await f.host.call('mail-preview', {sessionId: 'mail-task', url: background.message.conversation.messages[0].url, read: true, opened: true, providerId: opened.providerId});
  assert.equal(expanded.message.messageId, 'older-0'); assert.equal((await f.stats()).markReads, 2);
  const openedDraft = await f.host.call('mail-preview', {sessionId: 'mail-task', url: 'https://mail.example.invalid/draft', read: true, opened: true, providerId: opened.providerId});
  assert.ok(openedDraft.draftCallId); const draft = await f.card(openedDraft.draftCallId);
  assert.equal(draft.sessionId, 'mail-task'); assert.equal(draft.mail.accountId, openedDraft.message.accountId);
  assert.equal((await f.stats()).markReads, 3);
  await assert.rejects(f.host.call('mail-preview', {sessionId: 'other-task', url, read: true, opened: true}), /MAIL_SESSION_UNAVAILABLE/);
  assert.equal((await f.stats()).markReads, 3); assert.equal((await f.stats()).sends, 0);
});
