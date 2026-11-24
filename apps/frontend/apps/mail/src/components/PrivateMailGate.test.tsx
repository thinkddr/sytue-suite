/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La puerta del correo privado con la autoridad de Matrix retirada en Rooms (ADR
 * 0087, entrega 5): solo se ofrece pasar a la bóveda, sin «seguir con el correo
 * anterior», y la duda ya no abre Matrix sino un «Reintentar». La puerta lo sabe
 * solo por la respuesta de Rooms. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { en } from '../i18n/messages/en.js';
import { PrivateMailGate } from './PrivateMailGate.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const fetchOriginal = globalThis.fetch;
let respuestas: Array<() => Response> = [];
let pedidos = 0;

beforeEach(() => {
  respuestas = [];
  pedidos = 0;
  globalThis.localStorage.clear();
  globalThis.sessionStorage.clear();
  globalThis.fetch = (async () => {
    pedidos++;
    const siguiente = respuestas.shift();
    if (!siguiente) throw new Error('sin red');
    return siguiente();
  }) as unknown as typeof fetch;
});

afterEach(() => {
  globalThis.fetch = fetchOriginal;
  document.body.innerHTML = '';
});

const retirada = () =>
  Response.json({ error: { code: 'storage_matrix_retired' } }, { status: 409 });

async function pintar() {
  const host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () => {
    render(
      <PrivateMailGate vault actor="user-1" tenant="acme" locale="en">
        {() => <p>buzón</p>}
      </PrivateMailGate>,
      host,
    );
  });
  // La lectura de la autoridad y su respuesta.
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
  return host;
}

const botones = (host: HTMLElement) =>
  [...host.querySelectorAll('button')].map((b) => b.textContent?.trim());

test('con Matrix retirada solo se ofrece pasar a la bóveda, y avisa antes de la casilla', async () => {
  respuestas.push(retirada);
  const host = await pintar();
  expect(host.querySelector('h1')?.textContent).toBe(en['privateMail.vaultMoveRetiredTitle']);
  const texto = host.querySelector('p')?.textContent;
  expect(texto).toBe(en['privateMail.vaultMoveRetiredBody']);
  expect(host.textContent).toContain(en['privateMail.vaultMoveUnderstood']);
  expect(botones(host)).toContain(en['privateMail.handoverConfirm']);
  expect(botones(host)).not.toContain(en['privateMail.vaultMoveKeep']);
  // El aviso va antes de la casilla.
  const casilla = host.querySelector('input[type="checkbox"]')!;
  expect(
    host.querySelector('p')!.compareDocumentPosition(casilla) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
  expect(pedidos).toBe(1);
});

test('ante la duda, con Matrix retirada, no se abre Matrix: «Reintentar»', async () => {
  // Rooms ya dijo una vez que Matrix está retirada.
  respuestas.push(retirada);
  await pintar();
  document.body.innerHTML = '';

  // Ahora la lectura falla (sin red): ni Matrix ni propuesta de vaciar.
  const host = await pintar();
  expect(host.textContent).toContain(en['privateMail.vaultMoveCheckFailed']);
  expect(host.textContent).not.toContain(en['privateMail.handoverOffer']);
  expect(botones(host)).toEqual([en['common.retry']]);

  // «Reintentar» vuelve a preguntar; si Rooms contesta, se sigue por ahí.
  respuestas.push(retirada);
  const reintentar = [...host.querySelectorAll('button')][0]!;
  await act(async () => {
    reintentar.click();
  });
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
  expect(pedidos).toBe(3);
  expect(host.querySelector('h1')?.textContent).toBe(en['privateMail.vaultMoveRetiredTitle']);
});

test('confirmar el traspaso con Matrix retirada pide el traspaso en la pestaña', async () => {
  respuestas.push(retirada);
  const host = await pintar();
  const casilla = host.querySelector<HTMLInputElement>('input[type="checkbox"]')!;
  await act(async () => casilla.click());
  const vaciar = [...host.querySelectorAll('button')].find(
    (b) => b.textContent?.trim() === en['privateMail.handoverConfirm'],
  )!;
  expect(vaciar.disabled).toBe(false);
  // Al confirmar la puerta pasa a la de la bóveda del traspaso; aquí solo importa
  // que la marca quede en la pestaña (la puerta de la bóveda no se prueba aquí).
  await act(async () => {
    try {
      vaciar.click();
    } catch {
      /* la puerta de la bóveda necesita el núcleo; no es lo que se prueba */
    }
  });
  expect(globalThis.sessionStorage.getItem('sytue.mail.traspaso:acme:user-1')).toBe('1');
});
