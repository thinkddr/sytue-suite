/* SPDX-License-Identifier: Apache-2.0 */
import type { PqcRuntime } from '../pqc/runtime.js';
import type { StoragePqVault } from './storage-pq-keys.js';
import { MatrixSecurityError, type MatrixStorageScope } from './types.js';

/** Values supplied by the maintained SDK, never by application message content. */
export interface MatrixSessionDescriptor {
  room_id: string;
  session_id: string;
  sender_key: string;
  sender_user: string;
  signing_key: string;
}
interface Encoded {
  id: string;
  payload: Uint8Array;
}
type Vault = Pick<StoragePqVault, 'read' | 'write' | 'assertOpen'>;
const encoder = new TextEncoder();

/** Go owns canonical framing and IDs. SSSS supplies authentication; the caller
 * must establish direct SDK/device/root provenance before invoking attest(). */
export class MatrixSessionProvenance {
  private readonly hot = new Set<string>();
  private closed = false;
  constructor(
    private readonly core: PqcRuntime,
    private readonly scope: MatrixStorageScope,
    private readonly vault: Vault,
  ) {}
  private check(): void {
    if (this.closed) throw new MatrixSecurityError('device');
    this.vault.assertOpen();
  }
  private remember(id: string): void {
    this.hot.delete(id);
    this.hot.add(id);
    // Only the in-memory lookup cache is bounded. Immutable SSSS attestations
    // are never removed or truncated, and an evicted entry is fetched again.
    if (this.hot.size > 256) this.hot.delete(this.hot.values().next().value!);
  }
  private async encode(
    session: MatrixSessionDescriptor,
  ): Promise<{ encoded: Encoded; raw: Uint8Array }> {
    this.check();
    const raw = encoder.encode(JSON.stringify(session));
    try {
      const encoded = await this.core.invoke<Encoded>({
        operation: 'matrix.session.proof.encode',
        scope: this.scope,
        session: raw,
      });
      this.check();
      return { encoded, raw };
    } catch (error) {
      raw.fill(0);
      if (error instanceof DOMException && error.name === 'AbortError') throw error;
      this.check();
      throw new MatrixSecurityError('unverified');
    }
  }
  private async matches(encoded: Encoded, raw: Uint8Array): Promise<boolean> {
    this.check();
    const stored = await this.vault.read(encoded.id);
    try {
      this.check();
      if (!stored) return false;
      if (stored.revision !== 1) throw new MatrixSecurityError('unverified');
      const matches = await this.core.invoke<boolean>({
        operation: 'matrix.session.proof.decode',
        scope: this.scope,
        session: raw,
        payload: stored.payload,
      });
      this.check();
      if (!matches) throw new MatrixSecurityError('unverified');
      this.remember(encoded.id);
      return true;
    } finally {
      stored?.payload.fill(0);
    }
  }
  async contains(session: MatrixSessionDescriptor): Promise<boolean> {
    const { encoded, raw } = await this.encode(session);
    try {
      if (this.hot.has(encoded.id)) {
        this.remember(encoded.id);
        return true;
      }
      return await this.matches(encoded, raw);
    } finally {
      encoded.payload.fill(0);
      raw.fill(0);
    }
  }
  async attest(session: MatrixSessionDescriptor): Promise<void> {
    const { encoded, raw } = await this.encode(session);
    try {
      if (this.hot.has(encoded.id)) return;
      if (await this.matches(encoded, raw)) return;
      await this.vault.write(encoded.id, encoded.payload);
      this.check();
      if (!(await this.matches(encoded, raw))) throw new MatrixSecurityError('unverified');
    } finally {
      encoded.payload.fill(0);
      raw.fill(0);
    }
  }
  close(): void {
    this.closed = true;
    this.hot.clear();
  }
}

/** Keep the established unverified category while exposing the actionable
 * distinction to the host: recovery alone cannot attest an old unknown key. */
export class MatrixSessionProvenanceMissing extends MatrixSecurityError {
  readonly requiresOriginalDevice = true;
  constructor() {
    super('unverified');
    this.name = 'MatrixSessionProvenanceMissing';
  }
}
