/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Destacar desde la fila de la lista (`FavoriteToggle row`): un botón con nombre y
 * `aria-pressed` que destaca sin abrir la ficha; sin destacar se esconde hasta que el
 * puntero pasa por la fila, salvo con el dedo, donde no hay «pasar por encima».
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { I18nProvider } from '@sytue/i18n/preact';
import { ToastProvider } from '@sytue/ui';
import type { Contact } from '../client/contactsApi.js';
import { FavoriteToggle } from './ContactDetail.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

// La clave con el nombre: cada estrella dice de quién es.
const t = ((k: string, v?: { name?: string }) => (v?.name ? `${k}:${v.name}` : k)) as never;
const ana: Contact = {
  href: '/a.vcf',
  uid: 'a',
  fullName: 'Ana',
  emails: [],
  phones: [],
  vcard: '',
};
const hosts: HTMLElement[] = [];

afterEach(() => {
  for (const h of hosts.splice(0)) {
    render(null, h);
    h.remove();
  }
});

function montar(active: boolean, onToggle: (c: Contact) => Promise<void>): HTMLButtonElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  hosts.push(host);
  render(
    <I18nProvider locale="es">
      <ToastProvider>
        <FavoriteToggle row t={t} contact={ana} active={active} onToggle={onToggle} />
      </ToastProvider>
    </I18nProvider>,
    host,
  );
  const b = host.querySelector('button');
  if (!b) throw new Error('la fila no tiene botón de destacar');
  return b;
}

test('sin destacar: se ofrece «Destacar», oculto en reposo y visible al pasar, al enfocar y con el dedo', () => {
  const b = montar(false, async () => undefined);
  expect(b.getAttribute('aria-label')).toBe('list.star:Ana');
  expect(b.getAttribute('aria-pressed')).toBe('false');
  const cls = b.className.split(/\s+/);
  for (const c of [
    'opacity-0',
    'group-hover:opacity-100',
    'focus-visible:opacity-100',
    'pointer-coarse:opacity-100',
  ])
    expect(cls).toContain(c);
});

test('destacado: la estrella se ve siempre y el botón la quita', () => {
  const b = montar(true, async () => undefined);
  expect(b.getAttribute('aria-label')).toBe('list.unstar:Ana');
  expect(b.getAttribute('aria-pressed')).toBe('true');
  expect(b.className.split(/\s+/)).not.toContain('opacity-0');
});

test('pulsarla destaca ESE contacto', () => {
  const vistos: string[] = [];
  const b = montar(false, async (c) => {
    vistos.push(c.uid);
  });
  b.click();
  expect(vistos).toEqual(['a']);
});
