/* SPDX-License-Identifier: Apache-2.0 */
import { describe, expect, test } from 'bun:test';
import type { PqcEnvelope, PqcRuntime } from '../pqc/runtime.js';
import {
  pqVaultProfile,
  type PqVaultSecret,
  type PqVaultWrapper,
} from '../matrix/unlock-bridge.js';
import { PqVault, PqVaultDisabledError, PqVaultError, type PqVaultBridgePort } from './pq-vault.js';

const encoder = new TextEncoder();
const decoder = new TextDecoder('utf-8', { fatal: true });

function createMockCore(): PqcRuntime {
  let seq = 0;
  let closed = false;
  const identities = new Map<string, { key_id: string; recipient: string }>();

  return {
    invoke: async <T>(request: Record<string, unknown>): Promise<T> => {
      if (closed) throw new DOMException('Aborted', 'AbortError');
      const op = request.operation as string;
      if (op === 'identity.create') {
        seq++;
        const idText = `AGE-SECRET-KEY-PQ-MOCK-${seq}`;
        const key_id = `key-${seq}`.padEnd(64, '0');
        const recipient = `age1pq1mockrecipient${seq}`;
        identities.set(idText, { key_id, recipient });
        return {
          profile: pqVaultProfile,
          key_id,
          recipient,
          identity: encoder.encode(idText),
        } as T;
      }
      if (op === 'identity.public') {
        const idText = decoder.decode(request.identity as Uint8Array);
        const info = identities.get(idText);
        if (!info) throw new Error('unknown_identity');
        return { profile: pqVaultProfile, ...info } as T;
      }
      if (op === 'storage.seal') {
        const recipient = request.recipient as string;
        const found = [...identities.values()].find((v) => v.recipient === recipient);
        if (!found) throw new Error('unknown_recipient');
        const payload = JSON.stringify({
          room: request.room,
          record: Buffer.from(request.record as Uint8Array).toString('base64'),
          proof: Buffer.from(request.proof as Uint8Array).toString('base64'),
        });
        const env: PqcEnvelope = {
          v: 1,
          profile: pqVaultProfile,
          key_id: found.key_id,
          ciphertext: encoder.encode(payload),
        };
        return env as T;
      }
      if (op === 'storage.message.fits') return true as T;
      if (op === 'storage.message.encode') {
        const env = request.envelope as PqcEnvelope;
        const wire = {
          msgtype: 'm.text',
          body: '',
          'org.sytue.storage_pq': {
            v: 1,
            kind: request.kind,
            profile: env.profile,
            key_id: env.key_id,
            ciphertext: Buffer.from(env.ciphertext).toString('base64'),
          },
        };
        return encoder.encode(JSON.stringify(wire)) as T;
      }
      if (op === 'storage.message.decode') {
        const wire = JSON.parse(decoder.decode(request.message as Uint8Array)) as Record<
          string,
          unknown
        >;
        const pq = wire['org.sytue.storage_pq'] as Record<string, unknown> | undefined;
        if (!pq) return { found: false } as T;
        return {
          found: true,
          kind: pq.kind,
          envelope: {
            v: 1,
            profile: pq.profile,
            key_id: pq.key_id,
            ciphertext: new Uint8Array(Buffer.from(String(pq.ciphertext), 'base64')),
          },
        } as T;
      }
      if (op === 'storage.open') {
        const idText = decoder.decode(request.identity as Uint8Array);
        const info = identities.get(idText);
        const env = request.envelope as PqcEnvelope;
        if (!info || info.key_id !== env.key_id) throw new Error('wrong_key');
        const parsed = JSON.parse(decoder.decode(env.ciphertext)) as {
          room: string;
          record: string;
          proof: string;
        };
        if (parsed.room !== request.room) throw new Error('wrong_room');
        return {
          record: new Uint8Array(Buffer.from(parsed.record, 'base64')),
          proof: new Uint8Array(Buffer.from(parsed.proof, 'base64')),
        } as T;
      }
      throw new Error(`unexpected_op:${op}`);
    },
    close: () => {
      closed = true;
    },
    closeAndDrain: async () => {
      closed = true;
    },
  } as unknown as PqcRuntime;
}

function createHarness(
  options: { disabled?: boolean; bridgeMode?: 'ok' | 'signin' | 'wrong' } = {},
) {
  let vaultId = '';
  let activeKeyId = '';
  let wrappers: PqVaultWrapper[] = [];
  const wrappedSecrets = new Map<string, PqVaultSecret>();
  const records: Array<{
    event_id: string;
    vault_id: string;
    seq: number;
    tx: string;
    payload: Record<string, unknown>;
  }> = [];

  const bridge: PqVaultBridgePort = {
    wrap: async (_opts, req) => {
      if (options.bridgeMode === 'signin') return { ok: false, error: 'signin' };
      const ct = Buffer.from(JSON.stringify(req.secret)).toString('base64');
      wrappedSecrets.set(ct, req.secret);
      const wrapper: PqVaultWrapper = {
        v: 1,
        kind: 'sytue-id',
        profile: pqVaultProfile,
        key_id: req.keyID,
        recipient: req.recipient,
        kdf: { alg: 'sytue-id-unlock-hkdf-sha256', hint: 'hint-1' },
        nonce: 'YWJjZGVmZ2hpamts',
        ciphertext: ct,
      };
      return { ok: true, wrapper };
    },
    unwrap: async (_opts, req) => {
      if (options.bridgeMode === 'signin') return { ok: false, error: 'signin' };
      if (options.bridgeMode === 'wrong') return { ok: false, error: 'wrong' };
      for (const w of req.wrappers) {
        const secret = wrappedSecrets.get(w.ciphertext);
        if (secret) {
          return {
            ok: true,
            secret,
            keyID: w.key_id,
            recipient: w.recipient,
            profile: pqVaultProfile,
          };
        }
      }
      return { ok: false, error: 'wrong' };
    },
  };

  const fetchImpl = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), 'https://drive.sytue.eu');
    const method = init?.method ?? 'GET';
    if (url.pathname === '/drive/api/pq-vault/bootstrap') {
      if (options.disabled) return Response.json({ cifrado: 'desactivado' });
      return Response.json({
        actor: 'user-1',
        tenant: 'acme',
        product: 'drive',
        id_origin: 'https://id.sytue.eu',
        encryption: pqVaultProfile,
      });
    }
    if (url.pathname === '/drive/api/pq-vault/keys' && method === 'GET') {
      return Response.json({ vault_id: vaultId, active_key_id: activeKeyId, keys: wrappers });
    }
    if (url.pathname === '/drive/api/pq-vault/keys' && method === 'PUT') {
      const body = JSON.parse(String(init?.body)) as {
        wrapper: PqVaultWrapper;
        set_active?: boolean;
        reset?: boolean;
      };
      if (!vaultId || body.reset) {
        vaultId = `!pq_${Math.random().toString(16).slice(2, 10)}`;
        wrappers = [];
        records.length = 0;
      }
      wrappers.push(body.wrapper);
      if (body.set_active) activeKeyId = body.wrapper.key_id;
      return Response.json({ vault_id: vaultId, active_key_id: activeKeyId, keys: wrappers });
    }
    if (url.pathname === '/drive/api/pq-vault/records' && method === 'POST') {
      const body = JSON.parse(String(init?.body)) as {
        vault_id: string;
        transaction_id: string;
        payload: Record<string, unknown>;
      };
      const existing = records.find(
        (r) => r.vault_id === body.vault_id && r.tx === body.transaction_id,
      );
      if (existing) {
        return Response.json({
          event_id: existing.event_id,
          vault_id: existing.vault_id,
          seq: existing.seq,
          payload: existing.payload,
        });
      }
      const seq = records.length + 1;
      const entry = {
        event_id: `$pqe_${seq}`,
        vault_id: body.vault_id,
        seq,
        tx: body.transaction_id,
        payload: body.payload,
      };
      records.push(entry);
      return Response.json({
        event_id: entry.event_id,
        vault_id: entry.vault_id,
        seq: entry.seq,
        payload: entry.payload,
      });
    }
    if (url.pathname === '/drive/api/pq-vault/records' && method === 'GET') {
      const vid = url.searchParams.get('vault_id');
      const entries = records
        .filter((r) => r.vault_id === vid)
        .reverse()
        .map((r) => ({
          event_id: r.event_id,
          vault_id: r.vault_id,
          seq: r.seq,
          payload: r.payload,
        }));
      return Response.json({ entries });
    }
    if (url.pathname.startsWith('/drive/api/pq-vault/records/') && method === 'GET') {
      const eid = decodeURIComponent(url.pathname.slice('/drive/api/pq-vault/records/'.length));
      const found = records.find((r) => r.event_id === eid);
      if (!found) return new Response(null, { status: 404 });
      return Response.json({
        event_id: found.event_id,
        vault_id: found.vault_id,
        seq: found.seq,
        payload: found.payload,
      });
    }
    return new Response(null, { status: 404 });
  }) as unknown as typeof fetch;

  return { bridge, fetchImpl, records, getWrappers: () => wrappers };
}

describe('PqVault (ADR 0086)', () => {
  test('enrolls on first open, reopens existing vault, and round-trips authenticated encrypted records', async () => {
    const core = createMockCore();
    const h = createHarness();
    const vault1 = await PqVault.open({
      actor: 'user-1',
      tenant: 'acme',
      product: 'drive',
      basePath: '/drive',
      fetchImpl: h.fetchImpl,
      bridge: h.bridge,
      createCore: () => core,
    });
    expect(vault1.profile).toBe(pqVaultProfile);
    expect(vault1.isClosed).toBe(false);
    expect(h.getWrappers()).toHaveLength(1);

    const repo = vault1.storage();
    await repo.prepare();
    const target = repo.publicationTarget();
    await repo.recoverable(target);
    const payloadB64 = Buffer.from(JSON.stringify({ title: 'Confidencial PQ' })).toString('base64');
    const record = {
      v: 1 as const,
      scope: { tenant: 'acme', subject: 'user-1', product: 'drive' as const },
      object_id: 'doc-1',
      revision_id: 'rev-1',
      role: 'index' as const,
      payload: payloadB64,
    };
    const ref = await repo.exclusive((signal) =>
      repo.putRecord('tx-doc-1', record, signal, target),
    );
    expect(h.records).toHaveLength(1);
    expect(h.records[0]!.payload['org.sytue.storage_pq']).toBeDefined();
    expect(JSON.stringify(h.records[0]!.payload)).not.toContain('Confidencial PQ');

    const loaded = await repo.readRecord(ref, 'doc-1', 'index');
    expect(loaded.payload).toBe(payloadB64);
    const history = await repo.rawCatalogHistory(vault1.vaultId, null, 10);
    expect(history.entries).toHaveLength(1);
    expect(history.entries[0]!.record.object_id).toBe('doc-1');

    // Reopen on a second session using the persisted wrapper (same core mock holds identity table).
    const vault2 = await PqVault.open({
      actor: 'user-1',
      tenant: 'acme',
      product: 'drive',
      basePath: '/drive',
      fetchImpl: h.fetchImpl,
      bridge: h.bridge,
      createCore: () => core,
    });
    expect(vault2.keyId).toBe(vault1.keyId);
    expect(vault2.vaultId).toBe(vault1.vaultId);
    const repo2 = vault2.storage();
    expect((await repo2.readRecord(ref, 'doc-1', 'index')).payload).toBe(payloadB64);

    await vault1.closeAndDrain();
    expect(vault1.isClosed).toBe(true);
    await vault2.closeAndDrain();
    expect(vault2.isClosed).toBe(true);
  });

  test('propagates disabled mode, signin, wrong key, and supports explicit vault reset', async () => {
    const disabled = createHarness({ disabled: true });
    await expect(
      PqVault.open({
        actor: 'user-1',
        tenant: 'acme',
        product: 'drive',
        basePath: '/drive',
        fetchImpl: disabled.fetchImpl,
        bridge: disabled.bridge,
        createCore: () => createMockCore(),
      }),
    ).rejects.toBeInstanceOf(PqVaultDisabledError);

    const needSignIn = createHarness({ bridgeMode: 'signin' });
    try {
      await PqVault.open({
        actor: 'user-1',
        tenant: 'acme',
        product: 'drive',
        basePath: '/drive',
        fetchImpl: needSignIn.fetchImpl,
        bridge: needSignIn.bridge,
        createCore: () => createMockCore(),
      });
      throw new Error('expected_signin');
    } catch (error) {
      expect(error).toBeInstanceOf(PqVaultError);
      expect((error as PqVaultError).reason).toBe('signin');
    }

    const core = createMockCore();
    const h = createHarness();
    const first = await PqVault.open({
      actor: 'user-1',
      tenant: 'acme',
      product: 'drive',
      basePath: '/drive',
      fetchImpl: h.fetchImpl,
      bridge: h.bridge,
      createCore: () => core,
    });
    const reset = await PqVault.reset({
      actor: 'user-1',
      tenant: 'acme',
      product: 'drive',
      basePath: '/drive',
      fetchImpl: h.fetchImpl,
      bridge: h.bridge,
      createCore: () => core,
    });
    expect(reset.keyId).not.toBe(first.keyId);
    expect(reset.vaultId).not.toBe(first.vaultId);
    await first.closeAndDrain();
    await reset.closeAndDrain();
  });
});
