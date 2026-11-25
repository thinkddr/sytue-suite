/* SPDX-License-Identifier: Apache-2.0 */
import { describe, expect, test } from 'bun:test';
import type { PqcRuntime } from '../pqc/runtime.js';
import { pqVaultProfile } from '../matrix/unlock-bridge.js';
import {
  PqVault,
  PqVaultDisabledError,
  PqVaultError,
  cancelPqVaultCopy,
  pqVaultUserID,
  readPqVaultCopy,
  type PqVaultCopyMarker,
} from './pq-vault.js';
import { createHarness, createMockCore, decoder, encoder } from './pq-vault-fixture.js';

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

  test('a key wrapped with another unlock key restarts an empty vault, but never one with records', async () => {
    const opts = (h: ReturnType<typeof createHarness>, core: PqcRuntime) => ({
      actor: 'user-1',
      tenant: 'acme',
      product: 'drive' as const,
      basePath: '/drive',
      fetchImpl: h.fetchImpl,
      bridge: h.bridge,
      createCore: () => core,
    });

    // Vacía: se reinicia sola y la app abre.
    const core = createMockCore();
    const empty = createHarness();
    const before = await PqVault.open(opts(empty, core));
    empty.forgetUnlockKey();
    const after = await PqVault.open(opts(empty, core));
    expect(after.vaultId).not.toBe(before.vaultId);
    expect(after.keyId).not.toBe(before.keyId);
    expect(empty.getWrappers()).toHaveLength(1);
    await before.closeAndDrain();
    await after.closeAndDrain();

    // Con un registro: nada de reinicios silenciosos; decide la persona.
    // Núcleo propio: el simulado se cierra con la primera bóveda que se cierra.
    const full = createHarness();
    const owner = await PqVault.open(opts(full, createMockCore()));
    const repo = owner.storage();
    await repo.prepare();
    const target = repo.publicationTarget();
    await repo.recoverable(target);
    await repo.exclusive((signal) =>
      repo.putRecord(
        'tx-doc-1',
        {
          v: 1 as const,
          scope: { tenant: 'acme', subject: 'user-1', product: 'drive' as const },
          object_id: 'doc-1',
          revision_id: 'rev-1',
          role: 'index' as const,
          payload: Buffer.from('{}').toString('base64'),
        },
        signal,
        target,
      ),
    );
    full.forgetUnlockKey();
    try {
      await PqVault.open(opts(full, createMockCore()));
      throw new Error('expected_wrong');
    } catch (error) {
      expect(error).toBeInstanceOf(PqVaultError);
      expect((error as PqVaultError).reason).toBe('wrong');
    }
    expect(full.records).toHaveLength(1);
    await owner.closeAndDrain();
  });
});

describe('PqVault: vaciar sin carreras y la copia de otro dispositivo (ADR 0086)', () => {
  const opts = (h: ReturnType<typeof createHarness>, core: PqcRuntime) => ({
    actor: 'user-1',
    tenant: 'acme',
    product: 'drive' as const,
    basePath: '/drive',
    fetchImpl: h.fetchImpl,
    bridge: h.bridge,
    createCore: () => core,
  });
  const record = (id: string) => ({
    v: 1 as const,
    scope: { tenant: 'acme', subject: 'user-1', product: 'drive' as const },
    object_id: id,
    revision_id: 'rev-1',
    role: 'index' as const,
    payload: Buffer.from('{}').toString('base64'),
  });
  const put = async (vault: PqVault, id: string) => {
    const repo = vault.storage();
    await repo.prepare();
    const target = repo.publicationTarget();
    await repo.recoverable(target);
    await repo.exclusive((signal) => repo.putRecord(id, record(id), signal, target));
  };
  const failure = async (promise: Promise<unknown>): Promise<PqVaultError> => {
    const error = await promise.then(
      () => new Error('expected_failure'),
      (e: unknown) => e,
    );
    expect(error).toBeInstanceOf(PqVaultError);
    return error as PqVaultError;
  };

  test('vaciar solo vacía la bóveda que se vio: un recibo que llega después nunca se borra', async () => {
    const h = createHarness();
    const owner = await PqVault.open(opts(h, createMockCore()));
    await put(owner, 'tx-1');
    h.forgetUnlockKey();
    const wrong = await failure(PqVault.open(opts(h, createMockCore())));
    expect(wrong.reason).toBe('wrong');
    expect(wrong.expect?.version).toBeGreaterThan(0);

    // El recibo llega entre ver la bóveda y vaciarla: con lo visto, ni se pide.
    h.appendOther();
    const stale = await failure(
      PqVault.reset({ ...opts(h, createMockCore()), expect: wrong.expect }),
    );
    expect(stale.reason).toBe('changed');
    expect(h.records).toHaveLength(2);

    // Y si llega justo mientras se pide, el servidor no vacía.
    h.setBeforeReset(() => h.appendOther());
    const late = await failure(PqVault.reset(opts(h, createMockCore())));
    expect(late.reason).toBe('changed');
    expect(h.records).toHaveLength(3);
    await owner.closeAndDrain();
  });

  test('con otro dispositivo copiando no se publica ni se vacía; la copia se ve y se cancela', async () => {
    const h = createHarness();
    const vault = await PqVault.open(opts(h, createMockCore()));
    const marker: PqVaultCopyMarker = {
      id: 'ABCDEFGHIJKLMNOPQRSTUVWXYZ',
      holder: 'native-linux',
      kind: 'matrix-copy',
      epoch: 'e',
      started_at: '2026-11-23T10:00:00Z',
      expires_at: '2026-11-23T10:05:00Z',
      active: true,
    };
    h.setCopy(marker);
    expect((await failure(put(vault, 'tx-web'))).reason).toBe('copy');
    expect(h.records).toHaveLength(0);
    expect((await failure(PqVault.reset(opts(h, createMockCore())))).reason).toBe('copy');
    expect(await vault.copyMarker()).toEqual(marker);
    const copyOpts = {
      actor: 'user-1',
      tenant: 'acme',
      product: 'drive' as const,
      basePath: '/drive',
      fetchImpl: h.fetchImpl,
    };
    expect(await readPqVaultCopy(copyOpts)).toEqual(marker);
    // Una activa no se toca al abrir; cancelarla es explícito.
    await vault.clearStaleCopy();
    expect(h.getCopy()).toEqual(marker);
    await cancelPqVaultCopy(copyOpts, marker.id);
    expect(h.getCopy()).toBeNull();
    await put(vault, 'tx-web');
    expect(h.records).toHaveLength(1);
    // Una caducada se retira al abrir con el recibo.
    h.setCopy({ ...marker, active: false });
    await vault.clearStaleCopy();
    expect(h.getCopy()).toBeNull();
    await vault.closeAndDrain();
  });
});

describe('PqVault: abrir solo una bóveda que ya existe (Photos sobre la de Drive)', () => {
  const opts = (h: ReturnType<typeof createHarness>, core: PqcRuntime) => ({
    actor: 'user-1',
    tenant: 'acme',
    product: 'drive' as const,
    basePath: '/drive',
    fetchImpl: h.fetchImpl,
    bridge: h.bridge,
    createCore: () => core,
    existing: true,
  });
  const reason = async (pending: Promise<unknown>) => {
    try {
      await pending;
    } catch (error) {
      expect(error).toBeInstanceOf(PqVaultError);
      return (error as PqVaultError).reason;
    }
    throw new Error('expected_failure');
  };

  test('sin bóveda no la crea: «missing» y ningún envoltorio nuevo', async () => {
    const h = createHarness();
    expect(await reason(PqVault.open(opts(h, createMockCore())))).toBe('missing');
    expect(await reason(PqVault.reset(opts(h, createMockCore())))).toBe('missing');
    expect(h.getWrappers()).toHaveLength(0);
  });

  test('abre la que existe, y con la clave equivocada no la reinicia aunque esté vacía', async () => {
    const h = createHarness();
    const core = createMockCore();
    const owner = await PqVault.open({ ...opts(h, core), existing: false });
    const consumer = await PqVault.open(opts(h, core));
    expect(consumer.vaultId).toBe(owner.vaultId);
    expect(consumer.keyId).toBe(owner.keyId);
    h.forgetUnlockKey();
    expect(await reason(PqVault.open(opts(h, createMockCore())))).toBe('wrong');
    expect(h.getWrappers()).toHaveLength(1);
    expect(h.getWrappers()[0]!.key_id).toBe(owner.keyId);
    await consumer.closeAndDrain();
    await owner.closeAndDrain();
  });
});

describe('PqVault: clave de firma registrada en Sytue ID (ADR 0087)', () => {
  /** Un núcleo con las operaciones de firma y un Sytue ID con su registro. */
  /** Un rechazo de Sytue ID, en el sobre de verdad de `platform/pkg/httpx`. */
  const rechazo = (status: number, code: string) =>
    Response.json({ error: { status, code, message: code }, request_id: 'r' }, { status });

  function conFirma(
    opciones: { conFirma?: boolean; rancias?: number; firmasMalas?: boolean } = {},
  ) {
    let rancias = opciones.rancias ?? 0;
    let n = 0;
    // Un núcleo por apertura: cerrar una bóveda cierra el suyo.
    const nuevoNucleo = () => {
      const base = createMockCore();
      return {
        ...base,
        invoke: async <T>(request: Record<string, unknown>): Promise<T> => {
          if (request.operation === 'vault.signing.create') {
            if (opciones.conFirma === false) throw new Error('sin firma');
            n++;
            return {
              signing_key: encoder.encode(String.fromCharCode(64 + n).repeat(86) + '=='),
            } as T;
          }
          if (request.operation === 'vault.signing.enroll') {
            const key = decoder.decode(request.signing_key as Uint8Array);
            return {
              profile: 'ed25519-mldsa65-v1',
              key_id: key[0]!.repeat(64).toLowerCase(),
              ed25519: new Uint8Array(32),
              mldsa65: new Uint8Array(4),
              proof: new Uint8Array(8),
              previous_key_id: (request.previous_key_id as string | undefined) ?? '',
            } as T;
          }
          return base.invoke<T>(request);
        },
      } as unknown as PqcRuntime;
    };
    const h = createHarness();
    const registro: { generation: number; key_id: string; retired_at?: string }[] = [];
    let reciente = true;
    const fetchImpl = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'https://drive.sytue.eu');
      if (url.pathname !== '/drive/api/pq-vault/signing-key') return h.fetchImpl(input, init);
      if ((init?.method ?? 'GET') === 'GET')
        return Response.json({ keys: [...registro].reverse() });
      const body = JSON.parse(String(init?.body)) as { previous_key_id: string };
      const activa = registro.find((k) => !k.retired_at);
      if (opciones.firmasMalas) return rechazo(400, 'vault_key_proof');
      if (activa && !reciente) return rechazo(403, 'vault_key_reauth');
      if (rancias > 0) {
        rancias--;
        return rechazo(409, 'vault_key_stale');
      }
      if ((activa?.key_id ?? '') !== body.previous_key_id) return rechazo(409, 'vault_key_stale');
      if (activa) activa.retired_at = 'ayer';
      registro.push({ generation: registro.length + 1, key_id: 'x'.repeat(64) });
      return Response.json({});
    }) as unknown as typeof fetch;
    const abrir = (reset = false) =>
      (reset ? PqVault.reset : PqVault.open)({
        actor: 'user-1',
        tenant: 'acme',
        product: 'drive',
        basePath: '/drive',
        fetchImpl,
        bridge: h.bridge,
        createCore: nuevoNucleo,
      });
    return { h, registro, abrir, sinReautenticar: () => (reciente = false) };
  }

  test('una bóveda nueva nace con su clave de firma y la registra', async () => {
    const c = conFirma();
    const vault = await c.abrir();
    expect(vault.signingKeyStatus).toBe('registered');
    expect(c.registro).toHaveLength(1);
    // El secreto sellado lleva la clave de firma.
    const sellado = c.h.getWrappers().at(-1)!;
    expect(Buffer.from(sellado.ciphertext, 'base64').toString()).toContain('signing_key');
    await vault.closeAndDrain();
  });

  test('una bóveda de antes de la ADR 0087 gana la clave al abrirse, sin dejar de abrir', async () => {
    const c = conFirma({ conFirma: false });
    const vieja = await c.abrir();
    expect(vieja.signingKeyStatus).toBe('missing');
    await vieja.closeAndDrain();
    expect(c.registro).toHaveLength(0);
  });

  test('vaciar con otra clave activa pide reautenticar, y la bóveda abre igual', async () => {
    const c = conFirma();
    const primera = await c.abrir();
    expect(primera.signingKeyStatus).toBe('registered');
    await primera.closeAndDrain();
    c.sinReautenticar();
    const vaciada = await c.abrir(true);
    expect(vaciada.isClosed).toBe(false);
    expect(vaciada.signingKeyStatus).toBe('reauth');
    expect(c.registro).toHaveLength(1);
    await vaciada.closeAndDrain();
  });

  test('si otro dispositivo registró entre medias, se vuelve a leer y se repite una vez', async () => {
    const c = conFirma({ rancias: 1 });
    const vault = await c.abrir();
    expect(vault.signingKeyStatus).toBe('registered');
    expect(vault.signingKeyCheck).toBeUndefined();
    expect(c.registro).toHaveLength(1);
    await vault.closeAndDrain();
  });

  test('un rechazo de Sytue ID queda dicho: qué petición, qué estado y qué código', async () => {
    const c = conFirma({ firmasMalas: true });
    const vault = await c.abrir();
    expect(vault.isClosed).toBe(false);
    expect(vault.signingKeyStatus).toBe('unavailable');
    expect(vault.signingKeyCheck).toBe('signing_put_400_vault_key_proof');
    await vault.closeAndDrain();
  });
});

describe('PqVault: traspaso del correo de Matrix a la bóveda (ADR 0087)', () => {
  function conTraspaso(product: 'drive' | 'mail', handover: boolean) {
    const base = createMockCore();
    const core = {
      ...base,
      invoke: async <T>(request: Record<string, unknown>): Promise<T> => {
        if (request.operation === 'vault.signing.create')
          return { signing_key: encoder.encode('A'.repeat(86) + '==') } as T;
        if (request.operation === 'vault.signing.enroll')
          return {
            profile: 'ed25519-mldsa65-v1',
            key_id: 'a'.repeat(64),
            ed25519: new Uint8Array(32),
            mldsa65: new Uint8Array(4),
            proof: new Uint8Array(8),
            previous_key_id: '',
          } as T;
        return base.invoke<T>(request);
      },
    } as unknown as PqcRuntime;
    const h = createHarness({ product });
    const cuerpos: Record<string, unknown>[] = [];
    const fetchImpl = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'https://drive.sytue.eu');
      if (url.pathname !== '/drive/api/pq-vault/signing-key') return h.fetchImpl(input, init);
      if ((init?.method ?? 'GET') === 'GET') return Response.json({ keys: [] });
      cuerpos.push(JSON.parse(String(init?.body)) as Record<string, unknown>);
      return Response.json({});
    }) as unknown as typeof fetch;
    const abrir = () =>
      PqVault.reset({
        actor: 'user-1',
        tenant: 'acme',
        product,
        basePath: '/drive',
        fetchImpl,
        bridge: h.bridge,
        createCore: () => core,
        handover,
      });
    return { cuerpos, abrir };
  }

  test('el correo registra su clave con la marca de traspaso', async () => {
    const c = conTraspaso('mail', true);
    const vault = await c.abrir();
    expect(vault.signingKeyStatus).toBe('registered');
    expect(c.cuerpos).toHaveLength(1);
    expect(c.cuerpos[0]!.handover).toBe(true);
    await vault.closeAndDrain();
  });

  test('sin la marca no se pide traspaso, y fuera del correo no se pide nunca', async () => {
    const sin = conTraspaso('mail', false);
    const vault = await sin.abrir();
    expect(sin.cuerpos).toHaveLength(1);
    expect('handover' in sin.cuerpos[0]!).toBe(false);
    await vault.closeAndDrain();
    const drive = conTraspaso('drive', true);
    const otra = await drive.abrir();
    expect(otra.signingKeyStatus).toBe('unavailable');
    expect(drive.cuerpos).toHaveLength(0);
    await otra.closeAndDrain();
  });
});

describe('PqVault: el correo sobre la bóveda (ADR 0087, entrega 4)', () => {
  const nombre = 'org.sytue.ingress.key.v1.' + 'ab'.repeat(32);
  /** Bóveda con núcleo que firma y un Drive que busca por transaction_id. */
  async function abrirCorreo(opciones: { registrada?: boolean } = {}) {
    const firmados: Record<string, unknown>[] = [];
    const base = createMockCore();
    const core = {
      ...base,
      invoke: async <T>(request: Record<string, unknown>): Promise<T> => {
        if (request.operation === 'vault.signing.create')
          return { signing_key: encoder.encode('A'.repeat(86) + '==') } as T;
        if (request.operation === 'vault.signing.enroll')
          return {
            profile: 'ed25519-mldsa65-v1',
            key_id: 'a'.repeat(64),
            ed25519: new Uint8Array(32),
            mldsa65: new Uint8Array(4),
            proof: new Uint8Array(8),
            previous_key_id: '',
          } as T;
        if (request.operation === 'vault.ingress.sign') {
          firmados.push(request);
          return { proof: encoder.encode('{"firmada":true}') } as T;
        }
        return base.invoke<T>(request);
      },
    } as unknown as PqcRuntime;
    const h = createHarness();
    const fetchImpl = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), 'https://drive.sytue.eu');
      const method = init?.method ?? 'GET';
      if (url.pathname === '/drive/api/pq-vault/signing-key') {
        if (method === 'GET') return Response.json({ keys: [] });
        return opciones.registrada === false
          ? new Response(null, { status: 503 })
          : Response.json({});
      }
      if (url.pathname === '/drive/api/pq-vault/records' && method === 'POST') {
        // Como Drive: el mismo transaction_id con otro cuerpo es un conflicto.
        const body = JSON.parse(String(init?.body)) as { transaction_id: string; payload: unknown };
        const previo = h.records.find((r) => r.tx === body.transaction_id);
        if (previo && JSON.stringify(previo.payload) !== JSON.stringify(body.payload))
          return new Response(null, { status: 409 });
      }
      if (
        url.pathname === '/drive/api/pq-vault/records' &&
        method === 'GET' &&
        url.searchParams.has('transaction_id')
      ) {
        const r = h.records.find(
          (x) =>
            x.tx === url.searchParams.get('transaction_id') &&
            x.vault_id === url.searchParams.get('vault_id'),
        );
        return Response.json({
          entries: r
            ? [{ event_id: r.event_id, vault_id: r.vault_id, seq: r.seq, payload: r.payload }]
            : [],
        });
      }
      return h.fetchImpl(input, init);
    }) as unknown as typeof fetch;
    const vault = await PqVault.open({
      actor: 'user-1',
      tenant: 'acme',
      product: 'drive',
      basePath: '/drive',
      fetchImpl,
      bridge: h.bridge,
      createCore: () => core,
    });
    return { vault, h, firmados };
  }

  test('la identidad ante el correo es la de Go vaultproof.UserID', () => {
    // El mismo literal que comprueba apps/nucleo/storage/ingress/vaultproof.
    expect(
      pqVaultUserID({
        tenant: 'acme',
        subject: '0f3c9a52-6d1e-4c1a-9b8e-2a7d5c4e1f00',
        product: 'mail',
      }),
    ).toBe('@sytue_storage_mail_acme_0f3c9a52-6d1e-4c1a-9b8e-2a7d5c4e1f00:sytue-vault');
  });

  test('los secretos de entrada se guardan cifrados, una sola vez, y se leen por su nombre', async () => {
    const { vault, h } = await abrirCorreo();
    expect(await vault.readIngressSecret(nombre)).toBeNull();
    const valor = encoder.encode('{"v":1,"identity":"AGE-SECRET-KEY-PQ-ENTRADA"}');
    await vault.writeIngressSecret(nombre, valor);
    expect(JSON.stringify(h.records)).not.toContain('AGE-SECRET-KEY-PQ-ENTRADA');
    expect(h.records.at(-1)!.tx).toBe('ingress_secret_' + 'ab'.repeat(32));
    expect(decoder.decode((await vault.readIngressSecret(nombre))!)).toBe(decoder.decode(valor));
    // Repetir la misma escritura no hace nada; otra distinta nunca sustituye la clave.
    await vault.writeIngressSecret(nombre, valor);
    await expect(
      vault.writeIngressSecret(nombre, encoder.encode('{"v":1,"identity":"OTRA"}')),
    ).rejects.toThrow();
    expect(decoder.decode((await vault.readIngressSecret(nombre))!)).toBe(decoder.decode(valor));
    // Un nombre que no es de entrada no se acepta.
    await expect(vault.readIngressSecret('org.sytue.otra.cosa')).rejects.toThrow();
    // El secreto no aparece en el catálogo (solo cuenta `index`).
    expect(await vault.storage().catalog()).toHaveLength(0);
    // Un registro con ese identificador que no es un secreto de entrada no se toma por uno.
    const repo = vault.storage();
    const impostor = 'cd'.repeat(32);
    await repo.exclusive((signal) =>
      repo.putRecord(
        'ingress_secret_' + impostor,
        {
          v: 1,
          scope: vault.scope,
          object_id: 'ingress_' + impostor,
          revision_id: 'v1',
          role: 'index',
          payload: Buffer.from('{"v":1}').toString('base64'),
        },
        signal,
        repo.publicationTarget(),
      ),
    );
    await expect(vault.readIngressSecret('org.sytue.ingress.key.v1.' + impostor)).rejects.toThrow();
    await vault.closeAndDrain();
  });

  test('firma acciones de entrada en el núcleo, solo con la clave registrada', async () => {
    const { vault, firmados } = await abrirCorreo();
    const pedido = encoder.encode('{"expected":{},"action":{}}');
    const prueba = await vault.signIngressAction(pedido);
    expect(decoder.decode(prueba)).toBe('{"firmada":true}');
    expect(firmados).toHaveLength(1);
    expect(decoder.decode(firmados[0]!.request as Uint8Array)).toBe('{"expected":{},"action":{}}');
    expect(decoder.decode(firmados[0]!.signing_key as Uint8Array)).toBe('A'.repeat(86) + '==');
    expect(vault.publicationTarget.room_id).toBe(vault.vaultId);
    await vault.closeAndDrain();
    await expect(vault.signIngressAction(pedido)).rejects.toThrow();

    const sinRegistro = await abrirCorreo({ registrada: false });
    expect(sinRegistro.vault.signingKeyStatus).toBe('unavailable');
    await expect(sinRegistro.vault.signIngressAction(pedido)).rejects.toThrow();
    expect(sinRegistro.firmados).toHaveLength(0);
    await sinRegistro.vault.closeAndDrain();
  });
});
