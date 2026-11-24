/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La pantalla «Pasa tu correo a la nueva bóveda» (ADR 0087): a quien tiene el correo
 * cifrado con Matrix se le ofrece vaciarlo y empezar en la bóveda, como el resto de
 * las apps, sin tener que abrir Matrix antes. Vaciar exige marcar que se entiende la
 * pérdida; seguir con el correo anterior no exige nada. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { en } from '../i18n/messages/en.js';
import { PasarABoveda } from './TraspasoBoveda.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

afterEach(() => {
  document.body.innerHTML = '';
});

function pintar() {
  const llamadas = { vaciar: 0, seguir: 0 };
  const host = document.createElement('div');
  document.body.appendChild(host);
  act(() => {
    render(
      <PasarABoveda
        locale="en"
        onConfirm={() => void llamadas.vaciar++}
        onKeep={() => void llamadas.seguir++}
      />,
      host,
    );
  });
  const boton = (texto: string) =>
    [...host.querySelectorAll('button')].find((b) => b.textContent?.trim() === texto)!;
  return { host, llamadas, boton };
}

test('explica que se pierde el correo cifrado con el sistema anterior', () => {
  const { host } = pintar();
  expect(host.querySelector('h1')?.textContent).toBe(en['privateMail.vaultMoveTitle']);
  expect(host.textContent).toContain(en['privateMail.vaultMoveBody']);
  expect(host.textContent).toContain(en['privateMail.vaultMoveUnderstood']);
});

test('vaciar no se puede sin marcar que se entiende la pérdida', () => {
  const { host, llamadas, boton } = pintar();
  const vaciar = boton(en['privateMail.handoverConfirm']);
  expect(vaciar.disabled).toBe(true);
  act(() => vaciar.click());
  expect(llamadas.vaciar).toBe(0);
  const casilla = host.querySelector<HTMLInputElement>('input[type="checkbox"]')!;
  act(() => casilla.click());
  expect(boton(en['privateMail.handoverConfirm']).disabled).toBe(false);
  act(() => boton(en['privateMail.handoverConfirm']).click());
  expect(llamadas).toEqual({ vaciar: 1, seguir: 0 });
});

test('seguir con el correo anterior no vacía nada', () => {
  const { llamadas, boton } = pintar();
  act(() => boton(en['privateMail.vaultMoveKeep']).click());
  expect(llamadas).toEqual({ vaciar: 0, seguir: 1 });
});
