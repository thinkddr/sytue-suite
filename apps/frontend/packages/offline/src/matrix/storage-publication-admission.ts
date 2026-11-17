/* SPDX-License-Identifier: Apache-2.0 */
import type { MatrixEvent } from 'matrix-js-sdk';
import type { PqcRuntime } from '../pqc/runtime.js';
import { encodeHostJSON } from '../pqc/host-codec.js';
import type { MatrixStorageScope, MatrixReference } from './types.js';
import { MatrixSecurityError } from './types.js';
import type { StoragePublicationLease } from './storage-publication-jobs.js';
export class StoragePublicationPending extends Error {
  readonly code = 'publication_pending';
  constructor(readonly status: string) {
    super('matrix_publication_pending');
    this.name = 'StoragePublicationPending';
  }
}
export interface StorageAdmissionContext {
  readonly scope: Readonly<MatrixStorageScope>;
  readonly userID: string;
  readonly deviceID: string;
  readonly rootKey: string;
  readonly runtime: Pick<PqcRuntime, 'invoke'>;
  revalidate: (signal: AbortSignal) => Promise<void>;
  authority: (signal: AbortSignal) => Promise<Uint8Array<ArrayBuffer>>;
  reserve: (
    signed: Uint8Array<ArrayBuffer>,
    signal: AbortSignal,
  ) => Promise<Uint8Array<ArrayBuffer>>;
  sign: (payload: Record<string, unknown>) => Promise<Record<string, unknown>>;
  recovery: (
    roomID: string,
    transaction: string,
    signal: AbortSignal,
  ) => Promise<Uint8Array<ArrayBuffer> | undefined>;
  accepted: (reference: MatrixReference, signal: AbortSignal) => Promise<MatrixEvent>;
}
interface Marker {
  v: number;
  id: string;
}
interface Prepared {
  readonly payload: Uint8Array<ArrayBuffer>;
  readonly marker: Marker;
  readonly wireSHA: string;
  readonly rootKey: string;
  readonly generation: string;
}
interface Retained {
  readonly event: MatrixEvent;
  readonly lease: StoragePublicationLease;
  readonly buffers: Set<Uint8Array<ArrayBuffer>>;
  promise?: Promise<Prepared>;
  closed: boolean;
}
const markerName = 'org.sytue.publication.admission';
/** Thin host adapter: commitments, codecs and marker validation run in the SAME
 * Go/Wasm core. Only the maintained SDK signs. No REST state becomes authority.
 * This cache is deliberately limited to one live original SDK Entry/Event;
 * restart ACK resolution uses the durable ledger, not this in-memory permit. */
export class StoragePublicationAdmission {
  private readonly entries = new WeakMap<object, Retained>();
  constructor(private readonly context: StorageAdmissionContext) {}
  /** Public ledger state is only a hint. An accepted entry is fetched through
   * the bound SDK and its exact wire is checked before returning a reference.
   * The original Go reader compares the complete logical record before WAL ACK. */
  async recoverBeforeSend(
    roomID: string,
    transaction: string,
    signal: AbortSignal,
  ): Promise<MatrixReference | undefined> {
    const ctx = this.context;
    signal.throwIfAborted();
    await ctx.revalidate(signal);
    const authority = await ctx.authority(signal);
    try {
      const active = await ctx.runtime.invoke<{ root_key: string; user_id: string }>(
        { operation: 'publication.authority.decode', scope: ctx.scope, authority },
        signal,
      );
      if (active.root_key !== ctx.rootKey || active.user_id !== ctx.userID)
        throw new MatrixSecurityError('identity');
    } finally {
      authority.fill(0);
    }
    const raw = await ctx.recovery(roomID, transaction, signal);
    if (raw === undefined) {
      // Absence grants no send. A candidate still needs the atomic exact reserve
      // in beforeHTTP; a retained/conflicting request cannot pass that CAS.
      await ctx.revalidate(signal);
      return undefined;
    }
    let hint: {
      status: string;
      cipher_sha256: string;
      device_id: string;
      reference?: MatrixReference;
    };
    try {
      hint = await ctx.runtime.invoke(
        {
          operation: 'publication.recovery.decode',
          scope: ctx.scope,
          room_id: roomID,
          transaction_id: transaction,
          recovery: raw,
        },
        signal,
      );
    } finally {
      raw.fill(0);
    }
    if (hint.status !== 'accepted' || !hint.reference)
      throw new StoragePublicationPending(hint.status);
    const reference = hint.reference;
    if (reference.room_id !== roomID) throw new MatrixSecurityError('reference');
    await ctx.revalidate(signal);
    const event = await ctx.accepted(reference, signal);
    if (
      event.getId() !== reference.event_id ||
      event.getRoomId() !== roomID ||
      event.getSender() !== ctx.userID ||
      event.getWireType() !== 'm.room.encrypted' ||
      event.getWireContent().device_id !== hint.device_id
    )
      throw new MatrixSecurityError('identity');
    const wire = encodeHostJSON(event.getWireContent());
    try {
      const actual = await ctx.runtime.invoke<{ sha256: string }>(
        { operation: 'publication.wire.digest', wire, require_marker: true },
        signal,
      );
      if (actual.sha256 !== hint.cipher_sha256) throw new MatrixSecurityError('reference');
    } finally {
      wire.fill(0);
    }
    await ctx.revalidate(signal);
    signal.throwIfAborted();
    return Object.freeze({ ...reference });
  }
  private check(event: MatrixEvent, lease: StoragePublicationLease): void {
    lease.assertOpen();
    lease.signal.throwIfAborted();
    if (
      event.getWireType() !== 'm.room.encrypted' ||
      event.getRoomId() !== lease.target.room_id ||
      event.getTxnId() !== lease.transaction ||
      lease.target.device_id !== this.context.deviceID
    )
      throw new MatrixSecurityError('identity');
  }
  private async digest(
    event: MatrixEvent,
    requireMarker: boolean,
    signal: AbortSignal,
  ): Promise<{ sha256: string; marker: Marker }> {
    const wire = encodeHostJSON(event.getWireContent());
    try {
      return await this.context.runtime.invoke(
        { operation: 'publication.wire.digest', wire, require_marker: requireMarker },
        signal,
      );
    } finally {
      wire.fill(0);
    }
  }
  private async prepare(retained: Retained): Promise<Prepared> {
    const { event, lease } = retained,
      ctx = this.context,
      signal = lease.signal;
    this.check(event, lease);
    await ctx.revalidate(signal);
    this.check(event, lease);
    const wire = encodeHostJSON(event.getWireContent()),
      authority = await ctx.authority(signal);
    let payload: Uint8Array<ArrayBuffer> | undefined,
      signedRaw: Uint8Array<ArrayBuffer> | undefined,
      receiptRaw: Uint8Array<ArrayBuffer> | undefined;
    try {
      const digest = await this.digest(event, false, signal);
      const prepared = await ctx.runtime.invoke<{
        payload: Uint8Array<ArrayBuffer>;
        request_sha256: string;
        root_key: string;
        user_id: string;
        generation: string;
      }>(
        {
          operation: 'publication.request.build',
          scope: ctx.scope,
          target: lease.target,
          transaction: lease.transaction,
          wire,
          authority,
        },
        signal,
      );
      payload = prepared.payload;
      retained.buffers.add(payload);
      if (
        prepared.root_key !== ctx.rootKey ||
        prepared.user_id !== ctx.userID ||
        !/^[1-9][0-9]*$/.test(prepared.generation)
      )
        throw new MatrixSecurityError('identity');
      this.check(event, lease);
      await ctx.revalidate(signal);
      this.check(event, lease);
      const request = JSON.parse(
        new TextDecoder('utf-8', { fatal: true }).decode(payload),
      ) as Record<string, unknown>;
      const signed = await ctx.sign(request);
      this.check(event, lease);
      signedRaw = encodeHostJSON(signed);
      const exact = await ctx.runtime.invoke<{ sha256: string; payload: Uint8Array<ArrayBuffer> }>(
        { operation: 'publication.request.encode', scope: ctx.scope, request: signedRaw },
        signal,
      );
      try {
        if (exact.sha256 !== prepared.request_sha256) throw new MatrixSecurityError('identity');
      } finally {
        exact.payload.fill(0);
      }
      await ctx.revalidate(signal);
      this.check(event, lease);
      const unchanged = await this.digest(event, false, signal);
      if (unchanged.sha256 !== digest.sha256) throw new MatrixSecurityError('identity');
      receiptRaw = await ctx.reserve(signedRaw, signal);
      this.check(event, lease);
      const receipt = await ctx.runtime.invoke<{
        status: string;
        request_sha256: string;
        reserved_marker?: Marker;
      }>(
        {
          operation: 'publication.receipt.decode',
          scope: ctx.scope,
          request: payload,
          receipt: receiptRaw,
        },
        signal,
      );
      if (!receipt.reserved_marker) throw new StoragePublicationPending(receipt.status);
      await ctx.revalidate(signal);
      this.check(event, lease);
      return {
        payload,
        marker: Object.freeze({ ...receipt.reserved_marker }),
        wireSHA: digest.sha256,
        rootKey: prepared.root_key,
        generation: prepared.generation,
      };
    } finally {
      wire.fill(0);
      authority.fill(0);
      signedRaw?.fill(0);
      receiptRaw?.fill(0);
    }
  }
  async beforeHTTP(event: MatrixEvent, lease: Readonly<StoragePublicationLease>): Promise<void> {
    this.check(event, lease);
    let retained = this.entries.get(lease.token);
    if (!retained) {
      retained = { event, lease, buffers: new Set(), closed: false };
      this.entries.set(lease.token, retained);
      const current = retained;
      lease.onRelease(() => {
        current.closed = true;
        for (const bytes of current.buffers) bytes.fill(0);
        current.buffers.clear();
        this.entries.delete(lease.token);
      });
      retained.promise = this.prepare(retained);
      void retained.promise.catch(() => {});
    }
    if (retained.closed || retained.event !== event || retained.lease.token !== lease.token)
      throw new MatrixSecurityError('identity');
    const pending = retained.promise;
    if (!pending) throw new MatrixSecurityError('device');
    const prepared = await pending;
    this.check(event, lease);
    await this.context.revalidate(lease.signal);
    this.check(event, lease);
    const wire = encodeHostJSON(event.getWireContent());
    let attached: Uint8Array<ArrayBuffer> | undefined;
    try {
      const digest = await this.digest(event, false, lease.signal);
      if (digest.sha256 !== prepared.wireSHA) throw new MatrixSecurityError('identity');
      const result = await this.context.runtime.invoke<{ wire: Uint8Array<ArrayBuffer> }>(
        { operation: 'publication.wire.attach', wire, marker: prepared.marker },
        lease.signal,
      );
      attached = result.wire;
      this.check(event, lease);
      const marked = JSON.parse(
        new TextDecoder('utf-8', { fatal: true }).decode(attached),
      ) as Record<string, unknown>;
      // Mutate only the validated extension of the public SDK wire object. Calling
      // makeEncrypted twice would replace its retained clearEvent and break views.
      event.getWireContent()[markerName] = marked[markerName];
      const final = await this.digest(event, true, lease.signal);
      if (
        final.sha256 !== prepared.wireSHA ||
        final.marker.v !== prepared.marker.v ||
        final.marker.id !== prepared.marker.id
      )
        throw new MatrixSecurityError('identity');
      this.check(event, lease);
    } finally {
      wire.fill(0);
      attached?.fill(0);
    }
  }
}
