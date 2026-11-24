/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import {
  releaseBeforeEmpty,
  sourceReleaseClient,
  type ReleaseOutcome,
  type SourceReleaser,
} from './source-release.js';

const actor = '11111111-1111-4111-8111-111111111111';
const epoch = '0d3f5a1e-6b2c-4c1d-9e8f-7a6b5c4d3e2f';

test('Drive suelta su fuente por la ruta de su BFF; el resto, por la común', async () => {
  const urls: string[] = [];
  const fetchImpl = async (input: string, init?: RequestInit) => {
    urls.push(input + ' ' + (init?.method ?? 'GET'));
    if (input.endsWith('/auth/session'))
      return Response.json({
        owner: {
          v: 1,
          actor,
          tenant: 'acme',
          client_id: 'sytue-drive',
          generation: 'a'.repeat(64),
        },
      });
    if (input.endsWith('/release')) return Response.json({ mode: 'legacy' });
    return Response.json({ mode: 'frozen', epoch });
  };
  for (const [product, root] of [
    ['drive', '/drive/api/matrix/storage/migration'],
    ['calendar', '/drive/api/private-storage/migration'],
  ] as const) {
    urls.length = 0;
    const client = sourceReleaseClient({
      basePath: '/drive',
      actor,
      tenant: 'acme',
      product,
      fetchImpl,
    });
    expect(await client.frozen()).toEqual({ mode: 'frozen', epoch });
    expect(await client.release(epoch)).toBe('released');
    expect(urls.filter((u) => !u.includes('/auth/session'))).toEqual([
      root + ' GET',
      root + '/release POST',
    ]);
  }
});

function soltador(
  estados: ({ mode: 'frozen' | 'private'; epoch: string } | null | Error)[],
  resultado: ReleaseOutcome,
): SourceReleaser & { soltadas: string[] } {
  const soltadas: string[] = [];
  return {
    soltadas,
    frozen: async () => {
      const siguiente = estados.length > 1 ? estados.shift()! : estados[0]!;
      if (siguiente instanceof Error) throw siguiente;
      return siguiente;
    },
    release: async (e) => {
      soltadas.push(e);
      return resultado;
    },
  };
}

test('antes de vaciar: sin fuente congelada no suelta nada; con ella, la suelta', async () => {
  const libre = soltador([null], 'released');
  expect(await releaseBeforeEmpty(libre)).toEqual({ outcome: 'none' });
  expect(libre.soltadas).toEqual([]);

  const congelada = soltador([{ mode: 'private', epoch }], 'released');
  expect(await releaseBeforeEmpty(congelada)).toEqual({ outcome: 'released' });
  expect(congelada.soltadas).toEqual([epoch]);
});

test('antes de vaciar: reautenticación, carrera y fallos', async () => {
  expect(await releaseBeforeEmpty(soltador([{ mode: 'frozen', epoch }], 'reauth'))).toEqual({
    outcome: 'reauth',
    epoch,
  });
  // Otra pestaña ya la soltó: se puede vaciar.
  expect(await releaseBeforeEmpty(soltador([{ mode: 'frozen', epoch }, null], 'stale'))).toEqual({
    outcome: 'none',
  });
  // Se volvió a congelar con otra época: no se vacía a ciegas.
  expect(
    await releaseBeforeEmpty(
      soltador(
        [
          { mode: 'frozen', epoch },
          { mode: 'frozen', epoch: epoch.replace('0d', '1d') },
        ],
        'stale',
      ),
    ),
  ).toEqual({ outcome: 'failed' });
  expect(await releaseBeforeEmpty(soltador([new Error('red')], 'released'))).toEqual({
    outcome: 'failed',
  });
  expect(await releaseBeforeEmpty(soltador([{ mode: 'frozen', epoch }], 'failed'))).toEqual({
    outcome: 'failed',
  });
  // De vuelta de Sytue ID solo se suelta la época confirmada.
  const otra = soltador([{ mode: 'frozen', epoch }], 'released');
  expect(await releaseBeforeEmpty(otra, undefined, epoch.replace('0d', '2d'))).toEqual({
    outcome: 'failed',
  });
  expect(otra.soltadas).toEqual([]);
});
