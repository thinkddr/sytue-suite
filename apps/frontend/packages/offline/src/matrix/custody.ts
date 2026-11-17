/* SPDX-License-Identifier: Apache-2.0 */
/**
 * The recovery key of a Matrix account, wrapped IN THIS BROWSER under the unlock key of
 * the person's Sytue ID sign-in (ADR 0084). The person only ever types their password
 * (or uses their passkey) in Sytue ID: that sign-in leaves the unlock key in this
 * browser, and the shared gate unwraps and wraps with it without asking anything. The
 * service stores wrapped blobs only; neither it nor Sytue ID can open them.
 *
 * This is the port the gate works against; `BrowserKeyCustody` is the implementation.
 */

/** Kinds of wrapper an account can have: `sytue-id` (ADR 0084) and the older ADR 0082 ones. */
export type MatrixUnlockMethod = 'sytue-id' | 'password' | 'passkey';

export class MatrixCustodyError extends Error {
  constructor(
    /**
     * - `signin`: this browser has no unlock key from Sytue ID for this person, or it
     *   does not open the account: the person signs in to Sytue ID again (never a
     *   password field here).
     * - `wrong`: nothing here opens the wrapper (it was sealed with another unlock key).
     * - `unavailable`: the service could not be reached or refused to store the blob.
     */
    public readonly reason: 'signin' | 'wrong' | 'unavailable',
  ) {
    super(`matrix custody: ${reason}`);
    this.name = 'MatrixCustodyError';
  }
}

export interface MatrixCustodyState {
  /** Kinds with a wrapper for one of the account's current keys. */
  methods: readonly MatrixUnlockMethod[];
  /** The retired service-held key (Rooms account only) is still there to migrate. */
  legacy: boolean;
  /**
   * The public parameters of the account's ADR 0082 wrappers (`sytue_legacy`), so the
   * Sytue ID sign-in page can open them once; '' when there are none.
   */
  older: string;
}

/**
 * `account` is the bootstrap's matrix_user_id (bound into the AEAD data); `subject` is
 * the Sytue ID subject whose unlock key is used (the bootstrap's actor).
 */
export interface MatrixKeyCustody {
  /** Rejects `unavailable` when the service cannot answer (not the same as «nothing»). */
  state: (
    keyIDs: readonly string[] | undefined,
    signal?: AbortSignal,
  ) => Promise<MatrixCustodyState>;
  /**
   * Unwraps in this browser with the unlock key, or once with what the Sytue ID sign-in
   * page computed for an ADR 0082 wrapper. Rejects `signin` when there is nothing here
   * that opens it.
   */
  unwrap: (
    target: { account: string; subject: string; keyIDs: readonly string[] | undefined },
    signal?: AbortSignal,
  ) => Promise<string>;
  /**
   * Wraps in this browser with the unlock key and uploads. Resolves ONLY after the
   * service confirmed it stored the blob: the caller publishes the key as the account's
   * secret storage key after this, never before. Rejects `signin` without a key.
   */
  wrap: (
    recoveryKey: string,
    target: { account: string; subject: string; keyID: string },
    signal?: AbortSignal,
  ) => Promise<void>;
  /** Migration only: the key the service still holds from before wrapping, or null. */
  legacyRecoveryKey: (signal?: AbortSignal) => Promise<string | null>;
  /** Where to send the person to sign in to Sytue ID again to unlock this device. */
  signInURL: (returnTo: string, older: string) => string;
}
