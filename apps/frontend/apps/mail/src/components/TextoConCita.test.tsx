/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La cita plegada en el cuerpo de texto de un mensaje del hilo. Ejecutar: `bun test`.
 *
 * VISTO EN ROJO (2026-11-09): con el `<p>` de antes, la respuesta enseñaba el hilo
 * entero y no había botón; sin `aria-expanded`, el lector de pantalla no sabía si la
 * cita estaba abierta. Deshecho.
 *
 * VISTO EN ROJO (2026-11-13): con el texto pintado tal cual, las direcciones del
 * cuerpo no eran enlaces y la cita desplegada tampoco. Deshecho.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { en } from '../i18n/messages/en.js';
import { TextoConCita } from './TextoConCita.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = ((key: keyof typeof en) => en[key]) as unknown as Parameters<typeof TextoConCita>[0]['t'];

afterEach(() => {
  document.body.innerHTML = '';
});

function pintar(texto: string, alEscribirA?: (direccion: string) => void) {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(<TextoConCita t={t} texto={texto} alEscribirA={alEscribirA} />, host);
  return host;
}

const RESPUESTA =
  'Perfecto, lo miro.\n\nOn Mon, Jan 5, 2026 at 10:00 AM Ana <ana@example.eu> wrote:\n> ¿Lo revisas?\n> Ana';

test('una respuesta enseña solo lo nuevo y el botón de la cita', () => {
  const host = pintar(RESPUESTA);
  expect(host.textContent).toContain('Perfecto, lo miro.');
  expect(host.textContent).not.toContain('¿Lo revisas?');
  const boton = host.querySelector('button');
  expect(boton?.textContent?.trim()).toBe(en['reader.showQuoted']);
  expect(boton?.getAttribute('aria-expanded')).toBe('false');
});

test('desplegar recupera el texto íntegro, y plegar lo vuelve a esconder', () => {
  const host = pintar(RESPUESTA);
  act(() => host.querySelector('button')!.click());
  const boton = host.querySelector('button')!;
  expect(boton.getAttribute('aria-expanded')).toBe('true');
  expect(boton.textContent?.trim()).toBe(en['reader.hideQuoted']);
  for (const linea of RESPUESTA.split('\n').filter((l) => l.trim())) {
    expect(host.textContent).toContain(linea);
  }
  act(() => boton.click());
  expect(host.textContent).not.toContain('¿Lo revisas?');
});

test('un mensaje que es solo cita no se pliega', () => {
  const host = pintar('On Mon, Jan 5, 2026, Ana wrote:\n> Hola');
  expect(host.querySelector('button')).toBeNull();
  expect(host.textContent).toContain('> Hola');
});

test('las direcciones del cuerpo se abren en otra pestaña, sin opener ni Referer', () => {
  const host = pintar('Mira www.sytue.eu/precios y https://sytue.eu/ayuda.');
  const enlaces = [...host.querySelectorAll('a')];
  expect(enlaces.map((a) => a.textContent)).toEqual([
    'www.sytue.eu/precios',
    'https://sytue.eu/ayuda',
  ]);
  // La pista es el destino REAL, con el `https://` que abre el «www.».
  expect(enlaces[0]!.getAttribute('href')).toBe('https://www.sytue.eu/precios');
  expect(enlaces[0]!.getAttribute('title')).toBe('https://www.sytue.eu/precios');
  for (const a of enlaces) {
    expect(a.getAttribute('target')).toBe('_blank');
    expect(a.getAttribute('rel')).toBe('noopener noreferrer');
  }
  // El punto de la frase no es de la dirección.
  expect(host.textContent).toBe('Mira www.sytue.eu/precios y https://sytue.eu/ayuda.');
});

test('un mailto se escribe desde Sytue, y con una tecla modificadora lo abre el sistema', () => {
  const escritos: string[] = [];
  const host = pintar('Escribe a soporte@sytue.eu', (d) => escritos.push(d));
  const a = host.querySelector('a')!;
  expect(a.getAttribute('href')).toBe('mailto:soporte@sytue.eu');
  expect(a.getAttribute('target')).toBeNull();
  const clic = new MouseEvent('click', { bubbles: true, cancelable: true });
  act(() => {
    a.dispatchEvent(clic);
  });
  expect(escritos).toEqual(['soporte@sytue.eu']);
  expect(clic.defaultPrevented).toBe(true);
  const conCtrl = new MouseEvent('click', { bubbles: true, cancelable: true, ctrlKey: true });
  act(() => {
    a.dispatchEvent(conCtrl);
  });
  expect(escritos).toHaveLength(1);
  expect(conCtrl.defaultPrevented).toBe(false);
});

test('ni javascript: ni otros esquemas se convierten en enlace', () => {
  const host = pintar('javascript:alert(1) data:text/html,hola file:///etc/passwd');
  expect(host.querySelector('a')).toBeNull();
});

test('la cita desplegada también enlaza', () => {
  const host = pintar(RESPUESTA);
  expect(host.querySelectorAll('a')).toHaveLength(0);
  act(() => host.querySelector('button')!.click());
  expect(host.querySelector('a')?.getAttribute('href')).toBe('mailto:ana@example.eu');
});
