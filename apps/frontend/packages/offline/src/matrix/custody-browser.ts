/* SPDX-License-Identifier: Apache-2.0 */
/**
 * `MatrixKeyCustody` for the web apps: wrappers travel through the app's own BFF
 * (`<base>/api/matrix/recovery-wrappers`, shared handler in `@sytue/auth/server`) and
 * are wrapped and opened here with Web Crypto, hash-wasm's Argon2id and the WebAuthn
 * PRF extension of the person's Sytue ID passkey.
 */
import {
  MatrixCustodyError,
  type MatrixCustodyState,
  type MatrixKeyCustody,
  type MatrixUnlockFactor,
  type MatrixUnlockMethod,
} from './custody.js';
import {
  fromBase64,
  openRecoveryWrapper,
  passwordIKM,
  passwordKDF,
  sealRecoveryKey,
  toBase64,
  toBase64Url,
  type RecoveryWrapper,
} from './recovery-wrap.js';

const recoveryKeyShape = /^[1-9A-HJ-NP-Za-km-z]{4}(?: [1-9A-HJ-NP-Za-km-z]{1,4}){8,24}$/;

export interface BrowserKeyCustodyOptions {
  /** The app's base path (`/rooms`, `/drive`…): the BFF that serves this account. */
  base: string;
  /** The product's isolated storage account (identity v2); otherwise the Rooms account. */
  isolated: boolean;
  /** WebAuthn RP ID of Sytue ID (`WEBAUTHN_RP_ID`, the suite's apex domain). */
  rpId: string;
  fetchImpl?: typeof fetch;
  credentials?: Pick<CredentialsContainer, 'get'>;
}

interface PrfResults {
  prf?: { results?: { first?: BufferSource } };
}

export class BrowserKeyCustody implements MatrixKeyCustody {
  constructor(private readonly options: BrowserKeyCustodyOptions) {}

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
  private credentials(): Pick<CredentialsContainer, 'get'> | undefined {
    return (
      this.options.credentials ??
      (typeof navigator !== 'undefined' ? navigator.credentials : undefined)
    );
  }

  passkeySupported(): boolean {
    return (
      this.options.credentials !== undefined ||
      (typeof window !== 'undefined' &&
        'PublicKeyCredential' in window &&
        typeof navigator.credentials?.get === 'function')
    );
  }

  async state(
    keyIDs: readonly string[] | undefined,
    signal?: AbortSignal,
  ): Promise<MatrixCustodyState> {
    const { wrappers, legacy_escrow } = await this.wrappers(signal);
    const usable = wrappers.filter((w) => !keyIDs || keyIDs.includes(w.key_id));
    const methods = new Set<MatrixUnlockMethod>(usable.map((w) => w.kind));
    return { methods: [...methods], legacy: !this.options.isolated && legacy_escrow };
  }

  async unwrap(
    factor: MatrixUnlockFactor,
    target: { account: string; keyIDs: readonly string[] | undefined },
    signal?: AbortSignal,
  ): Promise<string> {
    const { wrappers } = await this.wrappers(signal);
    const usable = wrappers.filter(
      (w) => w.kind === factor.kind && (!target.keyIDs || target.keyIDs.includes(w.key_id)),
    );
    if (usable.length === 0) throw new MatrixCustodyError('wrong');
    if (factor.kind === 'password') {
      // Usually one; each has its own salt.
      for (const wrapper of usable) {
        if (wrapper.kdf.alg !== 'argon2id') continue;
        const ikm = await passwordIKM(factor.password, wrapper.kdf);
        try {
          return checked(await openRecoveryWrapper(wrapper, target.account, ikm));
        } catch (error) {
          if (!(error instanceof MatrixCustodyError && error.reason === 'wrong')) throw error;
        } finally {
          ikm.fill(0);
        }
      }
      throw new MatrixCustodyError('wrong');
    }
    const byCredential = new Map(usable.map((w) => [w.credential_id, w]));
    const evalByCredential: Record<string, { first: BufferSource }> = {};
    for (const wrapper of usable) {
      if (wrapper.kdf.alg !== 'webauthn-prf-hkdf-sha256') continue;
      evalByCredential[wrapper.credential_id] = { first: fromBase64(wrapper.kdf.prf_salt) };
    }
    const { credentialID, ikm } = await this.prf(
      {
        allowCredentials: [...byCredential.keys()].map((id) => ({
          type: 'public-key' as const,
          id: fromBase64(id),
        })),
        extensions: { prf: { evalByCredential } },
      },
      signal,
    );
    const wrapper = byCredential.get(credentialID);
    try {
      if (!wrapper) throw new MatrixCustodyError('wrong');
      return checked(await openRecoveryWrapper(wrapper, target.account, ikm));
    } finally {
      ikm.fill(0);
    }
  }

  async wrap(
    recoveryKey: string,
    factor: MatrixUnlockFactor,
    target: { account: string; keyID: string },
    signal?: AbortSignal,
  ): Promise<void> {
    let wrapper: RecoveryWrapper;
    if (factor.kind === 'password') {
      const kdf = {
        alg: 'argon2id' as const,
        ...passwordKDF,
        salt: toBase64(crypto.getRandomValues(new Uint8Array(16))),
      };
      const ikm = await passwordIKM(factor.password, kdf);
      try {
        wrapper = await sealRecoveryKey({
          kind: 'password',
          account: target.account,
          keyID: target.keyID,
          credentialID: '',
          kdf,
          ikm,
          recoveryKey,
        });
      } finally {
        ikm.fill(0);
      }
    } else {
      const salt = crypto.getRandomValues(new Uint8Array(32));
      // Any passkey of the person (discoverable): the one they pick is recorded.
      const { credentialID, ikm } = await this.prf(
        { extensions: { prf: { eval: { first: salt } } } },
        signal,
      );
      try {
        wrapper = await sealRecoveryKey({
          kind: 'passkey',
          account: target.account,
          keyID: target.keyID,
          credentialID,
          kdf: { alg: 'webauthn-prf-hkdf-sha256', prf_salt: toBase64(salt) },
          ikm,
          recoveryKey,
        });
      } finally {
        ikm.fill(0);
      }
    }
    let response: Response;
    try {
      response = await this.fetch(this.url('/api/matrix/recovery-wrappers'), {
        method: 'PUT',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify(wrapper),
        ...(signal ? { signal } : {}),
      });
    } catch {
      throw new MatrixCustodyError('unavailable');
    }
    if (!response.ok) throw new MatrixCustodyError('unavailable');
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

  /** One WebAuthn assertion, used ONLY for its PRF output: no server verifies it. */
  private async prf(
    request: Pick<PublicKeyCredentialRequestOptions, 'allowCredentials' | 'extensions'>,
    signal?: AbortSignal,
  ): Promise<{ credentialID: string; ikm: Uint8Array<ArrayBuffer> }> {
    const credentials = this.credentials();
    if (!credentials) throw new MatrixCustodyError('unsupported');
    let credential: Credential | null;
    try {
      credential = await credentials.get({
        publicKey: {
          challenge: crypto.getRandomValues(new Uint8Array(32)),
          rpId: this.options.rpId,
          userVerification: 'required',
          ...request,
        },
        ...(signal ? { signal } : {}),
      });
    } catch (error) {
      throw new MatrixCustodyError(
        error instanceof DOMException && error.name === 'NotAllowedError'
          ? 'cancelled'
          : 'unsupported',
      );
    }
    const assertion = credential as
      (Credential & { rawId: ArrayBuffer; getClientExtensionResults: () => PrfResults }) | null;
    const first = assertion?.getClientExtensionResults().prf?.results?.first;
    if (!assertion || !first) throw new MatrixCustodyError('unsupported');
    const view =
      first instanceof ArrayBuffer
        ? new Uint8Array(first)
        : new Uint8Array(first.buffer, first.byteOffset, first.byteLength);
    const bytes = view.slice();
    if (bytes.length !== 32) throw new MatrixCustodyError('unsupported');
    return { credentialID: toBase64Url(new Uint8Array(assertion.rawId)), ikm: bytes };
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
    (w.kind === 'password' || w.kind === 'passkey') &&
    typeof w.key_id === 'string' &&
    typeof w.credential_id === 'string' &&
    typeof w.nonce === 'string' &&
    typeof w.ciphertext === 'string' &&
    !!w.kdf &&
    ((w.kind === 'password' && w.kdf.alg === 'argon2id') ||
      (w.kind === 'passkey' && w.kdf.alg === 'webauthn-prf-hkdf-sha256'))
  );
}
