/* SPDX-License-Identifier: Apache-2.0 */
/**
 * The recovery key of a Matrix account, wrapped IN THIS BROWSER under a key only the
 * person can produce (ADR 0081): the PRF output of their Sytue ID passkey, or Argon2id
 * of their password. The service stores the wrapped blob and never sees the key, the
 * password or the PRF output, so the operator cannot open it.
 *
 * The person never sees, saves or types the recovery key itself: the shared gate asks
 * for the passkey or the password and the key goes straight from here to the SDK.
 * This is the port the gate works against; `BrowserKeyCustody` is the implementation.
 */

export type MatrixUnlockMethod = 'passkey' | 'password';

/** What the person gives to wrap or unwrap. Never stored, never sent to the service. */
export type MatrixUnlockFactor = { kind: 'passkey' } | { kind: 'password'; password: string };

export class MatrixCustodyError extends Error {
  constructor(
    /**
     * - `wrong`: the passkey or password does not open the blob; try again, nothing is
     *   locked out.
     * - `cancelled`: the person dismissed the system passkey dialog.
     * - `unsupported`: this passkey or browser has no PRF; the password still works.
     * - `unavailable`: the service could not be reached or refused to store the blob.
     */
    public readonly reason: 'wrong' | 'cancelled' | 'unsupported' | 'unavailable',
  ) {
    super(`matrix custody: ${reason}`);
    this.name = 'MatrixCustodyError';
  }
}

export interface MatrixCustodyState {
  /** Methods with a wrapper for one of the account's current keys. */
  methods: readonly MatrixUnlockMethod[];
  /** The retired service-held key (Rooms account only) is still there to migrate. */
  legacy: boolean;
}

/** `account` is the bootstrap's matrix_user_id: it is bound into the AEAD data. */
export interface MatrixKeyCustody {
  /** Rejects `unavailable` when the service cannot answer (not the same as «nothing»). */
  state: (
    keyIDs: readonly string[] | undefined,
    signal?: AbortSignal,
  ) => Promise<MatrixCustodyState>;
  /** Unwraps in this browser. Rejects `wrong`, `cancelled`, `unsupported` or `unavailable`. */
  unwrap: (
    factor: MatrixUnlockFactor,
    target: { account: string; keyIDs: readonly string[] | undefined },
    signal?: AbortSignal,
  ) => Promise<string>;
  /**
   * Wraps in this browser and uploads. Resolves ONLY after the service confirmed it
   * stored the blob: the caller publishes the key as the account's secret storage key
   * after this, never before.
   */
  wrap: (
    recoveryKey: string,
    factor: MatrixUnlockFactor,
    target: { account: string; keyID: string },
    signal?: AbortSignal,
  ) => Promise<void>;
  /** Migration only: the key the service still holds from before wrapping, or null. */
  legacyRecoveryKey: (signal?: AbortSignal) => Promise<string | null>;
  /** Whether this browser can offer the passkey (WebAuthn with the PRF extension). */
  passkeySupported: () => boolean;
}
