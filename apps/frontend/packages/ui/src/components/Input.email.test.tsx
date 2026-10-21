/**
 * El campo de correo frente al doble relleno de los gestores de contraseñas.
 *
 * Algunos escriben la dirección y, al ver que el valor parece un correo, la
 * escriben otra vez sin limpiar. Pasó en el acceso de Sytue ID y se corrige igual
 * en los dos sitios; aquí queda fijado para los doce campos de correo de la suite.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import type { VNode } from 'preact';
import { Input } from './Form.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

function mount(children: VNode): HTMLInputElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(children, host);
  return host.querySelector('input') as HTMLInputElement;
}

/** Así rellena un gestor: escribe el valor y dispara `input`. */
function rellena(el: HTMLInputElement, valor: string): void {
  el.value = valor;
  el.dispatchEvent(new Event('input', { bubbles: true }));
}

test('el doble relleno se colapsa a una sola dirección', () => {
  const vistos: string[] = [];
  const el = mount(<Input type="email" onInput={(e) => vistos.push(e.currentTarget.value)} />);

  rellena(el, 'ana@sytue.com');
  rellena(el, 'ana@sytue.com ana@sytue.com');

  expect(el.value).toBe('ana@sytue.com');
  // Lo que ve quien usa el componente ya viene limpio: un campo controlado no
  // tiene que acordarse de normalizar por su cuenta.
  expect(vistos).toEqual(['ana@sytue.com', 'ana@sytue.com']);
});

test('dos direcciones DISTINTAS se respetan: eso lo escribió alguien', () => {
  const el = mount(<Input type="email" />);
  rellena(el, 'ana@sytue.com otra@sytue.com');
  expect(el.value).toBe('ana@sytue.com otra@sytue.com');
});

test('escribir a mano no pierde caracteres', () => {
  const el = mount(<Input type="email" />);
  for (const parcial of ['a', 'an', 'ana', 'ana@', 'ana@sytue.com']) rellena(el, parcial);
  expect(el.value).toBe('ana@sytue.com');
});

test('solo se toca el campo de correo', () => {
  const el = mount(<Input type="text" />);
  rellena(el, 'ana ana');
  expect(el.value).toBe('ana ana');
});
