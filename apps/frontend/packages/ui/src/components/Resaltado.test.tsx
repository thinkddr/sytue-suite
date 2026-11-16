/**
 * Resaltado — marca lo buscado con la regla de búsqueda de la suite: sin mayúsculas
 * ni tildes, cada término por su lado, y el texto entero intacto alrededor. Lo usan
 * Notes, Mail y Contacts. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { Resaltado } from './Resaltado.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const hosts: HTMLElement[] = [];
afterEach(() => {
  for (const h of hosts.splice(0)) {
    render(null, h);
    h.remove();
  }
});

function montar(texto: string, consulta: string): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  hosts.push(host);
  render(<Resaltado texto={texto} consulta={consulta} />, host);
  return host;
}

const marcas = (host: HTMLElement): string[] =>
  [...host.querySelectorAll('mark')].map((m) => m.textContent ?? '');

test('marca cada término sin mirar tildes ni mayúsculas y deja el texto entero', () => {
  const host = montar('Reunión de equipo con Ana', 'reunion ana');
  expect(marcas(host)).toEqual(['Reunión', 'Ana']);
  expect(host.textContent).toBe('Reunión de equipo con Ana');
  for (const m of host.querySelectorAll('mark')) expect(m.className).toBe('sytue-resalte');
});

test('sin consulta, o sin coincidencias, el texto sale tal cual', () => {
  expect(marcas(montar('Presupuesto', ''))).toEqual([]);
  const host = montar('Presupuesto', 'factura');
  expect(marcas(host)).toEqual([]);
  expect(host.textContent).toBe('Presupuesto');
});
