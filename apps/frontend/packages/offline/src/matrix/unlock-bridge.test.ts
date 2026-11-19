/* SPDX-License-Identifier: Apache-2.0 */
/**
 * The app side of the Sytue ID bridge (ADR 0084, 0085): a hidden iframe of Sytue ID's
 * exact origin, spoken to by postMessage only. Anything from another origin or another
 * window is ignored, the request goes to Sytue ID's origin only, and the app asks for
 * operations: it never receives (nor asks for) the unlock key.
 */
import { expect, test } from 'bun:test';
import {
  forgetLegacyMaterial,
  passwordEnvelopeInBridge,
  takeHandoff,
  unwrapInBridge,
  wrapInBridge,
} from './unlock-bridge.js';
import type { RecoveryWrapper } from './recovery-wrap.js';

const idOrigin = 'https://id.sytue.eu';
const appOrigin = 'https://mail.sytue.eu';
type Listener = (event: MessageEvent) => void;

/** A page with one Sytue ID bridge in it; `answer` plays the bridge. */
function page(
  answer: (msg: Record<string, unknown>, reply: (data: unknown) => void) => void,
  version: number = 2,
  origin: string = appOrigin,
) {
  const listeners = new Set<Listener>();
  const posted: { msg: Record<string, unknown>; target: string }[] = [];
  const frames: { src: string; removed: boolean; hidden: boolean }[] = [];
  const dispatch = (data: unknown, origin: string, source: unknown) => {
    for (const fn of listeners) fn({ data, origin, source } as unknown as MessageEvent);
  };
  const win = {
    location: { origin },
    addEventListener: (_: string, fn: Listener) => listeners.add(fn),
    removeEventListener: (_: string, fn: Listener) => listeners.delete(fn),
  };
  const doc = {
    createElement: () => {
      const frame = {
        src: '',
        hidden: false,
        removed: false,
        tabIndex: 0,
        setAttribute: () => undefined,
        remove: () => (frame.removed = true),
        contentWindow: {
          postMessage: (msg: Record<string, unknown>, target: string) => {
            posted.push({ msg, target });
            answer(msg, (data) => dispatch(data, idOrigin, frame.contentWindow));
          },
        },
      };
      frames.push(frame);
      return frame;
    },
    body: {
      appendChild: (frame: { contentWindow: unknown }) => {
        // Somebody else talks first: a page of another origin, and a window that is not the bridge.
        dispatch({ type: 'sytue-e2e-ready', v: 2 }, 'https://evil.example', frame.contentWindow);
        dispatch({ type: 'sytue-e2e-ready', v: 2 }, idOrigin, {});
        dispatch({ type: 'sytue-e2e-ready', v: version }, idOrigin, frame.contentWindow);
      },
    },
  };
  return { win, doc, posted, frames, dispatch, listeners };
}

const account = '@sytue_ana:sytue.eu';
const wrapper: RecoveryWrapper = {
  v: 1,
  kind: 'sytue-id',
  key_id: 'K1',
  credential_id: '',
  kdf: { alg: 'sytue-id-unlock-hkdf-sha256', hint: 'aGludA==' },
  nonce: 'bm9uY2U=',
  ciphertext: 'Y2lwaGVy',
};

test('asks the bridge of Sytue ID to open its account wrapper and gets only the recovery key', async () => {
  const p = page((msg, reply) =>
    reply({
      type: 'sytue-e2e-result',
      id: msg.id,
      ok: true,
      recovery_key: 'EsT3 4kza',
      kind: 'sytue-id',
    }),
  );
  const got = await unwrapInBridge(
    { idOrigin, subject: 'ana', win: p.win as never, doc: p.doc as never },
    { account, wrappers: [wrapper] },
  );
  expect(got).toEqual({ ok: true, recoveryKey: 'EsT3 4kza', kind: 'sytue-id' });
  // One request, to Sytue ID's exact origin, for that person and account, after the real bridge said ready.
  expect(p.posted).toHaveLength(1);
  expect(p.posted[0]!.target).toBe(idOrigin);
  expect(p.posted[0]!.msg).toMatchObject({
    type: 'sytue-e2e-unwrap',
    sub: 'ana',
    account,
    wrappers: [wrapper],
  });
  expect(p.frames[0]!.src).toBe(`${idOrigin}/e2e/bridge?o=${encodeURIComponent(appOrigin)}`);
  expect(p.frames[0]!.hidden).toBe(true);
  expect(p.frames[0]!.removed).toBe(true);
  expect(p.listeners.size).toBe(0);
});

test('a bridge from before ADR 0085 is not spoken to: it would hand the key out', async () => {
  const p = page(() => undefined, 1);
  const got = await unwrapInBridge(
    { idOrigin, subject: 'ana', timeoutMs: 1000, win: p.win as never, doc: p.doc as never },
    { account, wrappers: [wrapper] },
  );
  expect(got).toEqual({ ok: false, error: 'unavailable' });
  expect(p.posted).toHaveLength(0);
  expect(p.frames[0]!.removed).toBe(true);
});

test('an answer from another origin or with the wrong id is ignored; no answer is unavailable', async () => {
  const p = page((_, reply) => {
    reply({
      type: 'sytue-e2e-result',
      id: 'another',
      ok: true,
      recovery_key: 'x',
      kind: 'sytue-id',
    });
  });
  const pending = unwrapInBridge(
    { idOrigin, subject: 'ana', timeoutMs: 50, win: p.win as never, doc: p.doc as never },
    { account, wrappers: [wrapper] },
  );
  p.dispatch(
    {
      type: 'sytue-e2e-result',
      id: String(p.posted[0]!.msg.id),
      ok: true,
      recovery_key: 'forged',
      kind: 'sytue-id',
    },
    'https://x.apps.sytue.eu',
    {},
  );
  expect(await pending).toEqual({ ok: false, error: 'unavailable' });
  expect(p.frames[0]!.removed).toBe(true);
});

test('refusals keep their reason; unknown ones are unavailable', async () => {
  for (const [error, expected] of [
    ['signin', 'signin'],
    ['wrong', 'wrong'],
    ['account', 'account'],
    ['busy', 'busy'],
    ['whatever', 'unavailable'],
  ] as const) {
    const p = page((msg, reply) =>
      reply({ type: 'sytue-e2e-result', id: msg.id, ok: false, error }),
    );
    expect(
      await unwrapInBridge(
        { idOrigin, subject: 'ana', win: p.win as never, doc: p.doc as never },
        { account, wrappers: [wrapper] },
      ),
    ).toEqual({ ok: false, error: expected });
  }
});

test('asks the bridge to seal and keeps only a well-formed wrapper of that key', async () => {
  const sealed = { ...wrapper, key_id: 'K2' };
  const p = page((msg, reply) =>
    reply({ type: 'sytue-e2e-result', id: msg.id, ok: true, wrapper: sealed, legacy: true }),
  );
  const got = await wrapInBridge(
    { idOrigin, subject: 'ana', win: p.win as never, doc: p.doc as never },
    { account, keyID: 'K2', recoveryKey: 'EsT3 4kza' },
  );
  expect(got).toEqual({ ok: true, wrapper: sealed, legacy: true });
  expect(p.posted[0]!.msg).toMatchObject({
    type: 'sytue-e2e-wrap',
    sub: 'ana',
    account,
    key_id: 'K2',
    recovery_key: 'EsT3 4kza',
  });
  const other = page((msg, reply) =>
    reply({ type: 'sytue-e2e-result', id: msg.id, ok: true, wrapper: sealed }),
  );
  expect(
    await wrapInBridge(
      { idOrigin, subject: 'ana', win: other.win as never, doc: other.doc as never },
      { account, keyID: 'K3', recoveryKey: 'EsT3 4kza' },
    ),
  ).toEqual({ ok: false, error: 'unavailable' });
});

test('forgets the legacy material through the same bridge', async () => {
  const p = page((msg, reply) => reply({ type: 'sytue-e2e-result', id: msg.id, ok: true }));
  await forgetLegacyMaterial({
    idOrigin,
    subject: 'ana',
    win: p.win as never,
    doc: p.doc as never,
  });
  expect(p.posted[0]!.msg).toMatchObject({ type: 'sytue-e2e-forget-legacy', sub: 'ana' });
});

test("«Mi cuenta», in Sytue ID's own origin, asks its own bridge for the new password envelope", async () => {
  const envelope = {
    v: 1,
    kind: 'password',
    credential_id: '',
    kdf: { alg: 'argon2id' },
    hint: 'aGludA==',
    nonce: 'bm9uY2U=',
    ciphertext: 'Y2lwaGVy',
  };
  const p = page(
    (msg, reply) => reply({ type: 'sytue-e2e-result', id: msg.id, ok: true, envelope }),
    2,
    idOrigin,
  );
  const got = await passwordEnvelopeInBridge(
    { idOrigin, subject: 'ana', win: p.win as never, doc: p.doc as never },
    { current: 'antes', next: 'después' },
  );
  expect(got).toEqual({ ok: true, envelope });
  expect(p.frames[0]!.src).toBe(`${idOrigin}/e2e/cuenta`);
  expect(p.posted[0]!.msg).toMatchObject({
    type: 'sytue-e2e-password',
    sub: 'ana',
    current: 'antes',
    next: 'después',
  });
  // Anything that is not a password envelope is not taken.
  const bad = page(
    (msg, reply) =>
      reply({
        type: 'sytue-e2e-result',
        id: msg.id,
        ok: true,
        envelope: { ...envelope, kind: 'passkey' },
      }),
    2,
    idOrigin,
  );
  expect(
    await passwordEnvelopeInBridge(
      { idOrigin, subject: 'ana', win: bad.win as never, doc: bad.doc as never },
      { current: 'antes', next: 'después' },
    ),
  ).toEqual({ ok: false, error: 'unavailable' });
});

test('a busy bridge response keeps the handoff for the next attempt until processed', async () => {
  takeHandoff({ hash: '#sytue_unlock=abc123', pathname: '/mail/', search: '' }, undefined);
  const busy = page((msg, reply) =>
    reply({ type: 'sytue-e2e-result', id: msg.id, ok: false, error: 'busy' }),
  );
  expect(
    await unwrapInBridge(
      { idOrigin, subject: 'ana', win: busy.win as never, doc: busy.doc as never },
      { account, wrappers: [wrapper] },
    ),
  ).toEqual({ ok: false, error: 'busy' });
  expect(busy.posted[0]!.msg.handoff).toBe('abc123');

  const ok = page((msg, reply) =>
    reply({
      type: 'sytue-e2e-result',
      id: msg.id,
      ok: true,
      recovery_key: 'EsT3 4kza',
      kind: 'sytue-id',
    }),
  );
  expect(
    await unwrapInBridge(
      { idOrigin, subject: 'ana', win: ok.win as never, doc: ok.doc as never },
      { account, wrappers: [wrapper] },
    ),
  ).toEqual({ ok: true, recoveryKey: 'EsT3 4kza', kind: 'sytue-id' });
  expect(ok.posted[0]!.msg.handoff).toBe('abc123');

  const after = page((msg, reply) =>
    reply({
      type: 'sytue-e2e-result',
      id: msg.id,
      ok: true,
      recovery_key: 'EsT3 4kza',
      kind: 'sytue-id',
    }),
  );
  await unwrapInBridge(
    { idOrigin, subject: 'ana', win: after.win as never, doc: after.doc as never },
    { account, wrappers: [wrapper] },
  );
  expect(after.posted[0]!.msg.handoff).toBeUndefined();
});
