/* SPDX-License-Identifier: Apache-2.0 */
/**
 * `MatrixKeyCustody` for the web apps (ADR 0084, 0085): wrappers travel through the app's
 * own BFF (`<base>/api/matrix/recovery-wrappers`, shared handler in `@sytue/auth/server`)
 * and are wrapped and opened by the Sytue ID bridge, in Sytue ID's origin, with the unlock
 * key that the person's Sytue ID sign-in left in this browser. This page never holds the
 * unlock key: it gets the recovery key of its own account back, which the SDK needs anyway.
 */
import {
  MatrixCustodyError,
  type MatrixCustodyState,
  type MatrixKeyCustody,
  type MatrixUnlockMethod,
} from './custody.js';
import { fromBase64, passwordKDF, toBase64Url, type RecoveryWrapper } from './recovery-wrap.js';
import {
  bridgeHandoffKey,
  forgetLegacyMaterial,
  unwrapInBridge,
  wrapInBridge,
  type UnlockBridgeError,
  type UnwrapResult,
  type WrapResult,
} from './unlock-bridge.js';

const recoveryKeyShape = /^[1-9A-HJ-NP-Za-km-z]{4}(?: [1-9A-HJ-NP-Za-km-z]{1,4}){8,24}$/;

export interface BrowserKeyCustodyOptions {
  /** The app's base path (`/rooms`, `/drive`…): the BFF that serves this account. */
  base: string;
  /** The product's isolated storage account (identity v2); otherwise the Rooms account. */
  isolated: boolean;
  /** Sytue ID (`https://id.<apex>`), whose bridge holds the unlock key. */
  idOrigin: string;
  fetchImpl?: typeof fetch;
  /** Test seam; the Sytue ID bridge by default. */
  bridge?: CustodyBridge;
}

/** What the custody asks of the Sytue ID bridge: operations, never the unlock key. */
export interface CustodyBridge {
  unwrap: (
    subject: string,
    input: { account: string; wrappers: readonly RecoveryWrapper[] },
  ) => Promise<UnwrapResult>;
  wrap: (
    subject: string,
    input: { account: string; keyID: string; recoveryKey: string },
  ) => Promise<WrapResult>;
  forgetLegacy: (subject: string) => Promise<void>;
}

function bridgeError(error: UnlockBridgeError): MatrixCustodyError {
  // No answer from the bridge is what it always was: this browser cannot unlock, so the
  // person signs in to Sytue ID again. `account` and `busy` refuse this request only: the
  // person must not be sent to sign in for them.
  if (error === 'signin' || error === 'unavailable') return new MatrixCustodyError('signin');
  return new MatrixCustodyError(error === 'wrong' ? 'wrong' : 'unavailable');
}

/** The ADR 0082 wrappers the Sytue ID sign-in page can open (`sytue_legacy`). */
function olderRequest(wrappers: readonly RecoveryWrapper[]): string {
  const items: string[] = [];
  for (const w of wrappers) {
    if (
      w.kind === 'password' &&
      w.kdf.alg === 'argon2id' &&
      w.kdf.m === passwordKDF.m &&
      w.kdf.t === passwordKDF.t &&
      w.kdf.p === passwordKDF.p
    )
      items.push('p.' + toBase64Url(fromBase64(w.kdf.salt)));
    else if (w.kind === 'passkey' && w.kdf.alg === 'webauthn-prf-hkdf-sha256')
      items.push('k.' + w.credential_id + '.' + toBase64Url(fromBase64(w.kdf.prf_salt)));
  }
  return items.slice(0, 4).join('~');
}

export class BrowserKeyCustody implements MatrixKeyCustody {
  private readonly bridge: CustodyBridge;

  constructor(private readonly options: BrowserKeyCustodyOptions) {
    const idOrigin = options.idOrigin;
    this.bridge = options.bridge ?? {
      unwrap: (subject, input) => unwrapInBridge({ idOrigin, subject }, input),
      wrap: (subject, input) => wrapInBridge({ idOrigin, subject }, input),
      forgetLegacy: (subject) => forgetLegacyMaterial({ idOrigin, subject }),
    };
  }

  private url(path: string): string {
    return (
      this.options.base.replace(/\/$/, '') +
      path +
      (this.options.isolated && path === '/api/matrix/recovery-wrappers'
        ? '?identity_version=2'
        : '')
    );
  }
  private fetch(input: string, init: RequestInit): Promise<Response> {
    return (this.options.fetchImpl ?? globalThis.fetch)(input, {
      credentials: 'same-origin',
      cache: 'no-store',
      ...init,
    });
  }
  private async wrappers(signal?: AbortSignal): Promise<{
    wrappers: RecoveryWrapper[];
    legacy_escrow: boolean;
  }> {
    let response: Response;
    try {
      response = await this.fetch(
        this.url('/api/matrix/recovery-wrappers'),
        signal ? { signal } : {},
      );
    } catch {
      throw new MatrixCustodyError('unavailable');
    }
    if (!response.ok) throw new MatrixCustodyError('unavailable');
    const body = (await response.json().catch(() => null)) as {
      wrappers?: unknown;
      legacy_escrow?: unknown;
    } | null;
    if (!body || !Array.isArray(body.wrappers)) throw new MatrixCustodyError('unavailable');
    return {
      wrappers: body.wrappers.filter(validWrapper),
      legacy_escrow: body.legacy_escrow === true,
    };
  }
  async state(
    keyIDs: readonly string[] | undefined,
    signal?: AbortSignal,
  ): Promise<MatrixCustodyState> {
    const { wrappers, legacy_escrow } = await this.wrappers(signal);
    const usable = wrappers.filter((w) => !keyIDs || keyIDs.includes(w.key_id));
    const methods = new Set<MatrixUnlockMethod>(usable.map((w) => w.kind));
    return {
      methods: [...methods],
      legacy: !this.options.isolated && legacy_escrow,
      older: methods.has('sytue-id') ? '' : olderRequest(usable),
    };
  }

  async unwrap(
    target: { account: string; subject: string; keyIDs: readonly string[] | undefined },
    signal?: AbortSignal,
  ): Promise<string> {
    const { wrappers } = await this.wrappers(signal);
    const usable = wrappers.filter((w) => !target.keyIDs || target.keyIDs.includes(w.key_id));
    const opened = await this.bridge
      .unwrap(target.subject, { account: target.account, wrappers: usable })
      .catch((): UnwrapResult => ({ ok: false, error: 'unavailable' }));
    if (!opened.ok) throw bridgeError(opened.error);
    return checked(opened.recoveryKey);
  }

  async wrap(
    recoveryKey: string,
    target: { account: string; subject: string; keyID: string },
    signal?: AbortSignal,
  ): Promise<void> {
    const sealed = await this.bridge
      .wrap(target.subject, { account: target.account, keyID: target.keyID, recoveryKey })
      .catch((): WrapResult => ({ ok: false, error: 'unavailable' }));
    if (!sealed.ok) throw bridgeError(sealed.error);
    let response: Response;
    try {
      response = await this.fetch(this.url('/api/matrix/recovery-wrappers'), {
        method: 'PUT',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify(sealed.wrapper),
        ...(signal ? { signal } : {}),
      });
    } catch {
      throw new MatrixCustodyError('unavailable');
    }
    if (!response.ok) throw new MatrixCustodyError('unavailable');
    // Moved to the new kind: what opened the old wrappers is not needed any more.
    if (sealed.legacy) await this.bridge.forgetLegacy(target.subject).catch(() => undefined);
  }

  async retireOlder(
    target: { account: string; subject: string; keyID: string },
    signal?: AbortSignal,
  ): Promise<boolean> {
    const { wrappers } = await this.wrappers(signal);
    const ofKey = wrappers.filter((w) => w.key_id === target.keyID);
    const current = ofKey.find((w) => w.kind === 'sytue-id');
    if (!current || !ofKey.some((w) => w.kind !== 'sytue-id')) return false;
    // Checked HERE first: that very wrapper opens to a recovery key with the unlock key.
    const opened = await this.bridge
      .unwrap(target.subject, { account: target.account, wrappers: [current] })
      .catch((): UnwrapResult => ({ ok: false, error: 'unavailable' }));
    if (!opened.ok || opened.kind !== 'sytue-id' || !recoveryKeyShape.test(opened.recoveryKey))
      return false;
    const body: Partial<RecoveryWrapper> = { ...current };
    delete body.id;
    delete body.created_at;
    const url = this.url('/api/matrix/recovery-wrappers');
    let response: Response;
    try {
      response = await this.fetch(url + (url.includes('?') ? '&' : '?') + 'retire_older=1', {
        method: 'PUT',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify(body),
        ...(signal ? { signal } : {}),
      });
    } catch {
      return false;
    }
    return response.ok;
  }

  async legacyRecoveryKey(signal?: AbortSignal): Promise<string | null> {
    if (this.options.isolated) return null;
    try {
      const response = await this.fetch(
        this.url('/api/matrix/recovery-key'),
        signal ? { signal } : {},
      );
      if (!response.ok) return null;
      const { recovery_key } = (await response.json()) as { recovery_key?: unknown };
      return typeof recovery_key === 'string' && recoveryKeyShape.test(recovery_key)
        ? recovery_key
        : null;
    } catch {
      return null;
    }
  }

  /**
   * The app's own login (`/auth/login`) with `unlock=1`: its BFF asks Sytue ID for a
   * fresh sign-in (`prompt=login`), which leaves the unlock key in this browser.
   */
  signInURL(returnTo: string, older: string): string {
    const params = new URLSearchParams({ unlock: '1', returnTo });
    if (older) params.set('legacy', older);
    // The bridge's one-time key: the relay seals the unlock key for it (unlock-bridge.ts).
    const key = bridgeHandoffKey();
    if (key) params.set('key', key);
    return `${this.options.base.replace(/\/$/, '')}/auth/login?${params.toString()}`;
  }
}

function checked(key: string): string {
  if (!recoveryKeyShape.test(key)) throw new MatrixCustodyError('wrong');
  return key;
}

function validWrapper(value: unknown): value is RecoveryWrapper {
  const w = value as Partial<RecoveryWrapper> | null;
  return (
    !!w &&
    w.v === 1 &&
    (w.kind === 'sytue-id' || w.kind === 'password' || w.kind === 'passkey') &&
    typeof w.key_id === 'string' &&
    typeof w.credential_id === 'string' &&
    typeof w.nonce === 'string' &&
    typeof w.ciphertext === 'string' &&
    !!w.kdf &&
    ((w.kind === 'sytue-id' &&
      w.kdf.alg === 'sytue-id-unlock-hkdf-sha256' &&
      typeof w.kdf.hint === 'string') ||
      (w.kind === 'password' && w.kdf.alg === 'argon2id') ||
      (w.kind === 'passkey' && w.kdf.alg === 'webauthn-prf-hkdf-sha256'))
  );
}
