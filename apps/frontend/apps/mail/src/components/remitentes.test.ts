/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El «De» del redactor contra los vectores COMPARTIDOS con Fyne
 * (apps/fyne/apps/mail/testdata/remitentes.json, que recorre `TestLosRemitentesSonLosDeLaWeb`),
 * y el envío por una cuenta externa, que antes hacía `ExternalCompose` aparte. Ejecutar:
 * `bun test`.
 *
 * VISTO EN ROJO (2026-11-06): sin mirar `capabilities.send`, salía la cuenta de
 * Calendario; sin comparar sin mayúsculas, el alias salía dos veces; sin el `?? lista[0]`,
 * un valor desconectado no elegía nada; con `errorKey` también para el 502, la respuesta
 * perdida decía «no se pudo» e invitaba a reenviar; sin `expected_address`, el servidor no
 * podía comprobar que la cuenta seguía siendo esa. Deshecho.
 */
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { HttpError, type ExternalMailOutgoing } from '@sytue/api-clients';
import {
  LIMITE_ADJUNTOS_EXTERNOS,
  enviarPorCuenta,
  remitenteElegido,
  remitentes,
  sinImagenesCid,
  type Escrito,
} from './remitentes.js';

interface Caso {
  nombre: string;
  sytue: string[];
  externas: { instance_id: string; address: string; send: boolean }[];
  esperado: { valor: string; direccion: string; cuenta: string }[];
  elegir: { valor: string; esperado: string }[];
}

const vectores = JSON.parse(
  readFileSync(
    new URL('../../../../../fyne/apps/mail/testdata/remitentes.json', import.meta.url),
    'utf8',
  ),
) as { casos: Caso[]; sinImagenesCid: { nombre: string; html: string; esperado: string }[] };

test('hay vectores que recorrer', () => {
  expect(vectores.casos.length).toBeGreaterThanOrEqual(5);
  expect(vectores.sinImagenesCid.length).toBeGreaterThanOrEqual(3);
});

for (const c of vectores.casos) {
  test(c.nombre, () => {
    const lista = remitentes(
      c.sytue,
      c.externas.map((e) => ({
        instance_id: e.instance_id,
        address: e.address,
        capabilities: {
          read: e.send,
          send: e.send,
          actions: e.send,
          attachments: e.send,
          continuous_sync: false,
        },
      })),
    );
    expect(lista).toEqual(c.esperado);
    for (const e of c.elegir) expect(remitenteElegido(e.valor, lista)?.valor).toBe(e.esperado);
  });
}

for (const c of vectores.sinImagenesCid) {
  test(`sinImagenesCid: ${c.nombre}`, () => {
    expect(sinImagenesCid(c.html)).toBe(c.esperado);
  });
}

const cuenta = {
  instance_id: 'g1',
  provider: 'gmail' as const,
  address: 'ana@gmail.com',
};
const escrito: Escrito = {
  to: [{ email: 'luis@example.com' }],
  cc: [],
  bcc: [],
  subject: 'Re: Factura',
  text: 'Gracias\n\nfoto.png',
  html: '<p>Gracias</p><img src="cid:img1">',
  files: [{ name: 'foto.png', type: 'image/png', size: 3, contentBase64: 'AAAA' }],
};

function proveedor(responde: () => Promise<{ sent_copy_saved?: boolean }>) {
  const llamadas: { instancia: string; salida: ExternalMailOutgoing }[] = [];
  return {
    llamadas,
    api: {
      send: (instancia: string, salida: ExternalMailOutgoing) => {
        llamadas.push({ instancia, salida });
        return responde();
      },
    },
  };
}

test('sale por la cuenta, con la dirección que se espera y la imagen como adjunto', async () => {
  const p = proveedor(async () => ({ sent_copy_saved: false }));
  expect(await enviarPorCuenta(p.api, cuenta, escrito)).toEqual({
    tipo: 'enviado',
    copiaGuardada: false,
  });
  expect(p.llamadas).toEqual([
    {
      instancia: 'g1',
      salida: {
        expected_address: 'ana@gmail.com',
        to: [{ email: 'luis@example.com' }],
        cc: [],
        bcc: [],
        subject: 'Re: Factura',
        text: 'Gracias\n\nfoto.png',
        html: '<p>Gracias</p>',
        attachments: [{ name: 'foto.png', type: 'image/png', size: 3, content_base64: 'AAAA' }],
      },
    },
  ]);
});

test('una respuesta perdida no se da por fallida: pide mirar en Enviados y no se repite', async () => {
  for (const fallo of [
    new HttpError(502, 'Bad Gateway', '/send', '{"error":{"code":"mail_unavailable"}}'),
    new TypeError('Failed to fetch'),
  ]) {
    const p = proveedor(() => Promise.reject(fallo));
    expect(await enviarPorCuenta(p.api, cuenta, escrito)).toEqual({
      tipo: 'error',
      clave: 'externalMail.sendUnknown',
    });
    expect(p.llamadas).toHaveLength(1);
  }
});

test('el inicio de sesión rechazado dice qué hacer con esa cuenta', async () => {
  const p = proveedor(() =>
    Promise.reject(new HttpError(401, 'Unauthorized', '/send', '{"error":{"code":"mail_auth"}}')),
  );
  expect(await enviarPorCuenta(p.api, cuenta, escrito)).toEqual({
    tipo: 'error',
    clave: 'externalMail.authError',
  });
});

test('lo que no cabe no llega a salir', async () => {
  const p = proveedor(async () => ({}));
  const grande = {
    ...escrito,
    files: [{ ...escrito.files[0]!, size: LIMITE_ADJUNTOS_EXTERNOS + 1 }],
  };
  expect(await enviarPorCuenta(p.api, cuenta, grande)).toEqual({
    tipo: 'error',
    clave: 'externalMail.tooLarge',
  });
  expect(p.llamadas).toEqual([]);
});
