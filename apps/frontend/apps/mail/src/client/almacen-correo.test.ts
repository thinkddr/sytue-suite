/* SPDX-License-Identifier: Apache-2.0 */
import { describe, expect, test } from 'bun:test';
import {
  correoSobreBoveda,
  destinoCorreo,
  elegirAlmacenCorreo,
  olvidarTraspaso,
  pedirTraspaso,
  traspasoPedido,
} from './almacenCorreo.js';

function con(res: Response | Error) {
  const pedidos: Request[] = [];
  const fetchImpl = (async (input: RequestInfo | URL, init?: RequestInit) => {
    pedidos.push(new Request(new URL(String(input), 'https://mail.sytue.eu'), init));
    if (res instanceof Error) throw res;
    return res.clone();
  }) as typeof fetch;
  return { fetchImpl, pedidos };
}

const elegir = (fetchImpl: typeof fetch, tenant: string | null = 'acme') =>
  elegirAlmacenCorreo({
    basePath: '/mail',
    actor: 'user-1',
    tenant: tenant ?? undefined,
    fetchImpl,
  });

describe('dónde vive el correo privado (ADR 0087)', () => {
  test('el conmutador solo se enciende con «activado»', () => {
    expect(correoSobreBoveda('activado')).toBe(true);
    for (const v of [undefined, '', '1', 'true', 'ACTIVADO'])
      expect(correoSobreBoveda(v)).toBe(false);
  });

  test('autoridad de la bóveda → bóveda; de Matrix → Matrix; ninguna → bóveda', async () => {
    const boveda = con(Response.json({ user_id: '@sytue_storage_mail_acme_user-1:sytue-vault' }));
    expect(await elegir(boveda.fetchImpl)).toBe('boveda');
    expect(boveda.pedidos[0]!.url).toBe('https://mail.sytue.eu/mail/api/matrix/storage/authority/');
    expect(boveda.pedidos[0]!.headers.get('x-sytue-storage-scope') ?? '').not.toBe('');
    expect(
      await elegir(
        con(Response.json({ user_id: '@sytue_storage_mail_acme_user-1:matrix.sytue.eu' }))
          .fetchImpl,
      ),
    ).toBe('matrix');
    expect(
      await elegir(
        con(Response.json({ code: 'storage_root_enrollment_required' }, { status: 409 })).fetchImpl,
      ),
    ).toBe('boveda');
  });

  test('ante la duda, Matrix: el camino de quien ya tiene correo', async () => {
    expect(await elegir(con(Response.json({ code: 'otra' }, { status: 409 })).fetchImpl)).toBe(
      'matrix',
    );
    expect(await elegir(con(new Response('x', { status: 503 })).fetchImpl)).toBe('matrix');
    expect(await elegir(con(new Error('red')).fetchImpl)).toBe('matrix');
    expect(await elegir(con(Response.json({})).fetchImpl)).toBe('matrix');
    const sinOrg = con(Response.json({ user_id: 'x:sytue-vault' }));
    expect(await elegir(sinOrg.fetchImpl, null)).toBe('matrix');
    expect(sinOrg.pedidos).toHaveLength(0);
  });
});

describe('vaciar y pasar a la bóveda (ADR 0087)', () => {
  function memoria() {
    const m = new Map<string, string>();
    return {
      getItem: (k: string) => m.get(k) ?? null,
      setItem: (k: string, v: string) => void m.set(k, v),
      removeItem: (k: string) => void m.delete(k),
    };
  }

  test('con la bóveda ya elegida nunca se vuelve a vaciar', () => {
    expect(destinoCorreo('boveda', true)).toBe('boveda');
    expect(destinoCorreo('boveda', false)).toBe('boveda');
    expect(destinoCorreo('matrix', true)).toBe('traspaso');
    expect(destinoCorreo('matrix', false)).toBe('matrix');
  });

  test('la marca es de esta persona y organización, y sin almacén no rompe', () => {
    const s = memoria();
    expect(traspasoPedido(s, 'user-1', 'acme')).toBe(false);
    pedirTraspaso(s, 'user-1', 'acme');
    expect(traspasoPedido(s, 'user-1', 'acme')).toBe(true);
    expect(traspasoPedido(s, 'user-2', 'acme')).toBe(false);
    expect(traspasoPedido(s, 'user-1', 'otra')).toBe(false);
    olvidarTraspaso(s, 'user-1', 'acme');
    expect(traspasoPedido(s, 'user-1', 'acme')).toBe(false);
    const bloqueado = () => {
      throw new Error('bloqueado');
    };
    const roto = { getItem: bloqueado, setItem: bloqueado, removeItem: bloqueado };
    expect(traspasoPedido(roto, 'user-1', 'acme')).toBe(false);
    pedirTraspaso(roto, 'user-1', 'acme');
    olvidarTraspaso(roto, 'user-1', 'acme');
    expect(traspasoPedido(undefined, 'user-1', 'acme')).toBe(false);
  });
});
