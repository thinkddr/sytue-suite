/* SPDX-License-Identifier: Apache-2.0 */
/**
 * La bóveda post-cuántica de mentira para las pruebas: un núcleo que «sella» en
 * JSON (sin criptografía real) y un servidor en memoria con las reglas de
 * services/drive que importan al cliente (versión, vaciado con lo visto, marca de
 * copia, publicación idempotente por transacción). Solo datos artificiales.
 */
// Es el doble de las pruebas, sacado de pq-vault.test.ts tal cual: sus `async` sin
// `await` y los cuerpos de `fetch` que lee como texto son lo que imita.
// oxlint-disable typescript/require-await, typescript/no-base-to-string
import type { PqcEnvelope, PqcRuntime } from '../pqc/runtime.js';
import {
  pqVaultProfile,
  type PqVaultSecret,
  type PqVaultWrapper,
} from '../matrix/unlock-bridge.js';
import type { PqVaultBridgePort, PqVaultCopyMarker } from './pq-vault.js';
export const encoder = new TextEncoder();
export const decoder = new TextDecoder('utf-8', { fatal: true });

export function createMockCore(): PqcRuntime {
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

export function createHarness(
  options: {
    disabled?: boolean;
    bridgeMode?: 'ok' | 'signin' | 'wrong';
    product?: 'drive' | 'mail';
  } = {},
) {
  let vaultId = '';
  let activeKeyId = '';
  let wrappers: PqVaultWrapper[] = [];
  // Como services/drive: la versión sube con cada registro y cada vaciado, y la
  // marca de copia de otro dispositivo bloquea publicar y vaciar.
  let version = 0;
  let copy: PqVaultCopyMarker | null = null;
  let beforeReset: (() => void) | null = null;
  const conflict = (code: string) =>
    Response.json({ error: { code, message: code } }, { status: 409 });
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

  // Tan estricto como el `fetch` del navegador: llamado como método de otro objeto
  // (`this.fetchImpl(...)`) lanza «Illegal invocation». Bun no lo hace, y por eso
  // las pruebas no vieron que la bóveda guardaba `fetch` sin atar y lo llamaba así:
  // en el navegador Mail y Drive se quedaban en `prepare/network`.
  const fetchImpl = async function (
    this: unknown,
    input: RequestInfo | URL,
    init?: RequestInit,
  ): Promise<Response> {
    if (this !== undefined && this !== globalThis)
      throw new TypeError("Failed to execute 'fetch' on 'Window': Illegal invocation");
    const url = new URL(String(input), 'https://drive.sytue.eu');
    const method = init?.method ?? 'GET';
    if (url.pathname === '/drive/api/pq-vault/bootstrap') {
      if (options.disabled) return Response.json({ cifrado: 'desactivado' });
      return Response.json({
        actor: 'user-1',
        tenant: 'acme',
        product: options.product ?? 'drive',
        id_origin: 'https://id.sytue.eu',
        encryption: pqVaultProfile,
      });
    }
    if (url.pathname === '/drive/api/pq-vault/keys' && method === 'GET') {
      return Response.json({
        vault_id: vaultId,
        active_key_id: activeKeyId,
        keys: wrappers,
        version,
        ...(copy ? { copy } : {}),
      });
    }
    if (url.pathname === '/drive/api/pq-vault/copy' && method === 'DELETE') {
      const id = url.searchParams.get('id');
      if (copy && copy.id !== id) return conflict('pq_vault_changed');
      copy = null;
      return Response.json({ vault_id: vaultId, active_key_id: activeKeyId, keys: wrappers });
    }
    if (url.pathname === '/drive/api/pq-vault/keys' && method === 'PUT') {
      const body = JSON.parse(String(init?.body)) as {
        wrapper: PqVaultWrapper;
        set_active?: boolean;
        reset?: boolean;
        expect?: { vault_id: string; version: number };
      };
      if (body.reset) {
        if (!body.expect) return new Response(null, { status: 400 });
        const hook = beforeReset;
        beforeReset = null;
        hook?.();
        if (body.expect.vault_id !== vaultId || body.expect.version !== version)
          return conflict('pq_vault_changed');
        if (copy?.active) return conflict('pq_vault_copy_in_progress');
        version++;
      }
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
      if (copy?.active) return conflict('pq_vault_copy_in_progress');
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
      version++;
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
      // Como en producción: Astro descodifica la ruta con `decodeURI`, que no toca
      // `%24`, y el BFF de entonces no reconocía `records/%24pqe_…`.
      if (url.pathname.includes('%24'))
        return Response.json({ error: 'pq_vault_not_found' }, { status: 404 });
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
  } as unknown as typeof fetch;

  return {
    bridge,
    fetchImpl,
    records,
    getWrappers: () => wrappers,
    // Simula que la clave de desbloqueo con la que se envolvió ya no es la de este navegador.
    forgetUnlockKey: () => wrappedSecrets.clear(),
    // Otro cliente publica (el recibo de una importación, por ejemplo).
    appendOther: () => {
      version++;
      records.push({
        event_id: `$pqe_otro_${records.length + 1}`,
        vault_id: vaultId,
        seq: records.length + 1,
        tx: `otro-${records.length + 1}`,
        payload: {},
      });
    },
    setBeforeReset: (hook: () => void) => {
      beforeReset = hook;
    },
    setCopy: (value: PqVaultCopyMarker | null) => {
      copy = value;
    },
    getCopy: () => copy,
  };
}
