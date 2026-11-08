/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La franja de una carpeta externa en el panel principal. Antes lo decía el buzón
 * del diálogo; ahora la carpeta se abre con la lista de siempre, y la lista sola
 * solo sabe decir «no se pudo, reintenta». Ejecutar: `bun test`.
 *
 * VISTO EN ROJO (2026-11-06): sin la franja, la contraseña de app revocada en iCloud
 * dejaba «Reintentar», que nunca la arregla; sin `caidas`, una Entrada que no
 * contestó desaparecía de «Todos los buzones de entrada» sin decir nada. Deshecho.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { en } from '../i18n/messages/en.js';
import { AvisoExterno } from './AvisoExterno.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = ((key: keyof typeof en) => en[key]) as unknown as Parameters<typeof AvisoExterno>[0]['t'];

afterEach(() => {
  document.body.innerHTML = '';
});

function pintar(props: Partial<Parameters<typeof AvisoExterno>[0]>, onReconnect?: () => void) {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(
    <AvisoExterno
      t={t}
      error=""
      caidas={false}
      pop3={false}
      onReconnect={onReconnect}
      {...props}
    />,
    host,
  );
  return host;
}

test('la contraseña de app revocada en iCloud lo dice y deja volver a conectar', () => {
  let pulsado = false;
  const host = pintar({ error: 'externalMail.icloudAuthError' }, () => {
    pulsado = true;
  });
  expect(host.textContent).toContain(en['externalMail.icloudAuthError']);
  const boton = [...host.querySelectorAll('button')].find(
    (b) => b.textContent?.trim() === en['externalMail.connect'],
  );
  boton?.click();
  expect(pulsado).toBe(true);
});

test('un fallo que no es del inicio de sesión no ofrece conectar', () => {
  const host = pintar({ error: 'externalMail.tlsError' }, () => undefined);
  expect(host.textContent).toContain(en['externalMail.tlsError']);
  expect(host.querySelector('button')).toBeNull();
});

test('una Entrada que no contestó en «Todos los buzones de entrada» se dice', () => {
  const host = pintar({ caidas: true });
  expect(host.textContent).toContain(en['externalMail.someUnavailable']);
});

test('una cuenta POP3 avisa de que solo se lee la Entrada', () => {
  expect(pintar({ pop3: true }).textContent).toContain(en['externalMail.pop3Note']);
});

test('sin nada que decir no ocupa sitio', () => {
  expect(pintar({}).innerHTML).toBe('');
});
