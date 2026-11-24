/* SPDX-License-Identifier: Apache-2.0 */
import { describe, expect, test } from 'bun:test';
import {
  aprenderRetirada,
  correoSobreBoveda,
  destinoCorreo,
  elegirAlmacenCorreo,
  olvidarTraspaso,
  pedirTraspaso,
  quiereSeguirConMatrix,
  seguirConMatrix,
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

  test('Matrix retirada en Rooms (entrega 5) → «retirada», distinta de cuenta nueva', async () => {
    for (const body of [
      { code: 'storage_matrix_retired' },
      { error: { code: 'storage_matrix_retired' } },
    ])
      expect(await elegir(con(Response.json(body, { status: 409 })).fetchImpl)).toBe('retirada');
    // El código con otro estado no es la retirada.
    expect(
      await elegir(
        con(Response.json({ code: 'storage_matrix_retired' }, { status: 503 })).fetchImpl,
      ),
    ).toBe('dudoso');
  });

  test('ante la duda, «dudoso»: ni bóveda ni propuesta de vaciar', async () => {
    expect(await elegir(con(Response.json({ code: 'otra' }, { status: 409 })).fetchImpl)).toBe(
      'dudoso',
    );
    expect(await elegir(con(new Response('x', { status: 503 })).fetchImpl)).toBe('dudoso');
    expect(await elegir(con(new Error('red')).fetchImpl)).toBe('dudoso');
    expect(await elegir(con(Response.json({})).fetchImpl)).toBe('dudoso');
    expect(await elegir(con(Response.json({ user_id: '' })).fetchImpl)).toBe('dudoso');
    const sinOrg = con(Response.json({ user_id: 'x:sytue-vault' }));
    expect(await elegir(sinOrg.fetchImpl, null)).toBe('dudoso');
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
    for (const seguir of [false, true]) {
      expect(destinoCorreo('boveda', true, seguir)).toBe('boveda');
      expect(destinoCorreo('boveda', false, seguir)).toBe('boveda');
    }
    expect(destinoCorreo('matrix', true)).toBe('traspaso');
    expect(destinoCorreo('dudoso', true)).toBe('traspaso');
  });

  test('con correo de Matrix se propone pasar a la bóveda, aunque Matrix abra', () => {
    expect(destinoCorreo('matrix', false)).toBe('elegir');
    // «Seguir con el correo anterior por ahora» en esta pestaña: Matrix, sin volver a proponer.
    expect(destinoCorreo('matrix', false, true)).toBe('matrix');
    // Ante la duda no se propone vaciar: Matrix, el camino de siempre.
    expect(destinoCorreo('dudoso', false)).toBe('matrix');
    expect(destinoCorreo('dudoso', false, true)).toBe('matrix');
  });

  test('con Matrix retirada solo queda pasar, y la duda no abre Matrix', () => {
    // Retirada: la pantalla sin «seguir», aunque en esta pestaña se eligiera seguir.
    expect(destinoCorreo('retirada', false)).toBe('pasar');
    expect(destinoCorreo('retirada', false, true)).toBe('pasar');
    expect(destinoCorreo('retirada', false, true, true)).toBe('pasar');
    // Un traspaso ya pedido sigue; la bóveda gana siempre.
    expect(destinoCorreo('retirada', true)).toBe('traspaso');
    expect(destinoCorreo('boveda', false, false, true)).toBe('boveda');
    // La duda, sabiendo que Matrix está retirada: «Reintentar», nunca Matrix.
    expect(destinoCorreo('dudoso', false, false, true)).toBe('reintentar');
    expect(destinoCorreo('dudoso', false, true, true)).toBe('reintentar');
    expect(destinoCorreo('dudoso', true, false, true)).toBe('traspaso');
    // Sin la retirada, lo de antes, exactamente.
    expect(destinoCorreo('dudoso', false, false, false)).toBe('matrix');
    expect(destinoCorreo('matrix', false, false, false)).toBe('elegir');
    expect(destinoCorreo('matrix', false, true, false)).toBe('matrix');
  });

  test('la retirada se aprende de Rooms: «retirada» la recuerda, «matrix» la olvida', () => {
    const s = memoria();
    expect(aprenderRetirada(s, 'user-1', 'acme', 'dudoso')).toBe(false);
    expect(aprenderRetirada(s, 'user-1', 'acme', 'retirada')).toBe(true);
    // La duda y la bóveda no la cambian.
    expect(aprenderRetirada(s, 'user-1', 'acme', 'dudoso')).toBe(true);
    expect(aprenderRetirada(s, 'user-1', 'acme', 'boveda')).toBe(true);
    // Es de esta persona y organización.
    expect(aprenderRetirada(s, 'user-2', 'acme', 'dudoso')).toBe(false);
    expect(aprenderRetirada(s, 'user-1', 'otra', 'dudoso')).toBe(false);
    // Una respuesta de Matrix (el conmutador de Rooms apagado) la olvida.
    expect(aprenderRetirada(s, 'user-1', 'acme', 'matrix')).toBe(false);
    expect(aprenderRetirada(s, 'user-1', 'acme', 'dudoso')).toBe(false);
    // Sin almacén, o con uno roto, vale la respuesta de ahora.
    const bloqueado = () => {
      throw new Error('bloqueado');
    };
    const roto = { getItem: bloqueado, setItem: bloqueado, removeItem: bloqueado };
    expect(aprenderRetirada(roto, 'user-1', 'acme', 'retirada')).toBe(true);
    expect(aprenderRetirada(roto, 'user-1', 'acme', 'dudoso')).toBe(false);
    expect(aprenderRetirada(undefined, 'user-1', 'acme', 'retirada')).toBe(true);
    expect(aprenderRetirada(undefined, 'user-1', 'acme', 'dudoso')).toBe(false);
  });

  test('«seguir con el anterior» vale para esta persona y organización', () => {
    const m = new Map<string, string>();
    const s = {
      getItem: (k: string) => m.get(k) ?? null,
      setItem: (k: string, v: string) => void m.set(k, v),
      removeItem: (k: string) => void m.delete(k),
    };
    expect(quiereSeguirConMatrix(s, 'user-1', 'acme')).toBe(false);
    seguirConMatrix(s, 'user-1', 'acme');
    expect(quiereSeguirConMatrix(s, 'user-1', 'acme')).toBe(true);
    expect(quiereSeguirConMatrix(s, 'user-2', 'acme')).toBe(false);
    expect(quiereSeguirConMatrix(s, 'user-1', 'otra')).toBe(false);
    expect(traspasoPedido(s, 'user-1', 'acme')).toBe(false);
    const bloqueado = () => {
      throw new Error('bloqueado');
    };
    const roto = { getItem: bloqueado, setItem: bloqueado, removeItem: bloqueado };
    seguirConMatrix(roto, 'user-1', 'acme');
    expect(quiereSeguirConMatrix(roto, 'user-1', 'acme')).toBe(false);
    expect(quiereSeguirConMatrix(undefined, 'user-1', 'acme')).toBe(false);
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
