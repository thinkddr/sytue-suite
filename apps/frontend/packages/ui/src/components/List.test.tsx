/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Test de List — regresión de teclado (WCAG 2.1.1): un ListItem interactivo
 * FUERA de un listbox (p.ej. NotificationCenter) debe activarse con Enter y
 * Espacio como con el ratón, exponer role=button, y Espacio no debe provocar
 * scroll de página (preventDefault). Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { List, ListItem } from './List.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

function renderItem(onClick: () => void) {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(
    <List label="Notificaciones">
      <ListItem title="Aviso" onClick={onClick} />
    </List>,
    host,
  );
  return { host, li: host.querySelector('li') as HTMLLIElement };
}

test('item interactivo fuera de listbox expone role=button', () => {
  const { host, li } = renderItem(() => {});
  expect(li.getAttribute('role')).toBe('button');
  render(null, host);
});

test('Enter activa el item (dispara onClick)', () => {
  let clicks = 0;
  const { host, li } = renderItem(() => clicks++);
  li.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }));
  expect(clicks).toBe(1);
  render(null, host);
});

test('Espacio activa el item y evita el scroll (preventDefault)', () => {
  let clicks = 0;
  const { host, li } = renderItem(() => clicks++);
  const ev = new KeyboardEvent('keydown', { key: ' ', bubbles: true, cancelable: true });
  li.dispatchEvent(ev);
  expect(clicks).toBe(1);
  expect(ev.defaultPrevented).toBe(true);
  render(null, host);
});

test('subtítulo como string aplica truncate; como JSX aplica min-w-0 sin truncate', () => {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(
    <List label="Prueba">
      <ListItem title="Item 1" subtitle="Texto simple" />
      <ListItem title="Item 2" subtitle={<span class="custom">JSX complejo</span>} />
    </List>,
    host,
  );
  const items = host.querySelectorAll('li');
  const sub1 = items[0].querySelector('.text-text-muted') as HTMLElement;
  const sub2 = items[1].querySelector('.text-text-muted') as HTMLElement;

  expect(sub1.className).toContain('truncate');
  expect(sub2.className).toContain('min-w-0');
  expect(sub2.className).not.toContain('truncate');
  render(null, host);
});

test('título como string aplica truncate; como JSX aplica min-w-0 sin truncate', () => {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(
    <List label="Prueba títulos">
      <ListItem title="Título simple" />
      <ListItem title={<span class="custom">Título complejo</span>} />
    </List>,
    host,
  );
  const items = host.querySelectorAll('li');
  const title1 = items[0].querySelector('.text-text') as HTMLElement;
  const title2 = items[1].querySelector('.text-text') as HTMLElement;

  expect(title1.className).toContain('truncate');
  expect(title2.className).toContain('min-w-0');
  expect(title2.className).not.toContain('truncate');
  render(null, host);
});
