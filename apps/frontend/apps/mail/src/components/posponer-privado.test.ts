/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Posponer y silenciar en el modo privado (PEND-MAIL-07). Son del servicio, que
 * mueve los hilos al vencer la fecha; los hilos del modo privado los guarda cifrados
 * el núcleo y nadie los despertaría. Antes MailApp llamaba a `mailApi` directamente,
 * así que en modo privado la orden salía al servicio en claro con el id de un hilo
 * que no es el suyo. Ahora todo va por `usePrivateMailApi`, que dice que no.
 * Ejecutar: `bun test`.
 */
import { readFileSync } from 'node:fs';
import { describe, expect, test } from 'bun:test';

process.env.BASE_URL ??= '/mail/';

let peticiones = 0;
globalThis.fetch = (async () => {
  peticiones++;
  return new Response('{}', { status: 200, headers: { 'content-type': 'application/json' } });
}) as unknown as typeof fetch;

const { createPrivateMailApi } = await import('../client/privateMailApi.js');
const mailApi = await import('../client/mailApi.js');
type Cliente = Parameters<typeof createPrivateMailApi>[0];

/** Un cliente privado que apunta cada acción que le piden. */
function clienteQueApunta(): { cliente: Cliente; acciones: string[] } {
  const acciones: string[] = [];
  const cliente = {
    call: (accion: string) => {
      acciones.push(accion);
      return Promise.resolve(null);
    },
  } as unknown as Cliente;
  return { cliente, acciones };
}

describe('posponer y silenciar en el modo privado', () => {
  test('el servicio los admite y el modo privado no', () => {
    expect(mailApi.admitePosponer).toBe(true);
    expect(createPrivateMailApi(clienteQueApunta().cliente).admitePosponer).toBe(false);
  });

  test('no hay nada pospuesto, y las órdenes se rechazan sin tocar la red ni el núcleo', async () => {
    const { cliente, acciones } = clienteQueApunta();
    const api = createPrivateMailApi(cliente);
    peticiones = 0;
    expect(await api.listSnoozes()).toEqual({ snoozes: [] });
    await expect(api.snoozeThread('t1', new Date(Date.now() + 3_600_000))).rejects.toMatchObject({
      code: 'private_mail_operation_unavailable',
    });
    await expect(api.unsnoozeThread('t1')).rejects.toMatchObject({
      code: 'private_mail_operation_unavailable',
    });
    await expect(api.muteThread('t1', true)).rejects.toMatchObject({
      code: 'private_mail_operation_unavailable',
    });
    expect(peticiones).toBe(0);
    expect(acciones).toEqual([]);
  });

  test('MailApp los pide a usePrivateMailApi y no los importa de mailApi', () => {
    const fuente = readFileSync(new URL('./MailApp.tsx', import.meta.url), 'utf8');
    const deMailApi = /import \{([^}]*)\} from '\.\.\/client\/mailApi\.js'/.exec(fuente)?.[1] ?? '';
    for (const f of ['listSnoozes', 'snoozeThread', 'unsnoozeThread', 'muteThread']) {
      expect(deMailApi).not.toMatch(new RegExp(`\\b${f}\\b`));
    }
    const delHook = /\{([^}]*)\} = usePrivateMailApi\(\)/.exec(fuente)?.[1] ?? '';
    for (const f of [
      'admitePosponer',
      'listSnoozes',
      'snoozeThread',
      'unsnoozeThread',
      'muteThread',
    ]) {
      expect(delHook).toMatch(new RegExp(`\\b${f}\\b`));
    }
    // Sin `admitePosponer` no se ofrece ni el botón ni el menú.
    expect(fuente).toMatch(/const posponible = admitePosponer && conversacionPropia;/);
  });
});
