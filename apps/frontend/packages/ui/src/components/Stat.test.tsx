/**
 * Stat — su contrato: una sola tipografía para las cifras de la suite, el valor
 * legible entero aunque se recorte, y el tono solo en la cifra. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import type { VNode } from 'preact';
import { Stat } from './Stat.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

function mount(node: VNode): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(node, host);
  return host;
}

function cifra(host: HTMLElement): HTMLElement {
  return host.querySelector<HTMLElement>('.sytue-h3')!;
}

test('rótulo y cifra, con la tipografía de título y cifras tabulares', () => {
  const host = mount(<Stat label="Canales" value={12} />);
  expect(host.textContent).toContain('Canales');
  const v = cifra(host);
  expect(v.textContent).toBe('12');
  expect(v.className).toContain('tabular-nums');
  expect(v.className).toContain('text-text');
});

test('el cero se pinta: «ninguno» también es dato', () => {
  expect(cifra(mount(<Stat label="Archivados" value={0} />)).textContent).toBe('0');
});

test('el valor largo se recorta pero se puede leer entero en el title', () => {
  const v = cifra(mount(<Stat label="Release esperada" value="2026.08.09-a1b2c3d" />));
  expect(v.className).toContain('truncate');
  expect(v.getAttribute('title')).toBe('2026.08.09-a1b2c3d');
});

test('el tono colorea la cifra y deja el rótulo en gris', () => {
  const host = mount(<Stat label="Errores" value={3} tone="danger" />);
  expect(cifra(host).className).toContain('text-danger-text');
  expect(host.querySelector('p')!.className).toContain('text-text-muted');
  expect(cifra(mount(<Stat label="Avisos" value={2} tone="warning" />)).className).toContain(
    'text-warning-text',
  );
});

test('la pista solo aparece si se pasa', () => {
  expect(mount(<Stat label="Visitas" value={5} />).querySelector('.sytue-meta')).toBeNull();
  const host = mount(<Stat label="Visitantes" value={5} hint="suma de días" />);
  expect(host.querySelector('.sytue-meta')!.textContent).toBe('suma de días');
});

test('`card` es una tarjeta plana; `plain` no pinta superficie', () => {
  const card = mount(<Stat label="Usuarios" value={1} />).firstElementChild!;
  expect(card.className).toContain('rounded-card');
  expect(card.className).not.toContain('shadow');
  const plain = mount(<Stat label="Usuarios" value={1} surface="plain" />).firstElementChild!;
  expect(plain.className).not.toContain('rounded-card');
  expect(plain.className).toContain('min-w-0');
});

test('`plain` respeta el ancho mínimo de quien llama sin pisarlo con `min-w-0`', () => {
  const el = mount(
    <Stat label="Visitas" value={5} surface="plain" class="min-w-28" />,
  ).firstElementChild!;
  expect(el.className).toContain('min-w-28');
  expect(el.className).not.toContain('min-w-0');
});
