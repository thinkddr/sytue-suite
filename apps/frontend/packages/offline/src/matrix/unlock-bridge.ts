/* SPDX-License-Identifier: Apache-2.0 */
/**
 * The Sytue ID bridge (ADR 0084, 0085): a hidden iframe of `id.<apex>` that answers only
 * this origin, by postMessage. It holds the unlock key of the person's Sytue ID sign-in in
 * this browser as a non-extractable key and never hands it out: this page asks it to
 * open (or seal) the recovery-key wrapper of one of the person's Matrix accounts and gets
 * only that result back. No app, and no script injected into one, can read the unlock
 * key; no server ever sees it.
 *
 * One sign-in in Sytue ID unlocks every app of this browser: the suite's subdomains are
 * one site, so the iframe sees the storage the Sytue ID sign-in page wrote.
 */
import type { RecoveryWrapper } from './recovery-wrap.js';

/** The bridge protocol this page speaks; an older bridge (which handed the key out) is not used. */
export const unlockBridgeVersion = 2;

/**
 * - `signin`: this browser has no unlock key from Sytue ID for this person.
 * - `wrong`: the account's wrappers were sealed with an unlock key the person no longer has.
 * - `account`: the bridge refuses this account for this app (not the person's, or another product's).
 * - `busy`: too many operations in a second.
 * - `unavailable`: no answer, an older bridge, or the operation failed.
 */
export type UnlockBridgeError = 'signin' | 'wrong' | 'account' | 'busy' | 'unavailable';

export interface UnlockBridgeOptions {
  /** `https://id.<apex>`. */
  idOrigin: string;
  /** The Sytue ID subject whose key is used. */
  subject: string;
  timeoutMs?: number;
  /** Test seams; the page's own by default. */
  win?: Pick<Window, 'addEventListener' | 'removeEventListener' | 'location'>;
  doc?: Pick<Document, 'createElement' | 'body'>;
}

type Answer = { ok: true; data: Record<string, unknown> } | { ok: false; error: UnlockBridgeError };

const errors: readonly UnlockBridgeError[] = ['signin', 'wrong', 'account', 'busy', 'unavailable'];

/** Asks the bridge once, in an iframe of its own that is removed afterwards. */
function ask(options: UnlockBridgeOptions, request: Record<string, unknown>): Promise<Answer> {
  const unavailable: Answer = { ok: false, error: 'unavailable' };
  const win = options.win ?? (typeof window !== 'undefined' ? window : undefined);
  const doc = options.doc ?? (typeof document !== 'undefined' ? document : undefined);
  if (!win || !doc?.body) return Promise.resolve(unavailable);
  let idOrigin: string;
  try {
    idOrigin = new URL(options.idOrigin).origin;
  } catch {
    return Promise.resolve(unavailable);
  }
  return new Promise((resolve) => {
    const frame = doc.createElement('iframe');
    const id = crypto.randomUUID();
    let done = false;
    const finish = (value: Answer) => {
      if (done) return;
      done = true;
      clearTimeout(timer);
      win.removeEventListener('message', listen);
      frame.remove();
      resolve(value);
    };
    // Only the bridge's own window, from Sytue ID's exact origin, is listened to.
    const listen = (event: MessageEvent) => {
      if (event.origin !== idOrigin || event.source !== frame.contentWindow) return;
      const data = event.data as Record<string, unknown> | null;
      if (!data || typeof data !== 'object') return;
      if (data.type === 'sytue-e2e-ready') {
        // A bridge from before ADR 0085 would hand the unlock key out: not spoken to.
        if (data.v !== unlockBridgeVersion) {
          finish(unavailable);
          return;
        }
        frame.contentWindow?.postMessage({ ...request, id, sub: options.subject }, idOrigin);
      } else if (data.type === 'sytue-e2e-result' && data.id === id) {
        if (data.ok === true) finish({ ok: true, data });
        else
          finish({
            ok: false,
            error: errors.includes(data.error as UnlockBridgeError)
              ? (data.error as UnlockBridgeError)
              : 'unavailable',
          });
      }
    };
    const timer = setTimeout(() => finish(unavailable), options.timeoutMs ?? 8000);
    win.addEventListener('message', listen);
    frame.hidden = true;
    frame.setAttribute('aria-hidden', 'true');
    frame.tabIndex = -1;
    frame.src = `${idOrigin}/e2e/bridge?o=${encodeURIComponent(win.location.origin)}`;
    doc.body.appendChild(frame);
  });
}

export type UnwrapResult =
  | { ok: true; recoveryKey: string; kind: RecoveryWrapper['kind'] }
  | { ok: false; error: UnlockBridgeError };

/** Opens one of the account's wrappers in Sytue ID's origin; only the recovery key comes back. */
export async function unwrapInBridge(
  options: UnlockBridgeOptions,
  input: { account: string; wrappers: readonly RecoveryWrapper[] },
): Promise<UnwrapResult> {
  const answer = await ask(options, {
    type: 'sytue-e2e-unwrap',
    account: input.account,
    wrappers: input.wrappers.slice(0, 32),
  });
  if (!answer.ok) return answer;
  const { recovery_key: recoveryKey, kind } = answer.data;
  if (
    typeof recoveryKey !== 'string' ||
    (kind !== 'sytue-id' && kind !== 'password' && kind !== 'passkey')
  )
    return { ok: false, error: 'unavailable' };
  return { ok: true, recoveryKey, kind };
}

export type WrapResult =
  { ok: true; wrapper: RecoveryWrapper; legacy: boolean } | { ok: false; error: UnlockBridgeError };

/** Seals the account's recovery key in Sytue ID's origin; only the (public) wrapper comes back. */
export async function wrapInBridge(
  options: UnlockBridgeOptions,
  input: { account: string; keyID: string; recoveryKey: string },
): Promise<WrapResult> {
  const answer = await ask(options, {
    type: 'sytue-e2e-wrap',
    account: input.account,
    key_id: input.keyID,
    recovery_key: input.recoveryKey,
  });
  if (!answer.ok) return answer;
  const wrapper = answer.data.wrapper as RecoveryWrapper | undefined;
  if (
    !wrapper ||
    wrapper.v !== 1 ||
    wrapper.kind !== 'sytue-id' ||
    wrapper.key_id !== input.keyID ||
    wrapper.credential_id !== '' ||
    wrapper.kdf?.alg !== 'sytue-id-unlock-hkdf-sha256' ||
    typeof wrapper.kdf.hint !== 'string' ||
    typeof wrapper.nonce !== 'string' ||
    typeof wrapper.ciphertext !== 'string'
  )
    return { ok: false, error: 'unavailable' };
  return {
    ok: true,
    wrapper: {
      v: 1,
      kind: 'sytue-id',
      key_id: wrapper.key_id,
      credential_id: '',
      kdf: { alg: 'sytue-id-unlock-hkdf-sha256', hint: wrapper.kdf.hint },
      nonce: wrapper.nonce,
      ciphertext: wrapper.ciphertext,
    },
    legacy: answer.data.legacy === true,
  };
}

/** Drops what opens the ADR 0082 wrappers once they have been moved to the new kind. */
export async function forgetLegacyMaterial(options: UnlockBridgeOptions): Promise<void> {
  await ask(options, { type: 'sytue-e2e-forget-legacy' });
}
