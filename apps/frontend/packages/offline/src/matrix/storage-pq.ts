/* SPDX-License-Identifier: Apache-2.0 */
import type { PqcRuntime, PqcEnvelope, PqcIdentityMaterial } from '../pqc/runtime.js';
import { type StoragePqKeys, samePrivateBytes } from './storage-pq-keys.js';
import {
  decodeStorageRecord,
  storageReference,
  StorageRecordError,
  type StorageRecord,
} from './storage.js';
import type { MatrixReference, MatrixStorageScope } from './types.js';
import { MatrixSecurityError } from './types.js';

interface Packet {
  found: boolean;
  kind?: string;
  envelope?: PqcEnvelope;
}
interface Root {
  v: 1;
  sha256: string;
  size_bytes: number;
  references: MatrixReference[];
  proof: Uint8Array;
}
interface Decoded {
  record: Uint8Array;
  proof: Uint8Array;
}
export type OpenedStoragePq = { fragment: true } | { record: StorageRecord; proof: unknown };
type ReadContent = (reference: MatrixReference) => Promise<Record<string, unknown>>;
interface StoragePqMessage extends Record<string, unknown> {
  msgtype: 'm.text';
  body: '';
}
type SendContent = (transaction: string, content: StoragePqMessage) => Promise<MatrixReference>;
const encoder = new TextEncoder();
const decoder = new TextDecoder('utf-8', { fatal: true });

/** Only transport orchestration lives here. The same Go package used by native
 * apps owns the record frames, hybrid encryption, fragments and wire JSON. */
export class StoragePq {
  constructor(
    private readonly core: PqcRuntime,
    readonly keys: StoragePqKeys,
    private readonly scope: MatrixStorageScope,
    private readonly assertOpen: () => void,
  ) {}
  private async invoke<T>(operation: string, args: Record<string, unknown>): Promise<T> {
    this.assertOpen();
    const value = await this.core.invoke<T>({ operation, scope: this.scope, ...args });
    this.assertOpen();
    return value;
  }
  private async packet(content: Record<string, unknown>): Promise<Packet> {
    const raw = encoder.encode(JSON.stringify(content));
    try {
      return await this.invoke<Packet>('storage.message.decode', { message: raw });
    } finally {
      raw.fill(0);
    }
  }
  private async message(kind: string, envelope: PqcEnvelope): Promise<StoragePqMessage> {
    const raw = await this.invoke<Uint8Array>('storage.message.encode', { kind, envelope });
    try {
      return JSON.parse(decoder.decode(raw)) as StoragePqMessage;
    } finally {
      raw.fill(0);
    }
  }
  private async fragment(
    reference: MatrixReference,
    key: PqcIdentityMaterial,
    room: string,
    digest: string,
    index: number,
    count: number,
    read: ReadContent,
  ): Promise<Uint8Array> {
    if (storageReference(reference).room_id !== room) throw new MatrixSecurityError('reference');
    const packet = await this.packet(await read(reference));
    if (!packet.found || packet.kind !== 'fragment' || packet.envelope?.key_id !== key.key_id)
      throw new StorageRecordError();
    return this.invoke<Uint8Array>('storage.fragment.open', {
      room,
      digest,
      index,
      count,
      identity: key.identity,
      envelope: packet.envelope,
    });
  }
  private async fragments(
    room: string,
    transaction: string,
    record: Uint8Array,
    proof: Uint8Array,
    key: PqcIdentityMaterial,
    send: SendContent,
    read: ReadContent,
  ): Promise<MatrixReference> {
    const encoded = await this.invoke<{
      payload: Uint8Array;
      digest: string;
      count: number;
      fragment_bytes: number;
    }>('storage.record.encode', { record });
    const references: MatrixReference[] = [];
    try {
      for (let index = 0; index < encoded.count; index++) {
        const payload = encoded.payload.subarray(
          index * encoded.fragment_bytes,
          (index + 1) * encoded.fragment_bytes,
        );
        const args = { room, digest: encoded.digest, index, count: encoded.count };
        const envelope = await this.invoke<PqcEnvelope>('storage.fragment.seal', {
          ...args,
          profile: key.profile,
          recipient: key.recipient,
          payload,
        });
        const ref = await send(
          transaction + '_pq_fragment_' + index,
          await this.message('fragment', envelope),
        );
        const accepted = await this.fragment(
          ref,
          key,
          room,
          encoded.digest,
          index,
          encoded.count,
          read,
        );
        try {
          if (!samePrivateBytes(accepted, payload)) throw new MatrixSecurityError('reference');
        } finally {
          accepted.fill(0);
        }
        references.push(storageReference(ref));
      }
      const root: Root = {
        v: 1,
        sha256: encoded.digest,
        size_bytes: encoded.payload.byteLength,
        references,
        proof,
      };
      const envelope = await this.invoke<PqcEnvelope>('storage.fragment.root.seal', {
        room,
        root,
        profile: key.profile,
        recipient: key.recipient,
      });
      return send(transaction, await this.message('fragment-root', envelope));
    } finally {
      encoded.payload.fill(0);
    }
  }
  async write(
    room: string,
    transaction: string,
    input: StorageRecord,
    proof: Uint8Array,
    send: SendContent,
    read: ReadContent,
  ): Promise<MatrixReference> {
    const record = encoder.encode(JSON.stringify(decodeStorageRecord(input, this.scope)));
    const key = await this.keys.key();
    try {
      const envelope = await this.invoke<PqcEnvelope>('storage.seal', {
        room,
        record,
        proof,
        profile: key.profile,
        recipient: key.recipient,
      });
      const fits = await this.invoke<boolean>('storage.message.fits', { kind: 'record', envelope });
      if (fits) return send(transaction, await this.message('record', envelope));
      return await this.fragments(room, transaction, record, proof, key, send, read);
    } finally {
      record.fill(0);
      key.identity.fill(0);
    }
  }
  async open(
    content: Record<string, unknown>,
    reference: MatrixReference,
    read: ReadContent,
  ): Promise<OpenedStoragePq | null> {
    // Routing only: even a null/invalid declared packet goes through the Go
    // decoder and cannot fall through to the classical legacy interpretation.
    if (!Object.prototype.hasOwnProperty.call(content, 'org.sytue.storage_pq')) return null;
    const packet = await this.packet(content);
    if (!packet.found || !packet.envelope) throw new StorageRecordError();
    if (packet.kind === 'fragment') return { fragment: true };
    const key = await this.keys.key(packet.envelope.key_id);
    let decoded: Decoded | undefined;
    let root: Root | undefined;
    const parts: Uint8Array[] = [];
    let payload: Uint8Array | undefined;
    try {
      if (packet.kind === 'record') {
        decoded = await this.invoke<Decoded>('storage.open', {
          room: reference.room_id,
          identity: key.identity,
          envelope: packet.envelope,
        });
      } else if (packet.kind === 'fragment-root') {
        root = await this.invoke<Root>('storage.fragment.root.open', {
          room: reference.room_id,
          identity: key.identity,
          envelope: packet.envelope,
        });
        for (let index = 0; index < root.references.length; index++)
          parts.push(
            await this.fragment(
              root.references[index]!,
              key,
              reference.room_id,
              root.sha256,
              index,
              root.references.length,
              read,
            ),
          );
        payload = await this.invoke<Uint8Array>('storage.fragment.assemble', {
          room: reference.room_id,
          root,
          parts,
        });
        decoded = {
          record: await this.invoke<Uint8Array>('storage.record.decode', { payload }),
          proof: root.proof,
        };
      } else throw new StorageRecordError();
      const record = decodeStorageRecord(JSON.parse(decoder.decode(decoded.record)), this.scope);
      const proof: unknown = JSON.parse(decoder.decode(decoded.proof));
      this.assertOpen();
      return { record, proof };
    } finally {
      key.identity.fill(0);
      decoded?.record.fill(0);
      decoded?.proof.fill(0);
      root?.proof.fill(0);
      payload?.fill(0);
      for (const part of parts) part.fill(0);
    }
  }
  close(): void {
    this.keys.close();
  }
  async destroy(): Promise<void> {
    await this.keys.destroy();
  }
}
